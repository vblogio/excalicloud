// Package auth fournit l'authentification OpenID Connect (flux Authorization
// Code + PKCE) et les sessions signées associées, sans dépendance externe.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// User est l'utilisateur authentifié, tel que décrit par le fournisseur.
type User struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// session est le contenu du cookie de session (signé, non chiffré : il ne
// contient que l'identité, pas de jeton).
type session struct {
	User
	Exp int64 `json:"exp"`
}

// loginState accompagne l'aller-retour vers le fournisseur (cookie de 10 min).
type loginState struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Next     string `json:"next"`
	Exp      int64  `json:"exp"`
}

var errBadCookie = errors.New("cookie invalide")

// signer signe et vérifie des valeurs JSON (HMAC-SHA256).
type signer struct{ key []byte }

var b64 = base64.RawURLEncoding

func (s signer) mac(payload string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return b64.EncodeToString(m.Sum(nil))
}

func (s signer) encode(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	p := b64.EncodeToString(raw)
	return p + "." + s.mac(p), nil
}

// decode vérifie la signature puis décode ; le contrôle d'expiration revient
// à l'appelant (champ Exp).
func (s signer) decode(value string, v any) error {
	p, sig, ok := strings.Cut(value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.mac(p))) {
		return errBadCookie
	}
	raw, err := b64.DecodeString(p)
	if err != nil {
		return errBadCookie
	}
	if json.Unmarshal(raw, v) != nil {
		return errBadCookie
	}
	return nil
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand ne échoue pas sur les systèmes supportés
	}
	return b64.EncodeToString(b)
}

// LoadSecret renvoie la clé de signature des sessions : secret si fourni
// (16 caractères minimum), sinon le contenu de dir/.session-secret, créé
// aléatoirement au premier démarrage (0600).
func LoadSecret(secret, dir string) ([]byte, error) {
	if secret != "" {
		if len(secret) < 16 {
			return nil, errors.New("SESSION_SECRET trop court (16 caractères minimum)")
		}
		return []byte(secret), nil
	}
	file := filepath.Join(dir, ".session-secret")
	if b, err := os.ReadFile(file); err == nil {
		if k, err := hex.DecodeString(strings.TrimSpace(string(b))); err == nil && len(k) >= 32 {
			return k, nil
		}
		return nil, errors.New(file + " invalide (supprimer le fichier pour en générer un nouveau)")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, []byte(hex.EncodeToString(k)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

func now() time.Time { return time.Now() }
