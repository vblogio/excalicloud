package storage

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTitle(t *testing.T) {
	s := newStore(t)
	if st, err := s.ReadSettings(); err != nil || st != (Settings{Channel: DefaultChannel}) {
		t.Fatalf("défauts : %+v, %v", st, err)
	}
	st, err := s.WriteTitle("  Schémas vLab  ")
	if err != nil || st.Title != "Schémas vLab" {
		t.Fatalf("titre : %+v, %v", st, err)
	}
	if st, _ := s.ReadSettings(); st.Title != "Schémas vLab" {
		t.Fatalf("titre non persisté : %+v", st)
	}
	for _, bad := range []string{"a\nb", strings.Repeat("é", MaxTitleLen+1)} {
		if _, err := s.WriteTitle(bad); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("titre %q accepté", bad)
		}
	}
	// Les fichiers de réglages n'apparaissent pas dans l'explorateur.
	if entries, _ := s.List(""); len(entries) != 0 {
		t.Fatalf("réglages visibles : %+v", entries)
	}
}

func TestChannel(t *testing.T) {
	s := newStore(t)
	if _, err := s.WriteTitle("Titre"); err != nil {
		t.Fatal(err)
	}
	st, err := s.WriteChannel("next")
	if err != nil || st.Channel != "next" || st.Title != "Titre" {
		t.Fatalf("canal : %+v, %v", st, err)
	}
	// Changer le titre conserve le canal, et inversement.
	if st, _ := s.WriteTitle("Autre"); st.Channel != "next" {
		t.Fatalf("canal perdu : %+v", st)
	}
	if _, err := s.WriteChannel("rc"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("canal invalide accepté : %v", err)
	}
	if st, _ := s.ReadSettings(); st.Channel != "next" || st.Title != "Autre" {
		t.Fatalf("relu : %+v", st)
	}
}

func TestLogo(t *testing.T) {
	s := newStore(t)
	if _, err := s.ReadLogo(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("logo absent : %v", err)
	}
	if _, err := s.WriteLogo([]byte("pas une image")); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("non-PNG accepté : %v", err)
	}

	// 300x200 : bande rouge à gauche (hors recadrage), vert au centre.
	src := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			c := color.NRGBA{0, 200, 0, 255}
			if x < 50 {
				c = color.NRGBA{255, 0, 0, 255}
			}
			src.Set(x, y, c)
		}
	}
	st, err := s.WriteLogo(encodePNG(t, src))
	if err != nil || st.Logo == "" {
		t.Fatalf("écriture : %+v, %v", st, err)
	}
	b, err := s.ReadLogo()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if sz := img.Bounds().Size(); sz != image.Pt(LogoSize, LogoSize) {
		t.Fatalf("taille %v", sz)
	}
	if r, g, _, _ := img.At(0, 0).RGBA(); r>>8 != 0 || g>>8 != 200 {
		t.Fatalf("recadrage centré attendu, coin = %v", img.At(0, 0))
	}

	// Agrandissement d'une petite image.
	if _, err := s.WriteLogo(encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 10, 10)))); err != nil {
		t.Fatalf("petite image : %v", err)
	}

	if st, err := s.DeleteLogo(); err != nil || st.Logo != "" {
		t.Fatalf("suppression : %+v, %v", st, err)
	}
	if _, err := s.DeleteLogo(); err != nil {
		t.Fatalf("double suppression : %v", err)
	}
}
