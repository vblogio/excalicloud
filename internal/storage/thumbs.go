package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Miniatures PNG des schémas, générées par le navigateur et rangées dans
// .thumbs/ en miroir de l'arborescence (a/b.excalidraw -> .thumbs/a/b.excalidraw.png).
// Une miniature plus ancienne que son schéma est considérée comme périmée.

// MaxThumbSize borne la taille d'une miniature acceptée.
const MaxThumbSize = 1 << 20

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

func (s *Store) thumbAbs(c string, isDir bool) string {
	p := filepath.Join(s.root, thumbsDir, filepath.FromSlash(c))
	if isDir {
		return p
	}
	return p + ".png"
}

// ReadThumb renvoie la miniature d'un schéma et sa date, ou ErrNotFound si
// elle est absente ou périmée.
func (s *Store) ReadThumb(rel string) ([]byte, time.Time, error) {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return nil, time.Time{}, err
	}
	if err := requireSchemaPath(c); err != nil {
		return nil, time.Time{}, err
	}
	src, err := os.Stat(abs)
	if err != nil {
		return nil, time.Time{}, mapErr(err)
	}
	ta := s.thumbAbs(c, false)
	ti, err := os.Stat(ta)
	if err != nil {
		return nil, time.Time{}, mapErr(err)
	}
	if ti.ModTime().Before(src.ModTime()) {
		return nil, time.Time{}, fmt.Errorf("%w : miniature périmée", ErrNotFound)
	}
	b, err := os.ReadFile(ta)
	return b, ti.ModTime(), mapErr(err)
}

// WriteThumb enregistre la miniature PNG d'un schéma existant.
func (s *Store) WriteThumb(rel string, png []byte) error {
	c, abs, err := s.resolve(rel)
	if err != nil {
		return err
	}
	if err := requireSchemaPath(c); err != nil {
		return err
	}
	if !bytes.HasPrefix(png, pngMagic) || len(png) > MaxThumbSize {
		return fmt.Errorf("%w : PNG de %d Ko max attendu", ErrInvalidPath, MaxThumbSize>>10)
	}
	if _, err := os.Stat(abs); err != nil {
		return mapErr(err)
	}
	ta := s.thumbAbs(c, false)
	if err := os.MkdirAll(filepath.Dir(ta), 0o755); err != nil {
		return err
	}
	return atomicWrite(ta, png)
}

// Les opérations suivantes accompagnent Move/Copy/Delete. Elles sont
// « au mieux » : une miniature manquante sera simplement régénérée.

func (s *Store) moveThumb(from, to string, isDir bool) {
	src, dst := s.thumbAbs(from, isDir), s.thumbAbs(to, isDir)
	if _, err := os.Stat(src); err != nil {
		return
	}
	_ = os.RemoveAll(dst)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
		_ = os.Rename(src, dst)
	}
}

func (s *Store) copyThumb(from, to string) {
	cf, err1 := Clean(from)
	ct, err2 := Clean(to)
	if err1 != nil || err2 != nil {
		return
	}
	b, err := os.ReadFile(s.thumbAbs(cf, false))
	if err != nil {
		return
	}
	dst := s.thumbAbs(ct, false)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
		_ = atomicWrite(dst, b) // écrite après la copie du schéma : donc à jour
	}
}

func (s *Store) removeThumb(c string, isDir bool) {
	_ = os.RemoveAll(s.thumbAbs(c, isDir))
}
