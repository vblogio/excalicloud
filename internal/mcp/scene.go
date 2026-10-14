package mcp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Normalisation des éléments écrits par un client MCP : un modèle produit
// rarement tous les champs d'un élément Excalidraw. normalize complète les
// valeurs par défaut et accepte deux raccourcis de l'API « skeleton »
// d'Excalidraw :
//   - "label": {"text": "..."} sur un rectangle, une ellipse, un losange ou
//     une flèche : texte lié, centré dans le conteneur ;
//   - "start"/"end": {"id": "..."} sur une flèche : liaison aux formes,
//     points calculés entre leurs bords si la flèche n'en donne pas.
//
// Excalidraw répare encore la scène au chargement (restore), mais un fichier
// complet s'affiche aussi correctement en miniature et dans la recherche.

type element = map[string]any

func randID() string {
	b := make([]byte, 15)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func randInt() int64 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return int64(binary.BigEndian.Uint32(b[:]) >> 1)
}

func num(e element, k string) (float64, bool) {
	switch v := e[k].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

func numOr(e element, k string, def float64) float64 {
	if v, ok := num(e, k); ok {
		return v
	}
	return def
}

func setDefault(e element, k string, v any) {
	if _, ok := e[k]; !ok {
		e[k] = v
	}
}

// textSize estime l'encombrement d'un texte (Excalidraw le recalcule à l'affichage).
func textSize(text string, fontSize, lineHeight float64) (w, h float64) {
	lines := strings.Split(text, "\n")
	longest := 0
	for _, l := range lines {
		longest = max(longest, utf8.RuneCountInString(l))
	}
	return math.Ceil(float64(longest) * fontSize * 0.6), math.Ceil(float64(len(lines)) * fontSize * lineHeight)
}

var containers = map[string]bool{"rectangle": true, "ellipse": true, "diamond": true, "arrow": true}

// normalize valide et complète les éléments ; ils sont modifiés en place.
func normalize(els []any) ([]any, error) {
	now := time.Now().UnixMilli()
	byID := map[string]element{}
	var out []any
	var labels, arrows []element
	for i, raw := range els {
		e, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("élément %d : objet attendu", i)
		}
		typ, _ := e["type"].(string)
		if typ == "" {
			return nil, fmt.Errorf("élément %d : champ type manquant", i)
		}
		if id, _ := e["id"].(string); id == "" {
			e["id"] = randID()
		}
		if _, dup := byID[e["id"].(string)]; dup {
			return nil, fmt.Errorf("élément %d : id %q en double", i, e["id"])
		}
		byID[e["id"].(string)] = e
		base(e, now)
		switch typ {
		case "text":
			if err := text(e); err != nil {
				return nil, fmt.Errorf("élément %d : %w", i, err)
			}
		case "arrow", "line":
			if typ == "arrow" {
				setDefault(e, "roundness", map[string]any{"type": 2})
			}
			arrows = append(arrows, e)
		case "rectangle", "diamond":
			setDefault(e, "roundness", map[string]any{"type": 3})
		}
		setDefault(e, "roundness", nil)
		if _, ok := e["label"]; ok && containers[typ] {
			labels = append(labels, e)
		}
		out = append(out, e)
	}
	for _, c := range labels {
		fit(c)
	}
	for _, a := range arrows {
		if err := bindArrow(a, byID); err != nil {
			return nil, err
		}
		linear(a)
	}
	for _, c := range labels {
		t, err := label(c, byID)
		if err != nil {
			return nil, err
		}
		base(t, now)
		out = append(out, t)
	}
	return out, nil
}

