package storage

import (
	"testing"
	"time"
)

const richScene = `{"type":"excalidraw","appState":{"viewBackgroundColor":"#121212"},"elements":[
{"id":"f1","type":"frame","name":"ingress","x":0,"y":0,"width":400,"height":300},
{"id":"r1","type":"rectangle","x":10,"y":20,"width":100,"height":50},
{"id":"t1","type":"text","text":"Service","x":20,"y":30,"width":60,"height":20},
{"id":"a1","type":"arrow","x":110,"y":45,"width":-200,"height":100},
{"id":"i1","type":"image","x":500,"y":-50,"width":100,"height":100},
{"id":"d1","type":"rectangle","isDeleted":true,"x":5000,"y":5000,"width":10,"height":10}]}`

func TestDetailsScene(t *testing.T) {
	sp := newSpaces(t)
	alice := space(t, sp, "alice@x.fr")
	if _, _, err := alice.Write("k8s.excalidraw", []byte(richScene), "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.CreateShare("k8s.excalidraw", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.CreateShare("k8s.excalidraw", time.Nanosecond); err != nil { // expire aussitôt
		t.Fatal(err)
	}
	if _, err := alice.Snapshot("k8s.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Mkdir("réseau/sdn"); err != nil {
		t.Fatal(err)
	}
	write(t, alice, "réseau/a.excalidraw")

	// Bob enregistre ensuite : il devient le dernier auteur, Alice reste la créatrice.
	bob := space(t, sp, "bob@x.fr")
	e, _, err := bob.Write("k8s.excalidraw", []byte(richScene), "", false)
	if err != nil || e.EditedBy != "bob@x.fr" {
		t.Fatalf("écriture de bob : %+v, %v", e, err)
	}

	es, err := bob.List("")
	if err != nil {
		t.Fatal(err)
	}
	ps := []*Entry{}
	for i := range es {
		ps = append(ps, &es[i])
	}
	bob.AddDetails(ps)
	got := map[string]Entry{}
	for _, e := range es {
		got[e.Path] = e
	}
	if got[PersoPrefix].Details != nil {
		t.Errorf("dossier virtuel avec détails : %+v", got[PersoPrefix].Details)
	}
	if d := got["réseau"].Details; d == nil || d.Dirs != 1 || d.Files != 1 {
		t.Errorf("dossier réseau : %+v", d)
	}
	k := got["k8s.excalidraw"]
	d := k.Details
	if d == nil {
		t.Fatal("pas de détails pour k8s.excalidraw")
	}
	if k.EditedBy != "bob@x.fr" || d.CreatedBy != "alice@x.fr" || d.Created == nil {
		t.Errorf("auteurs : editedBy=%q createdBy=%q created=%v", k.EditedBy, d.CreatedBy, d.Created)
	}
	if d.Elements != 5 || d.Texts != 1 || d.Images != 1 || d.Arrows != 1 || d.Frames != 1 ||
		len(d.FrameNames) != 1 || d.FrameNames[0] != "ingress" {
		t.Errorf("compteurs : %+v", d.SceneStats)
	}
	// Emprise : x de -90 (flèche de largeur négative) à 600, y de -50 à 300.
	if d.Width != 690 || d.Height != 350 || d.Background != "#121212" {
		t.Errorf("dimensions : %dx%d fond %q", d.Width, d.Height, d.Background)
	}
	if d.Shares != 1 || d.Versions != 1 {
		t.Errorf("partages=%d versions=%d", d.Shares, d.Versions)
	}
}

func TestNoteEditOnlyOnChange(t *testing.T) {
	sp := newSpaces(t)
	u := space(t, sp, "alice@x.fr")
	write(t, u, "a.excalidraw")
	m := sp.Main.metaSnapshot()["a.excalidraw"]
	if m.EditedBy != "alice@x.fr" || m.CreatedBy != "alice@x.fr" || m.Created == nil {
		t.Fatalf("méta après création : %+v", m)
	}
	created := *m.Created
	if _, _, err := u.Write("a.excalidraw", []byte(doc), "", false); err != nil {
		t.Fatal(err)
	}
	if m := sp.Main.metaSnapshot()["a.excalidraw"]; !m.Created.Equal(created) {
		t.Errorf("date de création modifiée : %v", m.Created)
	}

	// La copie a son propre créateur ; le déplacement garde les métadonnées.
	bob := space(t, sp, "bob@x.fr")
	if _, err := bob.Copy("a.excalidraw", "b.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Move("b.excalidraw", "c.excalidraw"); err != nil {
		t.Fatal(err)
	}
	if m := sp.Main.metaSnapshot()["c.excalidraw"]; m.CreatedBy != "bob@x.fr" || m.EditedBy != "bob@x.fr" {
		t.Errorf("méta de la copie déplacée : %+v", m)
	}
}
