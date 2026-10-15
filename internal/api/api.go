// Package api expose le Store en REST/JSON.
//
//	GET    /api/list?path=dir            contenu d'un dossier
//	GET    /api/search?q=texte[&tag=t][&fav=true] recherche (noms, tags, texte des schémas)
//	GET    /api/tags                     tags utilisés et nombre d'éléments
//	POST   /api/meta/favorite {"paths":[...],"favorite":true}  favoris
//	POST   /api/meta/tags {"paths":[...],"add":[...],"remove":[...]} tags
//	POST   /api/meta/icon {"paths":[...],"icon":"server"} icône de dossiers ("" : défaut)
//	GET    /api/file?path=a/b.excalidraw lecture (ETag en en-tête)
//	PUT    /api/file?path=a/b.excalidraw écriture (If-Match / If-None-Match: *)
//	POST   /api/folder   {"path":"a/b"}           création de dossier
//	POST   /api/move     {"from":"x","to":"y"}    renommage / déplacement
//	POST   /api/move-many {"paths":[...],"dir":"d"} déplacement multiple
//	POST   /api/copy     {"from":"x","to":"y"}    duplication d'un schéma
//	DELETE /api/entry?path=x[&recursive=true]     mise à la corbeille
//	GET    /api/thumb?path=x  /  PUT (image/png)   miniature d'un schéma
//	GET    /api/history?path=x                   versions d'un schéma
//	GET    /api/history/version?path=x&id=v      contenu d'une version
//	POST   /api/history/snapshot {"path"}        version du contenu courant
//	POST   /api/history/restore {"path","id"}    restauration (ETag en en-tête)
//	GET    /api/shares[?path=x]                  liens de partage (d'un schéma ou tous)
//	POST   /api/shares {"path","days"}           nouveau lien (days 0 : sans expiration)
//	DELETE /api/shares?id=x                      révocation d'un lien
//	POST   /api/shares/revoke-all                révocation de tous les liens
//	GET    /api/public/{jeton}                   schéma partagé (sans authentification)
//	GET    /api/library  /  PUT /api/library      bibliothèque Excalidraw
//	GET    /api/settings / PUT {"title","channel"} paramètres (titre, logo, canal)
//	GET    /api/logo  /  PUT (image/png)  /  DELETE   logo 64x64
//	GET    /api/update                    dernière version d'Excalidraw sur npm
//
// Les chemins sont vus par l'utilisateur connecté (storage.Space) : l'arborescence
// commune, plus ".perso/..." (son espace personnel) et ".favoris" (ses favoris,
// liens symboliques vers les originaux ; DELETE .favoris/x retire le lien).
package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/vblogio/excalidraw-go/internal/storage"
	"github.com/vblogio/excalidraw-go/internal/update"
)

// Handler regroupe les routes de l'API.
type Handler struct {
	Store   *storage.Store             // arborescence commune : bibliothèque, paramètres, logo
	Spaces  *storage.Spaces            // schémas : arborescence commune, espaces personnels, favoris
	User    func(*http.Request) string // identifiant de l'utilisateur (storage.UserID)
	MaxBody int64                      // taille max d'un schéma (images embarquées comprises)
	Update  *update.Checker            // nil : vérification des mises à jour désactivée
}

// space renvoie la vue de l'utilisateur de la requête (espace personnel et
// favoris) ; utilisateur unique LocalUser sans OIDC.
func (h *Handler) space(w http.ResponseWriter, r *http.Request) (*storage.Space, bool) {
	id := storage.LocalUser
	if h.User != nil {
		id = h.User(r)
	}
	u, err := h.Spaces.For(id)
	if err != nil {
		fail(w, err)
		return nil, false
	}
	return u, true
}

