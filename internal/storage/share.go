package storage

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Partage en lecture seule : un lien /s/<jeton> donne accès à un schéma sans
// authentification. Le jeton est <id>.<signature> : id aléatoire (128 bits)
// et HMAC-SHA256 de l'id par la clé du serveur. Un lien est valide si sa
// signature est correcte, si son id figure dans .shares.json (révocation
// lien par lien) et s'il n'a pas expiré. Changer de clé (RevokeAllShares)
// invalide tous les liens d'un coup.
//
// Clé : SHARE_SECRET si défini, sinon .share-secret (générée au premier
// partage, 0600). Le lien suit le schéma s'il est renommé ou déplacé et
// disparaît s'il est supprimé.

const (
	sharesFile  = ".shares.json"
	secretFile  = ".share-secret"
	shareIDLen  = 16 // octets aléatoires
	shareSigLen = 16 // octets de HMAC conservés
)

// Share décrit un lien de partage.
type Share struct {
	ID      string     `json:"id"`
	Token   string     `json:"token"`
	Path    string     `json:"path"`
	Created time.Time  `json:"created"`
	Expires *time.Time `json:"expires,omitempty"`
}

type shareDisk struct {
	Path    string     `json:"path"`
	Created time.Time  `json:"created"`
	Expires *time.Time `json:"expires,omitempty"`
}

var b64 = base64.RawURLEncoding

// SetShareSecret impose la clé de signature (variable SHARE_SECRET).
func (s *Store) SetShareSecret(secret string) {
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	s.shareKey = []byte(secret)
	s.fixedKey = true
}

