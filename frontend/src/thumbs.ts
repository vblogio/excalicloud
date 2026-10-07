// Miniatures PNG des schémas : rendues dans le navigateur par Excalidraw
// (exportToBlob), stockées côté serveur dans .thumbs/ (voir storage/thumbs.go).

import { exportToBlob, restoreElements } from "@excalidraw/excalidraw";
import type { AppState, BinaryFiles } from "@excalidraw/excalidraw/types";
import type { ExcalidrawElement } from "@excalidraw/excalidraw/element/types";
import { api } from "./api";

const THUMB_PX = 320;

/** Rend la miniature d'une scène ; null si la scène est vide. */
export async function renderThumb(
  elements: readonly ExcalidrawElement[],
  appState: Partial<AppState>,
  files: BinaryFiles,
): Promise<Blob | null> {
  const live = elements.filter((e) => !e.isDeleted);
  if (live.length === 0) return null;
  return exportToBlob({
    elements: live,
    appState: {
      viewBackgroundColor: appState.viewBackgroundColor ?? "#ffffff",
      exportBackground: true,
      exportWithDarkMode: false, // le thème sombre est appliqué en CSS
      exportEmbedScene: false,
    },
    files,
    maxWidthOrHeight: THUMB_PX,
    exportPadding: 16,
    mimeType: "image/png",
  });
}

/** Rend puis envoie la miniature ; renvoie false si la scène est vide. */
export async function storeThumb(
  path: string,
  elements: readonly ExcalidrawElement[],
  appState: Partial<AppState>,
  files: BinaryFiles,
): Promise<boolean> {
  const png = await renderThumb(elements, appState, files);
  if (png) await api.writeThumb(path, png);
  return png !== null;
}

// --- Génération à la demande (schémas importés, anciens, modifiés ailleurs) ---

export type ThumbResult = "ok" | "empty" | "error";

const MAX_PARALLEL = 2;
const results = new Map<string, ThumbResult>(); // "chemin@date" -> résultat
const pending = new Map<string, Promise<ThumbResult>>();
const queue: (() => void)[] = [];
let running = 0;

const next = () => {
  while (running < MAX_PARALLEL && queue.length) {
    running++;
    queue.shift()!();
  }
};

async function generate(path: string): Promise<ThumbResult> {
  try {
    const { text } = await api.read(path);
    const data = JSON.parse(text);
    const elements = restoreElements(data.elements ?? [], null);
    return (await storeThumb(path, elements, data.appState ?? {}, data.files ?? {})) ? "ok" : "empty";
  } catch (e) {
    console.warn("miniature", path, e);
    return "error";
  }
}

/** Résultat déjà connu pour cette version du schéma (évite de régénérer en boucle). */
export const knownThumb = (path: string, version: string) => results.get(`${path}@${version}`);

/** Génère la miniature manquante d'un schéma (file d'attente, 2 en parallèle). */
export function ensureThumb(path: string, version: string): Promise<ThumbResult> {
  const key = `${path}@${version}`;
  const known = results.get(key);
  if (known) return Promise.resolve(known);
  let p = pending.get(key);
  if (!p) {
    p = new Promise<ThumbResult>((resolve) => {
      queue.push(() =>
        generate(path)
          .then((r) => {
            results.set(key, r);
            resolve(r);
          })
          .finally(() => {
            pending.delete(key);
            running--;
            next();
          }),
      );
      next();
    });
    pending.set(key, p);
  }
  return p;
}
