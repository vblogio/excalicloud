import { useEffect, useMemo, useRef, useState } from "react";
import { exportToSvg, restoreElements } from "@excalidraw/excalidraw";
import type { BinaryFiles } from "@excalidraw/excalidraw/types";
import type { ExcalidrawElement } from "@excalidraw/excalidraw/element/types";
import { api, basename, dirname, join, stripExt, withExt, type Version } from "./api";
import { DownloadIcon, HistoryIcon } from "./icons";

interface Props {
  path: string;
  theme: "light" | "dark";
  /** Restaure la version (le parent enregistre et recharge le schéma ouvert). */
  onRestore: (id: string) => Promise<void>;
  /** Une copie a été créée à partir d'une version. */
  onCopied: (path: string) => void;
  onClose: () => void;
}

interface Scene {
  elements: readonly ExcalidrawElement[];
  files: BinaryFiles;
  background: string;
}

type View = "version" | "current" | "diff";

const CURRENT = "";

const parse = (text: string): Scene => {
  const data = JSON.parse(text);
  return {
    elements: restoreElements(data.elements ?? [], null).filter((e) => !e.isDeleted),
    files: data.files ?? {},
    background: data.appState?.viewBackgroundColor ?? "#ffffff",
  };
};

const fmtDate = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });

const fmtAge = (iso: string) => {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (s < 3600) return rtf.format(-Math.max(1, Math.round(s / 60)), "minute");
  if (s < 86400) return rtf.format(-Math.round(s / 3600), "hour");
  return rtf.format(-Math.round(s / 86400), "day");
};

const fmtSize = (n: number) => (n < 1 << 20 ? `${Math.max(1, Math.round(n / 1024))} Ko` : `${(n / (1 << 20)).toFixed(1)} Mo`);

// Différences élément par élément (identifiants stables dans Excalidraw).
const DIFF = {
  added: { stroke: "#2f9e44", fill: "#b2f2bb", label: "ajouté" },
  removed: { stroke: "#e03131", fill: "#ffc9c9", label: "supprimé" },
  changed: { stroke: "#f08c00", fill: "#ffec99", label: "modifié" },
} as const;
type Change = keyof typeof DIFF;

function diff(older: Scene, now: Scene) {
  const before = new Map(older.elements.map((e) => [e.id, e]));
  const after = new Map(now.elements.map((e) => [e.id, e]));
  const kind = new Map<string, Change>();
  for (const [id, e] of after) {
    const o = before.get(id);
    if (!o) kind.set(id, "added");
    else if (o.version !== e.version || o.versionNonce !== e.versionNonce) kind.set(id, "changed");
  }
  for (const id of before.keys()) if (!after.has(id)) kind.set(id, "removed");
  const count = { added: 0, removed: 0, changed: 0 };
  for (const k of kind.values()) count[k]++;

  // Scène de comparaison : l'état actuel plus les éléments disparus, colorés
  // selon le changement ; ce qui n'a pas bougé est estompé.
  const paint = (e: ExcalidrawElement): ExcalidrawElement => {
    const container = "containerId" in e ? e.containerId : null; // texte d'une forme
    const k = kind.get(e.id) ?? (container ? kind.get(container) : undefined);
    if (!k) return { ...e, opacity: 20 };
    const c = DIFF[k];
    return {
      ...e,
      opacity: 100,
      strokeColor: c.stroke,
      backgroundColor: e.backgroundColor === "transparent" ? "transparent" : c.fill,
      ...(k === "removed" ? { strokeStyle: "dashed" as const } : {}),
    };
  };
  const elements = [...now.elements, ...older.elements.filter((e) => !after.has(e.id))].map(paint);
  return { count, scene: { elements, files: { ...older.files, ...now.files }, background: now.background } };
}

