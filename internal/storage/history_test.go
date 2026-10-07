package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryInterval(t *testing.T) {
	s := newStore(t)
	s.History = HistoryPolicy{Keep: 3, Interval: time.Hour}
	p := "a/b.excalidraw"
	if _, _, err := s.Write(p, []byte(`{"v":1}`), "", true); err != nil {
		t.Fatal(err)
	}
	if vs, _ := s.Versions(p); len(vs) != 0 {
		t.Fatalf("création : aucune version attendue, %+v", vs)
	}
	s.Write(p, []byte(`{"v":2}`), "", false) // v1 conservée
	s.Write(p, []byte(`{"v":3}`), "", false) // < Interval : rien
	vs, err := s.Versions(p)
	if err != nil || len(vs) != 1 {
		t.Fatalf("Versions = %+v, %v", vs, err)
	}
	if b, _ := s.ReadVersion(p, vs[0].ID); string(b) != `{"v":1}` {
		t.Fatalf("version = %s", b)
	}

	// Intervalle nul : une version par écriture, rétention à 3.
	s.History.Interval = 0
	for i := 4; i <= 8; i++ {
		time.Sleep(2 * time.Millisecond) // dates de fichier distinctes
		s.Write(p, []byte(`{"v":`+string(rune('0'+i))+`}`), "", false)
	}
	vs, _ = s.Versions(p)
	if len(vs) != 3 {
		t.Fatalf("rétention : %d versions", len(vs))
	}
	if b, _ := s.ReadVersion(p, vs[0].ID); string(b) != `{"v":7}` {
		t.Fatalf("plus récente = %s", b)
	}
}

func TestHistoryRestoreMoveDelete(t *testing.T) {
	s := newStore(t)
	s.History = HistoryPolicy{Keep: 10, Interval: time.Hour}
	p := "b.excalidraw"
	s.Write(p, []byte(`{"v":1}`), "", true)
	v1, err := s.Snapshot(p)
	if err != nil || v1.ID == "" {
		t.Fatalf("Snapshot = %+v, %v", v1, err)
	}
	if again, _ := s.Snapshot(p); again.ID != v1.ID {
		t.Fatalf("version identique dupliquée : %+v", again)
	}
	time.Sleep(2 * time.Millisecond)
	s.Write(p, []byte(`{"v":2}`), "", false) // < Interval : pas de version

	_, etag, err := s.Restore(p, v1.ID)
	if err != nil || etag != ETag([]byte(`{"v":1}`)) {
		t.Fatalf("Restore : %s, %v", etag, err)
	}
	vs, _ := s.Versions(p)
	if len(vs) != 2 {
		t.Fatalf("la restauration doit conserver le contenu remplacé : %+v", vs)
	}
	if b, _ := s.ReadVersion(p, vs[0].ID); string(b) != `{"v":2}` {
		t.Fatalf("contenu remplacé = %s", b)
	}

	for _, bad := range []string{"../x", "2026", ""} {
		if _, err := s.ReadVersion(p, bad); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("ReadVersion(%q) : %v", bad, err)
		}
	}

	// L'historique suit les déplacements (fichier puis dossier parent).
	s.Mkdir("d")
	if _, err := s.Move(p, "d/c.excalidraw"); err != nil {
		t.Fatal(err)
	}
	s.Move("d", "e")
	if vs, _ := s.Versions("e/c.excalidraw"); len(vs) != 2 {
		t.Fatalf("après déplacement : %+v", vs)
	}
	// Supprimé : l'historique part dans la corbeille avec le schéma.
	if err := s.Delete("e/c.excalidraw", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.historyAbs("e/c.excalidraw")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("historique resté en place : %v", err)
	}
	m, _ := filepath.Glob(filepath.Join(s.root, trashDir, "*", historyDir, "c.excalidraw"))
	if len(m) != 1 {
		t.Fatalf("historique absent de la corbeille : %v", m)
	}
}

func TestHistoryDisabled(t *testing.T) {
	s := newStore(t)
	s.History = HistoryPolicy{}
	s.Write("x.excalidraw", []byte(`{"v":1}`), "", true)
	s.Write("x.excalidraw", []byte(`{"v":2}`), "", false)
	if vs, _ := s.Versions("x.excalidraw"); len(vs) != 0 {
		t.Fatalf("historique désactivé : %+v", vs)
	}
	if _, err := s.Snapshot("x.excalidraw"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Snapshot désactivé : %v", err)
	}
}
