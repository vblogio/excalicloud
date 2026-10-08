package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeIdP est un fournisseur OIDC minimal : découverte, autorisation (connexion
// immédiate), jetons, JWKS et déconnexion.
type fakeIdP struct {
	t      *testing.T
	srv    *httptest.Server
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
	alg    string // RS256, ES256 ou HS256
	secret string
	groups []string

	codes map[string]url.Values // code -> paramètres de la demande d'autorisation
}

func newIdP(t *testing.T, alg string) *fakeIdP {
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p := &fakeIdP{t: t, rsaKey: rk, ecKey: ek, alg: alg, secret: "secret-client-assez-long", codes: map[string]url.Values{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 p.srv.URL,
			"authorization_endpoint": p.srv.URL + "/authorize",
			"token_endpoint":         p.srv.URL + "/token",
			"jwks_uri":               p.srv.URL + "/jwks",
			"end_session_endpoint":   p.srv.URL + "/logout",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		e := big.NewInt(int64(rk.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": "r1", "use": "sig", "n": b64.EncodeToString(rk.N.Bytes()), "e": b64.EncodeToString(e)},
			{"kty": "EC", "kid": "e1", "crv": "P-256", "x": b64.EncodeToString(ek.X.FillBytes(make([]byte, 32))), "y": b64.EncodeToString(ek.Y.FillBytes(make([]byte, 32)))},
		}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := randomString(8)
		p.codes[code] = q
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		q, ok := p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		id, sec, _ := r.BasicAuth()
		ch := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		switch {
		case !ok, id != "excalicloud", sec != p.secret,
			r.Form.Get("redirect_uri") != q.Get("redirect_uri"),
			q.Get("code_challenge_method") != "S256", b64.EncodeToString(ch[:]) != q.Get("code_challenge"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": p.idToken(q.Get("nonce")), "token_type": "Bearer"})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeIdP) idToken(nonce string) string {
	claims := map[string]any{
		"iss": p.srv.URL, "sub": "u-42", "aud": "excalicloud", "nonce": nonce,
		"exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(),
		"preferred_username": "cedric", "email": "cedric@example.org",
	}
	if p.groups != nil {
		claims["groups"] = p.groups
	}
	return p.sign(claims)
}

func (p *fakeIdP) sign(claims map[string]any) string {
	hdr := map[string]string{"alg": p.alg, "typ": "JWT"}
	switch p.alg {
	case "RS256":
		hdr["kid"] = "r1"
	case "ES256":
		hdr["kid"] = "e1"
	}
	hb, _ := json.Marshal(hdr)
	cb, _ := json.Marshal(claims)
	signed := b64.EncodeToString(hb) + "." + b64.EncodeToString(cb)
	d := sha256.Sum256([]byte(signed))
	var sig []byte
	switch p.alg {
	case "RS256":
		sig, _ = rsa.SignPKCS1v15(rand.Reader, p.rsaKey, crypto.SHA256, d[:])
	case "ES256":
		r, s, _ := ecdsa.Sign(rand.Reader, p.ecKey, d[:])
		sig = append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	case "HS256":
		m := hmac.New(sha256.New, []byte(p.secret))
		m.Write([]byte(signed))
		sig = m.Sum(nil)
	}
	return signed + "." + b64.EncodeToString(sig)
}

// app démarre excalicloud (routes /auth/*, une API et une page protégées, un
// chemin public) avec l'authentification OIDC.
func app(t *testing.T, idp *fakeIdP, groups []string) (*httptest.Server, *http.Client) {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	o, err := NewOIDC(context.Background(), Config{
		Issuer: idp.srv.URL, ClientID: "excalicloud", ClientSecret: idp.secret,
		AllowedGroups: groups, EndSession: true, Secret: []byte("0123456789abcdef0123456789abcdef"),
	})
	if err != nil {
		t.Fatal(err)
	}
	o.Register(mux)
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(UserFrom(r.Context()))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("app")) })
	srv = httptest.NewServer(o.Middleware(mux, func(p string) bool { return strings.HasPrefix(p, "/s/") }))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return srv, &http.Client{Jar: jar}
}

func get(t *testing.T, c *http.Client, u string) (*http.Response, string) {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestLoginFlow(t *testing.T) {
	for _, alg := range []string{"RS256", "ES256", "HS256"} {
		t.Run(alg, func(t *testing.T) {
			idp := newIdP(t, alg)
			srv, c := app(t, idp, nil)

			// Sans session : 401 avec l'adresse de connexion pour l'API...
			res, _ := get(t, c, srv.URL+"/api/me")
			if res.StatusCode != 401 || res.Header.Get(LoginHeader) != "/auth/login" {
				t.Fatalf("API sans session : %d %q", res.StatusCode, res.Header.Get(LoginHeader))
			}
			// ... page de redirection (fragment conservé) pour la navigation.
			res, body := get(t, c, srv.URL+"/")
			if res.StatusCode != 401 || !strings.Contains(body, "location.hash") {
				t.Fatalf("navigation sans session : %d", res.StatusCode)
			}
			// Chemin public accessible.
			if res, _ := get(t, c, srv.URL+"/s/x.y"); res.StatusCode != 200 {
				t.Fatalf("chemin public : %d", res.StatusCode)
			}

			// Connexion complète : login -> fournisseur -> callback -> next.
			res, body = get(t, c, srv.URL+"/auth/login?next="+url.QueryEscape("/dossier?x=1"))
			if res.StatusCode != 200 || body != "app" || res.Request.URL.RequestURI() != "/dossier?x=1" {
				t.Fatalf("après connexion : %d %q %s", res.StatusCode, body, res.Request.URL)
			}
			res, body = get(t, c, srv.URL+"/api/me")
			if res.StatusCode != 200 || !strings.Contains(body, `"name":"cedric"`) || !strings.Contains(body, `"sub":"u-42"`) {
				t.Fatalf("/api/me : %d %s", res.StatusCode, body)
			}

			// Déconnexion : cookie effacé, renvoi vers end_session_endpoint.
			nc := &http.Client{Jar: c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			res, _ = get(t, nc, srv.URL+"/auth/logout")
			loc, _ := url.Parse(res.Header.Get("Location"))
			if res.StatusCode != 302 || loc.Path != "/logout" || loc.Query().Get("client_id") != "excalicloud" ||
				loc.Query().Get("post_logout_redirect_uri") != srv.URL+"/auth/logged-out" {
				t.Fatalf("logout : %d %s", res.StatusCode, loc)
			}
			if res, _ := get(t, c, srv.URL+"/api/me"); res.StatusCode != 401 {
				t.Fatalf("après déconnexion : %d", res.StatusCode)
			}
		})
	}
}

func TestAllowedGroups(t *testing.T) {
	idp := newIdP(t, "RS256")
	idp.groups = []string{"/excalicloud-users"} // chemin complet Keycloak
	srv, c := app(t, idp, []string{"excalicloud-users"})
	if res, body := get(t, c, srv.URL+"/auth/login"); res.StatusCode != 200 || body != "app" {
		t.Fatalf("membre du groupe : %d %q", res.StatusCode, body)
	}

	idp2 := newIdP(t, "RS256")
	idp2.groups = []string{"autres"}
	srv2, c2 := app(t, idp2, []string{"excalicloud-users"})
	if res, _ := get(t, c2, srv2.URL+"/auth/login"); res.StatusCode != 403 {
		t.Fatalf("hors groupe : %d", res.StatusCode)
	}
	if res, _ := get(t, c2, srv2.URL+"/api/me"); res.StatusCode != 401 {
		t.Fatalf("hors groupe, pas de session attendue : %d", res.StatusCode)
	}
}

func TestCallbackRejectsForgedState(t *testing.T) {
	idp := newIdP(t, "RS256")
	srv, c := app(t, idp, nil)
	// Callback sans cookie d'état (CSRF de connexion) : refusé.
	if res, _ := get(t, c, srv.URL+"/auth/callback?code=x&state=y"); res.StatusCode != 400 {
		t.Fatalf("callback forgé : %d", res.StatusCode)
	}
}

func TestVerifyIDToken(t *testing.T) {
	idp := newIdP(t, "RS256")
	o, err := NewOIDC(context.Background(), Config{Issuer: idp.srv.URL, ClientID: "excalicloud", Secret: []byte("0123456789abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	d, err := o.provider(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	base := func() map[string]any {
		return map[string]any{"iss": idp.srv.URL, "sub": "s", "aud": []string{"excalicloud"}, "nonce": "n",
			"exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix()}
	}
	if _, err := o.verifyIDToken(context.Background(), d, idp.sign(base()), "n"); err != nil {
		t.Fatalf("jeton valide refusé : %v", err)
	}
	bad := map[string]func(map[string]any){
		"émetteur": func(c map[string]any) { c["iss"] = "https://ailleurs" },
		"audience": func(c map[string]any) { c["aud"] = "autre" },
		"expiré":   func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"nonce":    func(c map[string]any) { c["nonce"] = "x" },
	}
	for name, mutate := range bad {
		c := base()
		mutate(c)
		if _, err := o.verifyIDToken(context.Background(), d, idp.sign(c), "n"); err == nil {
			t.Errorf("%s : jeton accepté", name)
		}
	}
	// Signature altérée, alg none et HS256 sans secret client.
	tok := idp.sign(base())
	if _, err := o.verifyIDToken(context.Background(), d, tok[:len(tok)-4]+"AAAA", "n"); err == nil {
		t.Error("signature altérée acceptée")
	}
	parts := strings.Split(tok, ".")
	none := b64.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "."
	if _, err := o.verifyIDToken(context.Background(), d, none, "n"); err == nil {
		t.Error("alg none accepté")
	}
	idp.alg = "HS256"
	if _, err := o.verifyIDToken(context.Background(), d, idp.sign(base()), "n"); err == nil {
		t.Error("HS256 accepté sans secret client")
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/a/b?c=1":           "/a/b?c=1",
		"//evil.example":     "/",
		"/\\evil.example":    "/",
		"https://evil":       "/",
		"/auth/logout":       "/",
		"":                   "/",
		"/x\r\nSet-Cookie:a": "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestSessionCookieTampering(t *testing.T) {
	s := signer{[]byte("0123456789abcdef")}
	v, _ := s.encode(session{User: User{Sub: "a"}, Exp: time.Now().Add(time.Hour).Unix()})
	var out session
	if err := s.decode(v, &out); err != nil || out.Sub != "a" {
		t.Fatalf("décodage : %v", err)
	}
	p, sig, _ := strings.Cut(v, ".")
	forged := b64.EncodeToString([]byte(`{"sub":"admin","exp":9999999999}`)) + "." + sig
	if s.decode(forged, &out) == nil || s.decode(p+".x", &out) == nil {
		t.Fatal("cookie altéré accepté")
	}
	if (signer{[]byte("autre-cle-0123456")}).decode(v, &out) == nil {
		t.Fatal("cookie signé par une autre clé accepté")
	}
}

func TestLoadSecret(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSecret("court", dir); err == nil {
		t.Fatal("secret trop court accepté")
	}
	k1, err := LoadSecret("", dir)
	if err != nil || len(k1) != 32 {
		t.Fatalf("génération : %v", err)
	}
	fi, _ := os.Stat(filepath.Join(dir, ".session-secret"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("droits %v", fi.Mode().Perm())
	}
	k2, _ := LoadSecret("", dir)
	if string(k1) != string(k2) {
		t.Fatal("clé non conservée entre deux démarrages")
	}
}
