// Client de l'API Go (voir internal/api/api.go).

export interface Entry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modTime: string;
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

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request(url: string, init?: RequestInit): Promise<Response> {
  const res = await fetch(url, init);
  if (!res.ok) {
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

  list: async (dir: string): Promise<Entry[]> =>
    (await request(`/api/list?path=${q(dir)}`)).json(),

  search: async (text: string): Promise<Entry[]> =>
    (await request(`/api/search?q=${q(text)}`)).json(),

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