func base(e element, now int64) {
	setDefault(e, "x", 0.0)
	setDefault(e, "y", 0.0)
	if e["type"] != "text" && e["type"] != "arrow" && e["type"] != "line" {
		setDefault(e, "width", 100.0)
		setDefault(e, "height", 100.0)
	}
	setDefault(e, "angle", 0)
	setDefault(e, "strokeColor", "#1e1e1e")
	setDefault(e, "backgroundColor", "transparent")
	setDefault(e, "fillStyle", "solid")
	setDefault(e, "strokeWidth", 2)
	setDefault(e, "strokeStyle", "solid")
	setDefault(e, "roughness", 1)
	setDefault(e, "opacity", 100)
	setDefault(e, "groupIds", []any{})
	setDefault(e, "frameId", nil)
	setDefault(e, "boundElements", nil)
	setDefault(e, "link", nil)
	setDefault(e, "locked", false)
	setDefault(e, "isDeleted", false)
	setDefault(e, "seed", randInt())
	setDefault(e, "version", 1)
	setDefault(e, "versionNonce", randInt())
	setDefault(e, "updated", now)
}

func text(e element) error {
	s, ok := e["text"].(string)
	if !ok {
		return errors.New("texte sans champ text")
	}
	setDefault(e, "originalText", s)
	setDefault(e, "fontSize", 20)
	setDefault(e, "fontFamily", 5) // Excalifont
	setDefault(e, "textAlign", "left")
	setDefault(e, "verticalAlign", "top")
	setDefault(e, "containerId", nil)
	setDefault(e, "autoResize", true)
	setDefault(e, "lineHeight", 1.25)
	w, h := textSize(s, numOr(e, "fontSize", 20), numOr(e, "lineHeight", 1.25))
	setDefault(e, "width", w)
	setDefault(e, "height", h)
	return nil
}

func addBound(c element, id, typ string) {
	list, _ := c["boundElements"].([]any)
	for _, b := range list {
		if m, ok := b.(map[string]any); ok && m["id"] == id {
			return
		}
	}
	c["boundElements"] = append(list, map[string]any{"id": id, "type": typ})
}

// label transforme le raccourci "label" d'un conteneur en élément texte lié.
func label(c element, byID map[string]element) (element, error) {
	l, _ := c["label"].(map[string]any)
	delete(c, "label")
	s, _ := l["text"].(string)
	if s == "" {
		return nil, fmt.Errorf("élément %v : label sans texte", c["id"])
	}
	id := c["id"].(string) + "-label"
	if _, dup := byID[id]; dup {
		id = randID()
	}
	fontSize := numOr(l, "fontSize", 20)
	w, h := textSize(s, fontSize, 1.25)
	cx, cy := center(c)
	t := element{
		"id": id, "type": "text", "text": s, "originalText": s,
		"fontSize": fontSize, "fontFamily": numOr(l, "fontFamily", 5),
		"textAlign": "center", "verticalAlign": "middle", "containerId": c["id"],
		"autoResize": true, "lineHeight": 1.25, "roundness": nil,
		"x": cx - w/2, "y": cy - h/2, "width": w, "height": h,
		"strokeColor": c["strokeColor"],
	}
	if col, ok := l["strokeColor"].(string); ok {
		t["strokeColor"] = col
	}
	addBound(c, id, "text")
	byID[id] = t
	return t, nil
}

// fit agrandit une forme (centre conservé) pour que son label y tienne.
func fit(c element) {
	l, _ := c["label"].(map[string]any)
	s, _ := l["text"].(string)
	if s == "" || c["type"] == "arrow" {
		return
	}
	tw, th := textSize(s, numOr(l, "fontSize", 20), 1.25)
	k := 1.0 // part utilisable de la forme pour le texte
	switch c["type"] {
	case "ellipse":
		k = 1 / math.Sqrt2
	case "diamond":
		k = 0.5
	}
	w, h := numOr(c, "width", 100), numOr(c, "height", 100)
	nw, nh := math.Max(w, math.Ceil((tw+20)/k)), math.Max(h, math.Ceil((th+20)/k))
	c["x"] = numOr(c, "x", 0) - (nw-w)/2
	c["y"] = numOr(c, "y", 0) - (nh-h)/2
	c["width"], c["height"] = nw, nh
}

