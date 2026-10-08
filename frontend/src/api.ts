// Client de l'API Go (voir internal/api/api.go).

export interface Entry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modTime: string;
  tags?: string[];
  favorite?: boolean;
}

export interface TagCount {
  name: string;
  count: number;
}

/** Filtres de recherche : tag exact et/ou favoris seulement. */
export interface SearchFilter {
  tag?: string | null;
  fav?: boolean;
}

/** Résultat de recherche ; snippet = extrait du contenu qui correspond. */
export interface Hit extends Entry {
  snippet?: string;
}

export interface MoveResult {
  from: string;
  to?: string;
  error?: string;
}

/** Canal npm d'Excalidraw : latest (stable) ou next (préversion). */
export type Channel = "latest" | "next";

/** Paramètres partagés ; logo = "" sans logo, sinon sa version. */
export interface Settings {
  title: string;
  logo: string;
  channel: Channel;
}

/** Versions d'Excalidraw publiées sur npm par canal (vide si inconnu ou désactivé). */
export interface UpdateStatus {
  enabled: boolean;
  tags?: Partial<Record<Channel, string>>;
  latest?: string;
  checkedAt?: string;
}

/** Version enregistrée d'un schéma (time : date de ce contenu). */
export interface Version {
  id: string;
  time: string;
  size: number;
}

/** Lien de partage en lecture seule (token : à placer dans /s/<token>). */
export interface Share {
  id: string;
  token: string;
  path: string;
  created: string;
  expires?: string;
}

/** Schéma partagé tel que servi sans authentification. */
export interface PublicScene {
  name: string;
  title: string;
  logo?: string;
  scene: { elements?: unknown[]; appState?: Record<string, unknown>; files?: Record<string, unknown> };
}

/** Session courante (GET /api/me) ; logout présent en mode OIDC. */
export interface Me {
  mode: "none" | "local" | "oidc";
  user?: { sub: string; name: string; email?: string };
  logout?: string;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request(url: string, init?: RequestInit): Promise<Response> {
  const res = await fetch(url, init);
  if (!res.ok) {
    // Session OIDC expirée : reconnexion, puis retour sur la page courante.
    const login = res.status === 401 ? res.headers.get("X-Auth-Login") : null;
    if (login) location.assign(`${login}?next=${encodeURIComponent(location.pathname + location.search + location.hash)}`);
    let msg = res.statusText;
    try {
      msg = (await res.json()).error ?? msg;
    } catch {
      /* corps non JSON */
    }
    throw new ApiError(res.status, msg);
  }
  return res;
}

const q = (p: string) => encodeURIComponent(p);
const json = (body: unknown): RequestInit => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

export const api = {
  update: async (): Promise<UpdateStatus> => (await request("/api/update")).json(),

  me: async (): Promise<Me> => (await request("/api/me")).json(),

  list: async (dir: string): Promise<Entry[]> =>
    (await request(`/api/list?path=${q(dir)}`)).json(),

  search: async (text: string, f: SearchFilter = {}): Promise<Hit[]> =>
    (
      await request(`/api/search?q=${q(text)}${f.tag ? `&tag=${q(f.tag)}` : ""}${f.fav ? "&fav=true" : ""}`)
    ).json(),

  tags: async (): Promise<TagCount[]> => (await request("/api/tags")).json(),

  setFavorite: async (paths: string[], favorite: boolean): Promise<void> => {
    await request("/api/meta/favorite", json({ paths, favorite }));
  },

  editTags: async (paths: string[], add: string[], remove: string[]): Promise<void> => {
    await request("/api/meta/tags", json({ paths, add, remove }));
  },

  read: async (path: string): Promise<{ text: string; etag: string }> => {
    const res = await request(`/api/file?path=${q(path)}`);
    return { text: await res.text(), etag: res.headers.get("ETag") ?? "" };
  },

  /** etag : "" = écrasement libre, "*" = création seule, sinon contrôle optimiste. */
  write: async (path: string, body: string, etag: string): Promise<string> => {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (etag === "*") headers["If-None-Match"] = "*";
    else if (etag) headers["If-Match"] = etag;
    const res = await request(`/api/file?path=${q(path)}`, { method: "PUT", headers, body });
    return res.headers.get("ETag") ?? "";
  },

  mkdir: async (path: string): Promise<Entry> => (await request("/api/folder", json({ path }))).json(),

  move: async (from: string, to: string): Promise<Entry> =>
    (await request("/api/move", json({ from, to }))).json(),

  /** Déplace plusieurs éléments dans dir ; le détail par élément indique les échecs. */
  moveMany: async (paths: string[], dir: string): Promise<MoveResult[]> =>
    (await request("/api/move-many", json({ paths, dir }))).json(),

  copy: async (from: string, to: string): Promise<Entry> =>
    (await request("/api/copy", json({ from, to }))).json(),

  remove: async (path: string, recursive: boolean): Promise<void> => {
    await request(`/api/entry?path=${q(path)}&recursive=${recursive}`, { method: "DELETE" });
  },

  history: async (path: string): Promise<Version[]> =>
    (await request(`/api/history?path=${q(path)}`)).json(),

  readVersion: async (path: string, id: string): Promise<string> =>
    (await request(`/api/history/version?path=${q(path)}&id=${q(id)}`)).text(),

  /** Crée une version du contenu courant (sans attendre l'intervalle automatique). */
  snapshot: async (path: string): Promise<Version> =>
    (await request("/api/history/snapshot", json({ path }))).json(),

  /** Restaure une version ; le contenu remplacé devient lui-même une version. */
  restore: async (path: string, id: string): Promise<Entry> =>
    (await request("/api/history/restore", json({ path, id }))).json(),

  versionUrl: (path: string, id: string) => `/api/history/version?path=${q(path)}&id=${q(id)}`,

  /** Liens d'un schéma, ou tous les liens si path est omis. */
  shares: async (path?: string): Promise<Share[]> =>
    (await request(`/api/shares${path ? `?path=${q(path)}` : ""}`)).json(),

  /** days = 0 : lien sans expiration. */
  createShare: async (path: string, days: number): Promise<Share> =>
    (await request("/api/shares", json({ path, days }))).json(),

  revokeShare: async (id: string): Promise<void> => {
    await request(`/api/shares?id=${q(id)}`, { method: "DELETE" });
  },

  revokeAllShares: async (): Promise<void> => {
    await request("/api/shares/revoke-all", { method: "POST" });
  },

  publicScene: async (token: string): Promise<PublicScene> =>
    (await request(`/api/public/${q(token)}`)).json(),

  shareUrl: (token: string) => `${location.origin}/s/${token}`,

  readLibrary: async (): Promise<unknown> => (await request("/api/library")).json(),

  writeLibrary: async (body: string): Promise<void> => {
    await request("/api/library", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
    });
  },

