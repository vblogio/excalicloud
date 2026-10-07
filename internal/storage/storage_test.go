package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
	if res, _ := s.Search("Y.EXCAL", 10); len(res) != 1 || res[0].Path != "dir/y.excalidraw" {
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
