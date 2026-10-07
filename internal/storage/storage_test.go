package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const doc = `{"type":"excalidraw","version":2,"elements":[]}`

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestClean(t *testing.T) {
	ok := map[string]string{"": "", "/": "", "a/b/../c": "a/c", "/../../x": "x"}
	for in, want := range ok {
		if got, err := Clean(in); err != nil || got != want {
			t.Errorf("Clean(%q) = %q, %v ; attendu %q", in, got, err, want)
		}
	}
	for _, in := range []string{".trash/x", "a/.git", `a\b`} {
		if _, err := Clean(in); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Clean(%q) aurait dû échouer", in)
		}
	}
}

func TestWriteReadConflict(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.Write("x.txt", []byte(doc), "", false); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("extension non vérifiée : %v", err)
	}
	if _, _, err := s.Write("a/b.excalidraw", []byte("{"), "", false); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("JSON non vérifié : %v", err)
	}
	_, etag, err := s.Write("a/b.excalidraw", []byte(doc), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Write("a/b.excalidraw", []byte(doc), "", true); !errors.Is(err, ErrExists) {
		t.Fatalf("création en double : %v", err)
	}
	if _, _, err := s.Write("a/b.excalidraw", []byte(`{"v":1}`), etag, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Write("a/b.excalidraw", []byte(`{"v":2}`), etag, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflit non détecté : %v", err)
	}
	b, _, err := s.Read("a/b.excalidraw")
	if err != nil || string(b) != `{"v":1}` {
		t.Fatalf("lecture : %s, %v", b, err)
	}
}

func TestListMoveDelete(t *testing.T) {
	s := newStore(t)
	s.Write("z.excalidraw", []byte(doc), "", false)
	s.Mkdir("dir")
	os.WriteFile(filepath.Join(s.root, "ignore.png"), nil, 0o644)

	l, err := s.List("")
	if err != nil || len(l) != 2 || !l[0].IsDir || l[1].Name != "z.excalidraw" {
		t.Fatalf("List = %+v, %v", l, err)
	}
	if _, err := s.Move("z.excalidraw", "dir/y.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move("dir", "dir/sub"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("déplacement dans soi-même : %v", err)
	}
	if res, _ := s.Search("Y.EXCAL", Filter{}, 10); len(res) != 1 || res[0].Path != "dir/y.excalidraw" {
		t.Fatalf("Search = %+v", res)
	}
	if err := s.Delete("dir", false); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("suppression non récursive : %v", err)
	}
	if err := s.Delete("dir", true); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.List(""); len(l) != 0 {
		t.Fatalf("racine non vide après suppression : %+v", l)
	}
}

func TestSymlinkEscape(t *testing.T) {
	s := newStore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.root, "evil")); err != nil {
		t.Skip(err)
	}
	if _, _, err := s.Write("evil/x.excalidraw", []byte(doc), "", false); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("échappement par symlink non bloqué : %v", err)
	}
}

func TestMoveMany(t *testing.T) {
	s := newStore(t)
	for _, p := range []string{"a.excalidraw", "b.excalidraw", "src/c.excalidraw", "dst/b.excalidraw"} {
		if _, _, err := s.Write(p, []byte(doc), "", true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Mkdir("src/sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveMany([]string{"a.excalidraw"}, "a.excalidraw"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("destination fichier acceptée : %v", err)
	}
	res, err := s.MoveMany([]string{"a.excalidraw", "b.excalidraw", "src/c.excalidraw", "src/sub", "dst/b.excalidraw", "dst", "nope.excalidraw"}, "dst")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		to  string
		err error
	}{
		{"dst/a.excalidraw", nil},
		{"", ErrExists},
		{"dst/c.excalidraw", nil},
		{"dst/sub", nil},
		{"dst/b.excalidraw", nil}, // déjà dans dst : ignoré
		{"", ErrInvalidPath},      // dossier dans lui-même
		{"", ErrNotFound},
	}
	for i, w := range want {
		if res[i].To != w.to || !errors.Is(res[i].Err(), w.err) {
			t.Errorf("%s : to=%q err=%v ; attendu %q, %v", res[i].From, res[i].To, res[i].Err(), w.to, w.err)
		}
	}
	if _, _, err := s.Read("b.excalidraw"); err != nil {
		t.Fatalf("b déplacé malgré le conflit : %v", err)
	}
	// Vers la racine.
	res, err = s.MoveMany([]string{"dst/sub", "b.excalidraw"}, "")
	if err != nil || res[0].To != "sub" || res[0].Err() != nil || res[1].To != "b.excalidraw" {
		t.Fatalf("vers la racine : %+v, %v", res, err)
	}
}

var fakePNG = []byte("\x89PNG\r\n\x1a\nfake")

func TestThumbs(t *testing.T) {
	s := newStore(t)
	if err := s.WriteThumb("d/a.excalidraw", fakePNG); !errors.Is(err, ErrNotFound) {
		t.Fatalf("miniature d'un schéma absent : %v", err)
	}
	if _, _, err := s.Write("d/a.excalidraw", []byte(doc), "", true); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteThumb("d/a.excalidraw", []byte("<svg/>")); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("non-PNG accepté : %v", err)
	}
	if _, _, err := s.ReadThumb("d/a.excalidraw"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("miniature inexistante : %v", err)
	}
	if err := s.WriteThumb("d/a.excalidraw", fakePNG); err != nil {
		t.Fatal(err)
	}
	if b, _, err := s.ReadThumb("d/a.excalidraw"); err != nil || string(b) != string(fakePNG) {
		t.Fatalf("lecture : %q, %v", b, err)
	}
	// Suit le déplacement du dossier, la copie, puis la suppression.
	if _, err := s.Move("d", "e"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReadThumb("e/a.excalidraw"); err != nil {
		t.Fatalf("après déplacement : %v", err)
	}
	if _, err := s.Copy("e/a.excalidraw", "e/b.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReadThumb("e/b.excalidraw"); err != nil {
		t.Fatalf("après copie : %v", err)
	}
	if err := s.Delete("e/a.excalidraw", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.root, thumbsDir, "e", "a.excalidraw.png")); !os.IsNotExist(err) {
		t.Fatalf("miniature non supprimée : %v", err)
	}
	// Périmée après modification du schéma.
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(filepath.Join(s.root, thumbsDir, "e", "b.excalidraw.png"), old, old)
	if _, _, err := s.ReadThumb("e/b.excalidraw"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("miniature périmée servie : %v", err)
	}
}
