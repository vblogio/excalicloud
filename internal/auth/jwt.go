package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	_ "crypto/sha256" // enregistre SHA-256 pour crypto.Hash
	_ "crypto/sha512" // enregistre SHA-384 et SHA-512
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// jwk est une clé publique d'un JWKS (RFC 7517), RSA ou EC.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err1 := b64.DecodeString(k.N)
		e, err2 := b64.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(n) == 0 || len(e) == 0 || len(e) > 4 {
			return nil, errors.New("clé RSA invalide")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	case "EC":
		var c elliptic.Curve
		switch k.Crv {
		case "P-256":
			c = elliptic.P256()
		case "P-384":
			c = elliptic.P384()
		case "P-521":
			c = elliptic.P521()
		default:
			return nil, fmt.Errorf("courbe %q non prise en charge", k.Crv)
		}
		x, err1 := b64.DecodeString(k.X)
		y, err2 := b64.DecodeString(k.Y)
		if err1 != nil || err2 != nil {
			return nil, errors.New("clé EC invalide")
		}
		pub := &ecdsa.PublicKey{Curve: c, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !c.IsOnCurve(pub.X, pub.Y) {
			return nil, errors.New("clé EC invalide")
		}
		return pub, nil
	}
	return nil, fmt.Errorf("type de clé %q non pris en charge", k.Kty)
}

var algHash = map[string]crypto.Hash{
	"RS256": crypto.SHA256, "RS384": crypto.SHA384, "RS512": crypto.SHA512,
	"PS256": crypto.SHA256, "PS384": crypto.SHA384, "PS512": crypto.SHA512,
	"ES256": crypto.SHA256, "ES384": crypto.SHA384, "ES512": crypto.SHA512,
	"HS256": crypto.SHA256, "HS384": crypto.SHA384, "HS512": crypto.SHA512,
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// parseJWT découpe un JWS compact et renvoie l'en-tête, les claims bruts, le
// contenu signé et la signature.
func parseJWT(token string) (h jwtHeader, claims []byte, signed string, sig []byte, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return h, nil, "", nil, errors.New("jeton mal formé")
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &h) != nil {
		return h, nil, "", nil, errors.New("en-tête de jeton invalide")
	}
	if claims, err = b64.DecodeString(parts[1]); err != nil {
		return h, nil, "", nil, errors.New("contenu de jeton invalide")
	}
	if sig, err = b64.DecodeString(parts[2]); err != nil {
		return h, nil, "", nil, errors.New("signature de jeton invalide")
	}
	return h, claims, parts[0] + "." + parts[1], sig, nil
}

// verifySignature vérifie la signature avec une clé publique (RS, PS, ES) ou
// le secret client (HS, clé symétrique utilisée par exemple par Authentik
// quand aucun certificat de signature n'est choisi).
func verifySignature(alg string, key crypto.PublicKey, secret []byte, signed string, sig []byte) error {
	hash, ok := algHash[alg]
	if !ok {
		return fmt.Errorf("algorithme %q refusé", alg)
	}
	if strings.HasPrefix(alg, "HS") {
		if len(secret) == 0 {
			return fmt.Errorf("jeton %s sans OIDC_CLIENT_SECRET", alg)
		}
		m := hmac.New(hash.New, secret)
		m.Write([]byte(signed))
		if !hmac.Equal(m.Sum(nil), sig) {
			return errors.New("signature invalide")
		}
		return nil
	}
	d := hash.New()
	d.Write([]byte(signed))
	digest := d.Sum(nil)
	switch k := key.(type) {
	case *rsa.PublicKey:
		var err error
		switch alg[:2] {
		case "RS":
			err = rsa.VerifyPKCS1v15(k, hash, digest, sig)
		case "PS":
			err = rsa.VerifyPSS(k, hash, digest, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
		default:
			err = errors.New("algorithme incompatible avec une clé RSA")
		}
		if err != nil {
			return errors.New("signature invalide")
		}
		return nil
	case *ecdsa.PublicKey:
		size := (k.Curve.Params().BitSize + 7) / 8
		if alg[:2] != "ES" || len(sig) != 2*size {
			return errors.New("signature invalide")
		}
		r, s := new(big.Int).SetBytes(sig[:size]), new(big.Int).SetBytes(sig[size:])
		if !ecdsa.Verify(k, digest, r, s) {
			return errors.New("signature invalide")
		}
		return nil
	}
	return errors.New("clé de vérification inadaptée")
}

// idClaims regroupe les claims utiles d'un ID Token.
type idClaims struct {
	Iss               string          `json:"iss"`
	Sub               string          `json:"sub"`
	Aud               json.RawMessage `json:"aud"`
	Azp               string          `json:"azp"`
	Exp               json.Number     `json:"exp"`
	Iat               json.Number     `json:"iat"`
	Nonce             string          `json:"nonce"`
	Name              string          `json:"name"`
	PreferredUsername string          `json:"preferred_username"`
	Email             string          `json:"email"`
}

func (c idClaims) audiences() []string {
	var one string
	if json.Unmarshal(c.Aud, &one) == nil {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(c.Aud, &many)
	return many
}

const clockSkew = 2 * time.Minute

// validate contrôle émetteur, audience, dates et nonce (OIDC Core 3.1.3.7).
func (c idClaims) validate(issuer, clientID, nonce string, t time.Time) error {
	if c.Iss != issuer {
		return fmt.Errorf("émetteur %q inattendu", c.Iss)
	}
	if c.Sub == "" {
		return errors.New("claim sub absent")
	}
	aud := c.audiences()
	found := false
	for _, a := range aud {
		found = found || a == clientID
	}
	if !found {
		return errors.New("jeton destiné à un autre client")
	}
	if len(aud) > 1 && c.Azp != "" && c.Azp != clientID {
		return errors.New("azp inattendu")
	}
	exp, err := c.Exp.Int64()
	if err != nil {
		f, ferr := c.Exp.Float64()
		if ferr != nil {
			return errors.New("claim exp invalide")
		}
		exp = int64(f)
	}
	if t.After(time.Unix(exp, 0).Add(clockSkew)) {
		return errors.New("jeton expiré")
	}
	if iat, err := c.Iat.Float64(); err == nil && time.Unix(int64(iat), 0).After(t.Add(clockSkew)) {
		return errors.New("jeton émis dans le futur (horloge ?)")
	}
	if !hmac.Equal([]byte(c.Nonce), []byte(nonce)) {
		return errors.New("nonce invalide")
	}
	return nil
}

// stringList lit un claim chaîne ou tableau de chaînes (groupes, rôles).
func stringList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// claimPath lit un claim éventuellement imbriqué (« realm_access.roles »).
func claimPath(all map[string]any, p string) any {
	var cur any = all
	for _, part := range strings.Split(p, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}
