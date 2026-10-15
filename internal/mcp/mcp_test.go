package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vblogio/excalicloud/internal/storage"
)

func newServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		Spaces:  storage.NewSpaces(st),
		User:    func(*http.Request) string { return "a@x.fr" },
		BaseURL: func(*http.Request) string { return "https://ex.test" },
		MaxBody: 1 << 20,
		Version: "test",
	}
}

func rpc(t *testing.T, h http.Handler, method string, params any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/mcp", strings.NewReader(string(b))))
	var res map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("%s : %d %s", method, rec.Code, rec.Body)
	}
	return res
}

// tool appelle un outil et renvoie son résultat structuré (ou son erreur).
func callTool(t *testing.T, h http.Handler, name string, args any) (map[string]any, string) {
	t.Helper()
	res := rpc(t, h, "tools/call", map[string]any{"name": name, "arguments": args})
	r, _ := res["result"].(map[string]any)
	if r == nil {
		t.Fatalf("%s : %v", name, res)
	}
	if r["isError"] == true {
		return nil, r["content"].([]any)[0].(map[string]any)["text"].(string)
	}
	return r["structuredContent"].(map[string]any), ""
}

func TestProtocol(t *testing.T) {
	h := newServer(t)
	res := rpc(t, h, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}})
	r := res["result"].(map[string]any)
	if r["protocolVersion"] != "2025-06-18" || r["capabilities"].(map[string]any)["tools"] == nil {
		t.Fatalf("initialize = %v", r)
	}
	if v := rpc(t, h, "initialize", map[string]any{"protocolVersion": "1999-01-01"})["result"].(map[string]any)["protocolVersion"]; v != protocolVersions[0] {
		t.Fatalf("version inconnue → %v", v)
	}
	tl := rpc(t, h, "tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	if len(tl) != len(handlers) {
		t.Fatalf("%d outils", len(tl))
	}
	if e := rpc(t, h, "nope", nil)["error"].(map[string]any); e["code"].(float64) != errMethod {
		t.Fatalf("méthode inconnue : %v", e)
	}
	// Notification : 202 sans corps ; GET : 405.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)))
	if rec.Code != 202 || rec.Body.Len() != 0 {
		t.Fatalf("notification : %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/mcp", nil))
	if rec.Code != 405 {
		t.Fatalf("GET : %d", rec.Code)
	}
}

func TestWriteReadListSearch(t *testing.T) {
	h := newServer(t)
	els := []any{
		map[string]any{"id": "web", "type": "rectangle", "x": 0, "y": 0, "width": 200, "height": 80, "label": map[string]any{"text": "Web T1"}},
		map[string]any{"id": "db", "type": "ellipse", "x": 400, "y": 0, "width": 160, "height": 80, "label": map[string]any{"text": "Base"}},
		map[string]any{"id": "link", "type": "arrow", "start": map[string]any{"id": "web"}, "end": map[string]any{"id": "db"}, "label": map[string]any{"text": "TCP 5432"}},
		map[string]any{"type": "text", "x": 0, "y": 200, "text": "Segment overlay 172.27.207.0/24"},
	}
	out, errMsg := callTool(t, h, "write_diagram", map[string]any{"path": "réseau/app trois tiers", "elements": els})
	if errMsg != "" || out["path"] != "réseau/app trois tiers.excalidraw" || out["created"] != true {
		t.Fatalf("write = %v %s", out, errMsg)
	}
	if out["url"] != "https://ex.test/#/r%C3%A9seau/app%20trois%20tiers.excalidraw" {
		t.Fatalf("url = %v", out["url"])
	}
	if _, errMsg := callTool(t, h, "write_diagram", map[string]any{"path": "réseau/app trois tiers.excalidraw", "elements": []any{}}); !strings.Contains(errMsg, "existe déjà") {
		t.Fatalf("écrasement sans overwrite : %q", errMsg)
	}

	got, _ := callTool(t, h, "read_diagram", map[string]any{"path": "réseau/app trois tiers.excalidraw"})
	byID := map[string]map[string]any{}
	for _, e := range got["elements"].([]any) {
		m := e.(map[string]any)
		byID[m["id"].(string)] = m
	}
	if len(byID) != 7 {
		t.Fatalf("%d éléments", len(byID))
	}
	lbl := byID["web-label"]
	if lbl["containerId"] != "web" || lbl["text"] != "Web T1" || lbl["verticalAlign"] != "middle" {
		t.Fatalf("label = %v", lbl)
	}
	arrow := byID["link"]
	if arrow["startBinding"].(map[string]any)["elementId"] != "web" || arrow["endBinding"].(map[string]any)["elementId"] != "db" {
		t.Fatalf("liaisons = %v", arrow)
	}
	// La flèche part du bord droit du rectangle et arrive au bord gauche de l'ellipse.
	pts := arrow["points"].([]any)
	if x := arrow["x"].(float64); x != 204 || pts[1].([]any)[0].(float64) != 400-4-204 {
		t.Fatalf("géométrie = x %v points %v", x, pts)
	}
	bound := byID["db"]["boundElements"].([]any)
	if len(bound) != 2 {
		t.Fatalf("boundElements de db = %v", bound)
	}
	for _, k := range []string{"seed", "version", "strokeColor", "roughness"} {
		if _, ok := byID["web"][k]; !ok {
			t.Errorf("champ %s non complété", k)
		}
	}

	l, _ := callTool(t, h, "list_diagrams", map[string]any{"recursive": true})
	paths := []string{}
	for _, e := range l["entries"].([]any) {
		paths = append(paths, e.(map[string]any)["path"].(string))
	}
	if strings.Join(paths, ",") != ".favoris,.perso,.poubelle,réseau,réseau/app trois tiers.excalidraw" {
		t.Fatalf("liste = %v", paths)
	}
	s, _ := callTool(t, h, "search_diagrams", map[string]any{"query": "172.27.207"})
	if r := s["results"].([]any); len(r) != 1 || !strings.Contains(r[0].(map[string]any)["snippet"].(string), "overlay") {
		t.Fatalf("recherche = %v", s)
	}
}