  settings: async (): Promise<Settings> => (await request("/api/settings")).json(),

  setTitle: async (title: string): Promise<Settings> =>
    (await request("/api/settings", { ...json({ title }), method: "PUT" })).json(),

  setChannel: async (channel: Channel): Promise<Settings> =>
    (await request("/api/settings", { ...json({ channel }), method: "PUT" })).json(),

  /** Le serveur recadre au centre et ramène le PNG à 64x64. */
  uploadLogo: async (png: Blob): Promise<Settings> =>
    (
      await request("/api/logo", { method: "PUT", headers: { "Content-Type": "image/png" }, body: png })
    ).json(),

  removeLogo: async (): Promise<Settings> => (await request("/api/logo", { method: "DELETE" })).json(),

  logoUrl: (v: string) => `/api/logo?v=${q(v)}`,

  downloadUrl: (path: string) => `/api/file?path=${q(path)}`,

  /** v = date du schéma : l'URL change à chaque nouvelle version (cache navigateur). */
  thumbUrl: (path: string, v: string) => `/api/thumb?path=${q(path)}&v=${q(v)}`,

  writeThumb: async (path: string, png: Blob): Promise<void> => {
    await request(`/api/thumb?path=${q(path)}`, {
      method: "PUT",
      headers: { "Content-Type": "image/png" },
      body: png,
    });
  },
};

// Helpers de chemins (format "slash", relatifs à la racine).
export const dirname = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "");
export const basename = (p: string) => p.slice(p.lastIndexOf("/") + 1);
export const join = (dir: string, name: string) => (dir ? `${dir}/${name}` : name);
export const EXT = ".excalidraw";
export const withExt = (name: string) => (name.toLowerCase().endsWith(EXT) ? name : name + EXT);
export const stripExt = (name: string) =>
  name.toLowerCase().endsWith(EXT) ? name.slice(0, -EXT.length) : name;
