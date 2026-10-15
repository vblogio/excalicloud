package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Tags et favoris des fichiers et dossiers, et icône des dossiers, dans
// .meta.json à la racine :
// {"items":{"a/b.excalidraw":{"tags":["NSX"],"favorite":true},"a":{"icon":"server"}}}.
// Ils suivent les renommages, déplacements et suppressions.

const metaFile = ".meta.json"

const (
	// MaxTagLen borne la longueur d'un tag (en caractères).
	MaxTagLen = 40
	// MaxTags borne le nombre de tags d'un élément.
	MaxTags = 20
)

type itemMeta struct {
	Tags     []string `json:"tags,omitempty"`
	Favorite bool     `json:"favorite,omitempty"`
	Icon     string   `json:"icon,omitempty"` // dossiers : nom d'icône du frontend
}

func (m itemMeta) empty() bool { return len(m.Tags) == 0 && !m.Favorite && m.Icon == "" }

// iconName : nom d'icône accepté (le frontend affiche le dossier classique
// pour un nom qu'il ne connaît pas).
var iconName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// TagCount est un tag utilisé et son nombre d'éléments.
type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// loadMeta charge .meta.json au premier accès ; appelé sous metaMu.
func (s *Store) loadMeta() error {
	if s.meta != nil {
		return nil
	}
	var d struct {
		Items map[string]itemMeta `json:"items"`
	}
	b, err := os.ReadFile(filepath.Join(s.root, metaFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &d); err != nil {
			return fmt.Errorf("%s : %w", metaFile, err)
		}
	}
	if d.Items == nil {
		d.Items = map[string]itemMeta{}
	}
	s.meta = d.Items
	return nil
}

// saveMeta écrit .meta.json ; appelé sous metaMu.
func (s *Store) saveMeta() error {
	for k, m := range s.meta {
		if m.empty() {
			delete(s.meta, k)
		}
	}
	b, err := json.Marshal(map[string]any{"items": s.meta})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.root, metaFile), b)
}

// metaSnapshot renvoie une copie des métadonnées (pour List et Search).
func (s *Store) metaSnapshot() map[string]itemMeta {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return map[string]itemMeta{}
	}
	out := make(map[string]itemMeta, len(s.meta))
	for k, v := range s.meta {
		out[k] = v
	}
	return out
}

func decorate(e *Entry, meta map[string]itemMeta) {
	if m, ok := meta[e.Path]; ok {
		e.Tags, e.Favorite = m.Tags, m.Favorite
		if e.IsDir {
			e.Icon = m.Icon
		}
	}
}

func normTag(t string) (string, error) {
	t = strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(t), "#")), " ")
	switch {
	case t == "", !utf8.ValidString(t), strings.ContainsAny(t, ",\x00"):
		return "", fmt.Errorf("%w : tag %q", ErrInvalidPath, t)
	case utf8.RuneCountInString(t) > MaxTagLen:
		return "", fmt.Errorf("%w : tag de %d caractères max", ErrInvalidPath, MaxTagLen)
	}
	return t, nil
}

// editMeta vérifie que chaque chemin existe puis applique edit à ses
// métadonnées, et enregistre le tout.
func (s *Store) editMeta(paths []string, edit func(*itemMeta) error) error {
	cs := make([]string, 0, len(paths))
	for _, p := range paths {
		c, abs, err := s.resolve(p)
		if err != nil {
			return err
		}
		if c == "" {
			return ErrInvalidPath
		}
		if _, err := os.Lstat(abs); err != nil {
			return mapErr(err)
		}
		cs = append(cs, c)
	}
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if err := s.loadMeta(); err != nil {
		return err
	}
	for _, c := range cs {
		m := s.meta[c]
		m.Tags = append([]string(nil), m.Tags...)
		if err := edit(&m); err != nil {
			return err
		}
		s.meta[c] = m
	}
	return s.saveMeta()
}

// SetFavorite ajoute ou retire des éléments des favoris.
func (s *Store) SetFavorite(paths []string, favorite bool) error {
	return s.editMeta(paths, func(m *itemMeta) error {
		m.Favorite = favorite
		return nil
	})
}

