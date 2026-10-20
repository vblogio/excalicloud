import { type CSSProperties, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  Excalidraw,
  hashElementsVersion,
  mergeLibraryItems,
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
import { api, ApiError, basename, dirname, label, stripExt, type Settings } from "./api";
import { Explorer, initialView, type View } from "./Explorer";
import { CloseIcon, HistoryIcon, MenuIcon, MoonIcon, PinIcon, SaveIcon, SettingsIcon, ShareIcon, SunIcon, UpdateIcon } from "./icons";
import { ShareDialog } from "./ShareDialog";
import { HistoryDialog } from "./HistoryDialog";
import { Splitter } from "./Splitter";
import { SettingsDialog, DEFAULT_TITLE } from "./SettingsDialog";
import { pendingVersion, useUpdateStatus } from "./update";
import { storeThumb } from "./thumbs";
import { fetchLibraryItems, libraryUrlFromHash, parseServerLibrary } from "./library";
import { fitScene } from "./fit";
import { UserMenu } from "./UserMenu";

type Theme = "light" | "dark";
type SaveState = "saved" | "dirty" | "saving" | "error";

interface Scene {
  id: number; // change à chaque ouverture -> remontage de l'éditeur
  path: string;
  initialData: ExcalidrawInitialDataState;
}

const AUTOSAVE_MS = 1500;
const LOGO_LINK = "https://vblog.io";
const isMobile = () => window.matchMedia("(max-width: 768px)").matches;
const pathFromHash = () => decodeURIComponent(location.hash.replace(/^#\/?/, "")) || null;

// « Ajouter à Excalidraw » sur libraries.excalidraw.com revient vers la fenêtre nommée
// window.name : avec un nom propre à l'onglet, l'ajout arrive ici sans ouvrir d'onglet.
if (!window.name) window.name = `excalicloud-${Math.random().toString(36).slice(2)}`;

const load = (key: string) => {
  try {
    return localStorage.getItem(key);
  } catch {
    return null; // stockage indisponible
  }
};
const store = (key: string, value: string) => {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* stockage indisponible */
  }
};

const initialTheme = (): Theme => {
  const t = load("theme");
  if (t === "light" || t === "dark") return t;
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
};

// Largeur de l'explorateur (bureau) : réglable par la poignée, mémorisée par navigateur.
const DRAWER_DEFAULT = 330;
const DRAWER_MIN = 220;
const drawerMax = () => Math.max(DRAWER_MIN, window.innerWidth - 360); // garde un canevas utilisable
const clampDrawer = (w: number) => Math.round(Math.min(Math.max(w, DRAWER_MIN), drawerMax()));
const initialDrawerWidth = () => clampDrawer(Number(load("drawerWidth")) || DRAWER_DEFAULT);

// Explorateur épinglé (colonne fixe) ou non (masqué, s'ouvre par-dessus le canevas).
const initialPinned = () => load("drawerPinned") !== "false";
const PEEK_OPEN_MS = 150; // survol du bord gauche avant ouverture
const PEEK_CLOSE_MS = 400; // sortie du panneau avant fermeture (ouverture par survol)

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
  const [pinned, setPinned] = useState(initialPinned);
  // Vue Galerie : l'explorateur couvre toute la zone de travail, comme un panneau
  // flottant (il se referme à l'ouverture d'un schéma).
  const [gallery, setGallery] = useState(() => initialView() === "gallery");
  // Non épinglé ou Galerie : ouvert d'office tant qu'aucun schéma n'est demandé.
  const [drawerOpen, setDrawerOpen] = useState(() => !isMobile() && ((pinned && !gallery) || !pathFromHash()));
  const [instant, setInstant] = useState(false); // bascule sans animation de la grille
  const [refreshKey, setRefreshKey] = useState(0);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [drawerWidth, setDrawerWidth] = useState(initialDrawerWidth);
  const [resizing, setResizing] = useState(false);
  const [settings, setSettings] = useState<Settings>({ title: "", logo: "", channel: "latest", trashDays: 0, sort: "name" });
  const updateStatus = useUpdateStatus();
  const pending = pendingVersion(updateStatus, settings.channel);
  const [customizing, setCustomizing] = useState<false | "all" | "excalidraw">(false);
  const [historyFor, setHistoryFor] = useState<string | null>(null);
  const [shareFor, setShareFor] = useState<string | null>(null);
  const [browseDir, setBrowseDir] = useState(""); // dossier affiché par l'explorateur
  const [goto, setGoto] = useState<{ dir: string; view: View; seq: number }>();

  const excalidraw = useRef<ExcalidrawImperativeAPI | null>(null);
  const pathRef = useRef<string | null>(null); // schéma ouvert (survit aux renommages)
  const etagRef = useRef("");
  const savedVersion = useRef("");
  const timer = useRef<number>(undefined);
  const inflight = useRef<Promise<void>>(Promise.resolve());
  const libTimer = useRef<number>(undefined);
  const libJson = useRef(""); // dernier état connu côté serveur
  const libMenuPending = useRef(false); // ouvrir la bibliothèque avec le prochain schéma
  const sceneSeq = useRef(0);
  const fitPending = useRef(false); // cadrage à faire au premier rendu du schéma ouvert
  const exTheme = useRef<string | null>(null); // dernier thème signalé par Excalidraw
  const openRef = useRef<(path: string, opts?: { skipFlush?: boolean }) => Promise<void>>(async () => {});
  const pinnedRef = useRef(pinned);
  pinnedRef.current = pinned;
  const galleryRef = useRef(gallery);
  galleryRef.current = gallery;
  const peek = useRef<"hover" | "click" | null>(null); // comment le panneau non épinglé a été ouvert
  const peekTimer = useRef<number>(undefined);
  const scrollShift = useRef(0); // décalage du canevas à compenser après une bascule épinglé/non épinglé

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
      // Miniature pour l'explorateur ; un échec n'affecte pas la sauvegarde.
      await storeThumb(path, elements, appState, files).catch((e) => console.warn("miniature", e));
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
      // Bascule depuis le menu d'Excalidraw. On ne réagit qu'à un changement
      // signalé par Excalidraw : sinon, après un clic sur le bouton de la barre,
      // un onChange portant encore l'ancien thème le rétablirait (boucle).
      if (appState.theme !== exTheme.current) {
        exTheme.current = appState.theme;
        if (appState.theme !== theme) setTheme(appState.theme as Theme);
      }
      if (!pathRef.current || appState.isLoading) return;
      if (fitPending.current && excalidraw.current) {
        fitPending.current = false;
        fitScene(excalidraw.current);
      }
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
      const showLibrary = libMenuPending.current;
      libMenuPending.current = false;
      try {
        const { text, etag } = await api.read(path);
        const data = JSON.parse(text);
        etagRef.current = etag;
        savedVersion.current = ""; // fixée au premier onChange (voir onChange)
        pathRef.current = path;
        fitPending.current = true;
        setScene({
          id: ++sceneSeq.current,
          path,
          initialData: {
            elements: data.elements ?? [],
            appState: {
              ...(data.appState ?? {}),
              theme,
              ...(showLibrary ? { openSidebar: { name: "default", tab: "library" } } : {}),
            },
            files: data.files ?? {},
          },
        });
        setSaveState("saved");
        if (location.hash !== `#/${path}`) history.replaceState(null, "", `#/${encodeURI(path)}`);
        store("lastScene", path);
        if (isMobile() || !pinnedRef.current || galleryRef.current) setDrawerOpen(false);
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
    // Non épinglé ou Galerie : l'explorateur revient pour choisir le schéma suivant.
    if ((!pinnedRef.current || galleryRef.current) && !isMobile()) {
      peek.current = "click";
      setDrawerOpen(true);
    }
  }, []);

  openRef.current = openFile;

  /** Le schéma ouvert est-il entièrement enregistré ? */
  const isSaved = () => {
    const ex = excalidraw.current;
    if (!ex) return true;
    return sceneVersion(ex.getSceneElements(), ex.getAppState(), ex.getFiles()) === savedVersion.current;
  };

  /**
   * Clic dans le chemin de la barre du haut : le dossier s'affiche en Galerie à la
   * place du schéma, qui n'est fermé qu'une fois enregistré (sinon il reste dessous).
   */
  const browse = async (dir: string) => {
    if (pathRef.current) {
      await save();
      if (isSaved()) closeFile();
    }
    setGoto((g) => ({ dir, view: "gallery", seq: (g?.seq ?? 0) + 1 }));
    clearTimeout(peekTimer.current);
    peek.current = "click";
    setGallery(true);
    setDrawerOpen(true);
  };

  const onApi = useCallback((ex: ExcalidrawImperativeAPI | null) => {
    excalidraw.current = ex;
  }, []);
  // La prop s'appelle excalidrawAPI jusqu'à la 0.18 et onExcalidrawAPI sur le canal
  // next : on passe les deux pour que l'image se construise avec l'un ou l'autre.
  const apiProps = { excalidrawAPI: onApi, onExcalidrawAPI: onApi } as object;

  // --- Effets globaux --------------------------------------------------------

  // Bibliothèque partagée entre tous les schémas, stockée côté serveur.
  const readLibrary = useCallback(async () => {
    const items = parseServerLibrary(await api.readLibrary());
    libJson.current = serializeLibraryAsJSON(items);
    return items;
  }, []);

  useEffect(() => {
    readLibrary()
      .then((items) => setLibrary((cur) => cur ?? items))
      .catch(() => setLibrary((cur) => cur ?? []));
  }, [readLibrary]);

  const onLibraryChange = useCallback((items: LibraryItems) => {
    clearTimeout(libTimer.current);
    libTimer.current = window.setTimeout(() => {
      const json = serializeLibraryAsJSON(items);
      if (json === libJson.current) return; // appel initial au montage
      libJson.current = json;
      setLibrary(items); // reprise à l'ouverture du schéma suivant
      api.writeLibrary(json).catch(console.error);
    }, 500);
  }, []);

  // Retour sur l'onglet : reprend la bibliothèque modifiée ailleurs (autre onglet,
  // autre poste) pour ne pas l'écraser ensuite avec une copie périmée.
  useEffect(() => {
    const onVisible = () => {
      if (document.hidden) return;
      const before = libJson.current;
      readLibrary()
        .then((items) => {
          if (libJson.current === before) return;
          setLibrary(items);
          excalidraw.current?.updateLibrary({ libraryItems: items });
        })
        .catch(() => {});
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [readLibrary]);

  /** Lien « Ajouter à Excalidraw » de libraries.excalidraw.com (#addLibrary=<url>&token=…). */
  const addLibrary = useCallback(
    async (url: string) => {
      try {
        const incoming = await fetchLibraryItems(url);
        if (!confirm(`Ajouter ${incoming.length} élément(s) à la bibliothèque ?\n\n${decodeURIComponent(url)}`)) return;
        const ex = excalidraw.current;
        if (ex) {
          // Schéma ouvert : fusion dans l'éditeur, onLibraryChange enregistre côté serveur.
          await ex.updateLibrary({ libraryItems: incoming, merge: true, openLibraryMenu: true });
          return;
        }
        const items = mergeLibraryItems(await readLibrary(), incoming);
        const json = serializeLibraryAsJSON(items);
        await api.writeLibrary(json);
        libJson.current = json;
        setLibrary(items);
        // Onglet ouvert par le site des bibliothèques : on rouvre le dernier schéma.
        const last = load("lastScene");
        if (last) {
          libMenuPending.current = true;
          await openRef.current(last);
        }
      } catch (e) {
        setLoadError(`Impossible d'ajouter la bibliothèque : ${e instanceof Error ? e.message : e}`);
      }
    },
    [readLibrary],
  );

  // Lien direct #/chemin/schema.excalidraw et navigation arrière/avant.
  useEffect(() => {
    const onHash = () => {
      const lib = libraryUrlFromHash();
      if (lib) {
        // Le fragment addLibrary n'est pas un chemin de schéma : on le retire.
        const cur = pathRef.current;
        history.replaceState(null, "", cur ? `#/${encodeURI(cur)}` : location.pathname);
        addLibrary(lib);
        return;
      }
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
    store("theme", theme);
  }, [theme]);

  // Largeur mémorisée en fin de glissement ; réajustée si la fenêtre rétrécit.
  useEffect(() => {
    if (!resizing) store("drawerWidth", String(drawerWidth));
  }, [drawerWidth, resizing]);

  useEffect(() => {
    const onResize = () => setDrawerWidth((w) => clampDrawer(w));
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  // --- Explorateur épinglé ou flottant ------------------------------------------

  useEffect(() => {
    store("drawerPinned", String(pinned));
  }, [pinned]);

  const togglePin = () => {
    const ex = excalidraw.current;
    // Panneau visible : le bord gauche du canevas se déplace de sa largeur. Le
    // défilement est compensé pour que le schéma reste en place à l'écran.
    if (ex && drawerOpen && !isMobile()) scrollShift.current = pinned ? -drawerWidth : drawerWidth;
    peek.current = "hover"; // désépinglé sous le pointeur : se referme quand il en sort
    setInstant(true);
    setPinned((p) => !p);
  };

  useLayoutEffect(() => {
    const shift = scrollShift.current;
    const ex = excalidraw.current;
    scrollShift.current = 0;
    if (shift && ex) {
      const { scrollX, zoom } = ex.getAppState();
      ex.updateScene({ appState: { scrollX: scrollX - shift / zoom.value } });
    }
    if (!instant) return;
    const id = requestAnimationFrame(() => setInstant(false));
    return () => cancelAnimationFrame(id);
  }, [pinned, instant]);

  // Panneau par-dessus le schéma : non épinglé, ou Galerie quand un schéma est ouvert.
  const floating = drawerOpen && (!pinned || (gallery && !!scene));

  const onViewChange = useCallback((v: View) => setGallery(v === "gallery"), []);

  const peekOpen = () => {
    clearTimeout(peekTimer.current);
    peekTimer.current = window.setTimeout(() => {
      peek.current = "hover";
      setDrawerOpen(true);
    }, PEEK_OPEN_MS);
  };
  const peekCancel = () => clearTimeout(peekTimer.current);
  const peekLeave = () => {
    if (pinnedRef.current || peek.current !== "hover" || isMobile()) return;
    clearTimeout(peekTimer.current);
    peekTimer.current = window.setTimeout(() => setDrawerOpen(false), PEEK_CLOSE_MS);
  };
  useEffect(() => () => clearTimeout(peekTimer.current), []);

  // Échap referme le panneau flottant (sauf si un dialogue ou la sélection l'utilise).
  useEffect(() => {
    if (!floating) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || document.querySelector(".modal-backdrop")) return;
      setDrawerOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [floating]);

  // Titre et logo personnalisés (partagés, stockés côté serveur).
  useEffect(() => {
    api.settings().then(setSettings).catch(console.error);
  }, []);

  const appTitle = settings.title || DEFAULT_TITLE;
  const sceneName = scene ? stripExt(basename(scene.path)) : null;
  // Chemin de la barre du haut : dossier et nom du schéma ouvert, sinon (ou Galerie
  // affichée par-dessus) dossier de l'explorateur. Premier élément vide : la racine.
  const sceneShown = !!scene && !(gallery && drawerOpen);
  const shownDir = sceneShown && scene ? dirname(scene.path) : browseDir;
  const pathParts = ["", ...(shownDir ? shownDir.split("/") : [])];
  useEffect(() => {
    document.title = sceneName ? `${sceneName} – ${appTitle}` : appTitle;
  }, [appTitle, sceneName]);

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

  // --- Historique -------------------------------------------------------------

  /** Les modifications en cours sont enregistrées avant d'afficher l'historique. */
  const showHistory = useCallback(
    async (path: string) => {
      if (path === pathRef.current) await save();
      setHistoryFor(path);
    },
    [save],
  );

  const restoreVersion = useCallback(
    async (path: string, id: string) => {
      const open = path === pathRef.current;
      if (open) await save();
      await api.restore(path, id);
      if (open) await openFile(path, { skipFlush: true });
      setRefreshKey((k) => k + 1);
    },
    [save, openFile],
  );

  // --- Rendu -----------------------------------------------------------------

  return (
    <div
      className={`app ${drawerOpen ? "drawer-open" : ""} ${pinned ? "pinned" : "unpinned"} ${gallery ? "gallery" : ""} ${resizing ? "resizing" : ""} ${instant ? "instant" : ""} ${scene ? "has-scene" : ""}`}
      style={{ "--drawer-w": `${drawerWidth}px` } as CSSProperties}
    >
      <header className="topbar">
        <button
          className={`ex-btn square ${drawerOpen ? "active" : ""}`}
          aria-label="Afficher/masquer l'explorateur"
          aria-pressed={drawerOpen}
          title="Explorateur"
          onClick={() => {
            clearTimeout(peekTimer.current);
            peek.current = "click";
            setDrawerOpen((o) => !o);
          }}
        >
          <MenuIcon />
        </button>
        <a className="brand" href={LOGO_LINK} target="_blank" rel="noopener" title={LOGO_LINK.replace(/^https:\/\//, "")}>
          {settings.logo ? (
            <img className="logo" src={api.logoUrl(settings.logo)} alt={appTitle} />
          ) : (
            <span className="brand-title">{appTitle}</span>
          )}
        </a>
        <nav className="path" aria-label="Emplacement">
          {pathParts.map((part, i) => {
            const p = pathParts.slice(1, i + 1).join("/");
            return (
              <span key={p} className="seg">
                {i > 0 && (
                  <span className="sep muted" aria-hidden>
                    /
                  </span>
                )}
                <button onClick={() => browse(p)} title={i ? `Afficher « ${label(part)} »` : "Afficher la racine"}>
                  {i ? label(part) : "Racine"}
                </button>
              </span>
            );
          })}
          {sceneShown && sceneName && (
            <span className="seg current">
              <span className="sep muted" aria-hidden>
                /
              </span>
              <span className="scene-name" aria-current="page" title={sceneName}>
                {sceneName}
              </span>
            </span>
          )}
        </nav>
        {scene && (
          <>
            <span className={`status ${saveState}`}>{SAVE_LABEL[saveState]}</span>
            <button
              className="ex-btn"
              onClick={() => save()}
              disabled={saveState === "saved" || saveState === "saving"}
              title="Enregistrer (enregistrement automatique)"
            >
              <SaveIcon />
              <span className="save-label">Enregistrer</span>
            </button>
            <button
              className="icon-btn"
              aria-label="Historique des versions"
              title="Historique des versions"
              onClick={() => showHistory(scene.path)}
            >
              <HistoryIcon />
            </button>
            <button
              className="icon-btn"
              aria-label="Partager en lecture seule"
              title="Partager en lecture seule"
              onClick={() => save().then(() => setShareFor(scene.path))}
            >
              <ShareIcon />
            </button>
            <button
              className="icon-btn"
              aria-label="Fermer le schéma"
              title="Fermer le schéma"
              onClick={() => save().then(closeFile)}
            >
              <CloseIcon />
            </button>
          </>
        )}
        <div className="topbar-tools">
          {pending && (
            <button
              className="icon-btn update-btn"
              title={`Excalidraw ${pending} disponible (installée : ${__EXCALIDRAW_VERSION__}) — reconstruire l'image`}
              aria-label={`Mise à jour d'Excalidraw disponible : ${pending}`}
              onClick={() => setCustomizing("excalidraw")}
            >
              <UpdateIcon />
            </button>
          )}
          <button
            className="icon-btn"
            aria-label={theme === "dark" ? "Passer en mode clair" : "Passer en mode sombre"}
            title={theme === "dark" ? "Mode clair" : "Mode sombre"}
            onClick={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
          >
            {theme === "dark" ? <SunIcon /> : <MoonIcon />}
          </button>
          <button
            className="icon-btn"
            aria-label="Paramètres"
            title="Paramètres (titre, logo, version d'Excalidraw)"
            onClick={() => setCustomizing("all")}
          >
            <SettingsIcon />
          </button>
          <UserMenu beforeLogout={save} />
        </div>
      </header>

      <aside className="drawer" onMouseEnter={peekCancel} onMouseLeave={peekLeave}>
        <Explorer
          current={scene?.path ?? null}
          refreshKey={refreshKey}
          onOpen={(p) => openFile(p)}
          onMoved={onMoved}
          onDeleted={onDeleted}
          onHistory={showHistory}
          onShare={setShareFor}
          onViewChange={onViewChange}
          onDirChange={setBrowseDir}
          goto={goto}
          defaultSort={settings.sort}
          toolbarEnd={
            <button
              className={`icon-btn pin-btn ${pinned ? "on" : ""}`}
              aria-pressed={pinned}
              aria-label={pinned ? "Désépingler l'explorateur" : "Épingler l'explorateur"}
              title={
                pinned
                  ? "Désépingler : l'explorateur se masque et s'ouvre par-dessus le schéma (bouton ☰ ou survol du bord gauche)"
                  : "Épingler l'explorateur à côté du schéma"
              }
              onClick={togglePin}
            >
              <PinIcon />
            </button>
          }
        />
      </aside>
      <Splitter
        width={drawerWidth}
        min={DRAWER_MIN}
        max={drawerMax()}
        onResize={(w) => setDrawerWidth(clampDrawer(w))}
        onDragging={setResizing}
        onReset={() => setDrawerWidth(clampDrawer(DRAWER_DEFAULT))}
      />
      <div className="backdrop" onClick={() => setDrawerOpen(false)} />
      {!pinned && !drawerOpen && !gallery && (
        <div className="peek-edge" aria-hidden onMouseEnter={peekOpen} onMouseLeave={peekCancel} />
      )}

      <main className="canvas">
        {loadError && <div className="error">{loadError}</div>}
        {scene && library ? (
          <Excalidraw
            // Remontage complet à chaque ouverture : historique et état propres.
            key={scene.id}
            initialData={{ ...scene.initialData, libraryItems: library }}
            {...apiProps}
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

      {historyFor && (
        <HistoryDialog
          path={historyFor}
          theme={theme}
          onRestore={(id) => restoreVersion(historyFor, id)}
          onCopied={(p) => {
            setRefreshKey((k) => k + 1);
            openFile(p);
          }}
          onClose={() => setHistoryFor(null)}
        />
      )}

      {shareFor && <ShareDialog path={shareFor} onClose={() => setShareFor(null)} />}

      {customizing && (
        <SettingsDialog
          settings={settings}
          update={updateStatus}
          focusExcalidraw={customizing === "excalidraw"}
          onChange={setSettings}
          onClose={() => setCustomizing(false)}
        />
      )}
    </div>
  );
}
