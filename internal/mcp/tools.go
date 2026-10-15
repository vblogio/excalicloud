package mcp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/vblogio/excalicloud/internal/storage"
)

const instructions = `Excalicloud stocke des schémas Excalidraw (.excalidraw) dans une arborescence de dossiers partagée entre les utilisateurs.
Chemins : relatifs à la racine, séparés par "/" ("" = racine). ".perso/..." est l'espace personnel de l'utilisateur (privé) ; ".favoris" liste ses favoris (lecture seule ici).
Pour créer un schéma : write_diagram avec des éléments Excalidraw. Raccourcis acceptés : "label": {"text": "..."} sur rectangle/ellipse/diamond/arrow, et "start"/"end": {"id": "<id d'une forme>"} sur une flèche pour la relier (points calculés automatiquement).
Avant de modifier un schéma existant, le lire avec read_diagram, puis le réécrire en entier avec overwrite: true (les versions précédentes restent dans l'historique). Donner à l'utilisateur le lien "url" renvoyé.`

// tool décrit un outil MCP (tools/list).
type tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

func obj(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

var readOnly = map[string]any{"readOnlyHint": true, "openWorldHint": false}

var tools = []tool{
	{
		Name:        "list_diagrams",
		Title:       "Lister les schémas",
		Description: "Liste les dossiers et schémas d'un dossier. À la racine apparaissent aussi .perso (espace personnel) et .favoris.",
		InputSchema: obj(nil, map[string]any{
			"path":      str(`Dossier à lister ("" ou absent : racine), ex. "réseau/nsx" ou ".perso".`),
			"recursive": boolean("Parcourir aussi les sous-dossiers (500 entrées au plus). Défaut : false."),
		}),
		Annotations: readOnly,
	},
	{
		Name:        "search_diagrams",
		Title:       "Rechercher des schémas",
		Description: "Recherche dans les noms, les tags et le texte des schémas (casse et accents ignorés, tous les mots requis).",
		InputSchema: obj(nil, map[string]any{
			"query":     str("Mots recherchés (peut être vide si tag ou favorites est donné)."),
			"tag":       str("Ne garder que les éléments portant ce tag."),
			"favorites": boolean("Ne garder que les favoris de l'utilisateur."),
			"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "description": "Nombre maximal de résultats (défaut 50)."},
		}),
		Annotations: readOnly,
	},
	{
		Name:        "read_diagram",
		Title:       "Lire un schéma",
		Description: "Renvoie les éléments Excalidraw et l'appState d'un schéma. Les images embarquées (files) sont omises par défaut ; elles sont conservées si le schéma est réécrit sans champ files.",
		InputSchema: obj([]string{"path"}, map[string]any{
			"path":          str(`Chemin du schéma, ex. "réseau/nsx.excalidraw".`),
			"include_files": boolean("Inclure les images embarquées (base64, volumineux). Défaut : false."),
		}),
		Annotations: readOnly,
	},
	{
		Name:  "write_diagram",
		Title: "Créer ou remplacer un schéma",
		Description: `Crée un schéma (ou le remplace avec overwrite: true). Les dossiers parents sont créés au besoin ; l'extension .excalidraw est ajoutée si absente.
Éléments : objets Excalidraw ; seuls type et la géométrie sont nécessaires, le reste est complété (id, couleurs, seed…).
- rectangle / ellipse / diamond : x, y, width, height ; option backgroundColor, strokeColor, fillStyle ("solid", "hachure"), strokeStyle ("dashed"…), "label": {"text": "...", "fontSize": 20}.
- text : x, y, text ("\n" pour les retours à la ligne), fontSize (20), fontFamily (5 Excalifont, 2 Helvetica, 3 Cascadia).
- arrow / line : x, y, points ([[0,0],[dx,dy],...] relatifs à x,y), ou pour relier deux formes "start": {"id": "a"}, "end": {"id": "b"} sans points ; "label" possible ; endArrowhead ("arrow", null…).
- frame : x, y, width, height, name ; ses enfants portent frameId.
Palette Excalidraw : #1e1e1e, #e03131, #2f9e44, #1971c2, #f08c00 ; fonds clairs #ffc9c9, #b2f2bb, #a5d8ff, #ffec99.`,
		InputSchema: obj([]string{"path", "elements"}, map[string]any{
			"path":      str(`Chemin du schéma, ex. "réseau/vcf-overlay.excalidraw" ou ".perso/brouillon".`),
			"elements":  map[string]any{"type": "array", "items": map[string]any{"type": "object"}, "description": "Éléments Excalidraw de la scène complète."},
			"appState":  map[string]any{"type": "object", "description": `Optionnel, ex. {"viewBackgroundColor": "#ffffff"}.`},
			"files":     map[string]any{"type": "object", "description": "Optionnel : images embarquées (fileId → {mimeType, id, dataURL})."},
			"overwrite": boolean("Remplacer le schéma s'il existe. Défaut : false (erreur si le schéma existe)."),
		}),
		Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": false},
	},
	{
		Name:        "create_folder",
		Title:       "Créer un dossier",
		Description: "Crée un dossier (et ses parents).",
		InputSchema: obj([]string{"path"}, map[string]any{
			"path": str(`Chemin du dossier, ex. "réseau/nsx" ou ".perso/projets".`),
		}),
		Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
	},
}

type call struct {
	s *Server
	r *http.Request
	u *storage.Space
}

var handlers = map[string]func(*call, json.RawMessage) (map[string]any, error){
	"list_diagrams":   (*call).list,
	"search_diagrams": (*call).search,
	"read_diagram":    (*call).read,
	"write_diagram":   (*call).write,
	"create_folder":   (*call).mkdir,
}

// argError signale des arguments invalides (message renvoyé au modèle).
type argError struct{ msg string }

func (e argError) Error() string { return "argument invalide : " + e.msg }

func decode(raw json.RawMessage, v any) error {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return argError{err.Error()}
	}
	return nil
}

