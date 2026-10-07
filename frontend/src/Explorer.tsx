import { useCallback, useEffect, useRef, useState } from "react";
import type { DragEvent } from "react";
import {
  api,
  ApiError,
  basename,
  dirname,
  EXT,
  join,
  stripExt,
  withExt,
  type Entry,
} from "./api";

export const EMPTY_SCENE = JSON.stringify({
  type: "excalidraw",
  version: 2,
  source: "excalidraw-go",
  elements: [],
  appState: {},
  files: {},
});

interface Props {
  /** Schéma ouvert dans l'éditeur (surligné). */
  current: string | null;
  /** Incrémenté par le parent pour forcer un rafraîchissement (ex. après sauvegarde). */
  refreshKey: number;
  onOpen: (path: string) => void;
  /** Un fichier ou dossier a été déplacé/renommé : le parent met à jour le schéma ouvert. */
  onMoved: (from: string, to: string) => void;
  /** Un fichier ou dossier a été supprimé. */
  onDeleted: (path: string) => void;
}

const fmtDate = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });

const fmtSize = (n: number) =>
  n < 1024 ? `${n} o` : n < 1 << 20 ? `${(n / 1024).toFixed(1)} Ko` : `${(n / (1 << 20)).toFixed(1)} Mo`;

const DND_TYPE = "application/x-excalidraw-path";