// key renvoie la clé de signature, créée au besoin ; appelé sous sharesMu.
func (s *Store) key() ([]byte, error) {
	if s.shareKey != nil {
		return s.shareKey, nil
	}
	p := filepath.Join(s.root, secretFile)
	b, err := os.ReadFile(p)
	if err == nil {
		if k, err := hex.DecodeString(strings.TrimSpace(string(b))); err == nil && len(k) >= 32 {
			s.shareKey = k
			return k, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return s.newKey()
}

func (s *Store) newKey() ([]byte, error) {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	p := filepath.Join(s.root, secretFile)
	if err := atomicWrite(p, []byte(hex.EncodeToString(k)+"\n")); err != nil {
		return nil, err
	}
	_ = os.Chmod(p, 0o600)
	s.shareKey = k
	return k, nil
}

func sign(key []byte, id string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("excalidraw-share:" + id))
	return b64.EncodeToString(m.Sum(nil)[:shareSigLen])
}

func (s *Store) readShares() (map[string]shareDisk, error) {
	out := map[string]shareDisk{}
	b, err := os.ReadFile(filepath.Join(s.root, sharesFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return out, nil
	case err != nil:
		return nil, err
	}
	var d struct {
		Links map[string]shareDisk `json:"links"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s : %w", sharesFile, err)
	}
	if d.Links != nil {
		out = d.Links
	}
	return out, nil
}

func (s *Store) writeShares(m map[string]shareDisk) error {
	b, err := json.Marshal(map[string]any{"links": m})
	if err != nil {
		return err
	}
	p := filepath.Join(s.root, sharesFile)
	if err := atomicWrite(p, b); err != nil {
		return err
	}
	return os.Chmod(p, 0o600)
}

// editShares applique edit à la liste des liens sous verrou ; edit renvoie
// false si rien n'a changé (pas d'écriture).
func (s *Store) editShares(edit func(map[string]shareDisk) (bool, error)) error {
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	m, err := s.readShares()
	if err != nil {
		return err
	}
	changed, err := edit(m)
	if err != nil || !changed {
		return err
	}
	return s.writeShares(m)
}

func (s *Store) toShare(key []byte, id string, d shareDisk) Share {
	return Share{ID: id, Token: id + "." + sign(key, id), Path: d.Path, Created: d.Created, Expires: d.Expires}
}

// CreateShare crée un lien de partage d'un schéma ; ttl = 0 : sans expiration.
func (s *Store) CreateShare(rel string, ttl time.Duration) (Share, error) {
	c, abs, err := s.schema(rel)
	if err != nil {
		return Share{}, err
	}
	if _, err := os.Stat(abs); err != nil {
		return Share{}, mapErr(err)
	}
	if ttl < 0 {
		return Share{}, fmt.Errorf("%w : durée négative", ErrInvalidPath)
	}
	raw := make([]byte, shareIDLen)
	if _, err := rand.Read(raw); err != nil {
		return Share{}, err
	}
	id := b64.EncodeToString(raw)
	d := shareDisk{Path: c, Created: time.Now().UTC().Truncate(time.Second)}
	if ttl > 0 {
		e := d.Created.Add(ttl)
		d.Expires = &e
	}
	var out Share
	err = s.editShares(func(m map[string]shareDisk) (bool, error) {
		key, err := s.key()
		if err != nil {
			return false, err
		}
		m[id] = d
		out = s.toShare(key, id, d)
		return true, nil
	})
	return out, err
}

// Shares liste les liens (d'un schéma, ou tous si rel est vide), les plus
// récents en premier. Les liens expirés sont purgés au passage.
func (s *Store) Shares(rel string) ([]Share, error) {
	c := ""
	if rel != "" {
		var err error
		if c, _, err = s.schema(rel); err != nil {
			return nil, err
		}
	}
	out := []Share{}
	err := s.editShares(func(m map[string]shareDisk) (bool, error) {
		key, err := s.key()
		if err != nil {
			return false, err
		}
		purged := false
		for id, d := range m {
			if d.Expires != nil && time.Now().After(*d.Expires) {
				delete(m, id)
				purged = true
			} else if c == "" || d.Path == c {
				out = append(out, s.toShare(key, id, d))
			}
		}
		return purged, nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, err
}

// RevokeShare supprime un lien.
func (s *Store) RevokeShare(id string) error {
	return s.editShares(func(m map[string]shareDisk) (bool, error) {
		if _, ok := m[id]; !ok {
			return false, ErrNotFound
		}
		delete(m, id)
		return true, nil
	})
}

// RevokeAllShares supprime tous les liens et change la clé de signature
// (sauf si elle est imposée par SHARE_SECRET).
func (s *Store) RevokeAllShares() error {
	return s.editShares(func(m map[string]shareDisk) (bool, error) {
		clear(m)
		if !s.fixedKey {
			if _, err := s.newKey(); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

// ResolveShare vérifie un jeton et renvoie le lien. Toute anomalie (jeton
// mal formé, signature fausse, lien révoqué ou expiré) donne ErrNotFound.
func (s *Store) ResolveShare(token string) (Share, error) {
	id, sig, ok := strings.Cut(token, ".")
	if !ok || len(id) != b64.EncodedLen(shareIDLen) || len(sig) != b64.EncodedLen(shareSigLen) {
		return Share{}, ErrNotFound
	}
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	key, err := s.key()
	if err != nil {
		return Share{}, err
	}
	if subtle.ConstantTimeCompare([]byte(sig), []byte(sign(key, id))) != 1 {
		return Share{}, ErrNotFound
	}
	m, err := s.readShares()
	if err != nil {
		return Share{}, err
	}
	d, ok := m[id]
	if !ok || (d.Expires != nil && time.Now().After(*d.Expires)) {
		return Share{}, ErrNotFound
	}
	return s.toShare(key, id, d), nil
}

// ReadShared renvoie le lien et le contenu du schéma partagé.
func (s *Store) ReadShared(token string) (Share, []byte, error) {
	sh, err := s.ResolveShare(token)
	if err != nil {
		return Share{}, nil, err
	}
	b, _, err := s.Read(sh.Path)
	return sh, b, err
}

// Les opérations suivantes accompagnent Move/Delete.

func (s *Store) moveShares(from, to string) {
	_ = s.editShares(func(m map[string]shareDisk) (bool, error) {
		changed := false
		for id, d := range m {
			if under(d.Path, from) {
				d.Path = to + d.Path[len(from):]
				m[id] = d
				changed = true
			}
		}
		return changed, nil
	})
}

func (s *Store) removeShares(c string) {
	_ = s.editShares(func(m map[string]shareDisk) (bool, error) {
		changed := false
		for id, d := range m {
			if under(d.Path, c) {
				delete(m, id)
				changed = true
			}
		}
		return changed, nil
	})
}
