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
		if r.URL.Path != "/-/package/@excalidraw/excalidraw/dist-tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"latest":"0.19.0","next":"0.19.0-abc1234","rc":"0.19.0-rc.1"}`))
	}))
	defer srv.Close()

	c := &Checker{Registry: srv.URL, Interval: time.Hour, Client: srv.Client()}
	if s := c.Status(); !s.Enabled || s.Latest != "" {
		t.Fatalf("avant vérification : %+v", s)
	}
	c.check(context.Background())
	s := c.Status()
	if s.Latest != "0.19.0" || s.Tags["next"] != "0.19.0-abc1234" || s.CheckedAt == nil {
		t.Fatalf("après vérification : %+v", s)
	}
	if _, ok := s.Tags["rc"]; ok {
		t.Fatalf("canal non proposé exposé : %+v", s.Tags)
	}
}

func TestCheckKeepsLastValueOnError(t *testing.T) {
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			http.Error(w, "panne", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"latest":"0.18.2"}`))
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
