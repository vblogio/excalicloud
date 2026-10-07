package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Historique des versions : avant qu'un schéma soit écrasé, son contenu
// précédent est copié dans .history/ en miroir de l'arborescence
// (a/b.excalidraw -> .history/a/b.excalidraw/<date>.excalidraw).
//
// Avec l'enregistrement automatique (toutes les 1,5 s pendant l'édition), une
// copie par écriture serait inutilisable : une version n'est prise que si la
// précédente date d'au moins HistoryPolicy.Interval. Restaurer une version ou
// créer une version à la demande copie toujours le contenu courant.

const historyDir = ".history"

// HistoryPolicy règle la prise et la rétention des versions.
type HistoryPolicy struct {
	Keep     int           // versions conservées par schéma (0 : historique désactivé)
	Interval time.Duration // écart minimal entre deux versions automatiques
	MaxAge   time.Duration // âge maximal d'une version (0 : illimité)
}

// DefaultHistory : 50 versions par schéma, au plus une toutes les 10 minutes.
var DefaultHistory = HistoryPolicy{Keep: 50, Interval: 10 * time.Minute}

// Version décrit une version enregistrée d'un schéma. Time est la date à
// laquelle ce contenu avait été enregistré.
type Version struct {
	ID   string    `json:"id"`
	Time time.Time `json:"time"`
	Size int64     `json:"size"`
}

const versionLayout = "20060102T150405.000000000Z"

