// Package update surveille les versions publiées de @excalidraw/excalidraw sur
// le registre npm (canaux latest et next). Le frontend compare celle du canal
// choisi dans les paramètres à la version embarquée au build et signale qu'une
// reconstruction de l'image est possible.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Package suivi sur le registre npm.
const Package = "@excalidraw/excalidraw"

// Channels sont les dist-tags proposés : latest (versions stables) et next
// (préversion publiée à chaque modification de la branche principale).
var Channels = []string{"latest", "next"}

// ValidChannel indique si ch est un canal proposé.
func ValidChannel(ch string) bool {
	for _, c := range Channels {
		if c == ch {
			return true
		}
	}
	return false
}

// Status est la réponse de GET /api/update.
type Status struct {
	Enabled   bool              `json:"enabled"`
	Tags      map[string]string `json:"tags,omitempty"`      // canal -> version ; vide tant que le registre n'a pas répondu
	Latest    string            `json:"latest,omitempty"`    // = Tags["latest"] (compatibilité)
	CheckedAt *time.Time        `json:"checkedAt,omitempty"` // dernière interrogation réussie
}

// Checker interroge le registre au démarrage puis à intervalle régulier.
// Une erreur réseau est journalisée et la dernière valeur connue conservée :
// sans accès Internet, l'interface n'affiche simplement rien.
type Checker struct {
	Registry string        // ex. https://registry.npmjs.org
	Interval time.Duration // ex. 12h
	Client   *http.Client

	mu     sync.RWMutex
	status Status
}

// Run boucle jusqu'à l'annulation de ctx.
func (c *Checker) Run(ctx context.Context) {
	for {
		c.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.Interval):
		}
	}
}

func (c *Checker) check(ctx context.Context) {
	tags, err := c.fetch(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("vérification de mise à jour d'Excalidraw", "err", err)
		}
		return
	}
	now := time.Now().UTC()
	c.mu.Lock()
	c.status = Status{Enabled: true, Tags: tags, Latest: tags["latest"], CheckedAt: &now}
	c.mu.Unlock()
}

// fetch lit les dist-tags du paquet (une seule requête, réponse de quelques
// centaines d'octets) et ne garde que les canaux proposés.
func (c *Checker) fetch(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Registry+"/-/package/"+Package+"/dist-tags", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registre npm : %s", res.Status)
	}
	var all map[string]string
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&all); err != nil {
		return nil, fmt.Errorf("registre npm : %w", err)
	}
	tags := map[string]string{}
	for _, ch := range Channels {
		if v := all[ch]; v != "" {
			tags[ch] = v
		}
	}
	if tags["latest"] == "" {
		return nil, fmt.Errorf("registre npm : dist-tag latest absent")
	}
	return tags, nil
}

// Status renvoie le dernier état connu. Un Checker nil signifie « désactivé ».
func (c *Checker) Status() Status {
	if c == nil {
		return Status{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.status
	s.Enabled = true
	if s.Tags != nil { // copie : la map est partagée avec les lectures concurrentes
		tags := make(map[string]string, len(s.Tags))
		for k, v := range s.Tags {
			tags[k] = v
		}
		s.Tags = tags
	}
	return s
}
