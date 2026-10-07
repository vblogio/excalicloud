// Commande server : API de gestion de schémas + frontend Excalidraw embarqué.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vblogio/excalidraw-go/internal/api"
	"github.com/vblogio/excalidraw-go/internal/storage"
	"github.com/vblogio/excalidraw-go/internal/update"
	"github.com/vblogio/excalidraw-go/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	addr := env("LISTEN_ADDR", ":8080")
	// `server healthcheck` : sonde pour l'image distroless (pas de curl/wget).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(addr))
	}
	dataDir := env("DATA_DIR", "./data")
	maxMB, err := strconv.ParseInt(env("MAX_BODY_MB", "50"), 10, 64)
	if err != nil || maxMB <= 0 {
		slog.Error("MAX_BODY_MB invalide")
		os.Exit(1)
	}

	store, err := storage.New(dataDir)
	if err != nil {
		slog.Error("initialisation du stockage", "dir", dataDir, "err", err)
		os.Exit(1)
	}

	// Historique des versions : HISTORY_KEEP versions par schéma (0 : désactivé),
	// au plus une toutes les HISTORY_INTERVAL, purgées après HISTORY_MAX_AGE (0 : jamais).
	if store.History, err = historyPolicy(); err != nil {
		slog.Error("historique", "err", err)
		os.Exit(1)
	}

	// Clé de signature des liens de partage : SHARE_SECRET, sinon DATA_DIR/.share-secret.
	if secret := os.Getenv("SHARE_SECRET"); secret != "" {
		if len(secret) < 16 {
			slog.Error("SHARE_SECRET trop court (16 caractères minimum)")
			os.Exit(1)
		}
		store.SetShareSecret(secret)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Surveillance des nouvelles versions d'Excalidraw (UPDATE_CHECK=false pour désactiver).
	var checker *update.Checker
	if b, err := strconv.ParseBool(env("UPDATE_CHECK", "true")); err != nil || b {
		interval, err := time.ParseDuration(env("UPDATE_CHECK_INTERVAL", "12h"))
		if err != nil || interval < time.Minute {
			slog.Error("UPDATE_CHECK_INTERVAL invalide (ex. 12h, minimum 1m)")
			os.Exit(1)
		}
		checker = &update.Checker{
			Registry: env("NPM_REGISTRY", "https://registry.npmjs.org"),
			Interval: interval,
			Client:   &http.Client{}, // respecte HTTPS_PROXY / NO_PROXY
		}
		go checker.Run(ctx)
	}

	mux := http.NewServeMux()
	(&api.Handler{Store: store, MaxBody: maxMB << 20, Update: checker}).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", spa())

	var h http.Handler = mux
	if user, pass := os.Getenv("AUTH_USER"), os.Getenv("AUTH_PASSWORD"); user != "" && pass != "" {
		h = basicAuth(h, user, pass, public)
		slog.Info("authentification Basic activée", "user", user)
	}
	h = logRequests(securityHeaders(h))

	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("démarrage", "addr", addr, "data", dataDir,
			"history_keep", store.History.Keep, "history_interval", store.History.Interval.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serveur", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	slog.Info("arrêt")
}

func historyPolicy() (storage.HistoryPolicy, error) {
	p := storage.DefaultHistory
	keep, err := strconv.Atoi(env("HISTORY_KEEP", strconv.Itoa(p.Keep)))
	if err != nil || keep < 0 {
		return p, errors.New("HISTORY_KEEP invalide (entier >= 0)")
	}
	p.Keep = keep
	if p.Interval, err = time.ParseDuration(env("HISTORY_INTERVAL", p.Interval.String())); err != nil || p.Interval < 0 {
		return p, errors.New("HISTORY_INTERVAL invalide (ex. 10m, 0 : à chaque enregistrement)")
	}
	if p.MaxAge, err = time.ParseDuration(env("HISTORY_MAX_AGE", "0")); err != nil || p.MaxAge < 0 {
		return p, errors.New("HISTORY_MAX_AGE invalide (ex. 2160h, 0 : illimité)")
	}
	return p, nil
}

func healthcheck(addr string) int {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return 1
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// spa sert le build Vite embarqué ; toute route inconnue renvoie index.html.
func spa() http.Handler {
	dist, _ := fs.Sub(web.Dist, "dist")
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		if p != "/" {
			if fi, err := fs.Stat(dist, p[1:]); err == nil && !fi.IsDir() {
				// Les assets Vite sont nommés par empreinte : cache long.
				if path.Dir(p) == "/assets" {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "frontend absent : lancer `npm run build` dans frontend/", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

// public indique les chemins servis sans authentification : sonde, schémas
// partagés (/s/<jeton> et son API) et fichiers statiques du build (code de
// l'application et polices, sans aucune donnée).
func public(p string) bool {
	switch {
	case p == "/healthz", strings.HasPrefix(p, "/s/"), strings.HasPrefix(p, "/api/public/"):
		return true
	case strings.HasPrefix(p, "/api/"), p == "/", p == "/index.html":
		return false
	}
	dist, _ := fs.Sub(web.Dist, "dist")
	fi, err := fs.Stat(dist, strings.TrimPrefix(path.Clean(p), "/"))
	return err == nil && !fi.IsDir()
}

func basicAuth(next http.Handler, user, pass string, public func(string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="excalidraw", charset="UTF-8"`)
			http.Error(w, "authentification requise", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		if strings.HasPrefix(r.URL.Path, "/s/") {
			h.Set("X-Robots-Tag", "noindex, nofollow") // liens de partage
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path != "/healthz" {
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
				"dur_ms", time.Since(start).Milliseconds())
		}
	})
}
