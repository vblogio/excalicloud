// Bibliothèque Excalidraw : stockage côté serveur et liens « Ajouter à Excalidraw »
// de libraries.excalidraw.com (#addLibrary=<url>&token=<id>).
import { loadLibraryFromBlob, parseLibraryTokensFromUrl, restoreLibraryItems } from "@excalidraw/excalidraw";
import type { LibraryItems } from "@excalidraw/excalidraw/types";

// Mêmes sources que le hook useHandleLibrary d'Excalidraw (validateLibraryUrl).
const ALLOWED = ["excalidraw.com", "raw.githubusercontent.com/excalidraw/excalidraw-libraries"];

/** URL de la bibliothèque à installer si le fragment est un lien addLibrary, sinon null. */
export const libraryUrlFromHash = (): string | null => parseLibraryTokensFromUrl()?.libraryUrl ?? null;

const isAllowed = (raw: string) => {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return false;
  }
  if (url.protocol !== "https:") return false;
  return ALLOWED.some((a) => {
    const [host, ...rest] = a.split("/");
    const path = rest.length ? `/${rest.join("/")}/` : "/";
    return (url.hostname === host || url.hostname.endsWith(`.${host}`)) && url.pathname.startsWith(path);
  });
};

/** Télécharge un .excalidrawlib depuis une source autorisée. */
export async function fetchLibraryItems(raw: string): Promise<LibraryItems> {
  const url = decodeURIComponent(raw);
  if (!isAllowed(url)) throw new Error(`source non autorisée (${url})`);
  let res: Response;
  try {
    res = await fetch(url);
  } catch {
    throw new Error("téléchargement impossible (réseau ou CORS)");
  }
  if (!res.ok) throw new Error(`téléchargement impossible (HTTP ${res.status})`);
  return loadLibraryFromBlob(await res.blob(), "published");
}

/** Contenu de GET /api/library (format .excalidrawlib, v1 ou v2). */
export const parseServerLibrary = (lib: unknown): LibraryItems => {
  const raw = (lib ?? {}) as { libraryItems?: unknown; library?: unknown };
  return restoreLibraryItems((raw.libraryItems ?? raw.library ?? []) as LibraryItems, "unpublished");
};
