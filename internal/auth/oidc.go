package auth

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "excalicloud_session"
	stateCookie   = "excalicloud_oidc"
	stateTTL      = 10 * time.Minute

	// LoginHeader accompagne les réponses 401 de l'API : le frontend y lit
	// l'adresse de connexion.
	LoginHeader = "X-Auth-Login"
)

// Config regroupe les réglages OIDC (variables OIDC_* et SESSION_*).
type Config struct {
	Issuer       string   // OIDC_ISSUER
	ClientID     string   // OIDC_CLIENT_ID
	ClientSecret string   // OIDC_CLIENT_SECRET (vide : client public, PKCE seul)
	RedirectURL  string   // OIDC_REDIRECT_URL (vide : https://<hôte>/auth/callback)
	Scopes       []string // OIDC_SCOPES
	// AllowedGroups restreint l'accès aux membres d'au moins un de ces groupes
	// (claim GroupsClaim) ; vide : tout utilisateur authentifié.
	AllowedGroups []string
	GroupsClaim   string // OIDC_GROUPS_CLAIM, « groups » par défaut
	EndSession    bool   // OIDC_LOGOUT : déconnexion aussi chez le fournisseur
	SessionTTL    time.Duration
	Secret        []byte // clé HMAC des cookies
	Client        *http.Client
}

// discovery est la partie utile de /.well-known/openid-configuration.
type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// OIDC authentifie les utilisateurs auprès d'un fournisseur OpenID Connect
// (Authentik, Keycloak, Entra ID, Google, Dex...).
type OIDC struct {
	cfg    Config
	signer signer

	mu      sync.Mutex
	disc    *discovery // nil : pas encore découvert
	keys    map[string]crypto.PublicKey
	keysAt  time.Time
	discErr time.Time // dernier échec de découverte (limite les tentatives)
}

// NewOIDC prépare l'authentification ; la découverte du fournisseur est
// tentée tout de suite puis, en cas d'échec, à la première connexion.
func NewOIDC(ctx context.Context, cfg Config) (*OIDC, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return nil, errors.New("OIDC_ISSUER et OIDC_CLIENT_ID sont requis en mode oidc")
	}
	if len(cfg.Secret) < 16 {
		return nil, errors.New("clé de session trop courte")
	}
	if cfg.RedirectURL != "" {
		u, err := url.Parse(cfg.RedirectURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, errors.New("OIDC_REDIRECT_URL invalide (URL absolue attendue)")
		}
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}
	if !slices.Contains(cfg.Scopes, "openid") {
		cfg.Scopes = append([]string{"openid"}, cfg.Scopes...)
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 15 * time.Second}
	}
	a := &OIDC{cfg: cfg, signer: signer{cfg.Secret}}
	if _, err := a.provider(ctx); err != nil {
		slog.Warn("fournisseur OIDC injoignable, nouvel essai à la première connexion", "issuer", cfg.Issuer, "err", err)
	}
	return a, nil
}

func (a *OIDC) getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := a.cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s : HTTP %d", u, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(v)
}