// SetIcon choisit l'icône de dossiers ; icon = "" rétablit l'icône par défaut.
func (s *Store) SetIcon(paths []string, icon string) error {
	if icon != "" && !iconName.MatchString(icon) {
		return fmt.Errorf("%w : icône %q", ErrInvalidPath, icon)
	}
	for _, p := range paths {
		_, abs, err := s.resolve(p)
		if err != nil {
			return err
		}
		if fi, err := os.Lstat(abs); err != nil {
			return mapErr(err)
		} else if !fi.IsDir() {
			return fmt.Errorf("%w : %q n'est pas un dossier", ErrInvalidPath, p)
		}
	}
	return s.editMeta(paths, func(m *itemMeta) error {
		m.Icon = icon
		return nil
	})
}

// EditTags ajoute et retire des tags sur des éléments. Les tags sont comparés
// sans casse ni accents ; un tag déjà utilisé garde son écriture d'origine.
func (s *Store) EditTags(paths, add, remove []string) error {
	var adds []string
	for _, t := range add {
		n, err := normTag(t)
		if err != nil {
			return err
		}
		adds = append(adds, n)
	}
	drop := map[string]bool{}
	for _, t := range remove {
		drop[fold(strings.TrimSpace(t))] = true
	}
	canon := map[string]string{} // écriture existante de chaque tag
	s.metaMu.Lock()
	err := s.loadMeta()
	for _, m := range s.meta {
		for _, t := range m.Tags {
			if _, ok := canon[fold(t)]; !ok {
				canon[fold(t)] = t
			}
		}
	}
	s.metaMu.Unlock()
	if err != nil {
		return err
	}
	return s.editMeta(paths, func(m *itemMeta) error {
		kept := m.Tags[:0]
		has := map[string]bool{}
		for _, t := range m.Tags {
			if !drop[fold(t)] && !has[fold(t)] {
				kept = append(kept, t)
				has[fold(t)] = true
			}
		}
		for _, t := range adds {
			if has[fold(t)] {
				continue
			}
			if c, ok := canon[fold(t)]; ok {
				t = c
			}
			kept = append(kept, t)
			has[fold(t)] = true
		}
		if len(kept) > MaxTags {
			return fmt.Errorf("%w : %d tags max par élément", ErrInvalidPath, MaxTags)
		}
		sort.Slice(kept, func(i, j int) bool { return fold(kept[i]) < fold(kept[j]) })
		m.Tags = kept
		return nil
	})
}

// Tags liste les tags utilisés, triés par nom, avec leur nombre d'éléments.
func (s *Store) Tags() ([]TagCount, error) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if err := s.loadMeta(); err != nil {
		return nil, err
	}
	idx := map[string]int{}
	out := []TagCount{}
	for _, m := range s.meta {
		for _, t := range m.Tags {
			k := fold(t)
			if i, ok := idx[k]; ok {
				out[i].Count++
			} else {
				idx[k] = len(out)
				out = append(out, TagCount{Name: t, Count: 1})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return fold(out[i].Name) < fold(out[j].Name) })
	return out, nil
}

// Les opérations suivantes accompagnent Move/Copy/Delete (au mieux).

func under(k, c string) bool { return k == c || strings.HasPrefix(k, c+"/") }

func (s *Store) moveMeta(from, to string) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return
	}
	moved := map[string]itemMeta{}
	for k, m := range s.meta {
		if under(k, from) {
			delete(s.meta, k)
			moved[to+k[len(from):]] = m
		}
	}
	for k, m := range moved {
		s.meta[k] = m
	}
	if len(moved) > 0 {
		_ = s.saveMeta()
	}
}

func (s *Store) copyMeta(from, to string) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return
	}
	if m, ok := s.meta[from]; ok && len(m.Tags) > 0 {
		s.meta[to] = itemMeta{Tags: append([]string(nil), m.Tags...)} // pas le favori
		_ = s.saveMeta()
	}
}

func (s *Store) removeMeta(c string) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return
	}
	changed := false
	for k := range s.meta {
		if under(k, c) {
			delete(s.meta, k)
			changed = true
		}
	}
	if changed {
		_ = s.saveMeta()
	}
}
