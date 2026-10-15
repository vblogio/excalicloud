// Package tokens gère les jetons d'accès personnels à l'API et au serveur MCP :
// jetons créés depuis l'interface (sans expiration) et jetons délivrés par le
// petit serveur OAuth 2.1 intégré (connecteurs claude.ai, Claude Desktop…).
//
// Les jetons ne sont conservés que hachés (SHA-256) dans DATA_DIR/.tokens.json
// (0600), rattachés à l'utilisateur qui les a créés : ils donnent accès à son
// espace personnel et à ses favoris, comme sa session.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/vblogio/excalicloud/internal/auth"
)

const (
	fileName = ".tokens.json"

	apiPrefix     = "exc_"  // jeton personnel
	accessPrefix  = "exca_" // jeton d'accès OAuth
	refreshPrefix = "excr_" // jeton de rafraîchissement OAuth

	KindAPI   = "api"
	KindOAuth = "oauth"

	// AccessTTL est la durée de vie d'un jeton d'accès OAuth ; le client le
	// renouvelle avec son jeton de rafraîchissement (rotation à chaque usage).
	AccessTTL = time.Hour
	// IdleTTL : une autorisation OAuth inutilisée pendant cette durée expire.
	IdleTTL = 90 * 24 * time.Hour

	MaxNameLen      = 80
	maxAPIPerUser   = 50
	maxOAuthPerUser = 20
	touchEvery      = time.Minute // fréquence d'enregistrement de LastUsed
)

var (
	ErrNotFound = errors.New("jeton introuvable")
	ErrInvalid  = errors.New("nom de jeton invalide")
	ErrTooMany  = errors.New("trop de jetons : en révoquer avant d'en créer")
)

// Token est un jeton enregistré. Les champs *Hash ne sortent jamais du paquet.
type Token struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	UserID      string     `json:"userId"`
	User        *auth.User `json:"user,omitempty"` // nil : authentification locale ou absente
	Hash        string     `json:"hash"`
	Expires     time.Time  `json:"expires,omitzero"`      // jeton d'accès OAuth
	RefreshHash string     `json:"refreshHash,omitempty"` // OAuth
	ClientID    string     `json:"clientId,omitempty"`    // OAuth
	Client      string     `json:"client,omitempty"`      // hôte de redirection du client OAuth
	Created     time.Time  `json:"created"`
	LastUsed    time.Time  `json:"lastUsed,omitzero"`
}

// Info est la vue d'un jeton renvoyée à l'interface.
type Info struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Client   string    `json:"client,omitempty"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"lastUsed,omitzero"`
}

func (t *Token) info() Info {
	return Info{ID: t.ID, Name: t.Name, Kind: t.Kind, Client: t.Client, Created: t.Created, LastUsed: t.LastUsed}
}

type fileData struct {
	Key    string   `json:"key"` // clé HMAC (identifiants de clients OAuth, formulaires)
	Tokens []*Token `json:"tokens"`
}

// Store conserve les jetons dans dir/.tokens.json.
type Store struct {
	mu     sync.Mutex
	file   string
	key    []byte
	tokens []*Token
	now    func() time.Time
}

// Open charge (ou crée) le fichier des jetons de dir.
func Open(dir string) (*Store, error) {
	s := &Store{file: filepath.Join(dir, fileName), now: time.Now}
	b, err := os.ReadFile(s.file)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		var d fileData
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, errors.New(s.file + " invalide : " + err.Error())
		}
		s.tokens = d.Tokens
		s.key, _ = hex.DecodeString(d.Key)
	}
	if len(s.key) < 32 {
		s.key = make([]byte, 32)
		if _, err := rand.Read(s.key); err != nil {
			return nil, err
		}
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Key renvoie la clé HMAC propre au serveur.
func (s *Store) Key() []byte { return s.key }

func (s *Store) save() error {
	b, err := json.MarshalIndent(fileData{Key: hex.EncodeToString(s.key), Tokens: s.tokens}, "", " ")
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file)
}

func random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func cleanName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > MaxNameLen {
		return "", ErrInvalid
	}
	return name, nil
}

