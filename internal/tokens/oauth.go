package tokens

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vblogio/excalicloud/internal/auth"
)

// Serveur d'autorisation OAuth 2.1 minimal, pour les clients MCP qui ne savent
// pas envoyer de jeton fixe (connecteurs personnalisés de claude.ai, Claude
// Desktop et mobile) :
//
//	GET  /.well-known/oauth-protected-resource[/mcp]  ressource protégée (RFC 9728)
//	GET  /.well-known/oauth-authorization-server      métadonnées (RFC 8414)
//	POST /oauth/register   enregistrement dynamique du client (RFC 7591)
//	GET  /oauth/authorize  page de consentement (session de l'interface requise)
//	POST /oauth/authorize  accord ou refus, puis redirection avec le code
//	POST /oauth/token      échange du code (PKCE S256) et rafraîchissement
//
// L'utilisateur se connecte comme d'habitude (OIDC, Basic) puis autorise le
// client : le jeton délivré est rattaché à son compte et révocable depuis les
// paramètres. Les identifiants de clients sont signés (aucun état serveur).

const (
	codeTTL    = 5 * time.Minute
	consentTTL = 10 * time.Minute
	Scope      = "mcp"
)

// OAuthPaths sont les chemins OAuth servis sans session (à déclarer publics).
var OAuthPaths = []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server", "/oauth/register", "/oauth/token"}

// IsPublic indique si p est un chemin OAuth servi sans session.
func IsPublic(p string) bool {
	for _, o := range OAuthPaths {
		if p == o || strings.HasPrefix(p, o+"/") {
			return true
		}
	}
	return false
}

type client struct {
	Name     string   `json:"n"`
	Redirect []string `json:"r"`
}

type authCode struct {
	clientID, redirect, challenge, name, client string
	userID                                      string
	user                                        *auth.User
	exp                                         time.Time
}

type codeStore struct {
	mu    sync.Mutex
	codes map[string]authCode
}

func (c *codeStore) put(code string, a authCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.codes == nil {
		c.codes = map[string]authCode{}
	}
	now := time.Now()
	for k, v := range c.codes {
		if now.After(v.exp) {
			delete(c.codes, k)
		}
	}
	c.codes[code] = a
}

func (c *codeStore) take(code string) (authCode, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.codes[code]
	delete(c.codes, code)
	return a, ok && time.Now().Before(a.exp)
}

func (s *Server) mac(parts ...string) string {
	m := hmac.New(sha256.New, s.Store.Key())
	m.Write([]byte(strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Server) encodeClient(c client) string {
	raw, _ := json.Marshal(c)
	p := base64.RawURLEncoding.EncodeToString(raw)
	return p + "." + s.mac("client", p)
}

func (s *Server) decodeClient(id string) (client, bool) {
	var c client
	p, sig, ok := strings.Cut(id, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.mac("client", p))) {
		return c, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	return c, err == nil && json.Unmarshal(raw, &c) == nil
}

func (s *Server) registerOAuth(mux *http.ServeMux) {
	resource := func(w http.ResponseWriter, r *http.Request) {
		base := s.BaseURL(r)
		res := base
		if len(s.ResourcePaths) > 0 {
			res += s.ResourcePaths[0]
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"resource":                 res,
			"authorization_servers":    []string{base},
			"scopes_supported":         []string{Scope},
			"bearer_methods_supported": []string{"header"},
			"resource_name":            "excalicloud",
		})
	}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", resource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/", resource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		base := s.BaseURL(r)
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                base,
			"authorization_endpoint":                base + "/oauth/authorize",
			"token_endpoint":                        base + "/oauth/token",
			"registration_endpoint":                 base + "/oauth/register",
			"scopes_supported":                      []string{Scope},
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"},
		})
	})
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("GET /oauth/authorize", s.authorize)
	mux.HandleFunc("POST /oauth/authorize", s.consent)
	mux.HandleFunc("POST /oauth/token", s.token)
}

