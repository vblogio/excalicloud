// Package mcp expose les schémas à un client MCP (Model Context Protocol) en
// transport « Streamable HTTP » : JSON-RPC 2.0 par POST sur une seule route,
// réponses JSON (pas de flux SSE ni de session : le serveur n'émet rien de
// lui-même). L'authentification est faite en amont (jeton Bearer, paquet
// tokens) ; les chemins sont ceux de l'utilisateur (storage.Space).
package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/vblogio/excalidraw-go/internal/storage"
)

// Versions du protocole acceptées, la plus récente d'abord.
var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Server répond aux requêtes MCP.
type Server struct {
	Spaces  *storage.Spaces
	User    func(*http.Request) string // identifiant de l'utilisateur (storage.UserID)
	BaseURL func(*http.Request) string // origine publique, pour les liens vers les schémas
	MaxBody int64
	Version string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethod         = -32601
	errParams         = -32602
)

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// Pas de flux serveur → client (GET) ni de session à fermer (DELETE).
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "méthode non prise en charge", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.MaxBody))
	if err != nil {
		reply(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{errInvalidRequest, "requête trop volumineuse"}})
		return
	}
	var req request
	if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" {
		reply(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{errParse, "message JSON-RPC 2.0 attendu (les lots ne sont pas pris en charge)"}})
		return
	}
	if len(req.ID) == 0 || req.Method == "" {
		// Notification (initialized, cancelled…) ou réponse du client.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	res := response{JSONRPC: "2.0", ID: req.ID}
	res.Result, res.Error = s.call(r, req)
	reply(w, res)
}

func reply(w http.ResponseWriter, res response) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) call(r *http.Request, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := protocolVersions[0]
		if slices.Contains(protocolVersions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "excalicloud", "title": "Excalicloud", "version": s.Version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			return nil, &rpcError{errParams, "paramètres invalides"}
		}
		h, ok := handlers[p.Name]
		if !ok {
			return nil, &rpcError{errParams, "outil inconnu : " + p.Name}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		u, err := s.Spaces.For(s.User(r))
		if err != nil {
			return toolError(err), nil
		}
		out, err := h(&call{s: s, r: r, u: u}, p.Arguments)
		if err != nil {
			return toolError(err), nil
		}
		text, _ := json.MarshalIndent(out, "", " ")
		return map[string]any{
			"content":           []any{map[string]any{"type": "text", "text": string(text)}},
			"structuredContent": out,
		}, nil
	}
	return nil, &rpcError{errMethod, "méthode inconnue : " + req.Method}
}

// toolError renvoie une erreur d'outil lisible par le modèle (isError).
func toolError(err error) map[string]any {
	msg := err.Error()
	var tooBig *http.MaxBytesError
	var arg argError
	switch {
	case errors.Is(err, storage.ErrNotFound):
		msg = "introuvable : " + msg
	case errors.Is(err, storage.ErrExists):
		msg = "existe déjà (overwrite: true pour remplacer) : " + msg
	case errors.Is(err, storage.ErrInvalidPath), errors.Is(err, storage.ErrInvalidJSON),
		errors.Is(err, storage.ErrNotEmpty), errors.As(err, &tooBig), errors.As(err, &arg):
	default:
		slog.Error("mcp", "err", err)
		msg = "erreur interne"
	}
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": msg}}, "isError": true}
}
