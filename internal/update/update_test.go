package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@excalidraw/excalidraw/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"name":"@excalidraw/excalidraw","version":"0.19.0"}`))
	}))
	defer srv.Close()

	c := &Checker{Registry: srv.URL, Interval: time.Hour, Client: srv.Client()}
	if s := c.Status(); !s.Enabled || s.Latest != "" {
		t.Fatalf("avant vérification : %+v", s)
	}
	c.check(context.Background())
	if s := c.Status(); s.Latest != "0.19.0" || s.CheckedAt == nil {
		t.Fatalf("après vérification : %+v", s)
	}
}

func TestCheckKeepsLastValueOnError(t *testing.T) {
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			http.Error(w, "panne", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"version":"0.18.2"}`))
	}))
	defer srv.Close()

	c := &Checker{Registry: srv.URL, Interval: time.Hour, Client: srv.Client()}
	c.check(context.Background())
	ok = false
	c.check(context.Background())
	if s := c.Status(); s.Latest != "0.18.2" {
		t.Fatalf("valeur perdue après une erreur : %+v", s)
	}
}

func TestNilCheckerIsDisabled(t *testing.T) {
	var c *Checker
	if s := c.Status(); s.Enabled {
		t.Fatalf("checker nil : %+v", s)
	}
}
