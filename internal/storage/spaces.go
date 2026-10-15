package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Espaces personnels et favoris par utilisateur :
//
//	DATA_DIR/.users/<id>/home     espace personnel, un Store complet (historique,
//	                              miniatures, corbeille, tags, partages)
//	DATA_DIR/.users/<id>/favoris  liens symboliques relatifs vers des schémas
//
// Côté API, l'arborescence commune garde ses chemins. L'espace personnel est
// exposé sous le préfixe virtuel ".perso", les favoris sous ".favoris" et la
// poubelle sous ".poubelle" : un préfixe caché ne peut pas entrer en conflit
// avec un dossier réel. Un favori pointe vers le schéma d'origine ; il suit ses
// renommages et déplacements et disparaît quand le schéma est supprimé. La
// poubelle réunit les corbeilles de l'arborescence commune (".poubelle/c-<id>",
// visibles de tous) et de l'espace personnel (".poubelle/p-<id>").

const (
	// PersoPrefix est la racine virtuelle de l'espace personnel.
	PersoPrefix = ".perso"
	// FavPrefix est le dossier virtuel des favoris.
	FavPrefix = ".favoris"
	// TrashPrefix est le dossier virtuel de la poubelle.
	TrashPrefix = ".poubelle"

	usersDir   = ".users"
	homeName   = "home"
	favName    = "favoris"
	seededFile = ".seeded" // favoris de .meta.json déjà repris pour cet utilisateur
)

// LocalUser est l'utilisateur unique hors OIDC.
const LocalUser = "local"