// Register enregistre les routes sur mux (motifs Go 1.22+).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/list", h.list)
	mux.HandleFunc("GET /api/search", h.search)
	mux.HandleFunc("GET /api/tags", h.tags)
	mux.HandleFunc("POST /api/meta/favorite", h.setFavorite)
	mux.HandleFunc("POST /api/meta/tags", h.editTags)
	mux.HandleFunc("POST /api/meta/icon", h.setIcon)
	mux.HandleFunc("GET /api/file", h.readFile)
	mux.HandleFunc("PUT /api/file", h.writeFile)
	mux.HandleFunc("POST /api/folder", h.mkdir)
	mux.HandleFunc("POST /api/move", h.move)
	mux.HandleFunc("POST /api/move-many", h.moveMany)
	mux.HandleFunc("POST /api/copy", h.copy)
	mux.HandleFunc("DELETE /api/entry", h.delete)
	mux.HandleFunc("GET /api/thumb", h.readThumb)
	mux.HandleFunc("PUT /api/thumb", h.writeThumb)
	mux.HandleFunc("GET /api/history", h.history)
	mux.HandleFunc("GET /api/history/version", h.readVersion)
	mux.HandleFunc("POST /api/history/snapshot", h.snapshot)
	mux.HandleFunc("POST /api/history/restore", h.restore)
	mux.HandleFunc("GET /api/shares", h.shares)
	mux.HandleFunc("POST /api/shares", h.createShare)
	mux.HandleFunc("DELETE /api/shares", h.revokeShare)
	mux.HandleFunc("POST /api/shares/revoke-all", h.revokeAllShares)
	mux.HandleFunc("GET /api/public/{token}", h.public)
	mux.HandleFunc("GET /api/library", h.readLibrary)
	mux.HandleFunc("PUT /api/library", h.writeLibrary)
	mux.HandleFunc("GET /api/settings", h.readSettings)
	mux.HandleFunc("PUT /api/settings", h.writeSettings)
	mux.HandleFunc("GET /api/logo", h.readLogo)
	mux.HandleFunc("PUT /api/logo", h.writeLogo)
	mux.HandleFunc("DELETE /api/logo", h.deleteLogo)
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, h.Update.Status())
	})
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
	case errors.Is(err, storage.ErrInvalidPath), errors.Is(err, storage.ErrInvalidJSON),
		errors.Is(err, storage.ErrInvalidImage):
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
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	entries, err := u.List(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := storage.Filter{Tag: q.Get("tag"), Favorite: q.Get("fav") == "true"}
	entries, err := u.Search(q.Get("q"), f, 200)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) tags(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	tags, err := u.Tags()
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, tags)
}

type metaReq struct {
	Paths    []string `json:"paths"`
	Favorite bool     `json:"favorite"`
	Add      []string `json:"add"`
	Remove   []string `json:"remove"`
	Icon     string   `json:"icon"`
}

func (h *Handler) decodeMeta(w http.ResponseWriter, r *http.Request) (metaReq, bool) {
	var req metaReq
	if !h.decode(w, r, &req) {
		return req, false
	}
	if len(req.Paths) == 0 || len(req.Paths) > 1000 {
		writeErr(w, http.StatusBadRequest, "entre 1 et 1000 chemins attendus")
		return req, false
	}
	return req, true
}

