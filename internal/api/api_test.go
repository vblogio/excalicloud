package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vblogio/excalicloud/internal/storage"
)

const doc = `{"type":"excalidraw","version":2,"elements":[]}`

// L'utilisateur vient de l'en-tête X-User (à la place de la session OIDC).
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{
		Store:   st,
		Spaces:  storage.NewSpaces(st),
		User:    func(r *http.Request) string { return storage.UserID(r.Header.Get("X-User")) },
		MaxBody: 1 << 20,
	}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, user, method, url, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+url, strings.NewReader(body))
	req.Header.Set("X-User", user)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestFavoritesPerUser(t *testing.T) {
	srv := newServer(t)
	for _, p := range []string{"a.excalidraw", ".perso/mine.excalidraw"} {
		if c := call(t, srv, "alice@x.fr", "PUT", "/api/file?path="+p, doc, nil); c != 200 {
			t.Fatalf("écriture %s : %d", p, c)
		}
	}
	if c := call(t, srv, "alice@x.fr", "POST", "/api/meta/favorite", `{"paths":["a.excalidraw",".perso/mine.excalidraw"],"favorite":true}`, nil); c != 204 {
		t.Fatalf("favori : %d", c)
	}
	var favs []storage.Entry
	call(t, srv, "alice@x.fr", "GET", "/api/list?path=.favoris", "", &favs)
	if len(favs) != 2 || favs[0].Path != "a.excalidraw" || favs[1].Path != ".perso/mine.excalidraw" {
		t.Fatalf("favoris d'alice = %+v", favs)
	}
	call(t, srv, "bob@x.fr", "GET", "/api/list?path=.favoris", "", &favs)
	if len(favs) != 0 {
		t.Fatalf("favoris de bob = %+v", favs)
	}
	var root []storage.Entry
	call(t, srv, "bob@x.fr", "GET", "/api/list?path=", "", &root)
	if len(root) != 4 || root[0].Special != "favoris" || root[2].Special != "poubelle" || root[3].Favorite {
		t.Fatalf("racine de bob = %+v", root)
	}
	if c := call(t, srv, "bob@x.fr", "GET", "/api/file?path=.perso/mine.excalidraw", "", nil); c != 404 {
		t.Fatalf("bob lit l'espace d'alice : %d", c)
	}
	// Supprimer depuis Favoris ne retire que le lien.
	call(t, srv, "alice@x.fr", "DELETE", "/api/entry?path=.favoris/a.excalidraw", "", nil)
	if c := call(t, srv, "alice@x.fr", "GET", "/api/file?path=a.excalidraw", "", nil); c != 200 {
		t.Fatalf("original supprimé avec le lien : %d", c)
	}
	call(t, srv, "alice@x.fr", "GET", "/api/list?path=.favoris", "", &favs)
	if len(favs) != 1 {
		t.Fatalf("favoris après retrait = %+v", favs)
	}
	// Écrire dans .favoris est refusé.
	if c := call(t, srv, "alice@x.fr", "PUT", "/api/file?path=.favoris/x.excalidraw", doc, nil); c != 400 {
		t.Fatalf("écriture dans Favoris : %d", c)
	}
}

func TestTrash(t *testing.T) {
	srv := newServer(t)
	call(t, srv, "alice@x.fr", "PUT", "/api/file?path=d/a.excalidraw", doc, nil)
	if c := call(t, srv, "alice@x.fr", "DELETE", "/api/entry?path=d/a.excalidraw", "", nil); c != 204 {
		t.Fatalf("suppression : %d", c)
	}
	var items []storage.Entry
	call(t, srv, "bob@x.fr", "GET", "/api/list?path=.poubelle", "", &items)
	if len(items) != 1 || items[0].Origin != "d/a.excalidraw" || items[0].By != "alice@x.fr" {
		t.Fatalf("poubelle vue par bob = %+v", items)
	}
	var res []storage.MoveResult
	if c := call(t, srv, "bob@x.fr", "POST", "/api/trash/restore", `{"paths":["`+items[0].Path+`"]}`, &res); c != 200 ||
		len(res) != 1 || res[0].To != "d/a.excalidraw" {
		t.Fatalf("restauration : %d %+v", c, res)
	}
	if c := call(t, srv, "alice@x.fr", "GET", "/api/file?path=d/a.excalidraw", "", nil); c != 200 {
		t.Fatalf("lecture après restauration : %d", c)
	}

	// Réglage de la purge, puis vidage.
	var st storage.Settings
	if c := call(t, srv, "alice@x.fr", "PUT", "/api/settings", `{"trashDays":7}`, &st); c != 200 || st.TrashDays != 7 {
		t.Fatalf("trashDays : %d %+v", c, st)
	}
	if c := call(t, srv, "alice@x.fr", "PUT", "/api/settings", `{"trashDays":-3}`, nil); c != 400 {
		t.Fatalf("trashDays négatif : %d", c)
	}
	call(t, srv, "alice@x.fr", "DELETE", "/api/entry?path=d/a.excalidraw", "", nil)
	if c := call(t, srv, "alice@x.fr", "DELETE", "/api/entry?path=.poubelle", "", nil); c != 204 {
		t.Fatalf("vidage : %d", c)
	}
	call(t, srv, "alice@x.fr", "GET", "/api/list?path=.poubelle", "", &items)
	if len(items) != 0 {
		t.Fatalf("poubelle après vidage = %+v", items)
	}
}