// UserID transforme un identifiant (email, sub) en nom de dossier : minuscules,
// caractères hors [a-z0-9._@+-] remplacés par « _ », jamais caché.
func UserID(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("._@+-", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	id := b.String()
	if strings.HasPrefix(id, ".") {
		id = "_" + id[1:]
	}
	if id == "" {
		return LocalUser
	}
	if len(id) > 80 {
		h := sha256.Sum256([]byte(s))
		id = id[:60] + "-" + hex.EncodeToString(h[:8])
	}
	return id
}

// Spaces réunit l'arborescence commune et les espaces des utilisateurs.
type Spaces struct {
	Main *Store

	mu    sync.Mutex
	homes map[string]*Store

	favMu sync.Mutex // dossiers de favoris (tous utilisateurs)

	purgeMu   sync.Mutex
	lastPurge time.Time // dernière purge automatique de la corbeille
}

// NewSpaces crée le gestionnaire d'espaces autour du Store principal.
func NewSpaces(main *Store) *Spaces {
	return &Spaces{Main: main, homes: map[string]*Store{}}
}

func (sp *Spaces) usersRoot() string        { return filepath.Join(sp.Main.root, usersDir) }
func (sp *Spaces) userDir(id string) string { return filepath.Join(sp.usersRoot(), id) }
func (sp *Spaces) favDir(id string) string  { return filepath.Join(sp.userDir(id), favName) }

// home renvoie l'espace personnel de id, créé au premier accès.
func (sp *Spaces) home(id string) (*Store, error) {
	if id == "" || UserID(id) != id {
		return nil, fmt.Errorf("%w : utilisateur %q", ErrInvalidPath, id)
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if st := sp.homes[id]; st != nil {
		return st, nil
	}
	if err := os.MkdirAll(sp.favDir(id), 0o755); err != nil {
		return nil, err
	}
	st, err := New(filepath.Join(sp.userDir(id), homeName))
	if err != nil {
		return nil, err
	}
	st.History = sp.Main.History
	st.keyFrom = sp.Main
	sp.homes[id] = st
	sp.seedFavorites(id)
	return st, nil
}

// seedFavorites reprend une fois les favoris de .meta.json (avant les espaces
// personnels, ils étaient communs à tous).
func (sp *Spaces) seedFavorites(id string) {
	mark := filepath.Join(sp.userDir(id), seededFile)
	if _, err := os.Stat(mark); err == nil {
		return
	}
	var paths []string
	for p, m := range sp.Main.metaSnapshot() {
		if m.Favorite && requireSchemaPath(p) == nil {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	sp.favMu.Lock()
	for _, p := range paths {
		if _, abs, err := sp.Main.schema(p); err == nil {
			if fi, err := os.Lstat(abs); err == nil && fi.Mode().IsRegular() {
				_ = sp.addLink(id, abs)
			}
		}
	}
	sp.favMu.Unlock()
	_ = os.WriteFile(mark, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// For renvoie la vue de l'utilisateur id.
func (sp *Spaces) For(id string) (*Space, error) {
	h, err := sp.home(id)
	if err != nil {
		return nil, err
	}
	return &Space{sp: sp, id: id, home: h}, nil
}

// users liste les utilisateurs qui ont un espace.
func (sp *Spaces) users() []string {
	des, _ := os.ReadDir(sp.usersRoot())
	var out []string
	for _, de := range des {
		if de.IsDir() && UserID(de.Name()) == de.Name() {
			out = append(out, de.Name())
		}
	}
	return out
}

// ReadShared résout un lien de partage, de l'arborescence commune ou d'un
// espace personnel.
func (sp *Spaces) ReadShared(token string) (Share, []byte, error) {
	sh, data, err := sp.Main.ReadShared(token)
	if !errors.Is(err, ErrNotFound) {
		return sh, data, err
	}
	for _, id := range sp.users() {
		st, herr := sp.home(id)
		if herr != nil {
			continue
		}
		if sh, data, herr := st.ReadShared(token); !errors.Is(herr, ErrNotFound) {
			return sh, data, herr
		}
	}
	return Share{}, nil, err
}

// --- Liens des favoris -------------------------------------------------------

type favLink struct {
	name   string // nom du lien dans le dossier des favoris
	target string // chemin absolu visé
}

// links lit les liens du dossier des favoris de id ; appelé sous favMu.
func (sp *Spaces) links(id string) []favLink {
	dir := sp.favDir(id)
	des, _ := os.ReadDir(dir)
	var out []favLink
	for _, de := range des {
		if de.Type()&fs.ModeSymlink == 0 {
			continue
		}
		t, err := os.Readlink(filepath.Join(dir, de.Name()))
		if err != nil {
			continue
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(dir, t)
		}
		out = append(out, favLink{name: de.Name(), target: filepath.Clean(t)})
	}
	return out
}

// visibleTo indique si l'utilisateur id peut viser abs : arborescence commune
// ou son propre espace personnel.
func (sp *Spaces) visibleTo(id, abs string) bool {
	sep := string(os.PathSeparator)
	if strings.HasPrefix(abs, sp.usersRoot()+sep) {
		return strings.HasPrefix(abs, filepath.Join(sp.userDir(id), homeName)+sep)
	}
	return strings.HasPrefix(abs, sp.Main.root+sep)
}

// addLink crée un lien vers abs (nom du schéma, suffixé en cas de doublon) ;
// appelé sous favMu.
func (sp *Spaces) addLink(id, abs string) error {
	for _, l := range sp.links(id) {
		if l.target == abs {
			return nil
		}
	}
	dir := sp.favDir(id)
	rel, err := filepath.Rel(dir, abs)
	if err != nil {
		return err
	}
	base := filepath.Base(abs)
	stem, ext := strings.TrimSuffix(base, filepath.Ext(base)), filepath.Ext(base)
	name := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	return os.Symlink(rel, filepath.Join(dir, name))
}

// relink fait suivre aux favoris de tous les utilisateurs un élément déplacé
// de from vers to (chemins absolus, fichier ou dossier). Un favori qui
// deviendrait invisible pour son utilisateur (schéma passé dans l'espace
// personnel d'un autre) est retiré.
func (sp *Spaces) relink(from, to string) {
	sp.favMu.Lock()
	defer sp.favMu.Unlock()
	for _, id := range sp.users() {
		dir := sp.favDir(id)
		for _, l := range sp.links(id) {
			if l.target != from && !strings.HasPrefix(l.target, from+string(os.PathSeparator)) {
				continue
			}
			nt := to + l.target[len(from):]
			lp := filepath.Join(dir, l.name)
			_ = os.Remove(lp)
			if sp.visibleTo(id, nt) {
				if rel, err := filepath.Rel(dir, nt); err == nil {
					_ = os.Symlink(rel, lp)
				}
			}
		}
	}
}

// unlink retire, chez tous les utilisateurs, les favoris visant abs ou son contenu.
func (sp *Spaces) unlink(abs string) {
	sp.favMu.Lock()
	defer sp.favMu.Unlock()
	for _, id := range sp.users() {
		for _, l := range sp.links(id) {
			if l.target == abs || strings.HasPrefix(l.target, abs+string(os.PathSeparator)) {
				_ = os.Remove(filepath.Join(sp.favDir(id), l.name))
			}
		}
	}
}

// --- Vue d'un utilisateur ------------------------------------------------------

// Space est la vue d'un utilisateur : arborescence commune, espace personnel
// (préfixe .perso) et favoris (.favoris). Ses méthodes prennent et renvoient
// des chemins virtuels.
type Space struct {
	sp   *Spaces
	id   string
	home *Store
}

// ID renvoie l'identifiant de l'utilisateur.
func (u *Space) ID() string { return u.id }

func cleanVirtual(p string) string { return strings.TrimPrefix(path.Clean("/"+p), "/") }

func favLinkName(p string) (string, bool) {
	name, ok := strings.CutPrefix(cleanVirtual(p), FavPrefix+"/")
	return name, ok && name != "" && !strings.Contains(name, "/")
}

// route traduit un chemin virtuel en Store et chemin relatif à ce Store.
func (u *Space) route(p string) (*Store, string, error) {
	c := cleanVirtual(p)
	switch {
	case c == PersoPrefix:
		return u.home, "", nil
	case strings.HasPrefix(c, PersoPrefix+"/"):
		return u.home, c[len(PersoPrefix)+1:], nil
	case c == FavPrefix || strings.HasPrefix(c, FavPrefix+"/"):
		return nil, "", fmt.Errorf("%w : le dossier Favoris ne contient que des liens", ErrInvalidPath)
	case c == TrashPrefix || strings.HasPrefix(c, TrashPrefix+"/"):
		return nil, "", fmt.Errorf("%w : élément de la poubelle (à restaurer d'abord)", ErrInvalidPath)
	}
	return u.sp.Main, p, nil
}

// virt traduit un chemin relatif à st en chemin virtuel.
func (u *Space) virt(st *Store, rel string) string {
	if st == u.sp.Main {
		return rel
	}
	if rel == "" {
		return PersoPrefix
	}
	return PersoPrefix + "/" + rel
}

// fromAbs traduit un chemin absolu en chemin virtuel, s'il est visible.
func (u *Space) fromAbs(abs string) (string, bool) {
	for _, st := range []*Store{u.home, u.sp.Main} {
		rel, err := filepath.Rel(st.root, abs)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		c, err := Clean(filepath.ToSlash(rel))
		if err != nil || c == "" {
			return "", false // .users, .trash, .history...
		}
		return u.virt(st, c), true
	}
	return "", false
}

func absOf(st *Store, c string) string { return filepath.Join(st.root, filepath.FromSlash(c)) }

func (u *Space) favSet() map[string]bool {
	u.sp.favMu.Lock()
	links := u.sp.links(u.id)
	u.sp.favMu.Unlock()
	out := make(map[string]bool, len(links))
	for _, l := range links {
		if p, ok := u.fromAbs(l.target); ok {
			out[p] = true
		}
	}
	return out
}

func (u *Space) entry(st *Store, e Entry, favs map[string]bool) Entry {
	e.Path = u.virt(st, e.Path)
	e.Favorite = favs[e.Path]
	return e
}

// Favorites liste les favoris de l'utilisateur ; Path est le chemin du schéma
// d'origine (ou .favoris/<lien> si l'original a disparu).
func (u *Space) Favorites() ([]Entry, error) {
	u.sp.favMu.Lock()
	links := u.sp.links(u.id)
	u.sp.favMu.Unlock()
	meta := map[*Store]map[string]itemMeta{u.sp.Main: u.sp.Main.metaSnapshot(), u.home: u.home.metaSnapshot()}
	out := make([]Entry, 0, len(links))
	for _, l := range links {
		e := Entry{Name: l.name, Path: FavPrefix + "/" + l.name, Link: l.name, Broken: true}
		if p, ok := u.fromAbs(l.target); ok {
			if fi, err := os.Stat(l.target); err == nil && fi.Mode().IsRegular() {
				st, rel, _ := u.route(p)
				e = toEntry(rel, fi)
				decorate(&e, meta[st])
				e.Path, e.Link, e.Favorite = p, l.name, true
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return fold(out[i].Name) < fold(out[j].Name) })
	return out, nil
}

// SetFavorite ajoute ou retire des schémas des favoris. Un chemin
// .favoris/<lien> désigne directement un lien (favori dont l'original a disparu).
func (u *Space) SetFavorite(paths []string, favorite bool) error {
	var targets []string
	var names []string
	for _, p := range paths {
		if name, ok := favLinkName(p); ok {
			if !favorite {
				names = append(names, name)
			}
			continue
		}
		st, rel, err := u.route(p)
		if err != nil {
			return err
		}
		_, abs, err := st.schema(rel)
		if err != nil {
			return fmt.Errorf("%w (seuls les schémas peuvent être favoris)", err)
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			return mapErr(err)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%w : %s n'est pas un schéma", ErrInvalidPath, p)
		}
		targets = append(targets, abs)
	}
	u.sp.favMu.Lock()
	defer u.sp.favMu.Unlock()
	dir := u.sp.favDir(u.id)
	for _, name := range names {
		if fi, err := os.Lstat(filepath.Join(dir, name)); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	for _, abs := range targets {
		if favorite {
			if err := u.sp.addLink(u.id, abs); err != nil {
				return err
			}
			continue
		}
		for _, l := range u.sp.links(u.id) {
			if l.target == abs {
				_ = os.Remove(filepath.Join(dir, l.name))
			}
		}
	}
	return nil
}

// --- Opérations sur les schémas, en chemins virtuels -----------------------------

func isRoot(rel string) bool {
	c, err := Clean(rel)
	return err == nil && c == ""
}

// List renvoie le contenu d'un dossier. La racine commune commence par les
// dossiers virtuels Favoris, Personnel et Poubelle.
func (u *Space) List(p string) ([]Entry, error) {
	u.sp.autoPurge()
	switch cleanVirtual(p) {
	case FavPrefix:
		return u.Favorites()
	case TrashPrefix:
		return u.Trash()
	}
	st, rel, err := u.route(p)
	if err != nil {
		return nil, err
	}
	es, err := st.List(rel)
	if err != nil {
		return nil, err
	}
	favs := u.favSet()
	out := make([]Entry, 0, len(es)+2)
	if st == u.sp.Main && isRoot(rel) {
		out = append(out,
			Entry{Name: "Favoris", Path: FavPrefix, IsDir: true, Special: "favoris"},
			Entry{Name: "Personnel", Path: PersoPrefix, IsDir: true, Special: "perso"},
			Entry{Name: "Poubelle", Path: TrashPrefix, IsDir: true, Special: "poubelle",
				Count: u.sp.Main.TrashCount() + u.home.TrashCount()})
	}
	for _, e := range es {
		out = append(out, u.entry(st, e, favs))
	}
	return out, nil
}

// Search cherche dans l'espace personnel puis l'arborescence commune.
func (u *Space) Search(q string, f Filter, limit int) ([]Hit, error) {
	favs := u.favSet()
	inner := Filter{Tag: f.Tag}
	tag := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f.Tag), "#"))
	if f.Favorite && strings.TrimSpace(q) == "" && tag == "" {
		es, err := u.Favorites()
		out := []Hit{}
		for _, e := range es {
			if !e.Broken && len(out) < limit {
				out = append(out, Hit{Entry: e})
			}
		}
		return out, err
	}
	lim := limit
	if f.Favorite {
		lim = 1 << 30 // filtrés ensuite
	}
	var all []Hit
	for _, st := range []*Store{u.home, u.sp.Main} {
		hs, err := st.Search(q, inner, lim)
		if err != nil {
			return nil, err
		}
		for _, h := range hs {
			h.Entry = u.entry(st, h.Entry, favs)
			if !f.Favorite || h.Favorite {
				all = append(all, h)
			}
		}
	}
	// Correspondances sur le nom (sans extrait) d'abord, comme Store.Search.
	sort.SliceStable(all, func(i, j int) bool { return all[i].Snippet == "" && all[j].Snippet != "" })
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []Hit{}
	}
	return all, nil
}

// Tags réunit les tags de l'arborescence commune et de l'espace personnel.
func (u *Space) Tags() ([]TagCount, error) {
	idx := map[string]int{}
	out := []TagCount{}
	for _, st := range []*Store{u.sp.Main, u.home} {
		ts, err := st.Tags()
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			if i, ok := idx[fold(t.Name)]; ok {
				out[i].Count += t.Count
			} else {
				idx[fold(t.Name)] = len(out)
				out = append(out, t)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return fold(out[i].Name) < fold(out[j].Name) })
	return out, nil
}

// EditTags ajoute et retire des tags (voir Store.EditTags).
func (u *Space) EditTags(paths, add, remove []string) error {
	groups := map[*Store][]string{}
	for _, p := range paths {
		st, rel, err := u.route(p)
		if err != nil {
			return err
		}
		groups[st] = append(groups[st], rel)
	}
	for st, rels := range groups {
		if err := st.EditTags(rels, add, remove); err != nil {
			return err
		}
	}
	return nil
}

// SetIcon choisit l'icône de dossiers (voir Store.SetIcon).
func (u *Space) SetIcon(paths []string, icon string) error {
	groups := map[*Store][]string{}
	for _, p := range paths {
		st, rel, err := u.route(p)
		if err != nil {
			return err
		}
		groups[st] = append(groups[st], rel)
	}
	for st, rels := range groups {
		if err := st.SetIcon(rels, icon); err != nil {
			return err
		}
	}
	return nil
}

func (u *Space) Read(p string) ([]byte, string, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return nil, "", err
	}
	return st.Read(rel)
}

func (u *Space) Write(p string, data []byte, ifMatch string, create bool) (Entry, string, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return Entry{}, "", err
	}
	e, etag, err := st.Write(rel, data, ifMatch, create)
	return u.entry(st, e, nil), etag, err
}

func (u *Space) Mkdir(p string) (Entry, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return Entry{}, err
	}
	e, err := st.Mkdir(rel)
	return u.entry(st, e, nil), err
}

// Move renomme ou déplace un élément, y compris entre l'arborescence commune
// et l'espace personnel. Les favoris suivent.
func (u *Space) Move(from, to string) (Entry, error) {
	sf, rf, err := u.route(from)
	if err != nil {
		return Entry{}, err
	}
	st, rt, err := u.route(to)
	if err != nil {
		return Entry{}, err
	}
	if sf != st {
		e, err := u.sp.moveAcross(sf, rf, st, rt)
		return u.entry(st, e, nil), err
	}
	cf, _ := Clean(rf)
	e, err := sf.Move(rf, rt)
	if err != nil {
		return Entry{}, err
	}
	u.sp.relink(absOf(sf, cf), absOf(sf, e.Path))
	return u.entry(st, e, nil), nil
}

// moveAcross déplace un élément d'un Store à un autre (même volume) avec ses
// miniatures, son historique, ses tags et ses liens de partage.
func (sp *Spaces) moveAcross(src *Store, from string, dst *Store, to string) (Entry, error) {
	cf, af, err := src.resolve(from)
	if err != nil {
		return Entry{}, err
	}
	ct, at, err := dst.resolve(to)
	if err != nil {
		return Entry{}, err
	}
	if cf == "" || ct == "" {
		return Entry{}, ErrInvalidPath
	}
	fi, err := os.Lstat(af)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	if !fi.IsDir() {
		if err := requireSchemaPath(ct); err != nil {
			return Entry{}, err
		}
	}
	if _, err := os.Lstat(at); err == nil {
		return Entry{}, ErrExists
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(af, at); err != nil {
		return Entry{}, mapErr(err)
	}
	moveAside(src.thumbAbs(cf, fi.IsDir()), dst.thumbAbs(ct, fi.IsDir()))
	moveAside(src.historyAbs(cf), dst.historyAbs(ct))
	dst.putMeta(ct, src.takeMeta(cf))
	dst.putShares(ct, src.takeShares(cf))
	sp.relink(af, at)
	if fi, err = os.Stat(at); err != nil {
		return Entry{}, err
	}
	return toEntry(ct, fi), nil
}

// moveAside déplace src vers dst s'il existe (miniatures, historique).
func moveAside(src, dst string) {
	if _, err := os.Stat(src); err != nil {
		return
	}
	_ = os.RemoveAll(dst)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
		_ = os.Rename(src, dst)
	}
}

// MoveMany déplace plusieurs éléments dans le dossier dir (voir Store.MoveMany).
func (u *Space) MoveMany(paths []string, dir string) ([]MoveResult, error) {
	sd, rd, err := u.route(dir)
	if err != nil {
		return nil, err
	}
	cd, ad, err := sd.resolve(rd)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(ad)
	if err != nil {
		return nil, mapErr(err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%w : la destination n'est pas un dossier", ErrInvalidPath)
	}
	vd := u.virt(sd, cd)
	out := make([]MoveResult, 0, len(paths))
	for _, p := range paths {
		r := MoveResult{From: p}
		sf, rf, err := u.route(p)
		var c string
		if err == nil {
			c, err = Clean(rf)
		}
		switch {
		case err != nil || c == "":
			r.err = ErrInvalidPath
		case sf == sd && strings.TrimPrefix(path.Dir("/"+c), "/") == cd:
			r.From = u.virt(sf, c)
			r.To = r.From // déjà dans le dossier cible
		default:
			r.From = u.virt(sf, c)
			var e Entry
			if e, r.err = u.Move(r.From, path.Join(vd, path.Base(c))); r.err == nil {
				r.To = e.Path
			}
		}
		if r.err != nil {
			r.Error = r.err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

// Copy duplique un schéma, éventuellement vers l'autre espace.
func (u *Space) Copy(from, to string) (Entry, error) {
	sf, rf, err := u.route(from)
	if err != nil {
		return Entry{}, err
	}
	st, rt, err := u.route(to)
	if err != nil {
		return Entry{}, err
	}
	if sf == st {
		e, err := sf.Copy(rf, rt)
		return u.entry(st, e, nil), err
	}
	data, _, err := sf.Read(rf)
	if err != nil {
		return Entry{}, err
	}
	e, _, err := st.Write(rt, data, "", true)
	return u.entry(st, e, nil), err
}

// Delete met un élément à la corbeille de son espace ; les favoris qui le
// visent disparaissent. Sur .favoris/<lien>, seul le lien est retiré. Dans la
// poubelle, l'élément est supprimé définitivement (.poubelle seul : la vide).
func (u *Space) Delete(p string, recursive bool) error {
	if _, ok := favLinkName(p); ok {
		return u.SetFavorite([]string{p}, false)
	}
	if cleanVirtual(p) == TrashPrefix {
		return u.EmptyTrash()
	}
	if st, id, ok := u.trashItem(p); ok {
		return st.DropTrash(id)
	}
	st, rel, err := u.route(p)
	if err != nil {
		return err
	}
	_, abs, err := st.resolve(rel)
	if err != nil {
		return err
	}
	if err := st.deleteAs(rel, recursive, u.id); err != nil {
		return err
	}
	u.sp.unlink(abs)
	return nil
}

func (u *Space) ReadThumb(p string) ([]byte, time.Time, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return nil, time.Time{}, err
	}
	return st.ReadThumb(rel)
}

func (u *Space) WriteThumb(p string, png []byte) error {
	st, rel, err := u.route(p)
	if err != nil {
		return err
	}
	return st.WriteThumb(rel, png)
}

func (u *Space) Versions(p string) ([]Version, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return nil, err
	}
	return st.Versions(rel)
}

func (u *Space) ReadVersion(p, id string) ([]byte, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return nil, err
	}
	return st.ReadVersion(rel, id)
}

func (u *Space) Snapshot(p string) (Version, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return Version{}, err
	}
	return st.Snapshot(rel)
}

func (u *Space) Restore(p, id string) (Entry, string, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return Entry{}, "", err
	}
	e, etag, err := st.Restore(rel, id)
	return u.entry(st, e, nil), etag, err
}