func (h *Handler) setFavorite(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	req, ok := h.decodeMeta(w, r)
	if !ok {
		return
	}
	if err := u.SetFavorite(req.Paths, req.Favorite); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) editTags(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	req, ok := h.decodeMeta(w, r)
	if !ok {
		return
	}
	if err := u.EditTags(req.Paths, req.Add, req.Remove); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setIcon(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	req, ok := h.decodeMeta(w, r)
	if !ok {
		return
	}
	if err := u.SetIcon(req.Paths, req.Icon); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readFile(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	data, etag, err := u.Read(r.URL.Query().Get("path"))
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
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	data, err := h.readBody(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	create := r.Header.Get("If-None-Match") == "*"
	entry, etag, err := u.Write(r.URL.Query().Get("path"), data, r.Header.Get("If-Match"), create)
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
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req pathReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, err := u.Mkdir(req.Path)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) move(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req moveReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, err := u.Move(req.From, req.To)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

type moveManyReq struct {
	Paths []string `json:"paths"`
	Dir   string   `json:"dir"`
}

// moveMany renvoie toujours 200 avec le détail par élément : le client
// affiche les échecs partiels (ex. nom déjà pris dans la destination).
func (h *Handler) moveMany(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req moveManyReq
	if !h.decode(w, r, &req) {
		return
	}
	if len(req.Paths) == 0 || len(req.Paths) > 1000 {
		writeErr(w, http.StatusBadRequest, "entre 1 et 1000 chemins attendus")
		return
	}
	res, err := u.MoveMany(req.Paths, req.Dir)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) readThumb(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	data, mod, err := u.ReadThumb(r.URL.Query().Get("path"))
	if err != nil {
		// Pas de mise en cache d'une absence : la miniature va être générée.
		w.Header().Set("Cache-Control", "no-store")
		fail(w, err)
		return
	}
	// L'URL porte la date du schéma (?v=...) : une nouvelle version change l'URL.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Last-Modified", mod.UTC().Format(http.TimeFormat))
	_, _ = w.Write(data)
}

func (h *Handler) writeThumb(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, storage.MaxThumbSize))
	if err == nil {
		err = u.WriteThumb(r.URL.Query().Get("path"), data)
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) copy(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req moveReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, err := u.Copy(req.From, req.To)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	if err := u.Delete(q.Get("path"), q.Get("recursive") == "true"); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	vs, err := u.Versions(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, vs)
}

func (h *Handler) readVersion(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	data, err := u.ReadVersion(q.Get("path"), q.Get("id"))
	if err != nil {
		fail(w, err)
		return
	}
	// Une version ne change jamais.
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req pathReq
	if !h.decode(w, r, &req) {
		return
	}
	v, err := u.Snapshot(req.Path)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type restoreReq struct {
	Path string `json:"path"`
	ID   string `json:"id"`
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req restoreReq
	if !h.decode(w, r, &req) {
		return
	}
	entry, etag, err := u.Restore(req.Path, req.ID)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) shares(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	l, err := u.Shares(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, l)
}

type shareReq struct {
	Path string `json:"path"`
	Days int    `json:"days"`
}

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	var req shareReq
	if !h.decode(w, r, &req) {
		return
	}
	if req.Days < 0 || req.Days > 3650 {
		writeErr(w, http.StatusBadRequest, "durée de 0 (sans expiration) à 3650 jours")
		return
	}
	sh, err := u.CreateShare(req.Path, time.Duration(req.Days)*24*time.Hour)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sh)
}

func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	if err := u.RevokeShare(r.URL.Query().Get("id")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) revokeAllShares(w http.ResponseWriter, r *http.Request) {
	u, ok := h.space(w, r)
	if !ok {
		return
	}
	if err := u.RevokeAllShares(); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// public sert un schéma partagé, hors authentification (voir cmd/server).
// Seuls le nom du schéma, son contenu et l'apparence (titre, logo) sont exposés.
func (h *Handler) public(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	sh, data, err := h.Spaces.ReadShared(r.PathValue("token"))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "lien invalide, expiré ou révoqué")
		} else {
			fail(w, err)
		}
		return
	}
	out := struct {
		Name  string          `json:"name"`
		Title string          `json:"title"`
		Logo  string          `json:"logo,omitempty"`
		Scene json.RawMessage `json:"scene"`
	}{Name: strings.TrimSuffix(path.Base(sh.Path), storage.Ext), Scene: data}
	if st, err := h.Store.ReadSettings(); err == nil {
		out.Title = st.Title
	}
	if logo, err := h.Store.ReadLogo(); err == nil {
		out.Logo = "data:image/png;base64," + base64.StdEncoding.EncodeToString(logo)
	}
	writeJSON(w, http.StatusOK, out)
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

func (h *Handler) readSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.Store.ReadSettings()
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, st)
}

// settingsReq : seuls les champs présents sont modifiés.
type settingsReq struct {
	Title   *string `json:"title"`
	Channel *string `json:"channel"`
}

func (h *Handler) writeSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsReq
	if !h.decode(w, r, &req) {
		return
	}
	st, err := h.Store.ReadSettings()
	if err == nil && req.Title != nil {
		st, err = h.Store.WriteTitle(*req.Title)
	}
	if err == nil && req.Channel != nil {
		st, err = h.Store.WriteChannel(*req.Channel)
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) readLogo(w http.ResponseWriter, r *http.Request) {
	data, err := h.Store.ReadLogo()
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		fail(w, err)
		return
	}
	// L'URL porte la version du logo (?v=...) : un nouveau logo change l'URL.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

func (h *Handler) writeLogo(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, storage.MaxLogoUpload))
	if err != nil {
		fail(w, err)
		return
	}
	st, err := h.Store.WriteLogo(data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) deleteLogo(w http.ResponseWriter, r *http.Request) {
	st, err := h.Store.DeleteLogo()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
