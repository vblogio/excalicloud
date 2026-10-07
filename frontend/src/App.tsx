import { useCallback, useEffect, useRef, useState } from "react";
import {
  Excalidraw,
  hashElementsVersion,
  restoreLibraryItems,
  serializeAsJSON,
  serializeLibraryAsJSON,
} from "@excalidraw/excalidraw";
import type {
  AppState,
  BinaryFiles,
  ExcalidrawImperativeAPI,
  ExcalidrawInitialDataState,
  LibraryItems,
} from "@excalidraw/excalidraw/types";
import type { OrderedExcalidrawElement } from "@excalidraw/excalidraw/element/types";
import { api, ApiError, basename, stripExt } from "./api";
import { Explorer } from "./Explorer";

type Theme = "light" | "dark";
type SaveState = "saved" | "dirty" | "saving" | "error";

interface Scene {
  id: number; // change à chaque ouverture -> remontage de l'éditeur
  path: string;
  initialData: ExcalidrawInitialDataState;
}

const AUTOSAVE_MS = 1500;
const isMobile = () => window.matchMedia("(max-width: 768px)").matches;
const pathFromHash = () => decodeURIComponent(location.hash.replace(/^#\/?/, "")) || null;

const initialTheme = (): Theme => {
  try {
    const t = localStorage.getItem("theme");
    if (t === "light" || t === "dark") return t;
  } catch {
    /* stockage indisponible */
  }
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
};

/** Empreinte du contenu « persistant » (hors scroll, sélection, zoom...). */
const sceneVersion = (
  elements: readonly OrderedExcalidrawElement[],
  appState: Pick<AppState, "viewBackgroundColor">,
  files: BinaryFiles,
) => `${hashElementsVersion(elements)}:${Object.keys(files).length}:${appState.viewBackgroundColor}`;

const SAVE_LABEL: Record<SaveState, string> = {
  saved: "Enregistré",
  dirty: "Modifié",
  saving: "Enregistrement…",
  error: "Erreur d'enregistrement",
};

export default function App() {
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const [library, setLibrary] = useState<LibraryItems | null>(null);
  const [scene, setScene] = useState<Scene | null>(null);
  const [saveState, setSaveState] = useState<SaveState>("saved");
  const [drawerOpen, setDrawerOpen] = useState(() => !isMobile());
  const [refreshKey, setRefreshKey] = useState(0);
  const [loadError, setLoadError] = useState<string | null>(null);

  const excalidraw = useRef<ExcalidrawImperativeAPI | null>(null);
  const pathRef = useRef<string | null>(null); // schéma ouvert (survit aux renommages)
  const etagRef = useRef("");
  const savedVersion = useRef("");
  const timer = useRef<number>(undefined);
  const inflight = useRef<Promise<void>>(Promise.resolve());
  const libTimer = useRef<number>(undefined);
  const libJson = useRef(""); // dernier état connu côté serveur
  const sceneSeq = useRef(0);
  const openRef = useRef<(path: string, opts?: { skipFlush?: boolean }) => Promise<void>>(async () => {});

  // --- Sauvegarde -----------------------------------------------------------

  const doSave = useCallback(async (force = false): Promise<void> => {
    const ex = excalidraw.current;
    const path = pathRef.current;
    if (!ex || !path) return;
    const elements = ex.getSceneElements();
    const appState = ex.getAppState();
    const files = ex.getFiles();
    const version = sceneVersion(elements, appState, files);
    if (version === savedVersion.current && !force) return;

    setSaveState("saving");
    const body = serializeAsJSON(elements, appState, files, "local");
    try {
      etagRef.current = await api.write(path, body, force ? "" : etagRef.current);
      savedVersion.current = version;
      // D'autres modifications ont pu arriver pendant l'envoi.
      const now = sceneVersion(ex.getSceneElements(), ex.getAppState(), ex.getFiles());
      setSaveState(now === version ? "saved" : "dirty");
      setRefreshKey((k) => k + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 412) {
        setSaveState("dirty");
        const overwrite = confirm(
          "Ce schéma a été modifié ailleurs depuis son ouverture.\n\n" +
            "OK : écraser avec votre version\nAnnuler : recharger la version du serveur (vos modifications seront perdues)",
        );
        if (overwrite) return doSave(true);
        savedVersion.current = version; // évite une re-sauvegarde avant rechargement
        await openRef.current(path, { skipFlush: true });
        return;
      }
      setSaveState("error");
      console.error(e);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** Sérialise les sauvegardes : jamais deux PUT concurrents sur le même fichier. */
  const save = useCallback(
    (force = false) => {
      clearTimeout(timer.current);
      inflight.current = inflight.current.then(() => doSave(force));
      return inflight.current;
    },
    [doSave],
  );

  const onChange = useCallback(
    (elements: readonly OrderedExcalidrawElement[], appState: AppState, files: BinaryFiles) => {
      if (appState.theme !== theme) setTheme(appState.theme as Theme);
      if (!pathRef.current || appState.isLoading) return;
      const version = sceneVersion(elements, appState, files);
      // Premier onChange après chargement : c'est la version de référence.
      if (savedVersion.current === "") {
        savedVersion.current = version;
        return;
      }
      if (version === savedVersion.current) return;
      setSaveState("dirty");
      clearTimeout(timer.current);
      timer.current = window.setTimeout(() => save(), AUTOSAVE_MS);
    },
    [save, theme],
  );

  // --- Ouverture / fermeture ------------------------------------------------

  const openFile = useCallback(
    async (path: string, opts: { skipFlush?: boolean } = {}) => {
      if (!opts.skipFlush) await save();
      setLoadError(null);
      try {
        const { text, etag } = await api.read(path);
        const data = JSON.parse(text);
        etagRef.current = etag;
        savedVersion.current = ""; // fixée au premier onChange (voir onChange)
        pathRef.current = path;
        setScene({
          id: ++sceneSeq.current,
          path,
          initialData: {
            elements: data.elements ?? [],
            appState: { ...(data.appState ?? {}), theme },
            files: data.files ?? {},
            scrollToContent: true,
          },
        });
        setSaveState("saved");
        if (location.hash !== `#/${path}`) history.replaceState(null, "", `#/${encodeURI(path)}`);
        if (isMobile()) setDrawerOpen(false);
      } catch (e) {
        setLoadError(`Impossible d'ouvrir « ${path} » : ${e instanceof Error ? e.message : e}`);
      }
    },
    [save, theme],
  );

  const closeFile = useCallback(() => {
    clearTimeout(timer.current);
    pathRef.current = null;
    excalidraw.current = null;
    setScene(null);
    history.replaceState(null, "", location.pathname);
  }, []);

  openRef.current = openFile;

  const onApi = useCallback((ex: ExcalidrawImperativeAPI) => {
    excalidraw.current = ex;
  }, []);

  // --- Effets globaux --------------------------------------------------------

  // Bibliothèque partagée, chargée une fois.
  useEffect(() => {
    api
      .readLibrary()
      .then((lib) => {
        const raw = lib as { libraryItems?: unknown; library?: unknown };
        const items = restoreLibraryItems((raw.libraryItems ?? raw.library ?? []) as LibraryItems, "unpublished");
        libJson.current = serializeLibraryAsJSON(items);
        setLibrary(items);
      })
      .catch(() => setLibrary([]));
  }, []);

  const onLibraryChange = useCallback((items: LibraryItems) => {
    clearTimeout(libTimer.current);
    libTimer.current = window.setTimeout(() => {
      const json = serializeLibraryAsJSON(items);
      if (json === libJson.current) return; // appel initial au montage
      libJson.current = json;
      api.writeLibrary(json).catch(console.error);
    }, 500);
  }, []);

  // Lien direct #/chemin/schema.excalidraw et navigation arrière/avant.
  useEffect(() => {
    const onHash = () => {
      const p = pathFromHash();
      if (p && p !== pathRef.current) openFile(p);
    };
    onHash();
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Ctrl/Cmd+S : sauvegarde immédiate (intercepte l'export disque d'Excalidraw).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        e.stopPropagation();
        save();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [save]);

  // Alerte si fermeture de l'onglet avec des modifications non enregistrées.
  useEffect(() => {
    const onUnload = (e: BeforeUnloadEvent) => {
      if (saveState !== "saved") e.preventDefault();
    };
    window.addEventListener("beforeunload", onUnload);
    return () => window.removeEventListener("beforeunload", onUnload);
  }, [saveState]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("theme", theme);
    } catch {
      /* stockage indisponible */
    }
  }, [theme]);

  // --- Synchronisation avec l'explorateur -----------------------------------

  const onMoved = useCallback((from: string, to: string) => {
    const cur = pathRef.current;
    if (!cur) return;
    if (cur === from || cur.startsWith(from + "/")) {
      const next = to + cur.slice(from.length);
      pathRef.current = next;
      setScene((s) => (s ? { ...s, path: next } : s));
      history.replaceState(null, "", `#/${encodeURI(next)}`);
    }
  }, []);

  const onDeleted = useCallback(
    (path: string) => {
      const cur = pathRef.current;
      if (cur && (cur === path || cur.startsWith(path + "/"))) closeFile();
    },
    [closeFile],
  );

  // --- Rendu -----------------------------------------------------------------

  return (
    <div className={`app ${drawerOpen ? "drawer-open" : ""}`}>
      <header className="topbar">
        <button
          className="icon-btn"
          aria-label="Afficher/masquer l'explorateur"
          onClick={() => setDrawerOpen((o) => !o)}
        >
          ☰
        </button>
        <span className="title" title={scene?.path}>
          {scene ? stripExt(basename(scene.path)) : "Excalidraw"}
        </span>
        {scene && (
          <>
            <span className={`status ${saveState}`}>{SAVE_LABEL[saveState]}</span>
            <button onClick={() => save()} disabled={saveState === "saved" || saveState === "saving"}>
              Enregistrer
            </button>
            <button className="icon-btn" aria-label="Fermer le schéma" onClick={() => save().then(closeFile)}>
              ✕
            </button>
          </>
        )}
      </header>

      <aside className="drawer">
        <Explorer
          current={scene?.path ?? null}
          refreshKey={refreshKey}
          onOpen={(p) => openFile(p)}
          onMoved={onMoved}
          onDeleted={onDeleted}
        />
      </aside>
      <div className="backdrop" onClick={() => setDrawerOpen(false)} />

      <main className="canvas">
        {loadError && <div className="error">{loadError}</div>}
        {scene && library ? (
          <Excalidraw
            // Remontage complet à chaque ouverture : historique et état propres.
            key={scene.id}
            initialData={{ ...scene.initialData, libraryItems: library }}
            excalidrawAPI={onApi}
            onChange={onChange}
            onLibraryChange={onLibraryChange}
            theme={theme}
            langCode="fr-FR"
            name={stripExt(basename(scene.path))}
            UIOptions={{
              canvasActions: { loadScene: false, saveToActiveFile: false, toggleTheme: true },
            }}
          />
        ) : (
          <div className="placeholder muted">
            <p>Sélectionnez un schéma dans l'explorateur ou créez-en un nouveau.</p>
          </div>
        )}
      </main>
    </div>
  );
}