func center(e element) (float64, float64) {
	x, y := numOr(e, "x", 0), numOr(e, "y", 0)
	if pts, ok := e["points"].([]any); ok && len(pts) >= 2 {
		// Milieu du segment central d'une flèche.
		i := len(pts) / 2
		a, b := point(pts[i-1]), point(pts[i])
		return x + (a[0]+b[0])/2, y + (a[1]+b[1])/2
	}
	return x + numOr(e, "width", 0)/2, y + numOr(e, "height", 0)/2
}

func point(v any) [2]float64 {
	p, _ := v.([]any)
	var out [2]float64
	for i := 0; i < 2 && i < len(p); i++ {
		out[i], _ = p[i].(float64)
	}
	return out
}

// edge renvoie le point du bord de la forme e dans la direction de (tx, ty).
func edge(e element, tx, ty, gap float64) (float64, float64) {
	cx, cy := center(e)
	dx, dy := tx-cx, ty-cy
	if dx == 0 && dy == 0 {
		return cx, cy
	}
	hw, hh := numOr(e, "width", 0)/2+gap, numOr(e, "height", 0)/2+gap
	var t float64
	switch e["type"] {
	case "ellipse":
		t = 1 / math.Sqrt(dx*dx/(hw*hw)+dy*dy/(hh*hh))
	case "diamond":
		t = 1 / (math.Abs(dx)/hw + math.Abs(dy)/hh)
	default:
		t = math.Min(hw/math.Max(math.Abs(dx), 1e-9), hh/math.Max(math.Abs(dy), 1e-9))
	}
	return cx + dx*t, cy + dy*t
}

// bindArrow applique les raccourcis start/end d'une flèche.
func bindArrow(a element, byID map[string]element) error {
	var ends [2]element
	for i, k := range []string{"start", "end"} {
		ref, ok := a[k].(map[string]any)
		if !ok {
			continue
		}
		delete(a, k)
		id, _ := ref["id"].(string)
		target, ok := byID[id]
		if !ok {
			return fmt.Errorf("flèche %v : %s vers l'élément inconnu %q", a["id"], k, id)
		}
		ends[i] = target
		a[k+"Binding"] = map[string]any{"elementId": id, "focus": 0, "gap": 4}
		addBound(target, a["id"].(string), "arrow")
	}
	if _, has := a["points"]; has || ends[0] == nil || ends[1] == nil {
		return nil
	}
	sx, sy := center(ends[0])
	ex, ey := center(ends[1])
	x1, y1 := edge(ends[0], ex, ey, 4)
	x2, y2 := edge(ends[1], sx, sy, 4)
	a["x"], a["y"] = x1, y1
	a["points"] = []any{[]any{0.0, 0.0}, []any{x2 - x1, y2 - y1}}
	return nil
}

// linear complète une flèche ou une ligne : points et encombrement.
func linear(e element) {
	pts, ok := e["points"].([]any)
	if !ok || len(pts) < 2 {
		pts = []any{[]any{0.0, 0.0}, []any{numOr(e, "width", 100), numOr(e, "height", 0)}}
		e["points"] = pts
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		q := point(p)
		minX, maxX = math.Min(minX, q[0]), math.Max(maxX, q[0])
		minY, maxY = math.Min(minY, q[1]), math.Max(maxY, q[1])
	}
	e["width"], e["height"] = maxX-minX, maxY-minY
	setDefault(e, "lastCommittedPoint", nil)
	setDefault(e, "startBinding", nil)
	setDefault(e, "endBinding", nil)
	setDefault(e, "startArrowhead", nil)
	if e["type"] == "arrow" {
		setDefault(e, "endArrowhead", "arrow")
		setDefault(e, "elbowed", false)
	} else {
		setDefault(e, "endArrowhead", nil)
	}
}