// Shares liste les liens d'un schéma, ou tous ceux visibles (arborescence
// commune et espace personnel) si p est vide.
func (u *Space) Shares(p string) ([]Share, error) {
	stores := []*Store{u.sp.Main, u.home}
	rel := ""
	if p != "" {
		st, r, err := u.route(p)
		if err != nil {
			return nil, err
		}
		stores, rel = []*Store{st}, r
	}
	out := []Share{}
	for _, st := range stores {
		l, err := st.Shares(rel)
		if err != nil {
			return nil, err
		}
		for _, sh := range l {
			sh.Path = u.virt(st, sh.Path)
			out = append(out, sh)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func (u *Space) CreateShare(p string, ttl time.Duration) (Share, error) {
	st, rel, err := u.route(p)
	if err != nil {
		return Share{}, err
	}
	sh, err := st.CreateShare(rel, ttl)
	sh.Path = u.virt(st, sh.Path)
	return sh, err
}

// RevokeShare supprime un lien de l'arborescence commune ou de l'espace personnel.
func (u *Space) RevokeShare(id string) error {
	err := u.sp.Main.RevokeShare(id)
	if errors.Is(err, ErrNotFound) {
		err = u.home.RevokeShare(id)
	}
	return err
}

// RevokeAllShares révoque tous les liens, espaces personnels compris.
func (u *Space) RevokeAllShares() error {
	if err := u.sp.Main.RevokeAllShares(); err != nil {
		return err
	}
	for _, id := range u.sp.users() {
		if st, err := u.sp.home(id); err == nil {
			if err := st.RevokeAllShares(); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Transferts de métadonnées entre Stores ----------------------------------------

// takeMeta retire et renvoie les métadonnées de c et de son contenu (clés
// relatives à c).
func (s *Store) takeMeta(c string) map[string]itemMeta {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return nil
	}
	out := map[string]itemMeta{}
	for k, m := range s.meta {
		if under(k, c) {
			out[k[len(c):]] = m
			delete(s.meta, k)
		}
	}
	if len(out) > 0 {
		_ = s.saveMeta()
	}
	return out
}

func (s *Store) putMeta(c string, m map[string]itemMeta) {
	if len(m) == 0 {
		return
	}
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.loadMeta() != nil {
		return
	}
	for suffix, v := range m {
		s.meta[c+suffix] = v
	}
	_ = s.saveMeta()
}

func (s *Store) takeShares(c string) map[string]shareDisk {
	out := map[string]shareDisk{}
	_ = s.editShares(func(m map[string]shareDisk) (bool, error) {
		for id, d := range m {
			if under(d.Path, c) {
				d.Path = d.Path[len(c):] // suffixe relatif à c
				out[id] = d
				delete(m, id)
			}
		}
		return len(out) > 0, nil
	})
	return out
}

func (s *Store) putShares(c string, add map[string]shareDisk) {
	if len(add) == 0 {
		return
	}
	_ = s.editShares(func(m map[string]shareDisk) (bool, error) {
		for id, d := range add {
			d.Path = c + d.Path
			m[id] = d
		}
		return true, nil
	})
}

// --- Poubelle ---------------------------------------------------------------------

// trashItem traduit .poubelle/c-<id> ou .poubelle/p-<id> en Store et identifiant.
func (u *Space) trashItem(p string) (*Store, string, bool) {
	key, ok := strings.CutPrefix(cleanVirtual(p), TrashPrefix+"/")
	if !ok {
		return nil, "", false
	}
	switch {
	case strings.HasPrefix(key, "c-"):
		return u.sp.Main, key[2:], true
	case strings.HasPrefix(key, "p-"):
		return u.home, key[2:], true
	}
	return nil, "", false
}

// Trash liste la poubelle : suppressions de l'arborescence commune et de
// l'espace personnel, les plus récentes en premier. ModTime est la date de
// suppression, Origin le chemin d'origine.
func (u *Space) Trash() ([]Entry, error) {
	out := []Entry{}
	for _, st := range []*Store{u.sp.Main, u.home} {
		items, err := st.TrashItems()
		if err != nil {
			return nil, err
		}
		prefix := "c-"
		if st == u.home {
			prefix = "p-"
		}
		for _, it := range items {
			out = append(out, Entry{
				Name:    it.Name,
				Path:    TrashPrefix + "/" + prefix + it.ID,
				IsDir:   it.IsDir,
				Size:    it.Size,
				ModTime: it.Deleted,
				Origin:  u.virt(st, it.Path),
				By:      it.By,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// RestoreTrash remet des éléments de la poubelle à leur emplacement d'origine ;
// To est le chemin restauré (renommé si la place était prise).
func (u *Space) RestoreTrash(paths []string) []MoveResult {
	out := make([]MoveResult, 0, len(paths))
	for _, p := range paths {
		r := MoveResult{From: p}
		st, id, ok := u.trashItem(p)
		if !ok {
			r.Error = ErrInvalidPath.Error()
		} else if e, err := st.RestoreTrash(id); err != nil {
			r.Error = err.Error()
		} else {
			r.To = u.virt(st, e.Path)
		}
		out = append(out, r)
	}
	return out
}

// EmptyTrash vide la poubelle (corbeille commune comprise).
func (u *Space) EmptyTrash() error {
	return errors.Join(u.sp.Main.EmptyTrash(), u.home.EmptyTrash())
}

// purgeEvery espace les purges automatiques déclenchées par List.
const purgeEvery = 10 * time.Minute

func (sp *Spaces) autoPurge() {
	sp.purgeMu.Lock()
	due := time.Since(sp.lastPurge) >= purgeEvery
	if due {
		sp.lastPurge = time.Now()
	}
	sp.purgeMu.Unlock()
	if due {
		_ = sp.PurgeTrash()
	}
}

// PurgeTrash supprime définitivement, dans toutes les corbeilles, les éléments
// plus anciens que Settings.TrashDays (rien si 0).
func (sp *Spaces) PurgeTrash() error {
	st, err := sp.Main.ReadSettings()
	if err != nil || st.TrashDays <= 0 {
		return err
	}
	maxAge := time.Duration(st.TrashDays) * 24 * time.Hour
	errs := []error{sp.Main.PurgeTrashOlder(maxAge)}
	for _, id := range sp.users() {
		h, err := sp.home(id)
		if err == nil {
			err = h.PurgeTrashOlder(maxAge)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
