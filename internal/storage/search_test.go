package storage

import (
	"strings"
	"testing"
	"time"
)

func scene(texts ...string) []byte {
	var els []string
	for i, t := range texts {
		els = append(els, `{"id":"e`+string(rune('a'+i))+`","type":"text","text":"`+t+`","originalText":"`+t+`"}`)
	}
	return []byte(`{"type":"excalidraw","elements":[` + strings.Join(els, ",") + `]}`)
}

func TestSearchContent(t *testing.T) {
	s := newStore(t)
	s.Write("réseau/sdn.excalidraw", scene(`Edge NSX 172.27.207.1`, `Transport zone overlay`), "", true)
	s.Write("stockage/vsan.excalidraw", scene(`Cluster vSAN\nRéplication`), "", true)
	s.Write("réplication.excalidraw", scene(`rien`), "", true)
	s.Write("supprimé.excalidraw", []byte(`{"elements":[{"type":"text","text":"172.27.207.99","isDeleted":true}]}`), "", true)

	hits, err := s.Search("172.27.207", 10)
	if err != nil || len(hits) != 1 || hits[0].Path != "réseau/sdn.excalidraw" {
		t.Fatalf("IP : %+v, %v", hits, err)
	}
	if hits[0].Snippet != "Edge NSX 172.27.207.1" {
		t.Fatalf("extrait = %q", hits[0].Snippet)
	}

	// Accents et casse ignorés ; le nom passe avant le contenu.
	hits, _ = s.Search("REPLICATION", 10)
	if len(hits) != 2 || hits[0].Path != "réplication.excalidraw" || hits[1].Path != "stockage/vsan.excalidraw" {
		t.Fatalf("accents : %+v", hits)
	}
	if hits[1].Snippet != "Cluster vSAN Réplication" {
		t.Fatalf("extrait replié = %q", hits[1].Snippet)
	}

	// Tous les mots doivent correspondre, dans le nom ou le contenu.
	if hits, _ := s.Search("sdn overlay", 10); len(hits) != 1 {
		t.Fatalf("nom + contenu : %+v", hits)
	}
	if hits, _ := s.Search("sdn vsan", 10); len(hits) != 0 {
		t.Fatalf("ET logique : %+v", hits)
	}
	// Les dossiers correspondent par leur nom.
	if hits, _ := s.Search("stock", 10); len(hits) != 1 || !hits[0].IsDir {
		t.Fatalf("dossier : %+v", hits)
	}

	// Une modification est prise en compte (date ou taille changée).
	time.Sleep(5 * time.Millisecond)
	s.Write("réseau/sdn.excalidraw", scene(`Edge NSX 10.0.0.1`), "", false)
	if hits, _ := s.Search("172.27.207", 10); len(hits) != 0 {
		t.Fatalf("index périmé : %+v", hits)
	}
	if hits, _ := s.Search("10.0.0", 10); len(hits) != 1 {
		t.Fatalf("index non mis à jour : %+v", hits)
	}
}

func TestSnippetLongLine(t *testing.T) {
	long := strings.Repeat("a", 100) + " cible " + strings.Repeat("b", 100)
	d := indexDoc{text: long + "\n"}
	d.fold = fold(d.text)
	got := snippet(d, strings.Index(d.fold, "cible"), len("cible"))
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") || !strings.Contains(got, " cible ") {
		t.Fatalf("extrait = %q", got)
	}
}
