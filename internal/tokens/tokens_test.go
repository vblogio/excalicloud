package tokens

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vblogio/excalidraw-go/internal/auth"
)

// newServer monte le serveur de jetons devant un mux de test : /whoami
// renvoie l'utilisateur vu par l'application, /mcp est la ressource protégée.
// L'utilisateur de session est simulé par l'en-tête X-User.
func newServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	userID := func(r *http.Request) string {
		if u := auth.UserFrom(r.Context()); u != nil {
			return u.Email
		}
		return "local"
	}
	s := &Server{Store: st, PublicURL: "https://ex.test", UserID: userID, RedirectHosts: []string{"claude.ai"}, ResourcePaths: []string{"/mcp"}}
	mux := http.NewServeMux()
	s.Register(mux)
	who := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(userID(r))) }
	mux.HandleFunc("/whoami", who)
	mux.HandleFunc("/mcp", who)
	// Session simulée : X-User porte l'email de l'utilisateur connecté.
	session := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e := r.Header.Get("X-User"); e != "" {
			r = r.WithContext(auth.WithUser(r.Context(), &auth.User{Sub: e, Name: e, Email: e}))
		}
		mux.ServeHTTP(w, r)
	})
	return s, s.Middleware(session, mux)
}

func do(h http.Handler, method, target, user, bearer string, body string, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPersonalTokens(t *testing.T) {
	_, h := newServer(t)
	rec := do(h, "POST", "/api/tokens", "a@x.fr", "", `{"name":"  Claude   Code "}`, "application/json")
	if rec.Code != 201 {
		t.Fatalf("création : %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Info
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !strings.HasPrefix(created.Token, apiPrefix) || created.Name != "Claude Code" {
		t.Fatalf("jeton = %+v", created)
	}
	if strings.Contains(do(h, "GET", "/api/tokens", "a@x.fr", "", "", "").Body.String(), created.Token[4:]) {
		t.Fatal("jeton en clair dans la liste")
	}
	if got := do(h, "GET", "/whoami", "", created.Token, "", "").Body.String(); got != "a@x.fr" {
		t.Fatalf("utilisateur du jeton = %q", got)
	}
	// Un jeton ne gère pas les jetons ; b ne voit ni ne révoque ceux de a.
	if rec := do(h, "GET", "/api/tokens", "", created.Token, "", ""); rec.Code != 403 {
		t.Fatalf("liste par jeton : %d", rec.Code)
	}
	if body := do(h, "GET", "/api/tokens", "b@x.fr", "", "", "").Body.String(); strings.TrimSpace(body) != "[]" {
		t.Fatalf("b voit : %s", body)
	}
	if rec := do(h, "DELETE", "/api/tokens/"+created.ID, "b@x.fr", "", "", ""); rec.Code != 404 {
		t.Fatalf("révocation par b : %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/tokens/"+created.ID, "a@x.fr", "", "", ""); rec.Code != 204 {
		t.Fatalf("révocation : %d", rec.Code)
	}
	if rec := do(h, "GET", "/whoami", "", created.Token, "", ""); rec.Code != 401 || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Fatalf("jeton révoqué : %d %v", rec.Code, rec.Header())
	}
	for _, name := range []string{"", strings.Repeat("x", MaxNameLen+1)} {
		if rec := do(h, "POST", "/api/tokens", "a@x.fr", "", `{"name":"`+name+`"}`, ""); rec.Code != 400 {
			t.Errorf("nom %q : %d", name, rec.Code)
		}
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	st, _ := Open(dir)
	secret, _, err := st.Create("a", nil, "x")
	if err != nil {
		t.Fatal(err)
	}
	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tok, ok := st2.Lookup(secret); !ok || tok.UserID != "a" || tok.User != nil {
		t.Fatalf("après rechargement : %+v %v", tok, ok)
	}
	if string(st2.Key()) != string(st.Key()) {
		t.Fatal("clé HMAC régénérée")
	}
}

func TestResourceRequiresToken(t *testing.T) {
	_, h := newServer(t)
	rec := do(h, "POST", "/mcp", "a@x.fr", "", "{}", "")
	want := `Bearer resource_metadata="https://ex.test/.well-known/oauth-protected-resource"`
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != want {
		t.Fatalf("/mcp sans jeton : %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	// Les autres routes suivent la session.
	if got := do(h, "GET", "/whoami", "a@x.fr", "", "", "").Body.String(); got != "a@x.fr" {
		t.Fatalf("session : %q", got)
	}
}

func pkce() (verifier, challenge string) {
	verifier = strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

var hidden = regexp.MustCompile(`name="(\w+)" value="([^"]*)"`)

func TestOAuthFlow(t *testing.T) {
	s, h := newServer(t)
	const cb = "https://claude.ai/api/mcp/auth_callback"

	var meta map[string]any
	_ = json.Unmarshal(do(h, "GET", "/.well-known/oauth-protected-resource/mcp", "", "", "", "").Body.Bytes(), &meta)
	if meta["resource"] != "https://ex.test/mcp" {
		t.Fatalf("ressource = %v", meta)
	}
	_ = json.Unmarshal(do(h, "GET", "/.well-known/oauth-authorization-server", "", "", "", "").Body.Bytes(), &meta)
	if meta["registration_endpoint"] != "https://ex.test/oauth/register" {
		t.Fatalf("métadonnées = %v", meta)
	}

	// Enregistrement dynamique : redirection limitée aux hôtes autorisés.
	if rec := do(h, "POST", "/oauth/register", "", "", `{"client_name":"Evil","redirect_uris":["https://evil.test/cb"]}`, "application/json"); rec.Code != 400 {
		t.Fatalf("redirection externe acceptée : %d", rec.Code)
	}
	rec := do(h, "POST", "/oauth/register", "", "", `{"client_name":"Claude","redirect_uris":["`+cb+`"]}`, "application/json")
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &reg) != nil || reg.ClientID == "" {
		t.Fatalf("enregistrement : %d %s", rec.Code, rec.Body)
	}

	verifier, challenge := pkce()
	q := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {cb},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {"xyz"}}
	if rec := do(h, "GET", "/oauth/authorize?"+strings.Replace(q.Encode(), "claude.ai", "evil.test", 1), "a@x.fr", "", "", ""); rec.Code != 400 {
		t.Fatalf("redirection non enregistrée : %d", rec.Code)
	}
	page := do(h, "GET", "/oauth/authorize?"+q.Encode(), "a@x.fr", "", "", "")
	if page.Code != 200 || !strings.Contains(page.Body.String(), "a@x.fr") {
		t.Fatalf("consentement : %d %s", page.Code, page.Body)
	}
	form := url.Values{}
	for _, m := range hidden.FindAllStringSubmatch(page.Body.String(), -1) {
		form.Set(m[1], strings.ReplaceAll(m[2], "&#43;", "+"))
	}
	form.Set("action", "allow")

	// Le formulaire est lié à l'utilisateur : b ne peut pas le rejouer.
	if rec := do(h, "POST", "/oauth/authorize", "b@x.fr", "", form.Encode(), "application/x-www-form-urlencoded"); rec.Code != 400 {
		t.Fatalf("formulaire rejoué par b : %d", rec.Code)
	}
	rec = do(h, "POST", "/oauth/authorize", "a@x.fr", "", form.Encode(), "application/x-www-form-urlencoded")
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")
	if rec.Code != 302 || code == "" || loc.Query().Get("state") != "xyz" || !strings.HasPrefix(loc.String(), cb) {
		t.Fatalf("redirection : %d %v", rec.Code, loc)
	}

	tokenReq := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {reg.ClientID},
		"redirect_uri": {cb}, "code_verifier": {"mauvais" + verifier}}
	if rec := do(h, "POST", "/oauth/token", "", "", tokenReq.Encode(), "application/x-www-form-urlencoded"); rec.Code != 400 {
		t.Fatalf("PKCE incorrect accepté : %d", rec.Code)
	}
	// Le code est à usage unique, même après un échec : recommencer.
	rec = do(h, "POST", "/oauth/authorize", "a@x.fr", "", form.Encode(), "application/x-www-form-urlencoded")
	loc, _ = url.Parse(rec.Header().Get("Location"))
	tokenReq.Set("code", loc.Query().Get("code"))
	tokenReq.Set("code_verifier", verifier)
	rec = do(h, "POST", "/oauth/token", "", "", tokenReq.Encode(), "application/x-www-form-urlencoded")
	var tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &tok) != nil || tok.Access == "" {
		t.Fatalf("échange : %d %s", rec.Code, rec.Body)
	}
	if got := do(h, "POST", "/mcp", "", tok.Access, "", "").Body.String(); got != "a@x.fr" {
		t.Fatalf("/mcp avec jeton OAuth : %q", got)
	}
	if rec := do(h, "POST", "/oauth/token", "", "", tokenReq.Encode(), "application/x-www-form-urlencoded"); rec.Code != 400 {
		t.Fatalf("code rejoué : %d", rec.Code)
	}

	// Rafraîchissement avec rotation.
	ref := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok.Refresh}, "client_id": {reg.ClientID}}
	rec = do(h, "POST", "/oauth/token", "", "", ref.Encode(), "application/x-www-form-urlencoded")
	var tok2 struct {
		Access string `json:"access_token"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &tok2) != nil || tok2.Access == tok.Access {
		t.Fatalf("rafraîchissement : %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/mcp", "", tok.Access, "", ""); rec.Code != 401 {
		t.Fatalf("ancien jeton d'accès encore valable : %d", rec.Code)
	}
	if rec := do(h, "POST", "/oauth/token", "", "", ref.Encode(), "application/x-www-form-urlencoded"); rec.Code != 400 {
		t.Fatalf("ancien jeton de rafraîchissement accepté : %d", rec.Code)
	}

	// Expiration du jeton d'accès, puis révocation depuis l'interface.
	s.Store.now = func() time.Time { return time.Now().Add(2 * AccessTTL) }
	if rec := do(h, "POST", "/mcp", "", tok2.Access, "", ""); rec.Code != 401 {
		t.Fatalf("jeton expiré accepté : %d", rec.Code)
	}
	s.Store.now = time.Now
	var list []Info
	_ = json.Unmarshal(do(h, "GET", "/api/tokens", "a@x.fr", "", "", "").Body.Bytes(), &list)
	if len(list) != 1 || list[0].Kind != KindOAuth || list[0].Name != "Claude" || list[0].Client != "claude.ai" {
		t.Fatalf("liste = %+v", list)
	}
	do(h, "DELETE", "/api/tokens/"+list[0].ID, "a@x.fr", "", "", "")
	if rec := do(h, "POST", "/mcp", "", tok2.Access, "", ""); rec.Code != 401 {
		t.Fatalf("autorisation révoquée encore valable : %d", rec.Code)
	}
}

func TestOAuthDenyAndLoopback(t *testing.T) {
	_, h := newServer(t)
	rec := do(h, "POST", "/oauth/register", "", "", `{"client_name":"Claude Code","redirect_uris":["http://localhost/callback"]}`, "")
	var reg struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &reg)
	_, challenge := pkce()
	// Port éphémère de Claude Code : accepté pour une adresse de bouclage.
	q := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {"http://localhost:3118/callback"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	page := do(h, "GET", "/oauth/authorize?"+q.Encode(), "", "", "", "")
	if page.Code != 200 || !strings.Contains(page.Body.String(), "utilisateur local") {
		t.Fatalf("consentement : %d %s", page.Code, page.Body)
	}
	form := url.Values{}
	for _, m := range hidden.FindAllStringSubmatch(page.Body.String(), -1) {
		form.Set(m[1], m[2])
	}
	form.Set("action", "deny")
	rec = do(h, "POST", "/oauth/authorize", "", "", form.Encode(), "application/x-www-form-urlencoded")
	if loc := rec.Header().Get("Location"); rec.Code != 302 || !strings.Contains(loc, "error=access_denied") || !strings.HasPrefix(loc, "http://localhost:3118/callback") {
		t.Fatalf("refus : %d %s", rec.Code, loc)
	}
	// Sans PKCE : erreur renvoyée au client.
	q.Del("code_challenge")
	if rec := do(h, "GET", "/oauth/authorize?"+q.Encode(), "", "", "", ""); rec.Code != 302 || !strings.Contains(rec.Header().Get("Location"), "invalid_request") {
		t.Fatalf("sans PKCE : %d", rec.Code)
	}
}
