// Package update surveille la dernière version publiée de @excalidraw/excalidraw
// sur le registre npm. Le frontend la compare à la version embarquée au build et
// signale qu'une reconstruction de l'image est possible.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Package suivi sur le registre npm.
const Package = "@excalidraw/excalidraw"

// Status est la réponse de GET /api/update.
type Status struct {
	Enabled   bool       `json:"enabled"`
	Latest    string     `json:"latest,omitempty"`    // vide tant que le registre n'a pas répondu
	CheckedAt *time.Time `json:"checkedAt,omitempty"` // dernière interrogation réussie
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
	v, err := c.fetch(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("vérification de mise à jour d'Excalidraw", "err", err)
		}
		return
	}
	now := time.Now().UTC()
	c.mu.Lock()
	c.status = Status{Enabled: true, Latest: v, CheckedAt: &now}
	c.mu.Unlock()
}

func (c *Checker) fetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Registry+"/"+Package+"/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registre npm : %s", res.Status)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("registre npm : %w", err)
	}
	if body.Version == "" {
		return "", fmt.Errorf("registre npm : version absente")
	}
	return body.Version, nil
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
	return s
}
