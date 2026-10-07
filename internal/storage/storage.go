// Package storage gère l'arborescence de schémas Excalidraw sur disque.
//
// Tous les chemins manipulés par l'API sont relatifs à la racine (DATA_DIR),
// au format "slash" (a/b/schema.excalidraw). Le package garantit qu'aucun
// chemin ne sort de la racine, y compris via un lien symbolique.
package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Extension des fichiers de schéma gérés.
const Ext = ".excalidraw"

const (
	trashDir    = ".trash"
	thumbsDir   = ".thumbs"
	libraryFile = ".library.excalidrawlib"
)

var (
	ErrNotFound    = errors.New("introuvable")
	ErrExists      = errors.New("existe déjà")
	ErrInvalidPath = errors.New("chemin invalide")
	ErrConflict    = errors.New("le fichier a été modifié entre-temps")
	ErrNotEmpty    = errors.New("dossier non vide")
	ErrInvalidJSON = errors.New("contenu JSON invalide")
)

// Entry décrit un élément de l'arborescence.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`

	Tags     []string `json:"tags,omitempty"`
	Favorite bool     `json:"favorite,omitempty"`
}

// Store est un dépôt de schémas enraciné dans un répertoire.
type Store struct {
	root string

	// History règle l'historique des versions (DefaultHistory par défaut).
	History HistoryPolicy

	settingsMu sync.Mutex // sérialise les lectures-modifications de .settings.json
	index      textIndex  // texte des schémas pour la recherche (search.go)

	metaMu sync.Mutex          // tags et favoris (meta.go)
	meta   map[string]itemMeta // nil : pas encore chargé
}

// New crée le répertoire racine si besoin et renvoie un Store.
func New(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	// Résout un éventuel lien symbolique sur la racine elle-même pour que
	// les comparaisons de préfixe soient fiables.
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return nil, err
	}
	return &Store{root: abs, History: DefaultHistory}, nil
}