// provider renvoie la configuration découverte (mise en cache).
func (a *OIDC) provider(ctx context.Context) (*discovery, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.disc != nil {
		return a.disc, nil
	}
	if time.Since(a.discErr) < 5*time.Second {
		return nil, errors.New("fournisseur OIDC injoignable")
	}
	var d discovery
	u := strings.TrimRight(a.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	if err := a.getJSON(ctx, u, &d); err != nil {
		a.discErr = time.Now()
		return nil, err
	}
	if strings.TrimRight(d.Issuer, "/") != strings.TrimRight(a.cfg.Issuer, "/") {
		a.discErr = time.Now()
		return nil, fmt.Errorf("l'émetteur annoncé (%s) ne correspond pas à OIDC_ISSUER", d.Issuer)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		a.discErr = time.Now()
		return nil, errors.New("configuration OIDC incomplète (authorization, token ou jwks absent)")
	}
	a.disc = &d
	slog.Info("fournisseur OIDC découvert", "issuer", d.Issuer)
	return a.disc, nil
}

// key renvoie la clé publique kid, en rechargeant le JWKS si elle est
// inconnue (rotation de clés), au plus une fois par minute.
func (a *OIDC) key(ctx context.Context, d *discovery, kid, alg string) (crypto.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	find := func() crypto.PublicKey {
		if k, ok := a.keys[kid]; ok {
			return k
		}
		if kid == "" && len(a.keys) == 1 {
			for _, k := range a.keys {
				return k
			}
		}
		return nil
	}
	if k := find(); k != nil {
		return k, nil
	}
	if time.Since(a.keysAt) < time.Minute && a.keys != nil {
		return nil, fmt.Errorf("clé %q inconnue", kid)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := a.getJSON(ctx, d.JWKSURI, &set); err != nil {
		return nil, fmt.Errorf("JWKS : %w", err)
	}
	a.keys = map[string]crypto.PublicKey{}
	a.keysAt = time.Now()
	for _, j := range set.Keys {
		if j.Use != "" && j.Use != "sig" {
			continue
		}
		if pk, err := j.publicKey(); err == nil {
			a.keys[j.Kid] = pk
		}
	}
	if k := find(); k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("clé %q absente du JWKS (alg %s)", kid, alg)
}

// --- Routes /auth/* -----------------------------------------------------------

// Register ajoute /auth/login, /auth/callback, /auth/logout et /auth/logged-out.
func (a *OIDC) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.HandleFunc("GET /auth/logout", a.logout)
	mux.HandleFunc("POST /auth/logout", a.logout)
	mux.HandleFunc("GET /auth/logged-out", func(w http.ResponseWriter, r *http.Request) {
		page(w, http.StatusOK, "Déconnecté", "Vous êtes déconnecté.", "/", "Se reconnecter")
	})
}

func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// baseURL est l'origine publique du service : celle de OIDC_REDIRECT_URL si
// elle est fixée, sinon celle de la requête (en-têtes X-Forwarded-* compris).
func (a *OIDC) baseURL(r *http.Request) string {
	if a.cfg.RedirectURL != "" {
		u, _ := url.Parse(a.cfg.RedirectURL)
		return u.Scheme + "://" + u.Host
	}
	scheme := "http"
	if secure(r) {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host, _, _ = strings.Cut(h, ",")
		host = strings.TrimSpace(host)
	}
	return scheme + "://" + host
}

func (a *OIDC) redirectURL(r *http.Request) string {
	if a.cfg.RedirectURL != "" {
		return a.cfg.RedirectURL
	}
	return a.baseURL(r) + "/auth/callback"
}

func (a *OIDC) setCookie(w http.ResponseWriter, r *http.Request, name, value, path string, ttl time.Duration) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		Secure:   secure(r) || strings.HasPrefix(a.cfg.RedirectURL, "https://"),
		SameSite: http.SameSiteLaxMode, // envoyé au retour du fournisseur (navigation GET)
	}
	if ttl < 0 {
		c.MaxAge = -1
	} else {
		c.Expires = now().Add(ttl)
		c.MaxAge = int(ttl / time.Second)
	}
	http.SetCookie(w, c)
}

// safeNext n'accepte que les chemins locaux (pas de redirection ouverte).
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") ||
		strings.HasPrefix(next, "/auth/") || strings.ContainsAny(next, "\r\n") {
		return "/"
	}
	return next
}

