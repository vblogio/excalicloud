package storage

import (
	"errors"
	"testing"
)

func TestTagsFavorites(t *testing.T) {
	s := newStore(t)
	s.Write("d/a.excalidraw", []byte(doc), "", true)
	s.Write("d/b.excalidraw", []byte(doc), "", true)
	s.Write("c.excalidraw", []byte(doc), "", true)

	if err := s.EditTags([]string{"d/a.excalidraw", "d/b.excalidraw"}, []string{"#NSX", " Réseau  cœur "}, nil); err != nil {
		t.Fatal(err)
	}
	// Même tag sans casse ni accents : l'écriture existante est reprise.
	if err := s.EditTags([]string{"c.excalidraw"}, []string{"nsx", "reseau cœur"}, nil); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.Tags()
	if len(tags) != 2 || tags[0] != (TagCount{"NSX", 3}) || tags[1] != (TagCount{"Réseau cœur", 3}) {
		t.Fatalf("Tags = %+v", tags)
	}
	if err := s.EditTags([]string{"d/b.excalidraw"}, nil, []string{"NSX"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite([]string{"d/b.excalidraw", "d"}, true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{""}, {"absent.excalidraw"}, {"../x"}} {
		if err := s.SetFavorite(bad, true); err == nil {
			t.Errorf("SetFavorite(%q) aurait dû échouer", bad)
		}
	}
	if err := s.EditTags([]string{"c.excalidraw"}, []string{"a,b"}, nil); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("tag invalide : %v", err)
	}

	l, _ := s.List("d")
	if len(l[0].Tags) != 2 || l[0].Favorite || len(l[1].Tags) != 1 || !l[1].Favorite {
		t.Fatalf("List = %+v", l)
	}

	// Filtres de recherche.
	if h, _ := s.Search("", Filter{Tag: "nsx"}, 10); len(h) != 2 {
		t.Fatalf("tag : %+v", h)
	}
	if h, _ := s.Search("", Filter{Favorite: true}, 10); len(h) != 2 {
		t.Fatalf("favoris : %+v", h)
	}
	if h, _ := s.Search("reseau", Filter{Favorite: true}, 10); len(h) != 1 || h[0].Path != "d/b.excalidraw" {
		t.Fatalf("mot (tag) + favoris : %+v", h)
	}

	// Les métadonnées suivent déplacement, copie (tags seuls) et suppression,
	// et survivent au rechargement.
	s.Move("d", "e")
	s.Copy("e/b.excalidraw", "copie.excalidraw")
	s.Delete("c.excalidraw", false)
	s2, _ := New(s.root)
	l, _ = s2.List("")
	got := map[string]Entry{}
	for _, e := range l {
		got[e.Path] = e
	}
	if !got["e"].Favorite || got["copie.excalidraw"].Favorite || len(got["copie.excalidraw"].Tags) != 1 {
		t.Fatalf("après déplacement/copie : %+v", l)
	}
	if tags, _ := s2.Tags(); len(tags) != 2 || tags[0] != (TagCount{"NSX", 1}) {
		t.Fatalf("après suppression : %+v", tags)
	}
}
