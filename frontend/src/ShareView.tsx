import { useCallback, useEffect, useRef, useState } from "react";
import { Excalidraw } from "@excalidraw/excalidraw";
import type { AppState, ExcalidrawImperativeAPI, ExcalidrawInitialDataState } from "@excalidraw/excalidraw/types";
import { api, type PublicScene } from "./api";
import { DEFAULT_TITLE } from "./SettingsDialog";
import { fitScene } from "./fit";
import { DownloadIcon, EyeIcon, MoonIcon, SunIcon } from "./icons";

type Theme = "light" | "dark";

const initialTheme = (): Theme => {
  try {
    const t = localStorage.getItem("theme");
    if (t === "light" || t === "dark") return t;
  } catch {
    /* stockage indisponible */
  }
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
};

/** Vue publique d'un schéma partagé (/s/<jeton>) : lecture seule, sans explorateur. */
export function ShareView({ token }: { token: string }) {
  const [data, setData] = useState<PublicScene | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const excalidraw = useRef<ExcalidrawImperativeAPI | null>(null);
  const fitted = useRef(false);

  const onApi = useCallback((ex: ExcalidrawImperativeAPI | null) => {
    excalidraw.current = ex;
  }, []);
  // excalidrawAPI jusqu'à la 0.18, onExcalidrawAPI sur le canal next.
  const apiProps = { excalidrawAPI: onApi, onExcalidrawAPI: onApi } as object;

  // Premier rendu : cadrage sur l'ensemble du schéma.
  const onChange = useCallback((_elements: unknown, appState: AppState) => {
    if (appState.isLoading || fitted.current || !excalidraw.current) return;
    fitted.current = true;
    fitScene(excalidraw.current);
  }, []);

  useEffect(() => {
    api
      .publicScene(token)
      .then(setData)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [token]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const title = data?.title || DEFAULT_TITLE;
  useEffect(() => {
    document.title = data ? `${data.name} – ${title}` : title;
  }, [data, title]);

  const download = () => {
    if (!data) return;
    const url = URL.createObjectURL(new Blob([JSON.stringify(data.scene)], { type: "application/json" }));
    const a = Object.assign(document.createElement("a"), { href: url, download: `${data.name}.excalidraw` });
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  const initialData: ExcalidrawInitialDataState | null = data && {
    elements: (data.scene.elements ?? []) as ExcalidrawInitialDataState["elements"],
    appState: { ...(data.scene.appState ?? {}), theme } as ExcalidrawInitialDataState["appState"],
    files: (data.scene.files ?? {}) as ExcalidrawInitialDataState["files"],
  };

  return (
    <div className="share-view">
      <header className="topbar">
        <span className="brand">
          {data?.logo && <img className="logo" src={data.logo} alt="" />}
          <span className="brand-title">{title}</span>
        </span>
        {data ? (
          <span className="title" title={data.name}>
            <span className="sep muted" aria-hidden>
              /
            </span>
            {data.name}
          </span>
        ) : (
          <span className="spacer" />
        )}
        <span className="status readonly" title="Ce schéma est partagé en lecture seule">
          <EyeIcon size={14} />
          <span className="save-label">Lecture seule</span>
        </span>
        <div className="topbar-tools">
          {data && (
            <button className="icon-btn" onClick={download} title="Télécharger le fichier .excalidraw" aria-label="Télécharger">
              <DownloadIcon />
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
        </div>
      </header>
      <main className="canvas">
        {initialData ? (
          <Excalidraw
            initialData={initialData}
            viewModeEnabled
            {...apiProps}
            onChange={onChange}
            theme={theme}
            langCode="fr-FR"
            name={data!.name}
            UIOptions={{
              canvasActions: {
                loadScene: false,
                saveToActiveFile: false,
                clearCanvas: false,
                changeViewBackgroundColor: false,
                toggleTheme: false,
                export: false,
                saveAsImage: true,
              },
            }}
          />
        ) : (
          <div className="placeholder muted">
            <p>{error ?? "Chargement…"}</p>
          </div>
        )}
      </main>
    </div>
  );
}