func TestOverwriteKeepsFilesAndPerso(t *testing.T) {
	h := newServer(t)
	img := map[string]any{"id": "img", "type": "image", "fileId": "f1", "x": 0, "y": 0, "width": 10, "height": 10}
	files := map[string]any{"f1": map[string]any{"id": "f1", "mimeType": "image/png", "dataURL": "data:image/png;base64,AA"}}
	if _, e := callTool(t, h, "write_diagram", map[string]any{"path": ".perso/logo", "elements": []any{img}, "files": files}); e != "" {
		t.Fatal(e)
	}
	r, _ := callTool(t, h, "read_diagram", map[string]any{"path": ".perso/logo.excalidraw"})
	if r["files"] != nil || r["filesOmitted"].(float64) != 1 {
		t.Fatalf("files non omis : %v", r)
	}
	// Réécriture sans files : l'image référencée est conservée.
	els := append(r["elements"].([]any), map[string]any{"type": "text", "text": "ajout"})
	out, e := callTool(t, h, "write_diagram", map[string]any{"path": ".perso/logo.excalidraw", "elements": els, "overwrite": true})
	if e != "" || out["created"] != false {
		t.Fatalf("overwrite : %v %s", out, e)
	}
	r, _ = callTool(t, h, "read_diagram", map[string]any{"path": ".perso/logo.excalidraw", "include_files": true})
	if r["files"].(map[string]any)["f1"] == nil || len(r["elements"].([]any)) != 2 {
		t.Fatalf("après overwrite : %v", r)
	}
	if f, _ := callTool(t, h, "create_folder", map[string]any{"path": ".perso/a/b"}); f["created"] != true {
		t.Fatalf("dossier : %v", f)
	}
	if f, _ := callTool(t, h, "create_folder", map[string]any{"path": ".perso/a/b"}); f["created"] != false {
		t.Fatalf("dossier existant : %v", f)
	}
}

func TestToolErrors(t *testing.T) {
	h := newServer(t)
	cases := []struct {
		name string
		args any
		want string
	}{
		{"read_diagram", map[string]any{"path": "absent.excalidraw"}, "introuvable"},
		{"read_diagram", map[string]any{"path": "x", "bogus": 1}, "argument invalide"},
		{"write_diagram", map[string]any{"path": "x", "elements": []any{map[string]any{"x": 1}}}, "type manquant"},
		{"write_diagram", map[string]any{"path": "x", "elements": []any{map[string]any{"type": "arrow", "start": map[string]any{"id": "nope"}}}}, "inconnu"},
		{"write_diagram", map[string]any{"path": ".users/x", "elements": []any{}}, "chemin"},
		{"search_diagrams", map[string]any{}, "requis"},
	}
	for _, c := range cases {
		if _, e := callTool(t, h, c.name, c.args); !strings.Contains(strings.ToLower(e), c.want) {
			t.Errorf("%s %v : %q, attendu %q", c.name, c.args, e, c.want)
		}
	}
}

func TestFitLabel(t *testing.T) {
	els, err := normalize([]any{map[string]any{"id": "d", "type": "diamond", "x": 0.0, "y": 0.0, "width": 100.0, "height": 100.0,
		"label": map[string]any{"text": "NSX Edge cluster"}}})
	if err != nil {
		t.Fatal(err)
	}
	d := els[0].(map[string]any)
	w, x := d["width"].(float64), d["x"].(float64)
	if w <= 100 || x+w/2 != 50 || d["y"].(float64)+d["height"].(float64)/2 != 50 {
		t.Fatalf("losange non agrandi autour de son centre : %v", d)
	}
}
