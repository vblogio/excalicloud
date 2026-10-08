package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAuthenticateModes(t *testing.T) {
	me := func(t *testing.T, h http.Handler, user string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/me", nil)
		if user != "" {
			req.SetBasicAuth(user, "p")
		}
		h.ServeHTTP(rec, req)
		return strings.TrimSpace(rec.Body.String())
	}

	t.Run("local sans identifiants", func(t *testing.T) {
		t.Setenv("AUTH_MODE", "")
		t.Setenv("AUTH_USER", "")
		h, err := authenticate(context.Background(), http.NewServeMux(), t.TempDir())
		if err != nil || me(t, h, "") != `{"mode":"none"}` {
			t.Fatalf("%v %s", err, me(t, h, ""))
		}
	})
	t.Run("local Basic", func(t *testing.T) {
		t.Setenv("AUTH_MODE", "local")
		t.Setenv("AUTH_USER", "u")
		t.Setenv("AUTH_PASSWORD", "p")
		h, err := authenticate(context.Background(), http.NewServeMux(), t.TempDir())
		if err != nil || !strings.Contains(me(t, h, "u"), `"mode":"local"`) {
			t.Fatalf("%v %s", err, me(t, h, "u"))
		}
	})
	t.Run("oidc sans émetteur", func(t *testing.T) {
		t.Setenv("AUTH_MODE", "oidc")
		t.Setenv("OIDC_ISSUER", "")
		if _, err := authenticate(context.Background(), http.NewServeMux(), t.TempDir()); err == nil {
			t.Fatal("configuration incomplète acceptée")
		}
	})
	t.Run("mode inconnu", func(t *testing.T) {
		t.Setenv("AUTH_MODE", "ldap")
		if _, err := authenticate(context.Background(), http.NewServeMux(), t.TempDir()); err == nil {
			t.Fatal("AUTH_MODE inconnu accepté")
		}
	})
}
