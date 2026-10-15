package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrash(t *testing.T) {
	main := newStore(t)
	sp := NewSpaces(main)
	u, err := sp.For("alice")
	if err != nil {
		t.Fatal(err)
	}
	main.Write("d/a.excalidraw", []byte(doc), "", true)
	main.Write("d/b.excalidraw", []byte(doc), "", true)
	u.Write(".perso/p.excalidraw", []byte(doc), "", true)
	if err := u.EditTags([]string{"d/a.excalidraw"}, []string{"NSX"}, nil); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"d/a.excalidraw", ".perso/p.excalidraw"} {
		if err := u.Delete(p, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := u.Delete("d", true); err != nil {
		t.Fatal(err)
	}
	root, _ := u.List("")
	if len(root) < 3 || root[2].Special != "poubelle" || root[2].Count != 3 {
		t.Fatalf("racine = %+v", root)
	}
	items, err := u.List(TrashPrefix)
	if err != nil || len(items) != 3 {
		t.Fatalf("poubelle = %+v, %v", items, err)
	}
	// Plus récent en premier : le dossier d, puis p, puis a.
	if items[0].Name != "d" || !items[0].IsDir || items[0].Origin != "d" || items[0].By != "alice" ||
		items[1].Origin != ".perso/p.excalidraw" || items[2].Origin != "d/a.excalidraw" {
		t.Fatalf("poubelle = %+v", items)
	}
	// Les éléments de la poubelle ne s'ouvrent pas.
	if _, _, err := u.Read(items[2].Path); err == nil {
		t.Fatal("lecture dans la poubelle acceptée")
	}

	// a revient dans d (recréé), avec son tag.
	res := u.RestoreTrash([]string{items[2].Path})
	if res[0].Error != "" || res[0].To != "d/a.excalidraw" {
		t.Fatalf("restauration = %+v", res)
	}
	if l, _ := u.List("d"); len(l) != 1 || len(l[0].Tags) != 1 {
		t.Fatalf("d = %+v", l)
	}
	// Le dossier d revient sous un autre nom, la place étant prise.
	res = u.RestoreTrash([]string{items[0].Path})
	if res[0].Error != "" || res[0].To != "d (restauré)" {
		t.Fatalf("restauration = %+v", res)
	}
	if l, _ := u.List("d (restauré)"); len(l) != 1 || l[0].Name != "b.excalidraw" {
		t.Fatalf("d (restauré) = %+v", l)
	}

	// Suppression définitive, puis poubelle vide.
	if err := u.Delete(items[1].Path, false); err != nil {
		t.Fatal(err)
	}
	if l, _ := u.Trash(); len(l) != 0 {
		t.Fatalf("poubelle = %+v", l)
	}
	if err := u.Delete(TrashPrefix+"/c-x", false); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("élément inconnu : %v", err)
	}

	// Vider la poubelle.
	u.Delete("d/a.excalidraw", false)
	if err := u.Delete(TrashPrefix, true); err != nil {
		t.Fatal(err)
	}
	if main.TrashCount() != 0 {
		t.Fatal("poubelle non vidée")
	}
}

func TestTrashPurge(t *testing.T) {
	main := newStore(t)
	sp := NewSpaces(main)
	u, _ := sp.For("bob")
	main.Write("old.excalidraw", []byte(doc), "", true)
	main.Write("new.excalidraw", []byte(doc), "", true)
	u.Write(".perso/old.excalidraw", []byte(doc), "", true)
	u.Delete("old.excalidraw", false)
	u.Delete(".perso/old.excalidraw", false)
	// Vieillit les deux entrées de 10 jours.
	for _, st := range []*Store{main, u.home} {
		items, _ := st.TrashItems()
		old := time.Now().UTC().Add(-240 * time.Hour).Format(trashLayout)
		if err := os.Rename(st.trashAbs(items[0].ID), st.trashAbs(old)); err != nil {
			t.Fatal(err)
		}
	}
	u.Delete("new.excalidraw", false)

	// Sans durée de conservation, rien n'est purgé.
	if err := sp.PurgeTrash(); err != nil || main.TrashCount() != 2 || u.home.TrashCount() != 1 {
		t.Fatalf("purge sans durée : %v", err)
	}
	if _, err := main.WriteTrashDays(-1); err == nil {
		t.Fatal("durée négative acceptée")
	}
	if st, err := main.WriteTrashDays(7); err != nil || st.TrashDays != 7 {
		t.Fatalf("WriteTrashDays = %+v, %v", st, err)
	}
	if err := sp.PurgeTrash(); err != nil {
		t.Fatal(err)
	}
	if main.TrashCount() != 1 || u.home.TrashCount() != 0 {
		t.Fatalf("après purge : %d / %d", main.TrashCount(), u.home.TrashCount())
	}
	// L'historique de l'élément purgé disparaît avec lui.
	if des, _ := os.ReadDir(filepath.Join(main.root, trashDir)); len(des) != 1 {
		t.Fatalf("corbeille = %v", des)
	}
}