func oauthErr(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// register : enregistrement dynamique (RFC 7591), client public uniquement.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"client_name"`
		Redirect []string `json:"redirect_uris"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_client_metadata", "JSON invalide")
		return
	}
	if len(req.Redirect) == 0 || len(req.Redirect) > 5 {
		oauthErr(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris requis (5 au plus)")
		return
	}
	for _, u := range req.Redirect {
		if _, ok := s.redirectHost(u); !ok {
			oauthErr(w, http.StatusBadRequest, "invalid_redirect_uri",
				"redirection non autorisée : "+u+" (voir OAUTH_REDIRECT_HOSTS)")
			return
		}
	}
	name, err := cleanName(req.Name)
	if err != nil {
		name = "Client MCP"
	}
	id := s.encodeClient(client{Name: name, Redirect: req.Redirect})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  id,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                name,
		"redirect_uris":              req.Redirect,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      Scope,
	})
}

type authReq struct {
	ClientID, Redirect, Challenge, State string
	client                               client
	host                                 string
}

// parseAuthorize valide les paramètres de la demande d'autorisation. Une
// erreur sur le client ou la redirection s'affiche (pas de redirection vers
// une adresse non vérifiée) ; les autres sont renvoyées au client.
func (s *Server) parseAuthorize(w http.ResponseWriter, r *http.Request, v url.Values) (authReq, bool) {
	a := authReq{ClientID: v.Get("client_id"), Redirect: v.Get("redirect_uri"), Challenge: v.Get("code_challenge"), State: v.Get("state")}
	c, ok := s.decodeClient(a.ClientID)
	if !ok {
		page(w, http.StatusBadRequest, "Client inconnu", "Identifiant de client OAuth invalide.")
		return a, false
	}
	a.client = c
	if a.Redirect == "" && len(c.Redirect) == 1 {
		a.Redirect = c.Redirect[0]
	}
	match := false
	for _, reg := range c.Redirect {
		match = match || sameRedirect(reg, a.Redirect)
	}
	if a.host, ok = s.redirectHost(a.Redirect); !match || !ok {
		page(w, http.StatusBadRequest, "Redirection refusée", "L'adresse de retour ne correspond pas au client enregistré.")
		return a, false
	}
	switch {
	case v.Get("response_type") != "code":
		s.redirectErr(w, r, a, "unsupported_response_type")
	case len(a.Challenge) < 43 || v.Get("code_challenge_method") != "S256":
		s.redirectErr(w, r, a, "invalid_request")
	default:
		return a, true
	}
	return a, false
}

func (s *Server) redirectErr(w http.ResponseWriter, r *http.Request, a authReq, code string) {
	u, _ := url.Parse(a.Redirect)
	q := u.Query()
	q.Set("error", code)
	if a.State != "" {
		q.Set("state", a.State)
	}
	q.Set("iss", s.BaseURL(r))
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// csrf lie le formulaire de consentement à l'utilisateur et à la demande.
func (s *Server) csrf(userID string, a authReq, exp int64) string {
	return s.mac("consent", userID, a.ClientID, a.Redirect, a.Challenge, a.State, strconv.FormatInt(exp, 10))
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	if FromContext(r.Context()) != nil {
		page(w, http.StatusForbidden, "Accès refusé", "Une autorisation se donne depuis le navigateur.")
		return
	}
	a, ok := s.parseAuthorize(w, r, r.URL.Query())
	if !ok {
		return
	}
	who := "l'utilisateur local"
	if u := auth.UserFrom(r.Context()); u != nil {
		who = u.Name
		if u.Email != "" {
			who += " (" + u.Email + ")"
		}
	}
	exp := time.Now().Add(consentTTL).Unix()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = consentPage.Execute(w, map[string]any{
		"Client": a.client.Name, "Host": a.host, "Who": who, "Req": a,
		"Exp": exp, "CSRF": s.csrf(s.UserID(r), a, exp),
	})
}

func (s *Server) consent(w http.ResponseWriter, r *http.Request) {
	if FromContext(r.Context()) != nil {
		page(w, http.StatusForbidden, "Accès refusé", "Une autorisation se donne depuis le navigateur.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if r.ParseForm() != nil {
		page(w, http.StatusBadRequest, "Requête invalide", "Formulaire illisible.")
		return
	}
	a, ok := s.parseAuthorize(w, r, r.PostForm)
	if !ok {
		return
	}
	exp, _ := strconv.ParseInt(r.PostForm.Get("exp"), 10, 64)
	userID := s.UserID(r)
	if time.Now().Unix() > exp || !hmac.Equal([]byte(r.PostForm.Get("csrf")), []byte(s.csrf(userID, a, exp))) {
		page(w, http.StatusBadRequest, "Demande expirée", "Relancer la connexion depuis le client.")
		return
	}
	if r.PostForm.Get("action") != "allow" {
		s.redirectErr(w, r, a, "access_denied")
		return
	}
	code := random(32)
	s.codes.put(code, authCode{
		clientID: a.ClientID, redirect: a.Redirect, challenge: a.Challenge, name: a.client.Name, client: a.host,
		userID: userID, user: auth.UserFrom(r.Context()), exp: time.Now().Add(codeTTL),
	})
	u, _ := url.Parse(a.Redirect)
	q := u.Query()
	q.Set("code", code)
	if a.State != "" {
		q.Set("state", a.State)
	}
	q.Set("iss", s.BaseURL(r))
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if r.ParseForm() != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_request", "formulaire illisible")
		return
	}
	f := r.PostForm
	clientID := f.Get("client_id")
	if _, ok := s.decodeClient(clientID); !ok {
		oauthErr(w, http.StatusUnauthorized, "invalid_client", "client inconnu")
		return
	}
	var access, refresh string
	var err error
	switch f.Get("grant_type") {
	case "authorization_code":
		c, ok := s.codes.take(f.Get("code"))
		sum := sha256.Sum256([]byte(f.Get("code_verifier")))
		verifier := base64.RawURLEncoding.EncodeToString(sum[:])
		if !ok || c.clientID != clientID ||
			(f.Get("redirect_uri") != "" && f.Get("redirect_uri") != c.redirect) ||
			subtle.ConstantTimeCompare([]byte(verifier), []byte(c.challenge)) != 1 {
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "code invalide, expiré ou vérificateur PKCE incorrect")
			return
		}
		access, refresh, err = s.Store.Grant(c.userID, c.user, c.name, clientID, c.client)
	case "refresh_token":
		access, refresh, err = s.Store.Refresh(f.Get("refresh_token"), clientID)
		if err == ErrNotFound {
			oauthErr(w, http.StatusBadRequest, "invalid_grant", "jeton de rafraîchissement invalide ou révoqué")
			return
		}
	default:
		oauthErr(w, http.StatusBadRequest, "unsupported_grant_type", "authorization_code ou refresh_token")
		return
	}
	if err != nil {
		oauthErr(w, http.StatusInternalServerError, "server_error", "enregistrement du jeton impossible")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(AccessTTL.Seconds()),
		"refresh_token": refresh,
		"scope":         Scope,
	})
}

var consentPage = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="fr"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Autoriser {{.Client}}</title>
<style>
body{font-family:system-ui,sans-serif;max-width:28rem;margin:4rem auto;padding:0 1rem;color:#1b1b1f;background:#fff}
@media(prefers-color-scheme:dark){body{color:#e3e3e8;background:#121212}}
h1{font-size:1.3rem}.host{font-family:monospace}button{font:inherit;padding:.5rem 1.2rem;border-radius:.5rem;border:1px solid #888;cursor:pointer;margin-right:.5rem}
.allow{background:#6965db;color:#fff;border-color:#6965db}small{opacity:.7}
</style>
<h1>Autoriser « {{.Client}} » ?</h1>
<p>Ce client demande l'accès à vos schémas excalicloud en tant que <b>{{.Who}}</b> : lister, lire, créer et modifier des schémas et des dossiers, y compris votre espace personnel.</p>
<p>Retour vers : <span class="host">{{.Host}}</span></p>
<form method="post" action="/oauth/authorize">
<input type="hidden" name="response_type" value="code">
<input type="hidden" name="code_challenge_method" value="S256">
<input type="hidden" name="client_id" value="{{.Req.ClientID}}">
<input type="hidden" name="redirect_uri" value="{{.Req.Redirect}}">
<input type="hidden" name="code_challenge" value="{{.Req.Challenge}}">
<input type="hidden" name="state" value="{{.Req.State}}">
<input type="hidden" name="exp" value="{{.Exp}}">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<button class="allow" name="action" value="allow" autofocus>Autoriser</button>
<button name="action" value="deny">Refuser</button>
</form>
<p><small>L'accès se révoque dans Paramètres › Accès API et MCP.</small></p>
</html>`))

func page(w http.ResponseWriter, status int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = simplePage.Execute(w, map[string]string{"Title": title, "Msg": msg})
}

var simplePage = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="fr"><meta charset="utf-8"><title>{{.Title}}</title>
<body style="font-family:system-ui,sans-serif;max-width:28rem;margin:4rem auto;padding:0 1rem">
<h1 style="font-size:1.3rem">{{.Title}}</h1><p>{{.Msg}}</p></body></html>`))
