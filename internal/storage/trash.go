package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Corbeille : Delete range chaque élément supprimé dans .trash/<id>/, avec
//
//	.trash/<id>/<nom>                l'élément lui-même
//	.trash/<id>/.history/<nom>       son historique de versions
//	.trash/<id>/.info.json           chemin d'origine, date, auteur, tags et icônes
//
// L'élément peut être restauré à son emplacement d'origine ou supprimé
// définitivement. Les éléments plus anciens que Settings.TrashDays (0 : jamais)
// sont purgés automatiquement (Spaces.PurgeTrash).

const trashInfoFile = ".info.json"

const trashLayout = "20060102T150405.000000000"

var trashID = regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}$`)

type trashInfo struct {
	Path    string              `json:"path"`
	Deleted time.Time           `json:"deleted"`
	By      string              `json:"by,omitempty"`
	Meta    map[string]itemMeta `json:"meta,omitempty"` // clés relatives à Path
}

// TrashItem décrit un élément de la corbeille. Path est son chemin d'origine.
type TrashItem struct {
	ID      string
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Deleted time.Time
	By      string
}

func (s *Store) trashAbs(id string) string { return filepath.Join(s.root, trashDir, id) }

// trash range l'élément c (chemin absolu abs) dans une nouvelle entrée de
// corbeille, avec son historique et ses métadonnées.
func (s *Store) trash(c, abs, by string) error {
	now := time.Now().UTC()
	id := now.Format(trashLayout)
	dir := s.trashAbs(id)
	for i := 1; ; i++ { // deux suppressions dans la même nanoseconde
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			break
		}
		id = now.Add(time.Duration(i)).Format(trashLayout)
		dir = s.trashAbs(id)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Rename(abs, filepath.Join(dir, filepath.Base(abs))); err != nil {
		_ = os.Remove(dir)
		return err
	}
	info := trashInfo{Path: c, Deleted: now, By: by, Meta: s.takeMeta(c)}
	if b, err := json.Marshal(info); err == nil {
		_ = atomicWrite(filepath.Join(dir, trashInfoFile), b)
	}
	s.trashHistory(c, dir)
	return nil
}

// trashEntry lit une entrée de la corbeille : nom de l'élément et informations
// (déduites du nom et de l'identifiant pour les entrées d'avant .info.json).
func (s *Store) trashEntry(id string) (string, trashInfo, error) {
	if !trashID.MatchString(id) {
		return "", trashInfo{}, fmt.Errorf("%w : élément de corbeille %q", ErrInvalidPath, id)
	}
	dir := s.trashAbs(id)
	des, err := os.ReadDir(dir)
	if err != nil {
		return "", trashInfo{}, mapErr(err)
	}
	name := ""
	for _, de := range des {
		if !strings.HasPrefix(de.Name(), ".") {
			name = de.Name()
			break
		}
	}
	if name == "" {
		return "", trashInfo{}, ErrNotFound
	}
	var info trashInfo
	if b, err := os.ReadFile(filepath.Join(dir, trashInfoFile)); err == nil {
		_ = json.Unmarshal(b, &info)
	}
	if c, err := Clean(info.Path); err != nil || c == "" || path.Base(c) != name {
		info.Path = name // emplacement inconnu : restauration à la racine
	}
	if info.Deleted.IsZero() {
		info.Deleted, _ = time.Parse(trashLayout, id)
	}
	return name, info, nil
}

// TrashItems liste la corbeille, les suppressions les plus récentes en premier.
func (s *Store) TrashItems() ([]TrashItem, error) {
	des, err := os.ReadDir(filepath.Join(s.root, trashDir))
	if errors.Is(err, fs.ErrNotExist) {
		return []TrashItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []TrashItem{}
	for _, de := range des {
		if !de.IsDir() || !trashID.MatchString(de.Name()) {
			continue
		}
		name, info, err := s.trashEntry(de.Name())
		if err != nil {
			continue
		}
		fi, err := os.Lstat(filepath.Join(s.trashAbs(de.Name()), name))
		if err != nil {
			continue
		}
		it := TrashItem{ID: de.Name(), Name: name, Path: info.Path, IsDir: fi.IsDir(), Deleted: info.Deleted, By: info.By}
		if !it.IsDir {
			it.Size = fi.Size()
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// TrashCount renvoie le nombre d'éléments dans la corbeille.
func (s *Store) TrashCount() int {
	des, _ := os.ReadDir(filepath.Join(s.root, trashDir))
	n := 0
	for _, de := range des {
		if de.IsDir() && trashID.MatchString(de.Name()) {
			n++
		}
	}
	return n
}

// freeName renvoie c s'il est libre, sinon « nom (restauré) », « nom (restauré 2) »...
func (s *Store) freeName(c string, isDir bool) string {
	if _, err := os.Lstat(absOf(s, c)); errors.Is(err, fs.ErrNotExist) {
		return c
	}
	dir, base, ext := path.Dir(c), path.Base(c), ""
	if !isDir {
		ext = path.Ext(base)
		base = strings.TrimSuffix(base, ext)
	}
	for i := 1; ; i++ {
		suffix := " (restauré)"
		if i > 1 {
			suffix = fmt.Sprintf(" (restauré %d)", i)
		}
		n := base + suffix + ext
		if dir != "." {
			n = dir + "/" + n
		}
		if _, err := os.Lstat(absOf(s, n)); errors.Is(err, fs.ErrNotExist) {
			return n
		}
	}
}

// RestoreTrash remet un élément à son emplacement d'origine (dossiers parents
// recréés au besoin), sous un autre nom si la place est prise.
func (s *Store) RestoreTrash(id string) (Entry, error) {
	name, info, err := s.trashEntry(id)
	if err != nil {
		return Entry{}, err
	}
	dir := s.trashAbs(id)
	src := filepath.Join(dir, name)
	fi, err := os.Lstat(src)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	c := s.freeName(info.Path, fi.IsDir())
	_, abs, err := s.resolve(c)
	if err != nil {
		return Entry{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(src, abs); err != nil {
		return Entry{}, mapErr(err)
	}
	moveAside(filepath.Join(dir, historyDir, name), s.historyAbs(c))
	s.putMeta(c, info.Meta)
	_ = os.RemoveAll(dir)
	if fi, err = os.Stat(abs); err != nil {
		return Entry{}, err
	}
	e := toEntry(c, fi)
	decorate(&e, s.metaSnapshot())
	return e, nil
}

// DropTrash supprime définitivement un élément de la corbeille.
func (s *Store) DropTrash(id string) error {
	if _, _, err := s.trashEntry(id); err != nil {
		return err
	}
	return os.RemoveAll(s.trashAbs(id))
}

// EmptyTrash vide la corbeille.
func (s *Store) EmptyTrash() error {
	return s.purgeTrash(func(string) bool { return true })
}

// PurgeTrashOlder supprime définitivement les éléments supprimés il y a plus de maxAge.
func (s *Store) PurgeTrashOlder(maxAge time.Duration) error {
	limit := time.Now().Add(-maxAge)
	return s.purgeTrash(func(id string) bool {
		t, err := time.Parse(trashLayout, id)
		return err == nil && t.Before(limit)
	})
}

func (s *Store) purgeTrash(drop func(id string) bool) error {
	root := filepath.Join(s.root, trashDir)
	des, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var first error
	for _, de := range des {
		if !de.IsDir() || !trashID.MatchString(de.Name()) || !drop(de.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, de.Name())); err != nil && first == nil {
			first = err
		}
	}
	return first
}