var versionID = regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}Z$`)

func (s *Store) historyAbs(c string) string {
	return filepath.Join(s.root, historyDir, filepath.FromSlash(c))
}

// versions liste les versions d'un schéma, la plus récente en premier.
// taken renvoie en plus la date de prise de la plus récente (date du fichier).
func (s *Store) versions(c string) ([]Version, time.Time, error) {
	des, err := os.ReadDir(s.historyAbs(c))
	if errors.Is(err, fs.ErrNotExist) {
		return []Version{}, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	out := []Version{}
	var taken time.Time
	for _, de := range des {
		id, ok := strings.CutSuffix(de.Name(), Ext)
		if !ok || de.IsDir() || !versionID.MatchString(id) {
			continue
		}
		t, err := time.Parse(versionLayout, id)
		if err != nil {
			continue
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, Version{ID: id, Time: t, Size: fi.Size()})
		if fi.ModTime().After(taken) {
			taken = fi.ModTime()
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, taken, nil
}

// snapshot enregistre data (contenu d'un schéma daté de mod) comme version.
// Sans force, rien n'est pris si la dernière version a moins de Interval.
// Une version identique à la plus récente n'est jamais dupliquée.
func (s *Store) snapshot(c string, data []byte, mod time.Time, force bool) (Version, error) {
	p := s.History
	if p.Keep <= 0 {
		return Version{}, nil
	}
	vs, taken, err := s.versions(c)
	if err != nil {
		return Version{}, err
	}
	if !force && len(vs) > 0 && time.Since(taken) < p.Interval {
		return Version{}, nil
	}
	if len(vs) > 0 && vs[0].Size == int64(len(data)) {
		if last, err := os.ReadFile(s.versionAbs(c, vs[0].ID)); err == nil && bytes.Equal(last, data) {
			return vs[0], nil
		}
	}
	v := Version{ID: mod.UTC().Format(versionLayout), Time: mod.UTC(), Size: int64(len(data))}
	dir := s.historyAbs(c)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Version{}, err
	}
	if err := atomicWrite(s.versionAbs(c, v.ID), data); err != nil {
		return Version{}, err
	}
	s.pruneHistory(c)
	return v, nil
}

func (s *Store) versionAbs(c, id string) string {
	return filepath.Join(s.historyAbs(c), id+Ext)
}

// pruneHistory applique la rétention (nombre et âge) aux versions d'un schéma.
func (s *Store) pruneHistory(c string) {
	vs, _, err := s.versions(c)
	if err != nil {
		return
	}
	for i, v := range vs {
		if i >= s.History.Keep || (s.History.MaxAge > 0 && time.Since(v.Time) > s.History.MaxAge) {
			_ = os.Remove(s.versionAbs(c, v.ID))
		}
	}
}

// autoSnapshot est appelé par Write avant d'écraser un schéma ; un échec est
// journalisé mais n'empêche pas l'enregistrement.
func (s *Store) autoSnapshot(c string, cur []byte, mod time.Time) {
	if _, err := s.snapshot(c, cur, mod, false); err != nil {
		slog.Warn("historique : version non enregistrée", "path", c, "err", err)
	}
}

func (s *Store) schema(rel string) (string, string, error) {
	c, abs, err := s.resolve(rel)
	if err == nil {
		err = requireSchemaPath(c)
	}
	return c, abs, err
}

// Versions liste les versions d'un schéma existant, la plus récente en premier.
func (s *Store) Versions(rel string) ([]Version, error) {
	c, abs, err := s.schema(rel)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, mapErr(err)
	}
	vs, _, err := s.versions(c)
	return vs, err
}

// ReadVersion renvoie le contenu d'une version.
func (s *Store) ReadVersion(rel, id string) ([]byte, error) {
	c, _, err := s.schema(rel)
	if err != nil {
		return nil, err
	}
	if !versionID.MatchString(id) {
		return nil, fmt.Errorf("%w : version %q", ErrInvalidPath, id)
	}
	b, err := os.ReadFile(s.versionAbs(c, id))
	return b, mapErr(err)
}

// Snapshot crée une version du contenu courant, quel que soit l'intervalle.
func (s *Store) Snapshot(rel string) (Version, error) {
	c, abs, err := s.schema(rel)
	if err != nil {
		return Version{}, err
	}
	if s.History.Keep <= 0 {
		return Version{}, fmt.Errorf("%w : historique désactivé (HISTORY_KEEP=0)", ErrInvalidPath)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Version{}, mapErr(err)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return Version{}, mapErr(err)
	}
	return s.snapshot(c, b, fi.ModTime(), true)
}

// Restore remplace le contenu d'un schéma par une de ses versions. Le contenu
// courant est d'abord enregistré comme version : une restauration s'annule.
func (s *Store) Restore(rel, id string) (Entry, string, error) {
	data, err := s.ReadVersion(rel, id)
	if err != nil {
		return Entry{}, "", err
	}
	c, abs, _ := s.schema(rel)
	fi, err := os.Stat(abs)
	if err != nil {
		return Entry{}, "", mapErr(err)
	}
	cur, err := os.ReadFile(abs)
	if err != nil {
		return Entry{}, "", mapErr(err)
	}
	if !bytes.Equal(cur, data) {
		if _, err := s.snapshot(c, cur, fi.ModTime(), true); err != nil {
			return Entry{}, "", err
		}
		if err := atomicWrite(abs, data); err != nil {
			return Entry{}, "", err
		}
	}
	if fi, err = os.Stat(abs); err != nil {
		return Entry{}, "", err
	}
	return toEntry(c, fi), ETag(data), nil
}

// Les opérations suivantes accompagnent Move/Delete.

func (s *Store) moveHistory(from, to string) {
	src, dst := s.historyAbs(from), s.historyAbs(to)
	if _, err := os.Stat(src); err != nil {
		return
	}
	_ = os.RemoveAll(dst)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
		_ = os.Rename(src, dst)
	}
}

// trashHistory range l'historique d'un élément supprimé avec lui dans la
// corbeille (trash/.history/<nom>), pour pouvoir le récupérer à la main.
func (s *Store) trashHistory(c, trash string) {
	src := s.historyAbs(c)
	if _, err := os.Stat(src); err != nil {
		return
	}
	dst := filepath.Join(trash, historyDir, filepath.Base(src))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
		_ = os.Rename(src, dst)
	}
}
