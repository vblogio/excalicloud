package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShares(t *testing.T) {
	s := newStore(t)
	s.Write("d/a.excalidraw", []byte(doc), "", true)

	if _, err := s.CreateShare("d/absent.excalidraw", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("schéma absent : %v", err)
	}
	if _, err := s.CreateShare("d", 0); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("dossier : %v", err)
	}
	sh, err := s.CreateShare("d/a.excalidraw", 0)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(s.root, secretFile)); fi == nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("clé : %v", fi)
	}
	got, data, err := s.ReadShared(sh.Token)
	if err != nil || got.Path != "d/a.excalidraw" || string(data) != doc {
		t.Fatalf("ReadShared = %+v, %s, %v", got, data, err)
	}

	// Jetons altérés ou mal formés.
	bad := []string{"", "x", sh.ID, sh.ID + ".", sh.Token[:len(sh.Token)-1] + "A", "A" + sh.Token[1:]}
	for _, tok := range bad {
		if tok == sh.Token {
			continue
		}
		if _, err := s.ResolveShare(tok); !errors.Is(err, ErrNotFound) {
			t.Errorf("jeton %q accepté : %v", tok, err)
		}
	}

	// La clé survit au redémarrage ; le lien suit le schéma déplacé.
	s2, _ := New(s.root)
	s2.Move("d", "e")
	if got, err := s2.ResolveShare(sh.Token); err != nil || got.Path != "e/a.excalidraw" {
		t.Fatalf("après déplacement : %+v, %v", got, err)
	}
	if l, _ := s2.Shares("e/a.excalidraw"); len(l) != 1 || l[0].Token != sh.Token {
		t.Fatalf("Shares = %+v", l)
	}

	// Révocation unitaire.
	if err := s2.RevokeShare(sh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.ResolveShare(sh.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lien révoqué accepté : %v", err)
	}

	// Expiration : le lien expiré est refusé puis purgé de la liste.
	exp, _ := s2.CreateShare("e/a.excalidraw", time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, err := s2.ResolveShare(exp.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lien expiré accepté : %v", err)
	}
	if l, _ := s2.Shares(""); len(l) != 0 {
		t.Fatalf("lien expiré listé : %+v", l)
	}

	// Tout révoquer change la clé : un lien recréé avec un ancien id ne passe plus.
	keep, _ := s2.CreateShare("e/a.excalidraw", 0)
	if err := s2.RevokeAllShares(); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.ResolveShare(keep.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("après révocation globale : %v", err)
	}

	// Suppression du schéma : ses liens disparaissent.
	del, _ := s2.CreateShare("e/a.excalidraw", 0)
	s2.Delete("e", true)
	if _, err := s2.ResolveShare(del.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("schéma supprimé : %v", err)
	}
}

func TestShareFixedSecret(t *testing.T) {
	s := newStore(t)
	s.SetShareSecret("une-clé-imposée-assez-longue")
	s.Write("a.excalidraw", []byte(doc), "", true)
	sh, _ := s.CreateShare("a.excalidraw", 0)
	if _, err := os.Stat(filepath.Join(s.root, secretFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clé écrite sur disque malgré SHARE_SECRET : %v", err)
	}
	s2, _ := New(s.root)
	s2.SetShareSecret("une-clé-imposée-assez-longue")
	if _, err := s2.ResolveShare(sh.Token); err != nil {
		t.Fatal(err)
	}
	s3, _ := New(s.root)
	s3.SetShareSecret("une-autre-clé-assez-longue")
	if _, err := s3.ResolveShare(sh.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("autre clé acceptée : %v", err)
	}
}