/** Historique d'un schéma : aperçu, comparaison avec l'état actuel, restauration. */
export function HistoryDialog({ path, theme, onRestore, onCopied, onClose }: Props) {
  const [versions, setVersions] = useState<Version[] | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [view, setView] = useState<View>("diff");
  const [scenes, setScenes] = useState<Record<string, Scene>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const preview = useRef<HTMLDivElement>(null);
  const name = stripExt(basename(path));

  const run = async (op: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await op();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const loadList = () =>
    run(async () => {
      const [vs, cur] = await Promise.all([api.history(path), api.read(path)]);
      setScenes((s) => ({ ...s, [CURRENT]: parse(cur.text) }));
      setVersions(vs);
      setSelected((sel) => sel ?? vs[0]?.id ?? null);
    });

  useEffect(() => {
    loadList();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Contenu de la version choisie (mis en cache : une version ne change pas).
  useEffect(() => {
    if (!selected || scenes[selected]) return;
    let live = true;
    api
      .readVersion(path, selected)
      .then((t) => live && setScenes((s) => ({ ...s, [selected]: parse(t) })))
      .catch((e) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, [path, selected, scenes]);

  const older = selected ? scenes[selected] : undefined;
  const now = scenes[CURRENT];
  const comparison = useMemo(() => (older && now ? diff(older, now) : null), [older, now]);
  const shown = view === "current" ? now : view === "version" ? older : comparison?.scene;

  // Aperçu SVG rendu par Excalidraw.
  useEffect(() => {
    const box = preview.current;
    if (!box) return;
    box.replaceChildren();
    if (!shown) return;
    if (shown.elements.length === 0) {
      box.textContent = "Schéma vide";
      return;
    }
    let live = true;
    exportToSvg({
      elements: shown.elements,
      appState: {
        viewBackgroundColor: shown.background,
        exportBackground: true,
        exportWithDarkMode: theme === "dark",
      },
      files: shown.files,
      exportPadding: 16,
    })
      .then((svg: SVGSVGElement) => {
        if (!live) return;
        svg.removeAttribute("width");
        svg.removeAttribute("height");
        box.replaceChildren(svg);
      })
      .catch((e: Error) => live && setError(`Aperçu impossible : ${e.message}`));
    return () => {
      live = false;
    };
  }, [shown, theme]);

  const current = versions?.find((v) => v.id === selected);

  const restore = () =>
    current &&
    confirm(`Restaurer la version du ${fmtDate(current.time)} ?\n\nL'état actuel est conservé dans l'historique.`) &&
    run(async () => {
      await onRestore(current.id);
      onClose();
    });

  const copy = () =>
    current &&
    run(async () => {
      const d = new Date(current.time);
      const stamp = `${d.toLocaleDateString().replace(/\//g, "-")} ${String(d.getHours()).padStart(2, "0")}h${String(d.getMinutes()).padStart(2, "0")}`;
      const to = join(dirname(path), withExt(`${name} (version du ${stamp})`));
      await api.write(to, await api.readVersion(path, current.id), "*");
      onCopied(to);
      onClose();
    });

  const snapshot = () =>
    run(async () => {
      const v = await api.snapshot(path);
      setSelected(v.id);
      await loadList();
    });

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal history"
        role="dialog"
        aria-modal="true"
        aria-label={`Historique de ${name}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="history-head">
          <h3>Historique · {name}</h3>
          <button disabled={busy} onClick={snapshot} title="Enregistrer l'état actuel comme version">
            <HistoryIcon />
            <span>Créer une version</span>
          </button>
        </div>

        <div className="history-body">
          <ul className="versions-list">
            {versions?.map((v) => (
              <li key={v.id}>
                <button className={v.id === selected ? "on" : ""} onClick={() => setSelected(v.id)}>
                  <span>{fmtDate(v.time)}</span>
                  <small className="muted">
                    {fmtAge(v.time)} · {fmtSize(v.size)}
                  </small>
                </button>
              </li>
            ))}
            {versions?.length === 0 && (
              <li className="muted empty">
                Aucune version pour l'instant. Une version est conservée automatiquement quand le schéma est
                modifié après une pause ; « Créer une version » en fixe une tout de suite.
              </li>
            )}
          </ul>

          <div className="history-preview">
            <div className="segmented" role="group" aria-label="Aperçu">
              {(
                [
                  ["diff", "Différences"],
                  ["version", "Cette version"],
                  ["current", "Actuelle"],
                ] as [View, string][]
              ).map(([id, label]) => (
                <button key={id} className={view === id ? "on" : ""} disabled={!selected} onClick={() => setView(id)}>
                  {label}
                </button>
              ))}
            </div>
            <div className="svg-box" ref={preview} />
            {view === "diff" && comparison && (
              <p className="legend">
                <span className="muted">Depuis cette version :</span>
                {(Object.keys(DIFF) as Change[]).map((k) => (
                  <span key={k}>
                    <i style={{ background: DIFF[k].stroke }} />
                    {comparison.count[k]} {DIFF[k].label}
                    {comparison.count[k] > 1 ? "s" : ""}
                  </span>
                ))}
              </p>
            )}
          </div>
        </div>

        {error && <div className="error">{error}</div>}

        <div className="modal-actions">
          {current && (
            <a className="link" href={api.versionUrl(path, current.id)} download={`${name}.excalidraw`} title="Télécharger cette version">
              <DownloadIcon />
            </a>
          )}
          <span className="spacer" />
          <button onClick={onClose}>Fermer</button>
          <button disabled={busy || !current} onClick={copy} title="Créer un nouveau schéma à partir de cette version">
            Restaurer en copie
          </button>
          <button className="primary" disabled={busy || !current} onClick={restore}>
            Restaurer
          </button>
        </div>
      </div>
    </div>
  );
}
