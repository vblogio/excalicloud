package storage

import (
	"math"
	"os"
	"strings"
	"time"
)

// Détails des schémas et dossiers pour la vue Galerie (/api/list?details=1) :
// contenu d'un schéma (compteurs d'éléments, cadres, dimensions), versions,
// liens de partage actifs, création et dernier auteur ; contenu d'un dossier.
// Le contenu vient de l'index de recherche (search.go), relu seulement quand
// le fichier change ; le reste est lu à chaque appel.

// maxFrameNames borne la liste des noms de cadres renvoyée par schéma.
const maxFrameNames = 30

// SceneStats résume le contenu d'un schéma (éléments non supprimés).
type SceneStats struct {
	Elements   int      `json:"elements,omitempty"`
	Texts      int      `json:"texts,omitempty"`
	Images     int      `json:"images,omitempty"`
	Arrows     int      `json:"arrows,omitempty"`
	Frames     int      `json:"frames,omitempty"`
	FrameNames []string `json:"frameNames,omitempty"`
	Width      int      `json:"width,omitempty"` // emprise des éléments (px du canevas)
	Height     int      `json:"height,omitempty"`
	Background string   `json:"background,omitempty"` // couleur de fond du canevas
}

func (st *SceneStats) count(typ, name string) {
	st.Elements++
	switch typ {
	case "text":
		st.Texts++
	case "image":
		st.Images++
	case "arrow":
		st.Arrows++
	case "frame", "magicframe":
		st.Frames++
		if name = strings.TrimSpace(name); name != "" && len(st.FrameNames) < maxFrameNames {
			st.FrameNames = append(st.FrameNames, name)
		}
	}
}

// bounds calcule l'emprise d'un ensemble de rectangles.
type bounds struct {
	minX, minY, maxX, maxY float64
	set                    bool
}

func (b *bounds) add(x, y, w, h float64) {
	x2, y2 := x+w, y+h
	if w < 0 {
		x, x2 = x2, x
	}
	if h < 0 {
		y, y2 = y2, y
	}
	if !b.set {
		b.minX, b.minY, b.maxX, b.maxY, b.set = x, y, x2, y2, true
		return
	}
	b.minX, b.minY = math.Min(b.minX, x), math.Min(b.minY, y)
	b.maxX, b.maxY = math.Max(b.maxX, x2), math.Max(b.maxY, y2)
}

func (b bounds) size() (int, int) {
	if !b.set {
		return 0, 0
	}
	return int(math.Round(b.maxX - b.minX)), int(math.Round(b.maxY - b.minY))
}

// Details complète une entrée de la vue Galerie.
type Details struct {
	SceneStats

	Versions  int        `json:"versions,omitempty"`  // versions dans l'historique
	Shares    int        `json:"shares,omitempty"`    // liens de partage actifs
	Created   *time.Time `json:"created,omitempty"`   // création (ou plus ancienne version connue)
	CreatedBy string     `json:"createdBy,omitempty"` // vide si inconnu

	Dirs  int `json:"dirs,omitempty"`  // dossier : sous-dossiers
	Files int `json:"files,omitempty"` // dossier : schémas
}

// sceneDetails renvoie les détails d'un schéma ; shares compte les liens
// actifs par chemin (shareCounts).
func (s *Store) sceneDetails(rel string, meta map[string]itemMeta, shares map[string]int) (*Details, error) {
	c, abs, err := s.schema(rel)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, mapErr(err)
	}
	s.index.mu.Lock()
	if s.index.docs == nil {
		s.index.docs = map[string]indexDoc{}
	}
	doc := s.content(c, abs, fi)
	s.index.mu.Unlock()

	d := &Details{SceneStats: doc.stats, Shares: shares[c]}
	var oldest string
	d.Versions, oldest = s.versionCount(c)
	if m := meta[c]; m.Created != nil {
		d.Created, d.CreatedBy = m.Created, m.CreatedBy
	} else if t, err := time.Parse(versionLayout, oldest); err == nil {
		d.Created = &t
	}
	return d, nil
}

// versionCount compte les versions d'un schéma et renvoie l'identifiant de la
// plus ancienne, sans lire leurs métadonnées (voir versions).
func (s *Store) versionCount(c string) (int, string) {
	des, err := os.ReadDir(s.historyAbs(c))
	if err != nil {
		return 0, ""
	}
	n, oldest := 0, ""
	for _, de := range des {
		id, ok := strings.CutSuffix(de.Name(), Ext)
		if !ok || de.IsDir() || !versionID.MatchString(id) {
			continue
		}
		n++
		if oldest == "" || id < oldest {
			oldest = id
		}
	}
	return n, oldest
}

// dirDetails compte les sous-dossiers et schémas visibles d'un dossier.
func (s *Store) dirDetails(rel string) (*Details, error) {
	_, abs, err := s.resolve(rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return nil, mapErr(err)
	}
	d := &Details{}
	for _, de := range des {
		if !visible(de.Name(), de.IsDir()) || de.Type()&os.ModeSymlink != 0 {
			continue
		}
		if de.IsDir() {
			d.Dirs++
		} else {
			d.Files++
		}
	}
	return d, nil
}

// shareCounts compte les liens de partage actifs par schéma.
func (s *Store) shareCounts() map[string]int {
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	m, err := s.readShares()
	if err != nil {
		return nil
	}
	out := map[string]int{}
	now := time.Now()
	for _, d := range m {
		if d.Expires == nil || now.Before(*d.Expires) {
			out[d.Path]++
		}
	}
	return out
}

// noteEdit retient le dernier utilisateur qui a enregistré un schéma et, pour
// un schéma nouveau, sa date de création et son auteur. .meta.json n'est
// réécrit que si quelque chose change (pas à chaque enregistrement automatique).
func (s *Store) noteEdit(c, by string, created bool) {
	if c == "" || by == "" {
		return
	}
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return
	}
	m := s.meta[c]
	changed := false
	if m.EditedBy != by {
		m.EditedBy, changed = by, true
	}
	if created && m.Created == nil {
		now := time.Now().UTC().Truncate(time.Second)
		m.Created, m.CreatedBy, changed = &now, by, true
	}
	if changed {
		s.meta[c] = m
		_ = s.saveMeta()
	}
}

// AddDetails complète des entrées (chemins virtuels) de leurs détails. Les
// dossiers virtuels, favoris cassés et éléments de la poubelle sont laissés
// tels quels.
func (u *Space) AddDetails(es []*Entry) {
	metas := map[*Store]map[string]itemMeta{}
	shares := map[*Store]map[string]int{}
	for _, e := range es {
		if e.Special != "" || e.Broken || e.Origin != "" {
			continue
		}
		st, rel, err := u.route(e.Path)
		if err != nil {
			continue
		}
		if e.IsDir {
			e.Details, _ = st.dirDetails(rel)
			continue
		}
		if _, ok := metas[st]; !ok {
			metas[st], shares[st] = st.metaSnapshot(), st.shareCounts()
		}
		e.Details, _ = st.sceneDetails(rel, metas[st], shares[st])
	}
}