// Clean normalise un chemin relatif : "/a/../b/" -> "b", "" -> "".
// Les composants cachés (".trash", ".git"...) sont refusés.
func Clean(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) || strings.Contains(rel, `\`) {
		return "", ErrInvalidPath
	}
	c := strings.TrimPrefix(path.Clean("/"+rel), "/")
	if c == "" {
		return "", nil
	}
	for _, part := range strings.Split(c, "/") {
		if strings.HasPrefix(part, ".") {
			return "", ErrInvalidPath
		}
	}
	return c, nil
}

// resolve convertit un chemin relatif en chemin absolu sous la racine et
// vérifie que l'ancêtre existant le plus proche ne s'échappe pas via symlink.
func (s *Store) resolve(rel string) (string, string, error) {
	c, err := Clean(rel)
	if err != nil {
		return "", "", err
	}
	abs := filepath.Join(s.root, filepath.FromSlash(c))
	probe := abs
	for {
		real, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if real != s.root && !strings.HasPrefix(real, s.root+string(os.PathSeparator)) {
				return "", "", ErrInvalidPath
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", "", ErrInvalidPath
		}
		probe = parent
	}
	return c, abs, nil
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return ErrNotFound
	case errors.Is(err, fs.ErrExist):
		return ErrExists
	}
	return err
}

func toEntry(rel string, fi fs.FileInfo) Entry {
	return Entry{
		Name:    fi.Name(),
		Path:    rel,
		IsDir:   fi.IsDir(),
		Size:    fi.Size(),
		ModTime: fi.ModTime().UTC(),
	}
}

func visible(name string, isDir bool) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	return isDir || strings.EqualFold(filepath.Ext(name), Ext)
}

// List renvoie le contenu d'un dossier : dossiers d'abord, puis schémas,
// triés par nom (insensible à la casse).
func (s *Store) List(rel string) ([]Entry, error) {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]Entry, 0, len(des))
	meta := s.metaSnapshot()
	for _, de := range des {
		if !visible(de.Name(), de.IsDir()) || de.Type()&fs.ModeSymlink != 0 {
			continue
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		e := toEntry(path.Join(c, de.Name()), fi)
		decorate(&e, meta)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ETag calcule l'empreinte d'un contenu (utilisée pour l'écriture optimiste).
func ETag(b []byte) string {
	h := sha256.Sum256(b)
	return `"` + hex.EncodeToString(h[:16]) + `"`
}

func requireSchemaPath(c string) error {
	if c == "" || !strings.EqualFold(path.Ext(c), Ext) {
		return fmt.Errorf("%w : extension %s attendue", ErrInvalidPath, Ext)
	}
	return nil
}

// Read renvoie le contenu d'un schéma et son ETag.
func (s *Store) Read(rel string) ([]byte, string, error) {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return nil, "", err
	}
	if err := requireSchemaPath(c); err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", mapErr(err)
	}
	return b, ETag(b), nil
}

// Write enregistre un schéma de façon atomique (fichier temporaire + rename).
//   - ifMatch != "" : l'écriture échoue avec ErrConflict si le contenu actuel
//     ne correspond pas à cet ETag (protection contre l'écrasement).
//   - create == true : l'écriture échoue avec ErrExists si le fichier existe.
func (s *Store) Write(rel string, data []byte, ifMatch string, create bool) (Entry, string, error) {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return Entry{}, "", err
	}
	if err := requireSchemaPath(c); err != nil {
		return Entry{}, "", err
	}
	if !json.Valid(data) {
		return Entry{}, "", ErrInvalidJSON
	}
	cur, err := os.ReadFile(abs)
	switch {
	case err == nil && create:
		return Entry{}, "", ErrExists
	case err == nil && ifMatch != "" && ifMatch != "*" && ETag(cur) != ifMatch:
		return Entry{}, "", ErrConflict
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return Entry{}, "", err
	case err == nil && !bytes.Equal(cur, data):
		if fi, err := os.Stat(abs); err == nil {
			s.autoSnapshot(c, cur, fi.ModTime())
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Entry{}, "", err
	}
	if err := atomicWrite(abs, data); err != nil {
		return Entry{}, "", err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Entry{}, "", err
	}
	return toEntry(c, fi), ETag(data), nil
}

func atomicWrite(abs string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op après un rename réussi
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), abs)
}

// Mkdir crée un dossier (et ses parents).
func (s *Store) Mkdir(rel string) (Entry, error) {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return Entry{}, err
	}
	if c == "" {
		return Entry{}, ErrInvalidPath
	}
	if _, err := os.Stat(abs); err == nil {
		return Entry{}, ErrExists
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return Entry{}, mapErr(err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Entry{}, err
	}
	return toEntry(c, fi), nil
}

// Move renomme ou déplace un fichier ou un dossier. La destination ne doit
// pas exister ; un dossier ne peut pas être déplacé dans lui-même.
func (s *Store) Move(from, to string) (Entry, error) {
	cf, af, err := s.resolve(from)
	if err != nil {
		return Entry{}, err
	}
	ct, at, err := s.resolve(to)
	if err != nil {
		return Entry{}, err
	}
	if cf == "" || ct == "" {
		return Entry{}, ErrInvalidPath
	}
	fi, err := os.Lstat(af)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	if fi.IsDir() {
		if ct == cf || strings.HasPrefix(ct, cf+"/") {
			return Entry{}, fmt.Errorf("%w : déplacement d'un dossier dans lui-même", ErrInvalidPath)
		}
	} else if err := requireSchemaPath(ct); err != nil {
		return Entry{}, err
	}
	if _, err := os.Lstat(at); err == nil {
		return Entry{}, ErrExists
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(af, at); err != nil {
		return Entry{}, mapErr(err)
	}
	s.moveThumb(cf, ct, fi.IsDir())
	s.moveHistory(cf, ct)
	s.moveMeta(cf, ct)
	fi, err = os.Stat(at)
	if err != nil {
		return Entry{}, err
	}
	return toEntry(ct, fi), nil
}

// MoveResult est le résultat d'un déplacement unitaire dans MoveMany.
type MoveResult struct {
	From  string `json:"from"`
	To    string `json:"to,omitempty"`
	Error string `json:"error,omitempty"`
	err   error
}

// Err renvoie l'erreur du déplacement (nil si réussi).
func (r MoveResult) Err() error { return r.err }

// MoveMany déplace plusieurs fichiers ou dossiers dans le dossier dir en
// conservant leur nom. Chaque élément est traité indépendamment : un échec
// (destination existante, dossier déplacé dans lui-même...) n'interrompt pas
// les autres. Les éléments déjà présents dans dir sont ignorés.
func (s *Store) MoveMany(paths []string, dir string) ([]MoveResult, error) {
	cd, ad, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(ad)
	if err != nil {
		return nil, mapErr(err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%w : la destination n'est pas un dossier", ErrInvalidPath)
	}
	out := make([]MoveResult, 0, len(paths))
	for _, p := range paths {
		r := MoveResult{From: p}
		if c, err := Clean(p); err != nil || c == "" {
			r.err = ErrInvalidPath
		} else if strings.TrimPrefix(path.Dir("/"+c), "/") == cd {
			r.From, r.To = c, c // déjà dans le dossier cible
		} else {
			r.From = c
			var e Entry
			if e, r.err = s.Move(c, path.Join(cd, path.Base(c))); r.err == nil {
				r.To = e.Path
			}
		}
		if r.err != nil {
			r.Error = r.err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

// Copy duplique un schéma (les dossiers ne sont pas copiés).
func (s *Store) Copy(from, to string) (Entry, error) {
	data, _, err := s.Read(from)
	if err != nil {
		return Entry{}, err
	}
	e, _, err := s.Write(to, data, "", true)
	if err == nil {
		s.copyThumb(from, e.Path)
		if cf, err := Clean(from); err == nil {
			s.copyMeta(cf, e.Path)
		}
	}
	return e, err
}

// Delete déplace un fichier ou un dossier dans la corbeille (.trash/), sans
// suppression physique. Un dossier non vide n'est supprimé que si recursive.
func (s *Store) Delete(rel string, recursive bool) error {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return err
	}
	if c == "" {
		return ErrInvalidPath
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return mapErr(err)
	}
	if fi.IsDir() && !recursive {
		des, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		if len(des) > 0 {
			return ErrNotEmpty
		}
	}
	trash := filepath.Join(s.root, trashDir, time.Now().UTC().Format("20060102T150405.000000000"))
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	if err := os.Rename(abs, filepath.Join(trash, filepath.Base(abs))); err != nil {
		return err
	}
	s.removeThumb(c, fi.IsDir())
	s.trashHistory(c, trash)
	s.removeMeta(c)
	return nil
}

// ReadLibrary renvoie la bibliothèque Excalidraw partagée (ou nil si absente).
func (s *Store) ReadLibrary() ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(s.root, libraryFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// WriteLibrary enregistre la bibliothèque Excalidraw partagée.
func (s *Store) WriteLibrary(data []byte) error {
	if !json.Valid(data) {
		return ErrInvalidJSON
	}
	return atomicWrite(filepath.Join(s.root, libraryFile), data)
}
