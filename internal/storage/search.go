package storage

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Recherche plein texte : noms des fichiers et dossiers, et contenu des
// schémas (textes et libellés, noms de cadres, liens). Le texte de chaque
// schéma est extrait une fois puis gardé en mémoire tant que le fichier ne
// change pas (date et taille) : seule la première recherche lit tous les
// schémas, les suivantes ne relisent que ceux modifiés.

// Hit est un résultat de recherche. Snippet est l'extrait du contenu qui
// correspond (vide si seul le nom correspond).
type Hit struct {
	Entry
	Snippet string `json:"snippet,omitempty"`
}

type indexDoc struct {
	mod   time.Time
	size  int64
	text  string     // texte extrait, une ligne par élément
	fold  string     // texte normalisé (minuscules, sans accents), même nombre de runes
	stats SceneStats // compteurs d'éléments, dimensions (details.go)
}

type textIndex struct {
	mu   sync.Mutex
	docs map[string]indexDoc
}

// fold met en minuscules et retire les accents des lettres latines courantes,
// rune pour rune : la position d'une correspondance vaut pour le texte d'origine.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if f, ok := foldMap[r]; ok {
			return f
		}
		return r
	}, s)
}

var foldMap = func() map[rune]rune {
	m := map[rune]rune{}
	for base, accented := range map[rune]string{
		'a': "àáâãäåāăą", 'c': "çćĉċč", 'd': "ď", 'e': "èéêëēĕėęě", 'g': "ĝğġģ",
		'i': "ìíîïĩīĭįı", 'n': "ñńņňŉ", 'o': "òóôõöøōŏő", 'r': "ŕŗř", 's': "śŝşš",
		't': "ţťŧ", 'u': "ùúûüũūŭůűų", 'y': "ýÿŷ", 'z': "źżž",
	} {
		for _, r := range accented {
			m[r] = base
		}
	}
	return m
}()

// parseScene extrait le texte d'un schéma (textes et libellés, noms de
// cadres, liens ; une ligne par élément) et ses statistiques (details.go).
func parseScene(data []byte) (string, SceneStats) {
	var doc struct {
		Elements []struct {
			Type         string  `json:"type"`
			Text         string  `json:"text"`
			OriginalText string  `json:"originalText"`
			Name         string  `json:"name"`
			Link         string  `json:"link"`
			IsDeleted    bool    `json:"isDeleted"`
			X            float64 `json:"x"`
			Y            float64 `json:"y"`
			Width        float64 `json:"width"`
			Height       float64 `json:"height"`
		} `json:"elements"`
		AppState struct {
			ViewBackgroundColor string `json:"viewBackgroundColor"`
		} `json:"appState"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return "", SceneStats{}
	}
	var b strings.Builder
	add := func(s string) {
		s = strings.Join(strings.Fields(s), " ") // retours à la ligne du texte replié
		if s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	st := SceneStats{Background: doc.AppState.ViewBackgroundColor}
	var box bounds
	for _, e := range doc.Elements {
		if e.IsDeleted {
			continue
		}
		if e.OriginalText != "" {
			add(e.OriginalText)
		} else {
			add(e.Text)
		}
		add(e.Name)
		add(e.Link)
		st.count(e.Type, e.Name)
		box.add(e.X, e.Y, e.Width, e.Height)
	}
	st.Width, st.Height = box.size()
	return b.String(), st
}

// content renvoie le texte indexé d'un schéma, relu seulement s'il a changé ;
// appelé sous index.mu.
func (s *Store) content(rel, abs string, fi fs.FileInfo) indexDoc {
	d, ok := s.index.docs[rel]
	if ok && d.mod.Equal(fi.ModTime()) && d.size == fi.Size() {
		return d
	}
	d = indexDoc{mod: fi.ModTime(), size: fi.Size()}
	if b, err := os.ReadFile(abs); err == nil {
		d.text, d.stats = parseScene(b)
		d.fold = fold(d.text)
	}
	s.index.docs[rel] = d
	return d
}

// snippet renvoie la ligne du texte qui contient la position i (en octets
// de fold), raccourcie autour de la correspondance.
func snippet(d indexDoc, i, n int) string {
	runes := []rune(d.text)
	at := utf8.RuneCountInString(d.fold[:i])
	end := at + utf8.RuneCountInString(d.fold[i:i+n])
	lo, hi := at, end
	for lo > 0 && runes[lo-1] != '\n' {
		lo--
	}
	for hi < len(runes) && runes[hi] != '\n' {
		hi++
	}
	const ctx = 40
	pre, post := "", ""
	if at-lo > ctx {
		lo, pre = at-ctx, "…"
	}
	if hi-end > ctx {
		hi, post = end+ctx, "…"
	}
	return pre + string(runes[lo:hi]) + post
}

// Filter restreint une recherche aux éléments portant un tag ou favoris.
type Filter struct {
	Tag      string
	Favorite bool
}

// Search renvoie les entrées dont le nom, les tags ou, pour un schéma, le
// contenu contiennent tous les mots de q (casse et accents ignorés), et qui
// passent le filtre. Les correspondances sur le nom viennent en premier.
// q peut être vide si un filtre est donné.
func (s *Store) Search(q string, f Filter, limit int) ([]Hit, error) {
	terms := strings.Fields(fold(q))
	tag := fold(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f.Tag), "#")))
	if len(terms) == 0 && tag == "" && !f.Favorite {
		return []Hit{}, nil
	}
	meta := s.metaSnapshot()
	s.index.mu.Lock()
	defer s.index.mu.Unlock()
	if s.index.docs == nil {
		s.index.docs = map[string]indexDoc{}
	}
	seen := map[string]bool{}
	var byName, byContent []Hit
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == s.root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !visible(d.Name(), d.IsDir()) || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		hit := Hit{Entry: toEntry(rel, fi)}
		decorate(&hit.Entry, meta)
		if f.Favorite && !hit.Favorite {
			return nil
		}
		tags := fold(strings.Join(hit.Tags, "\n"))
		if tag != "" && !strings.Contains("\n"+tags+"\n", "\n"+tag+"\n") {
			return nil
		}
		name := fold(d.Name()) + "\n" + tags
		var doc indexDoc
		if !d.IsDir() {
			seen[rel] = true
			if len(terms) > 0 {
				doc = s.content(rel, p, fi)
			}
		}
		inName := true
		for _, t := range terms {
			if strings.Contains(name, t) {
				continue
			}
			inName = false
			i := strings.Index(doc.fold, t)
			if i < 0 {
				return nil
			}
			if hit.Snippet == "" {
				hit.Snippet = snippet(doc, i, len(t))
			}
		}
		if inName {
			byName = append(byName, hit)
		} else {
			byContent = append(byContent, hit)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for rel := range s.index.docs {
		if !seen[rel] {
			delete(s.index.docs, rel) // schéma supprimé ou déplacé
		}
	}
	out := append(byName, byContent...)
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []Hit{}
	}
	return out, nil
}
