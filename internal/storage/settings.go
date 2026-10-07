package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vblogio/excalidraw-go/internal/update"
)

// Paramètres de l'interface (titre, logo, canal d'Excalidraw), partagés par
// tous les navigateurs : .settings.json et .logo.png à la racine.

const (
	settingsFile = ".settings.json"
	logoFile     = ".logo.png"

	// LogoSize est le côté du logo enregistré (carré, affiché en rond).
	LogoSize = 64
	// MaxLogoUpload borne la taille d'un PNG envoyé avant redimensionnement.
	MaxLogoUpload = 5 << 20
	// MaxTitleLen borne la longueur du titre (en caractères).
	MaxTitleLen = 80

	maxLogoSide = 8192 // protège contre les « bombes » de décompression
)

// ErrInvalidImage signale un logo illisible ou hors limites.
var ErrInvalidImage = errors.New("image invalide")

// DefaultChannel est le canal npm d'Excalidraw suivi par défaut (versions stables).
const DefaultChannel = "latest"

// Settings est l'état exposé au client. Logo vaut "" sans logo, sinon une
// version (date) à ajouter à l'URL pour invalider le cache navigateur.
// Channel est le canal npm d'Excalidraw (latest ou next) dont l'interface
// signale les nouvelles versions.
type Settings struct {
	Title   string `json:"title"`
	Logo    string `json:"logo"`
	Channel string `json:"channel"`
}

type settingsDisk struct {
	Title   string `json:"title"`
	Channel string `json:"channel,omitempty"`
}

func (s *Store) readSettingsDisk() (settingsDisk, error) {
	var d settingsDisk
	b, err := os.ReadFile(filepath.Join(s.root, settingsFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return d, err
	default:
		if err := json.Unmarshal(b, &d); err != nil {
			return d, fmt.Errorf("%s : %w", settingsFile, err)
		}
	}
	return d, nil
}

// updateSettings applique edit au fichier de paramètres (lecture, modification,
// écriture atomique) sous le verrou du Store.
func (s *Store) updateSettings(edit func(*settingsDisk)) (Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	d, err := s.readSettingsDisk()
	if err != nil {
		return Settings{}, err
	}
	edit(&d)
	b, err := json.Marshal(d)
	if err != nil {
		return Settings{}, err
	}
	if err := atomicWrite(filepath.Join(s.root, settingsFile), b); err != nil {
		return Settings{}, err
	}
	return s.ReadSettings()
}

// ReadSettings renvoie les paramètres courants (valeurs par défaut si absents).
func (s *Store) ReadSettings() (Settings, error) {
	d, err := s.readSettingsDisk()
	if err != nil {
		return Settings{}, err
	}
	st := Settings{Title: d.Title, Channel: d.Channel}
	if !update.ValidChannel(st.Channel) {
		st.Channel = DefaultChannel
	}
	if fi, err := os.Stat(filepath.Join(s.root, logoFile)); err == nil {
		st.Logo = strconv.FormatInt(fi.ModTime().UnixNano(), 36)
	}
	return st, nil
}

// WriteChannel enregistre le canal npm d'Excalidraw suivi (latest ou next).
func (s *Store) WriteChannel(channel string) (Settings, error) {
	if !update.ValidChannel(channel) {
		return Settings{}, fmt.Errorf("%w : canal %q (latest ou next)", ErrInvalidPath, channel)
	}
	return s.updateSettings(func(d *settingsDisk) { d.Channel = channel })
}

// WriteTitle enregistre le titre affiché ("" = titre par défaut).
func (s *Store) WriteTitle(title string) (Settings, error) {
	title = strings.TrimSpace(title)
	if !utf8.ValidString(title) || strings.ContainsAny(title, "\r\n\x00") {
		return Settings{}, fmt.Errorf("%w : titre", ErrInvalidPath)
	}
	if utf8.RuneCountInString(title) > MaxTitleLen {
		return Settings{}, fmt.Errorf("%w : titre de %d caractères max", ErrInvalidPath, MaxTitleLen)
	}
	return s.updateSettings(func(d *settingsDisk) { d.Title = title })
}

// ReadLogo renvoie le logo PNG 64x64, ou ErrNotFound.
func (s *Store) ReadLogo() ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(s.root, logoFile))
	return b, mapErr(err)
}

// WriteLogo décode un PNG, le recadre au centre en carré, le ramène à
// LogoSize x LogoSize et l'enregistre (ré-encodé : aucune donnée annexe du
// fichier d'origine n'est conservée).
func (s *Store) WriteLogo(data []byte) (Settings, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Settings{}, fmt.Errorf("%w : PNG attendu", ErrInvalidImage)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxLogoSide || cfg.Height > maxLogoSide {
		return Settings{}, fmt.Errorf("%w : %dx%d px max", ErrInvalidImage, maxLogoSide, maxLogoSide)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return Settings{}, fmt.Errorf("%w : PNG illisible", ErrInvalidImage)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, squareResize(img, LogoSize)); err != nil {
		return Settings{}, err
	}
	if err := atomicWrite(filepath.Join(s.root, logoFile), buf.Bytes()); err != nil {
		return Settings{}, err
	}
	return s.ReadSettings()
}

// DeleteLogo supprime le logo (sans erreur s'il est absent).
func (s *Store) DeleteLogo() (Settings, error) {
	if err := os.Remove(filepath.Join(s.root, logoFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Settings{}, err
	}
	return s.ReadSettings()
}

// squareResize recadre img au centre (carré) puis le rééchantillonne en n x n
// par moyenne de surface (filtre « boîte » pondéré), en alpha prémultiplié
// pour éviter les franges sombres sur les bords transparents.
func squareResize(img image.Image, n int) *image.RGBA {
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(0, 0, side, side).Add(b.Min).
		Add(image.Pt((b.Dx()-side)/2, (b.Dy()-side)/2))
	src := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(src, src.Bounds(), img, crop.Min, draw.Src)

	w := boxWeights(side, n)
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for dy := 0; dy < n; dy++ {
		for dx := 0; dx < n; dx++ {
			var acc [4]float64
			var total float64
			for _, wy := range w[dy] {
				row := wy.i * src.Stride
				for _, wx := range w[dx] {
					k := wy.w * wx.w
					p := src.Pix[row+wx.i*4:]
					for c := range acc {
						acc[c] += k * float64(p[c])
					}
					total += k
				}
			}
			o := dst.PixOffset(dx, dy)
			for c := range acc {
				dst.Pix[o+c] = uint8(acc[c]/total + 0.5)
			}
		}
	}
	return dst
}

type weight struct {
	i int
	w float64
}

// boxWeights calcule, pour chaque pixel cible, les pixels source qu'il couvre
// et la fraction couverte. Fonctionne aussi en agrandissement (côté < n).
func boxWeights(side, n int) [][]weight {
	scale := float64(side) / float64(n)
	out := make([][]weight, n)
	for d := range out {
		lo, hi := float64(d)*scale, float64(d+1)*scale
		for i := int(lo); i < side && float64(i) < hi; i++ {
			w := min(hi, float64(i+1)) - max(lo, float64(i))
			if w > 0 {
				out[d] = append(out[d], weight{i, w})
			}
		}
	}
	return out
}
