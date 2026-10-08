package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUserID(t *testing.T) {
	for in, want := range map[string]string{
		" Cedric.Killy@VLab.bzh ": "cedric.killy@vlab.bzh",
		"jean dupont/../x":        "jean_dupont_.._x",
		"..":                      "_.",
		"":                        LocalUser,
	} {
		if got := UserID(in); got != want {
			t.Errorf("UserID(%q) = %q ; attendu %q", in, got, want)
		}
	}
}

func newSpaces(t *testing.T) *Spaces {
	t.Helper()
	return NewSpaces(newStore(t))
}

func space(t *testing.T, sp *Spaces, id string) *Space {
	t.Helper()
	u, err := sp.For(id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func write(t *testing.T, u *Space, p string) {
	t.Helper()
	if _, _, err := u.Write(p, []byte(doc), "", true); err != nil {
		t.Fatalf("écriture de %s : %v", p, err)
	}
}

func favPaths(t *testing.T, u *Space) []string {
	t.Helper()
	es, err := u.Favorites()
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range es {
		if e.Broken {
			out = append(out, "!"+e.Path)
		} else {
			out = append(out, e.Path)
		}
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSpacesListAndPrivacy(t *testing.T) {
	sp := newSpaces(t)
	a, b := space(t, sp, "a@x.fr"), space(t, sp, "b@x.fr")
	write(t, a, ".perso/secret.excalidraw")
	write(t, a, "commun.excalidraw")

	l, err := a.List("")
	if err != nil || len(l) != 3 || l[0].Special != "favoris" || l[1].Special != "perso" || l[2].Path != "commun.excalidraw" {
		t.Fatalf("racine = %+v, %v", l, err)
	}
	if l, _ := a.List(".perso"); len(l) != 1 || l[0].Path != ".perso/secret.excalidraw" {
		t.Fatalf("espace personnel de a = %+v", l)
	}
	if l, _ := b.List(".perso"); len(l) != 0 {
		t.Fatalf("b voit l'espace de a : %+v", l)
	}
	if _, _, err := b.Read(".perso/secret.excalidraw"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("b lit le schéma de a : %v", err)
	}
	// Ni l'arborescence commune ni la recherche ne montrent .users.
	if _, _, err := b.Read(".users/a@x.fr/home/secret.excalidraw"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("accès direct à .users : %v", err)
	}
	if res, _ := b.Search("secret", Filter{}, 10); len(res) != 0 {
		t.Fatalf("recherche de b = %+v", res)
	}
	if res, _ := a.Search("secret", Filter{}, 10); len(res) != 1 || res[0].Path != ".perso/secret.excalidraw" {
		t.Fatalf("recherche de a = %+v", res)
	}
}

func TestSpacesFavorites(t *testing.T) {
	sp := newSpaces(t)
	a, b := space(t, sp, "a@x.fr"), space(t, sp, "b@x.fr")
	write(t, a, "d/s.excalidraw")
	write(t, a, ".perso/p.excalidraw")
	if err := a.SetFavorite([]string{"d"}, true); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("dossier en favori : %v", err)
	}
	if err := a.SetFavorite([]string{"d/s.excalidraw", ".perso/p.excalidraw"}, true); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFavorite([]string{"d/s.excalidraw"}, true); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, a); !same(got, []string{".perso/p.excalidraw", "d/s.excalidraw"}) {
		t.Fatalf("favoris de a = %v", got)
	}
	if l, _ := a.List("d"); len(l) != 1 || !l[0].Favorite {
		t.Fatalf("étoile absente : %+v", l)
	}
	if res, _ := a.Search("", Filter{Favorite: true}, 10); len(res) != 2 {
		t.Fatalf("filtre favoris = %+v", res)
	}
	// Le lien est relatif et vise l'original.
	fi, err := os.Lstat(filepath.Join(sp.favDir("a@x.fr"), "s.excalidraw"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("lien symbolique attendu : %v", err)
	}

	// Renommage du dossier : les favoris des deux utilisateurs suivent.
	if _, err := a.Move("d", "e"); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, b); !same(got, []string{"e/s.excalidraw"}) {
		t.Fatalf("favoris de b après renommage = %v", got)
	}

	// Passage dans l'espace personnel de a : a garde son favori, b le perd.
	if _, err := a.Move("e/s.excalidraw", ".perso/s.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, a); !same(got, []string{".perso/p.excalidraw", ".perso/s.excalidraw"}) {
		t.Fatalf("favoris de a après déplacement = %v", got)
	}
	if got := favPaths(t, b); len(got) != 0 {
		t.Fatalf("b garde un favori vers l'espace de a : %v", got)
	}

	// Retour dans l'arborescence commune par MoveMany.
	res, err := a.MoveMany([]string{".perso/p.excalidraw"}, "")
	if err != nil || res[0].To != "p.excalidraw" || res[0].Err() != nil {
		t.Fatalf("MoveMany = %+v, %v", res, err)
	}
	if got := favPaths(t, a); !same(got, []string{"p.excalidraw", ".perso/s.excalidraw"}) {
		t.Fatalf("favoris de a après MoveMany = %v", got)
	}

	// Suppression : le favori disparaît.
	if err := a.Delete("p.excalidraw", false); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, a); !same(got, []string{".perso/s.excalidraw"}) {
		t.Fatalf("favoris de a après suppression = %v", got)
	}

	// Original supprimé hors de l'application : favori cassé, retirable.
	if err := os.Remove(filepath.Join(a.home.root, "s.excalidraw")); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, a); !same(got, []string{"!.favoris/s.excalidraw"}) {
		t.Fatalf("favori cassé = %v", got)
	}
	if err := a.Delete(".favoris/s.excalidraw", false); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, a); len(got) != 0 {
		t.Fatalf("favori cassé non retiré : %v", got)
	}
}