func (a *OIDC) login(w http.ResponseWriter, r *http.Request) {
	d, err := a.provider(r.Context())
	if err != nil {
		slog.Error("OIDC : découverte", "err", err)
		page(w, http.StatusBadGateway, "Connexion impossible",
			"Le fournisseur d'identité est injoignable pour l'instant.", r.URL.RequestURI(), "Réessayer")
		return
	}
	st := loginState{
		State:    randomString(24),
		Nonce:    randomString(24),
		Verifier: randomString(48), // 64 caractères (RFC 7636 : 43 à 128)
		Next:     safeNext(r.URL.Query().Get("next")),
		Exp:      now().Add(stateTTL).Unix(),
	}
	v, err := a.signer.encode(st)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.setCookie(w, r, stateCookie, v, "/auth/", stateTTL)

	challenge := sha256.Sum256([]byte(st.Verifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {a.cfg.ClientID},
		"redirect_uri":          {a.redirectURL(r)},
		"scope":                 {strings.Join(a.cfg.Scopes, " ")},
		"state":                 {st.State},
		"nonce":                 {st.Nonce},
		"code_challenge":        {b64.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	sep := "?"
	if strings.Contains(d.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, d.AuthorizationEndpoint+sep+q.Encode(), http.StatusFound)
}

func (a *OIDC) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	retry := "/auth/login"
	var st loginState
	c, err := r.Cookie(stateCookie)
	if err == nil {
		err = a.signer.decode(c.Value, &st)
	}
	if err == nil && st.Next != "/" {
		retry += "?next=" + url.QueryEscape(st.Next)
	}
	a.setCookie(w, r, stateCookie, "", "/auth/", -1) // usage unique
	if e := q.Get("error"); e != "" {
		slog.Warn("OIDC : refus du fournisseur", "error", e, "description", q.Get("error_description"))
		page(w, http.StatusForbidden, "Connexion refusée", "Le fournisseur d'identité a refusé la connexion ("+e+").", retry, "Réessayer")
		return
	}
	if err != nil || st.Exp < now().Unix() || st.State == "" || q.Get("state") != st.State {
		page(w, http.StatusBadRequest, "Connexion expirée",
			"La demande de connexion est expirée ou invalide (cookies bloqués ?).", retry, "Recommencer")
		return
	}
	user, err := a.exchange(r, q.Get("code"), st)
	if errors.Is(err, errForbidden) {
		slog.Warn("OIDC : accès refusé", "sub", user.Sub, "name", user.Name)
		page(w, http.StatusForbidden, "Accès refusé",
			"Votre compte ("+user.Name+") n'appartient à aucun groupe autorisé.", "/auth/logout", "Changer de compte")
		return
	}
	if err != nil {
		slog.Error("OIDC : échange du code", "err", err)
		page(w, http.StatusBadGateway, "Connexion impossible", "La vérification de l'identité a échoué.", retry, "Réessayer")
		return
	}
	if err := a.startSession(w, r, user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("connexion OIDC", "sub", user.Sub, "name", user.Name)
	http.Redirect(w, r, st.Next, http.StatusFound)
}

var errForbidden = errors.New("utilisateur hors des groupes autorisés")

// exchange échange le code contre les jetons, vérifie l'ID Token et en tire
// l'utilisateur.
func (a *OIDC) exchange(r *http.Request, code string, st loginState) (User, error) {
	ctx := r.Context()
	d, err := a.provider(ctx)
	if err != nil {
		return User{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {a.redirectURL(r)},
		"code_verifier": {st.Verifier},
		"client_id":     {a.cfg.ClientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if a.cfg.ClientSecret != "" { // client_secret_basic (RFC 6749 2.3.1)
		req.SetBasicAuth(url.QueryEscape(a.cfg.ClientID), url.QueryEscape(a.cfg.ClientSecret))
	}
	res, err := a.cfg.Client.Do(req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	var tok struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
		Desc    string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil {
		return User{}, fmt.Errorf("réponse du token endpoint (HTTP %d) : %w", res.StatusCode, err)
	}
	if res.StatusCode != http.StatusOK || tok.IDToken == "" {
		return User{}, fmt.Errorf("token endpoint : HTTP %d %s %s", res.StatusCode, tok.Error, tok.Desc)
	}
	return a.verifyIDToken(ctx, d, tok.IDToken, st.Nonce)
}

func (a *OIDC) verifyIDToken(ctx context.Context, d *discovery, raw, nonce string) (User, error) {
	h, body, signed, sig, err := parseJWT(raw)
	if err != nil {
		return User{}, err
	}
	var key crypto.PublicKey
	if !strings.HasPrefix(h.Alg, "HS") {
		if key, err = a.key(ctx, d, h.Kid, h.Alg); err != nil {
			return User{}, err
		}
	}
	if err := verifySignature(h.Alg, key, []byte(a.cfg.ClientSecret), signed, sig); err != nil {
		return User{}, err
	}
	var c idClaims
	if err := json.Unmarshal(body, &c); err != nil {
		return User{}, errors.New("claims illisibles")
	}
	if err := c.validate(d.Issuer, a.cfg.ClientID, nonce, now()); err != nil {
		return User{}, err
	}
	u := User{Sub: c.Sub, Email: c.Email, Name: c.PreferredUsername}
	for _, n := range []string{c.Name, c.Email, c.Sub} {
		if u.Name == "" {
			u.Name = n
		}
	}
	if len(a.cfg.AllowedGroups) > 0 {
		var all map[string]any
		_ = json.Unmarshal(body, &all)
		groups := stringList(claimPath(all, a.cfg.GroupsClaim))
		ok := slices.ContainsFunc(groups, func(g string) bool {
			// Keycloak préfixe les groupes par « / » quand le chemin complet est inclus.
			return slices.Contains(a.cfg.AllowedGroups, g) || slices.Contains(a.cfg.AllowedGroups, strings.TrimPrefix(g, "/"))
		})
		if !ok {
			return u, errForbidden
		}
	}
	return u, nil
}

func (a *OIDC) startSession(w http.ResponseWriter, r *http.Request, u User) error {
	v, err := a.signer.encode(session{User: u, Exp: now().Add(a.cfg.SessionTTL).Unix()})
	if err != nil {
		return err
	}
	a.setCookie(w, r, sessionCookie, v, "/", a.cfg.SessionTTL)
	return nil
}

func (a *OIDC) logout(w http.ResponseWriter, r *http.Request) {
	a.setCookie(w, r, sessionCookie, "", "/", -1)
	w.Header().Set("Cache-Control", "no-store")
	back := a.baseURL(r) + "/auth/logged-out"
	if a.cfg.EndSession {
		if d, err := a.provider(r.Context()); err == nil && d.EndSessionEndpoint != "" {
			// RP-Initiated Logout 1.0 : client_id identifie le client sans id_token_hint.
			q := url.Values{"client_id": {a.cfg.ClientID}, "post_logout_redirect_uri": {back}}
			sep := "?"
			if strings.Contains(d.EndSessionEndpoint, "?") {
				sep = "&"
			}
			http.Redirect(w, r, d.EndSessionEndpoint+sep+q.Encode(), http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, back, http.StatusFound)
}

// --- Protection des routes -------------------------------------------------------

type ctxKey struct{}

// UserFrom renvoie l'utilisateur de la requête (nil hors session OIDC).
func UserFrom(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

// WithUser renvoie ctx portant l'utilisateur u (authentification par jeton d'API).
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// Session renvoie l'utilisateur du cookie de session s'il est valide.
func (a *OIDC) session(r *http.Request) (session, bool) {
	var s session
	c, err := r.Cookie(sessionCookie)
	if err != nil || a.signer.decode(c.Value, &s) != nil || s.Exp < now().Unix() || s.Sub == "" {
		return s, false
	}
	return s, true
}

// Middleware exige une session sauf pour /auth/* et les chemins publics.
// Navigation sans session : page qui renvoie vers /auth/login en conservant
// l'URL complète (fragment #/dossier/schéma compris). API : 401 + LoginHeader.
// La session est prolongée à chaque requête passée la moitié de sa durée.
func (a *OIDC) Middleware(next http.Handler, public func(string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/auth/") || public(p) {
			next.ServeHTTP(w, r)
			return
		}
		s, ok := a.session(r)
		if !ok {
			if strings.HasPrefix(p, "/api/") || r.Method != http.MethodGet {
				w.Header().Set(LoginHeader, "/auth/login")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"session expirée : reconnexion nécessaire"}`))
				return
			}
			loginRedirect(w)
			return
		}
		if time.Until(time.Unix(s.Exp, 0)) < a.cfg.SessionTTL/2 {
			_ = a.startSession(w, r, s.User)
		}
		u := s.User
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, &u)))
	})
}

// loginRedirect renvoie vers /auth/login par script pour conserver le
// fragment de l'URL, que le navigateur n'envoie pas au serveur.
func loginRedirect(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>Connexion</title>
<script>location.replace("/auth/login?next="+encodeURIComponent(location.pathname+location.search+location.hash))</script>
<noscript><a href="/auth/login">Se connecter</a></noscript>`)
}

// page affiche un message simple (erreur de connexion, déconnexion).
func page(w http.ResponseWriter, status int, title, msg, link, label string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	e := html.EscapeString
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="fr"><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1"><title>%s</title>
<style>body{font:16px system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0;color:#1b1b1f;background:#fff}
@media(prefers-color-scheme:dark){body{color:#e3e3e8;background:#121212}}main{max-width:28rem;padding:1rem;text-align:center}
a{display:inline-block;margin-top:1rem;padding:.6rem 1.2rem;border-radius:8px;background:#6965db;color:#fff;text-decoration:none}</style>
<main><h1>%s</h1><p>%s</p><a href="%s">%s</a></main></html>`, e(title), e(title), e(msg), e(link), e(label))
}

// Me décrit la session courante pour le frontend.
type Me struct {
	Mode   string `json:"mode"` // none, local ou oidc
	User   *User  `json:"user,omitempty"`
	Logout string `json:"logout,omitempty"` // URL de déconnexion, si disponible
}