// purge retire les autorisations OAuth inactives et, au-delà de max jetons
// d'un type pour un utilisateur, les moins récemment utilisés.
func (s *Store) purge(userID, kind string, max int) {
	now := s.now()
	keep := s.tokens[:0]
	var mine []*Token
	for _, t := range s.tokens {
		if t.Kind == KindOAuth && now.Sub(lastActivity(t)) > IdleTTL {
			continue
		}
		keep = append(keep, t)
		if t.UserID == userID && t.Kind == kind {
			mine = append(mine, t)
		}
	}
	s.tokens = keep
	if len(mine) < max {
		return
	}
	sort.Slice(mine, func(i, j int) bool { return lastActivity(mine[i]).Before(lastActivity(mine[j])) })
	drop := map[*Token]bool{}
	for _, t := range mine[:len(mine)-max+1] {
		drop[t] = true
	}
	keep = s.tokens[:0]
	for _, t := range s.tokens {
		if !drop[t] {
			keep = append(keep, t)
		}
	}
	s.tokens = keep
}

func lastActivity(t *Token) time.Time {
	if t.LastUsed.After(t.Created) {
		return t.LastUsed
	}
	return t.Created
}

// Create crée un jeton personnel pour l'utilisateur ; le jeton en clair n'est
// renvoyé qu'ici.
func (s *Store) Create(userID string, u *auth.User, name string) (string, Info, error) {
	name, err := cleanName(name)
	if err != nil {
		return "", Info{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range s.tokens {
		if t.UserID == userID && t.Kind == KindAPI {
			n++
		}
	}
	if n >= maxAPIPerUser {
		return "", Info{}, ErrTooMany
	}
	secret := apiPrefix + random(32)
	t := &Token{ID: random(9), Name: name, Kind: KindAPI, UserID: userID, User: u, Hash: hash(secret), Created: s.now().UTC()}
	s.tokens = append(s.tokens, t)
	if err := s.save(); err != nil {
		s.tokens = s.tokens[:len(s.tokens)-1]
		return "", Info{}, err
	}
	return secret, t.info(), nil
}

// Grant enregistre une autorisation OAuth et renvoie ses jetons.
func (s *Store) Grant(userID string, u *auth.User, name, clientID, client string) (access, refresh string, err error) {
	if name, err = cleanName(name); err != nil {
		name = "Client OAuth"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purge(userID, KindOAuth, maxOAuthPerUser)
	access, refresh = accessPrefix+random(32), refreshPrefix+random(32)
	now := s.now().UTC()
	s.tokens = append(s.tokens, &Token{
		ID: random(9), Name: name, Kind: KindOAuth, UserID: userID, User: u,
		Hash: hash(access), Expires: now.Add(AccessTTL), RefreshHash: hash(refresh),
		ClientID: clientID, Client: client, Created: now, LastUsed: now,
	})
	return access, refresh, s.save()
}

// Refresh renouvelle les jetons d'une autorisation OAuth (rotation : l'ancien
// jeton de rafraîchissement n'est plus valable).
func (s *Store) Refresh(refresh, clientID string) (access, newRefresh string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := hash(refresh)
	for _, t := range s.tokens {
		if t.Kind != KindOAuth || t.RefreshHash != h {
			continue
		}
		if t.ClientID != clientID || s.now().Sub(lastActivity(t)) > IdleTTL {
			return "", "", ErrNotFound
		}
		access, newRefresh = accessPrefix+random(32), refreshPrefix+random(32)
		t.Hash, t.RefreshHash = hash(access), hash(newRefresh)
		t.Expires = s.now().UTC().Add(AccessTTL)
		t.LastUsed = s.now().UTC()
		return access, newRefresh, s.save()
	}
	return "", "", ErrNotFound
}

// Lookup renvoie le jeton correspondant à secret s'il est valide.
func (s *Store) Lookup(secret string) (Token, bool) {
	if !strings.HasPrefix(secret, apiPrefix) && !strings.HasPrefix(secret, accessPrefix) {
		return Token{}, false
	}
	h := hash(secret)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, t := range s.tokens {
		if t.Hash != h {
			continue
		}
		if t.Kind == KindOAuth && now.After(t.Expires) {
			return Token{}, false
		}
		if now.Sub(t.LastUsed) > touchEvery {
			t.LastUsed = now.UTC()
			_ = s.save()
		}
		return *t, true
	}
	return Token{}, false
}

// List renvoie les jetons de l'utilisateur, les plus récents d'abord.
func (s *Store) List(userID string) []Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Info{}
	for _, t := range s.tokens {
		if t.UserID == userID {
			out = append(out, t.info())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// Revoke supprime un jeton de l'utilisateur.
func (s *Store) Revoke(userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tokens {
		if t.ID == id && t.UserID == userID {
			s.tokens = append(s.tokens[:i], s.tokens[i+1:]...)
			return s.save()
		}
	}
	return ErrNotFound
}