// link renvoie l'adresse du schéma dans l'interface.
func (c *call) link(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return c.s.BaseURL(c.r) + "/#/" + strings.Join(parts, "/")
}

type entry struct {
	Path     string    `json:"path"`
	Type     string    `json:"type"` // folder ou diagram
	Modified time.Time `json:"modified,omitzero"`
	Tags     []string  `json:"tags,omitempty"`
	Favorite bool      `json:"favorite,omitempty"`
	URL      string    `json:"url,omitempty"`
	Snippet  string    `json:"snippet,omitempty"`
}

func (c *call) entry(e storage.Entry) entry {
	out := entry{Path: e.Path, Type: "diagram", Modified: e.ModTime, Tags: e.Tags, Favorite: e.Favorite}
	if e.IsDir || e.Special != "" {
		out.Type = "folder"
		out.Modified = time.Time{}
	} else {
		out.URL = c.link(e.Path)
	}
	return out
}

const maxList = 500

func (c *call) list(raw json.RawMessage) (map[string]any, error) {
	var a struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	out := []entry{}
	queue := []string{strings.Trim(a.Path, "/")}
	truncated := false
	for len(queue) > 0 && !truncated {
		dir := queue[0]
		queue = queue[1:]
		es, err := c.u.List(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range es {
			if len(out) == maxList {
				truncated = true
				break
			}
			out = append(out, c.entry(e))
			if a.Recursive && (e.IsDir || e.Special == "perso") {
				queue = append(queue, e.Path)
			}
		}
	}
	return map[string]any{"path": a.Path, "entries": out, "truncated": truncated}, nil
}

func (c *call) search(raw json.RawMessage) (map[string]any, error) {
	var a struct {
		Query     string `json:"query"`
		Tag       string `json:"tag"`
		Favorites bool   `json:"favorites"`
		Limit     int    `json:"limit"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.Limit <= 0 || a.Limit > 200 {
		a.Limit = 50
	}
	if strings.TrimSpace(a.Query) == "" && a.Tag == "" && !a.Favorites {
		return nil, argError{"query, tag ou favorites requis"}
	}
	hits, err := c.u.Search(a.Query, storage.Filter{Tag: a.Tag, Favorite: a.Favorites}, a.Limit)
	if err != nil {
		return nil, err
	}
	out := []entry{}
	for _, h := range hits {
		e := c.entry(h.Entry)
		e.Snippet = h.Snippet
		out = append(out, e)
	}
	return map[string]any{"results": out}, nil
}

type scene struct {
	Type     string         `json:"type"`
	Version  int            `json:"version"`
	Source   string         `json:"source"`
	Elements []any          `json:"elements"`
	AppState map[string]any `json:"appState"`
	Files    map[string]any `json:"files"`
}

func (c *call) read(raw json.RawMessage) (map[string]any, error) {
	var a struct {
		Path         string `json:"path"`
		IncludeFiles bool   `json:"include_files"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	data, _, err := c.u.Read(a.Path)
	if err != nil {
		return nil, err
	}
	var sc scene
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, storage.ErrInvalidJSON
	}
	els := []any{}
	for _, e := range sc.Elements {
		if m, ok := e.(map[string]any); ok && m["isDeleted"] == true {
			continue
		}
		els = append(els, e)
	}
	out := map[string]any{"path": a.Path, "url": c.link(a.Path), "elements": els, "appState": sc.AppState}
	if a.IncludeFiles {
		out["files"] = sc.Files
	} else if len(sc.Files) > 0 {
		out["filesOmitted"] = len(sc.Files)
	}
	return out, nil
}

func (c *call) write(raw json.RawMessage) (map[string]any, error) {
	var a struct {
		Path      string         `json:"path"`
		Elements  []any          `json:"elements"`
		AppState  map[string]any `json:"appState"`
		Files     map[string]any `json:"files"`
		Overwrite bool           `json:"overwrite"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	p := strings.Trim(a.Path, "/")
	if p == "" {
		return nil, argError{"path requis"}
	}
	if path.Ext(p) != storage.Ext {
		p += storage.Ext
	}
	if a.Elements == nil {
		return nil, argError{"elements requis (tableau, éventuellement vide)"}
	}
	els, err := normalize(a.Elements)
	if err != nil {
		return nil, argError{err.Error()}
	}
	sc := scene{Type: "excalidraw", Version: 2, Source: "excalicloud-mcp", Elements: els, AppState: a.AppState, Files: a.Files}
	if sc.AppState == nil {
		sc.AppState = map[string]any{}
	}
	if _, ok := sc.AppState["viewBackgroundColor"]; !ok {
		sc.AppState["viewBackgroundColor"] = "#ffffff"
	}
	if sc.Files == nil {
		sc.Files = map[string]any{}
	}
	created := true
	if a.Overwrite {
		old, _, err := c.u.Read(p)
		switch {
		case err == nil:
			created = false
			keepFiles(&sc, old)
		case !errors.Is(err, storage.ErrNotFound):
			return nil, err
		}
	}
	data, err := json.Marshal(sc)
	if err != nil {
		return nil, err
	}
	e, _, err := c.u.Write(p, data, "", !a.Overwrite)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": e.Path, "url": c.link(e.Path), "created": created, "elements": len(els)}, nil
}

// keepFiles reprend de l'ancien contenu les images encore référencées que la
// nouvelle scène ne fournit pas (read_diagram les omet par défaut).
func keepFiles(sc *scene, old []byte) {
	var prev scene
	if json.Unmarshal(old, &prev) != nil {
		return
	}
	for _, e := range sc.Elements {
		m, _ := e.(map[string]any)
		id, _ := m["fileId"].(string)
		if id == "" {
			continue
		}
		if _, ok := sc.Files[id]; !ok && prev.Files[id] != nil {
			sc.Files[id] = prev.Files[id]
		}
	}
}

func (c *call) mkdir(raw json.RawMessage) (map[string]any, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	e, err := c.u.Mkdir(a.Path)
	if errors.Is(err, storage.ErrExists) {
		return map[string]any{"path": strings.Trim(a.Path, "/"), "created": false}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": e.Path, "created": true}, nil
}
