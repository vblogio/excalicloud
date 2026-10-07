// Client de l'API Go (voir internal/api/api.go).

export interface Entry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modTime: string;
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

  copy: async (from: string, to: string): Promise<Entry> =>
    (await request("/api/copy", json({ from, to }))).json(),

  remove: async (path: string, recursive: boolean): Promise<void> => {
    await request(`/api/entry?path=${q(path)}&recursive=${recursive}`, { method: "DELETE" });
  },

  readLibrary: async (): Promise<unknown> => (await request("/api/library")).json(),

  writeLibrary: async (body: string): Promise<void> => {
    await request("/api/library", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
    });
  },

  downloadUrl: (path: string) => `/api/file?path=${q(path)}`,
};

// Helpers de chemins (format "slash", relatifs à la racine).
export const dirname = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "");
export const basename = (p: string) => p.slice(p.lastIndexOf("/") + 1);
export const join = (dir: string, name: string) => (dir ? `${dir}/${name}` : name);
export const EXT = ".excalidraw";
export const withExt = (name: string) => (name.toLowerCase().endsWith(EXT) ? name : name + EXT);
export const stripExt = (name: string) =>
  name.toLowerCase().endsWith(EXT) ? name.slice(0, -EXT.length) : name;
