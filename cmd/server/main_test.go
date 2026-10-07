package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuthPublicPaths(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
	h := basicAuth(ok, "u", "p", public)
	cases := map[string]int{
		"/healthz":            200,
		"/s/abc.def":          200,
		"/api/public/abc.def": 200,
		"/":                   401,
		"/index.html":         401,
		"/api/list":           401,
		"/api/file?path=x":    401,
		"/api/publicx":        401,
		"/assets/":            401, // dossier : pas un fichier statique
		"/inconnu":            401,
	}
	for p, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != want {
			t.Errorf("%s : %d, attendu %d", p, rec.Code, want)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/list", nil)
	req.SetBasicAuth("u", "p")
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("authentifié : %d", rec.Code)
	}
}
