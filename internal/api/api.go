// Package api expose le Store en REST/JSON.
//
//	GET    /api/list?path=dir            contenu d'un dossier
//	GET    /api/search?q=texte           recherche par nom
//	GET    /api/file?path=a/b.excalidraw lecture (ETag en en-tête)
//	PUT    /api/file?path=a/b.excalidraw écriture (If-Match / If-None-Match: *)
//	POST   /api/folder   {"path":"a/b"}           création de dossier
//	POST   /api/move     {"from":"x","to":"y"}    renommage / déplacement
//	POST   /api/copy     {"from":"x","to":"y"}    duplication d'un schéma
//	DELETE /api/entry?path=x[&recursive=true]     mise à la corbeille
//	GET    /api/library  /  PUT /api/library      bibliothèque Excalidraw
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/vblogio/excalidraw-go/internal/storage"
)

// Handler regroupe les routes de l'API.
type Handler struct {
	Store   *storage.Store
	MaxBody int64 // taille max d'un schéma (images embarquées comprises)
}

// Register enregistre les routes sur mux (motifs Go 1.22+).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/list", h.list)
	mux.HandleFunc("GET /api/search", h.search)
	mux.HandleFunc("GET /api/file", h.readFile)
	mux.HandleFunc("PUT /api/file", h.writeFile)
	mux.HandleFunc("POST /api/folder", h.mkdir)
	mux.HandleFunc("POST /api/move", h.move)
	mux.HandleFunc("POST /api/copy", h.copy)
	mux.HandleFunc("DELETE /api/entry", h.delete)
	mux.HandleFunc("GET /api/library", h.readLibrary)
	mux.HandleFunc("PUT /api/library", h.writeLibrary)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "route inconnue")
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fail traduit une erreur du Store en code HTTP.
func fail(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, storage.ErrExists), errors.Is(err, storage.ErrNotEmpty):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, storage.ErrConflict):
		writeErr(w, http.StatusPreconditionFailed, err.Error())
	case errors.Is(err, storage.ErrInvalidPath), errors.Is(err, storage.ErrInvalidJSON):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &tooBig):
		writeErr(w, http.StatusRequestEntityTooLarge, "contenu trop volumineux")
	default:
		slog.Error("erreur interne", "err", err)
		writeErr(w, http.StatusInternalServerError, "erreur interne")
	}
}

func (h *Handler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, h.MaxBody))
}

func (h *Handler) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := h.readBody(w, r)
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "requête JSON invalide")
		return false
	}
	return true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Store.List(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Store.Search(r.URL.Query().Get("q"), 200)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) readFile(w http.ResponseWriter, r *http.Request) {
	data, etag, err := h.Store.Read(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handler) writeFile(w http.ResponseWriter, r *http.Request) {
	data, err := h.readBody(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	create := r.Header.Get("If-None-Match") == "*"
	entry, etag, err := h.Store.Write(r.URL.Query().Get("path"), data, r.Header.Get("If-Match"), create)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, http.StatusOK, entry)
}

type pathReq struct {
	Path string `json:"path"`
}

type moveReq struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (h *Handler) mkdir(w http.ResponseWriter, r *http.Request) {
	var req pathReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, err := h.Store.Mkdir(req.Path)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) move(w http.ResponseWriter, r *http.Request) {
	var req moveReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, err := h.Store.Move(req.From, req.To)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) copy(w http.ResponseWriter, r *http.Request) {
	var req moveReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, err := h.Store.Copy(req.From, req.To)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := h.Store.Delete(q.Get("path"), q.Get("recursive") == "true"); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readLibrary(w http.ResponseWriter, r *http.Request) {
	data, err := h.Store.ReadLibrary()
	if err != nil {
		fail(w, err)
		return
	}
	if data == nil {
		data = []byte(`{"type":"excalidrawlib","version":2,"libraryItems":[]}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (h *Handler) writeLibrary(w http.ResponseWriter, r *http.Request) {
	data, err := h.readBody(w, r)
	if err == nil {
		err = h.Store.WriteLibrary(data)
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