export function Explorer({ current, refreshKey, onOpen, onMoved, onDeleted }: Props) {
  const [dir, setDir] = useState(() => (current ? dirname(current) : ""));
  const [entries, setEntries] = useState<Entry[]>([]);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Entry[] | null>(null);
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const guard = useCallback(async (fn: () => Promise<unknown>) => {
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const refresh = useCallback(
    () =>
      guard(async () => {
        setEntries(await api.list(dir));
        setMenuFor(null);
      }),
    [dir, guard],
  );

  useEffect(() => {
    refresh();
  }, [refresh, refreshKey]);

  // Suit le dossier du schéma ouvert (lien direct, création, déplacement).
  useEffect(() => {
    if (current) setDir(dirname(current));
  }, [current]);

  // Recherche avec anti-rebond.
  useEffect(() => {
    if (!query.trim()) {
      setResults(null);
      return;
    }
    const t = setTimeout(() => guard(async () => setResults(await api.search(query))), 250);
    return () => clearTimeout(t);
  }, [query, guard, refreshKey]);

  // --- Actions -------------------------------------------------------------

  const newDrawing = () =>
    guard(async () => {
      const name = prompt("Nom du nouveau schéma :", "Sans titre");
      if (!name?.trim()) return;
      const path = join(dir, withExt(name.trim()));
      await api.write(path, EMPTY_SCENE, "*");
      await refresh();
      onOpen(path);
    });

  const newFolder = () =>
    guard(async () => {
      const name = prompt("Nom du dossier :");
      if (!name?.trim()) return;
      await api.mkdir(join(dir, name.trim()));
      await refresh();
    });

  const move = (e: Entry, to: string) =>
    guard(async () => {
      if (to === e.path) return;
      await api.move(e.path, to);
      onMoved(e.path, to);
      await refresh();
    });

  const rename = (e: Entry) => {
    const name = prompt("Nouveau nom :", e.isDir ? e.name : stripExt(e.name));
    if (!name?.trim()) return;
    move(e, join(dirname(e.path), e.isDir ? name.trim() : withExt(name.trim())));
  };

  const moveTo = (e: Entry) => {
    const dest = prompt("Dossier de destination (vide = racine) :", dirname(e.path));
    if (dest === null) return;
    move(e, join(dest.trim().replace(/^\/+|\/+$/g, ""), e.name));
  };

  const duplicate = (e: Entry) =>
    guard(async () => {
      const base = join(dirname(e.path), stripExt(e.name));
      for (let i = 1; i < 100; i++) {
        const to = `${base} (copie${i > 1 ? ` ${i}` : ""})${EXT}`;
        try {
          await api.copy(e.path, to);
          break;
        } catch (err) {
          if (!(err instanceof ApiError && err.status === 409)) throw err;
        }
      }
      await refresh();
    });

  const remove = (e: Entry) =>
    guard(async () => {
      const what = e.isDir ? `le dossier « ${e.name} » et tout son contenu` : `« ${e.name} »`;
      if (!confirm(`Mettre ${what} à la corbeille ?`)) return;
      await api.remove(e.path, e.isDir);
      onDeleted(e.path);
      await refresh();
    });

  const importFiles = (files: FileList | null) =>
    guard(async () => {
      for (const f of Array.from(files ?? [])) {
        await api.write(join(dir, withExt(stripExt(f.name.replace(/\.json$/i, "")))), await f.text(), "*");
      }
      await refresh();
    });

  // --- Glisser-déposer (souris) ; sur mobile : menu « Déplacer… » ----------

  const onDragStart = (ev: DragEvent, e: Entry) => {
    ev.dataTransfer.setData(DND_TYPE, e.path);
    ev.dataTransfer.effectAllowed = "move";
  };

  const dropProps = (targetDir: string) => ({
    onDragOver: (ev: DragEvent) => {
      if (!ev.dataTransfer.types.includes(DND_TYPE)) return;
      ev.preventDefault();
      setDropTarget(targetDir);
    },
    onDragLeave: () => setDropTarget(null),
    onDrop: (ev: DragEvent) => {
      ev.preventDefault();
      setDropTarget(null);
      const from = ev.dataTransfer.getData(DND_TYPE);
      if (!from || dirname(from) === targetDir) return;
      move({ path: from, name: basename(from) } as Entry, join(targetDir, basename(from)));
    },
  });

  // --- Rendu ---------------------------------------------------------------

  const crumbs = dir ? dir.split("/") : [];
  const list = results ?? entries;

  const row = (e: Entry) => (
    <li key={e.path} className={e.path === current ? "active" : ""}>
      <div
        className={`row ${dropTarget === e.path ? "drop" : ""}`}
        draggable
        onDragStart={(ev) => onDragStart(ev, e)}
        {...(e.isDir ? dropProps(e.path) : {})}
      >
        <button
          className="name"
          onClick={() => (e.isDir ? (setQuery(""), setDir(e.path)) : onOpen(e.path))}
          title={e.path}
        >
          <span className="icon" aria-hidden>
            {e.isDir ? "📁" : "📄"}
          </span>
          <span className="label">
            {e.isDir ? e.name : stripExt(e.name)}
            {results && <small className="muted"> {dirname(e.path) || "/"}</small>}
          </span>
        </button>
        {!e.isDir && <span className="meta muted">{fmtDate(e.modTime)}</span>}
        <button
          className="icon-btn"
          aria-label="Actions"
          onClick={() => setMenuFor(menuFor === e.path ? null : e.path)}
        >
          ⋯
        </button>
      </div>
      {menuFor === e.path && (
        <div className="actions">
          <button onClick={() => rename(e)}>Renommer</button>
          <button onClick={() => moveTo(e)}>Déplacer…</button>
          {!e.isDir && <button onClick={() => duplicate(e)}>Dupliquer</button>}
          {!e.isDir && (
            <a href={api.downloadUrl(e.path)} download={e.name}>
              Télécharger
            </a>
          )}
          <button className="danger" onClick={() => remove(e)}>
            Supprimer
          </button>
          {!e.isDir && <span className="muted">{fmtSize(e.size)}</span>}
        </div>
      )}
    </li>
  );

  return (
    <div className="explorer">
      <div className="toolbar">
        <button onClick={newDrawing} title="Nouveau schéma">＋ Schéma</button>
        <button onClick={newFolder} title="Nouveau dossier">＋ Dossier</button>
        <button onClick={() => fileInput.current?.click()} title="Importer des fichiers .excalidraw">
          Importer
        </button>
        <button className="icon-btn" onClick={refresh} title="Rafraîchir" aria-label="Rafraîchir">
          ⟳
        </button>
        <input
          ref={fileInput}
          type="file"
          accept=".excalidraw,application/json"
          multiple
          hidden
          onChange={(ev) => {
            importFiles(ev.target.files);
            ev.target.value = "";
          }}
        />
      </div>

      <input
        className="search"
        type="search"
        placeholder="Rechercher…"
        value={query}
        onChange={(ev) => setQuery(ev.target.value)}
      />

      {!results && (
        <nav className="crumbs">
          <button className={dropTarget === "" ? "drop" : ""} onClick={() => setDir("")} {...dropProps("")}>
            Racine
          </button>
          {crumbs.map((c, i) => {
            const p = crumbs.slice(0, i + 1).join("/");
            return (
              <span key={p}>
                <span className="muted"> / </span>
                <button className={dropTarget === p ? "drop" : ""} onClick={() => setDir(p)} {...dropProps(p)}>
                  {c}
                </button>
              </span>
            );
          })}
        </nav>
      )}

      {error && (
        <div className="error" onClick={() => setError(null)}>
          {error}
        </div>
      )}

      <ul className="entries">
        {!results && dir && (
          <li>
            <div className={`row ${dropTarget === dirname(dir) ? "drop" : ""}`} {...dropProps(dirname(dir))}>
              <button className="name" onClick={() => setDir(dirname(dir))}>
                <span className="icon" aria-hidden>↩</span>
                <span className="label">..</span>
              </button>
            </div>
          </li>
        )}
        {list.map(row)}
        {list.length === 0 && <li className="muted empty">{results ? "Aucun résultat" : "Dossier vide"}</li>}
      </ul>
    </div>
  );
}
