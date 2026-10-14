package tokens

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/vblogio/excalidraw-go/internal/auth"
)

// Server expose les jetons en HTTP : authentification Bearer, gestion des
// jetons personnels (/api/tokens) et serveur d'autorisation OAuth (oauth.go).
type Server struct {
	Store *Store
	// PublicURL est l'origine publique (PUBLIC_URL) ; vide : déduite de la
	// requête (en-têtes X-Forwarded-Proto et X-Forwarded-Host compris).
	PublicURL string
	// UserID identifie l'utilisateur d'une requête authentifiée (session ou jeton).
	UserID func(*http.Request) string
	// RedirectHosts : hôtes autorisés pour les redirections OAuth en https
	// (les adresses de bouclage en http le sont toujours, pour Claude Code).
	RedirectHosts []string
	// ResourcePaths : chemins réservés aux jetons (serveur MCP).
	ResourcePaths []string

	codes codeStore
}

type tokenKey struct{}

// FromContext renvoie le jeton qui a authentifié la requête (nil : session).
func FromContext(ctx context.Context) *Token {
	t, _ := ctx.Value(tokenKey{}).(*Token)
	return t
}

// BaseURL renvoie l'origine publique du service.
func (s *Server) BaseURL(r *http.Request) string {
	if s.PublicURL != "" {
		return strings.TrimRight(s.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host, _, _ = strings.Cut(h, ",")
		host = strings.TrimSpace(host)
	}
	return scheme + "://" + host
}

func (s *Server) resource(p string) bool {
	for _, r := range s.ResourcePaths {
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// metadataURL est l'adresse des métadonnées de ressource protégée (RFC 9728).
func (s *Server) metadataURL(r *http.Request) string {
	return s.BaseURL(r) + "/.well-known/oauth-protected-resource"
}

func bearer(r *http.Request) (string, bool) {
	scheme, tok, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return strings.TrimSpace(tok), true
}

// Middleware authentifie les requêtes porteuses d'un jeton (Authorization:
// Bearer) et les transmet directement à mux avec l'utilisateur du jeton ; les
// autres suivent next (session OIDC, Basic…). Les chemins ResourcePaths
// exigent un jeton : sans jeton, la réponse 401 indique où obtenir un jeton
// OAuth (WWW-Authenticate, RFC 9728).
func (s *Server) Middleware(next, mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := bearer(r)
		if !ok && !s.resource(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		meta := `Bearer resource_metadata="` + s.metadataURL(r) + `"`
		t, valid := s.Store.Lookup(tok)
		if !valid {
			if ok {
				meta += `, error="invalid_token"`
			}
			w.Header().Set("WWW-Authenticate", meta)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "jeton d'accès absent, invalide ou expiré"})
			return
		}
		ctx := context.WithValue(r.Context(), tokenKey{}, &t)
		if t.User != nil {
			ctx = auth.WithUser(ctx, t.User)
		}
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Register enregistre les routes des jetons et du serveur OAuth.
//
//	GET    /api/tokens         jetons de l'utilisateur
//	POST   /api/tokens {"name"} nouveau jeton personnel (renvoyé une seule fois)
//	DELETE /api/tokens/{id}    révocation
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tokens", s.session(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Store.List(s.UserID(r)))
	}))
	mux.HandleFunc("POST /api/tokens", s.session(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "requête JSON invalide"})
			return
		}
		secret, info, err := s.Store.Create(s.UserID(r), auth.UserFrom(r.Context()), req.Name)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrInvalid) || errors.Is(err, ErrTooMany) {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, struct {
			Info
			Token string `json:"token"`
		}{info, secret})
	}))
	mux.HandleFunc("DELETE /api/tokens/{id}", s.session(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Revoke(s.UserID(r), r.PathValue("id")); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrNotFound) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	s.registerOAuth(mux)
}

// session refuse les requêtes authentifiées par un jeton : un jeton ne peut
// pas créer, lister ni révoquer de jetons.
func (s *Server) session(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "gestion des jetons réservée à la session de l'interface"})
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// redirectHost renvoie l'hôte d'une URI de redirection acceptable : https vers
// un hôte autorisé, ou http vers une adresse de bouclage (tout port).
func (s *Server) redirectHost(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	switch u.Scheme {
	case "http":
		return host, loopback(host)
	case "https":
		for _, h := range s.RedirectHosts {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" && (host == h || strings.HasSuffix(host, "."+h)) {
				return host, true
			}
		}
	}
	return "", false
}

func loopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// sameRedirect compare deux URI de redirection ; le port d'une adresse de
// bouclage est ignoré (RFC 8252 §7.3 : port éphémère choisi par le client).
func sameRedirect(registered, got string) bool {
	if registered == got {
		return true
	}
	a, err1 := url.Parse(registered)
	b, err2 := url.Parse(got)
	if err1 != nil || err2 != nil || a.Scheme != "http" || b.Scheme != "http" || !loopback(a.Hostname()) {
		return false
	}
	return a.Hostname() == b.Hostname() && a.Path == b.Path && a.RawQuery == b.RawQuery
}