func TestSpacesFavoriteNames(t *testing.T) {
	sp := newSpaces(t)
	a := space(t, sp, "a@x.fr")
	write(t, a, "x/s.excalidraw")
	write(t, a, "y/s.excalidraw")
	if err := a.SetFavorite([]string{"x/s.excalidraw", "y/s.excalidraw", "x/s.excalidraw"}, true); err != nil {
		t.Fatal(err)
	}
	es, _ := a.Favorites()
	if len(es) != 2 || es[0].Link == es[1].Link {
		t.Fatalf("favoris homonymes = %+v", es)
	}
	if err := a.SetFavorite([]string{"y/s.excalidraw"}, false); err != nil {
		t.Fatal(err)
	}
	if got := favPaths(t, a); !same(got, []string{"x/s.excalidraw"}) {
		t.Fatalf("après retrait = %v", got)
	}
}

func TestSpacesSeedFromMeta(t *testing.T) {
	sp := newSpaces(t)
	write(t, space(t, sp, "a@x.fr"), "old.excalidraw")
	if err := sp.Main.SetFavorite([]string{"old.excalidraw"}, true); err != nil {
		t.Fatal(err)
	}
	// Un nouvel utilisateur reprend les favoris communs, une seule fois.
	c := space(t, sp, "c@x.fr")
	if got := favPaths(t, c); !same(got, []string{"old.excalidraw"}) {
		t.Fatalf("favoris repris = %v", got)
	}
	if err := c.SetFavorite([]string{"old.excalidraw"}, false); err != nil {
		t.Fatal(err)
	}
	sp.homes = map[string]*Store{} // redémarrage
	if got := favPaths(t, space(t, sp, "c@x.fr")); len(got) != 0 {
		t.Fatalf("favoris repris deux fois : %v", got)
	}
}

func TestSpacesSharesAndCrossMove(t *testing.T) {
	sp := newSpaces(t)
	a := space(t, sp, "a@x.fr")
	write(t, a, ".perso/p.excalidraw")
	if err := a.EditTags([]string{".perso/p.excalidraw"}, []string{"NSX"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.home.Snapshot("p.excalidraw"); err != nil {
		t.Fatal(err)
	}
	sh, err := a.CreateShare(".perso/p.excalidraw", 0)
	if err != nil || sh.Path != ".perso/p.excalidraw" {
		t.Fatalf("partage = %+v, %v", sh, err)
	}
	if got, _, err := sp.ReadShared(sh.Token); err != nil || got.Path != "p.excalidraw" {
		t.Fatalf("lien personnel : %+v, %v", got, err)
	}
	// Vers l'arborescence commune : tags, historique et lien suivent.
	if _, err := a.Move(".perso/p.excalidraw", "q.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if l, _ := a.List(""); len(l) != 3 || len(l[2].Tags) != 1 {
		t.Fatalf("tags après déplacement : %+v", l)
	}
	if vs, err := a.Versions("q.excalidraw"); err != nil || len(vs) != 1 {
		t.Fatalf("historique après déplacement : %+v, %v", vs, err)
	}
	if got, _, err := sp.ReadShared(sh.Token); err != nil || got.Path != "q.excalidraw" {
		t.Fatalf("lien après déplacement : %+v, %v", got, err)
	}
	if tags, _ := a.Tags(); len(tags) != 1 || tags[0].Count != 1 {
		t.Fatalf("tags = %+v", tags)
	}
	// Révocation globale : clé changée, liens personnels compris.
	sh2, _ := a.CreateShare("q.excalidraw", 0)
	write(t, a, ".perso/r.excalidraw")
	sh3, _ := a.CreateShare(".perso/r.excalidraw", 0)
	if err := a.RevokeAllShares(); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{sh2.Token, sh3.Token} {
		if _, _, err := sp.ReadShared(tok); !errors.Is(err, ErrNotFound) {
			t.Fatalf("lien encore valide après révocation : %v", err)
		}
	}
}
