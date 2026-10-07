import { useCallback, useEffect, useRef, useState } from "react";
import type { DragEvent, KeyboardEvent, MouseEvent } from "react";
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
import { MoveDialog } from "./MoveDialog";
import { Thumb } from "./Thumb";

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

type View = "list" | "grid";

const fmtDate = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });

const fmtSize = (n: number) =>
  n < 1024 ? `${n} o` : n < 1 << 20 ? `${(n / 1024).toFixed(1)} Ko` : `${(n / (1 << 20)).toFixed(1)} Mo`;

const plural = (n: number, word: string) => `${n} ${word}${n > 1 ? "s" : ""}`;

const DND_TYPE = "application/x-excalidraw-paths";

const initialView = (): View => {
  try {
    return localStorage.getItem("explorerView") === "list" ? "list" : "grid";
  } catch {
    return "grid";
  }
};

/** Un élément peut-il être déplacé dans targetDir ? (pas sur place, pas dans lui-même) */
const canMoveInto = (path: string, targetDir: string) =>
  dirname(path) !== targetDir && targetDir !== path && !targetDir.startsWith(path + "/");

export function Explorer({ current, refreshKey, onOpen, onMoved, onDeleted }: Props) {
  const [dir, setDir] = useState(() => (current ? dirname(current) : ""));
  const [entries, setEntries] = useState<Entry[]>([]);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Entry[] | null>(null);
  const [view, setView] = useState<View>(initialView);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [anchor, setAnchor] = useState<string | null>(null); // origine d'une sélection Maj+clic
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [moving, setMoving] = useState<string[] | null>(null); // éléments du dialogue « Déplacer vers »
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const list = results ?? entries;

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
        const next = await api.list(dir);
        setEntries(next);
        setMenuFor(null);
        // Oublie les éléments sélectionnés qui ont disparu.
        setSelected((s) => {
          if (s.size === 0) return s;
          const still = new Set(next.map((e) => e.path));
          return new Set([...s].filter((p) => still.has(p)));
        });
      }),
    [dir, guard],
  );

  useEffect(() => {
    refresh();
  }, [refresh, refreshKey]);

  // Changer de dossier ou de recherche réinitialise la sélection.
  useEffect(() => {
    setSelected(new Set());
    setAnchor(null);
  }, [dir, results === null]);

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

  useEffect(() => {
    try {
      localStorage.setItem("explorerView", view);
    } catch {
      /* stockage indisponible */
    }
  }, [view]);

  // --- Sélection -----------------------------------------------------------

  const clearSelection = () => {
    setSelected(new Set());
    setAnchor(null);
  };

  const toggle = (path: string) => {
    setSelected((s) => {
      const n = new Set(s);
      if (!n.delete(path)) n.add(path);
      return n;
    });
    setAnchor(path);
  };

  const selectRange = (path: string) => {
    const paths = list.map((e) => e.path);
    const a = anchor ? paths.indexOf(anchor) : -1;
    const b = paths.indexOf(path);
    if (a < 0 || b < 0) return toggle(path);
    const [lo, hi] = a < b ? [a, b] : [b, a];
    setSelected((s) => new Set([...s, ...paths.slice(lo, hi + 1)]));
  };

  const selectAll = () => setSelected(new Set(list.map((e) => e.path)));

  const onItemClick = (ev: MouseEvent, e: Entry) => {
    if (ev.shiftKey) return selectRange(e.path);
    // Ctrl/Cmd+clic, ou clic quand une sélection est en cours (mode sélection tactile).
    if (ev.ctrlKey || ev.metaKey || selected.size > 0) return toggle(e.path);
    if (e.isDir) {
      setQuery("");
      setDir(e.path);
    } else onOpen(e.path);
  };

  const onKeyDown = (ev: KeyboardEvent) => {
    if ((ev.target as HTMLElement).tagName === "INPUT") return;
    if (ev.key === "Escape" && selected.size > 0) {
      ev.preventDefault();
      clearSelection();
    } else if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === "a") {
      ev.preventDefault();
      selectAll();
    } else if (ev.key === "Delete" && selected.size > 0) {
      ev.preventDefault();
      removeMany([...selected]);
    }
  };

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

  const rename = (e: Entry) =>
    guard(async () => {
      const name = prompt("Nouveau nom :", e.isDir ? e.name : stripExt(e.name));
      if (!name?.trim()) return;
      const to = join(dirname(e.path), e.isDir ? name.trim() : withExt(name.trim()));
      if (to === e.path) return;
      await api.move(e.path, to);
      onMoved(e.path, to);
      await refresh();
    });

  /** Déplace plusieurs éléments dans targetDir ; les échecs partiels sont listés. */
  const moveMany = (paths: string[], targetDir: string) =>
    guard(async () => {
      const todo = paths.filter((p) => canMoveInto(p, targetDir));
      if (todo.length === 0) return;
      const res = await api.moveMany(todo, targetDir);
      const failed: string[] = [];
      for (const r of res) {
        if (r.error) failed.push(`${basename(r.from)} : ${r.error}`);
        else if (r.to && r.to !== r.from) onMoved(r.from, r.to);
      }
      clearSelection();
      setMoving(null);
      await refresh();
      if (results) setResults(await api.search(query));
      if (failed.length) {
        setError(`${plural(failed.length, "élément")} non déplacé${failed.length > 1 ? "s" : ""} — ${failed.join(" ; ")}`);
      }
    });

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

  const removeMany = (paths: string[]) =>
    guard(async () => {
      const one = list.find((e) => e.path === paths[0]);
      const what =
        paths.length > 1
          ? `${plural(paths.length, "élément")} (dossiers compris avec leur contenu)`
          : one?.isDir
            ? `le dossier « ${one.name} » et tout son contenu`
            : `« ${basename(paths[0])} »`;
      if (!confirm(`Mettre ${what} à la corbeille ?`)) return;
      for (const p of paths) {
        await api.remove(p, true);
        onDeleted(p);
      }
      clearSelection();
      await refresh();
      if (results) setResults(await api.search(query));
    });

  const importFiles = (files: FileList | null) =>
    guard(async () => {
      for (const f of Array.from(files ?? [])) {
        await api.write(join(dir, withExt(stripExt(f.name.replace(/\.json$/i, "")))), await f.text(), "*");
      }
      await refresh();
    });

  // --- Glisser-déposer (souris) ; sur mobile : sélection + « Déplacer vers… » -

  const onDragStart = (ev: DragEvent, e: Entry) => {
    // Glisser un élément sélectionné emporte toute la sélection.
    const paths = selected.has(e.path) ? [...selected] : [e.path];
    ev.dataTransfer.setData(DND_TYPE, JSON.stringify(paths));
    ev.dataTransfer.effectAllowed = "move";
    if (paths.length > 1) {
      const ghost = document.createElement("div");
      ghost.className = "drag-ghost";
      ghost.textContent = plural(paths.length, "élément");
      document.body.appendChild(ghost);
      ev.dataTransfer.setDragImage(ghost, 12, 12);
      setTimeout(() => ghost.remove());
    }
  };

  const dropProps = (targetDir: string) => ({
    onDragOver: (ev: DragEvent) => {
      if (!ev.dataTransfer.types.includes(DND_TYPE)) return;
      ev.preventDefault();
      ev.dataTransfer.dropEffect = "move";
      setDropTarget(targetDir);
    },
    onDragLeave: () => setDropTarget(null),
    onDrop: (ev: DragEvent) => {
      ev.preventDefault();
      setDropTarget(null);
      let paths: string[] = [];
      try {
        paths = JSON.parse(ev.dataTransfer.getData(DND_TYPE));
      } catch {
        return;
      }
      moveMany(paths, targetDir);
    },
  });

  // --- Rendu ---------------------------------------------------------------

  const crumbs = dir ? dir.split("/") : [];

  const icon = (e: Entry) =>
    e.isDir ? (
      <span className="icon" aria-hidden>
        📁
      </span>
    ) : (
      <Thumb path={e.path} version={e.modTime} />
    );

  const actions = (e: Entry) => (
    <div className="actions">
      <button onClick={() => rename(e)}>Renommer</button>
      <button onClick={() => setMoving([e.path])}>Déplacer…</button>
      {!e.isDir && <button onClick={() => duplicate(e)}>Dupliquer</button>}
      {!e.isDir && (
        <a href={api.downloadUrl(e.path)} download={e.name}>
          Télécharger
        </a>
      )}
      <button className="danger" onClick={() => removeMany([e.path])}>
        Supprimer
      </button>
      {!e.isDir && <span className="muted">{fmtSize(e.size)}</span>}
    </div>
  );

  const item = (e: Entry) => {
    const isSel = selected.has(e.path);
    const cls = [
      "item",
      e.isDir ? "dir" : "file",
      e.path === current ? "active" : "",
      isSel ? "selected" : "",
      dropTarget === e.path ? "drop" : "",
    ].join(" ");
    return [
      <li key={e.path} className={cls}>
        <div
          className="row"
          draggable
          onDragStart={(ev) => onDragStart(ev, e)}
          {...(e.isDir ? dropProps(e.path) : {})}
        >
          <input
            type="checkbox"
            className="check"
            checked={isSel}
            aria-label={`Sélectionner ${e.name}`}
            onClick={(ev) => {
              ev.stopPropagation();
              if (ev.shiftKey) {
                ev.preventDefault();
                selectRange(e.path);
              }
            }}
            onChange={() => toggle(e.path)}
          />
          <button className="name" onClick={(ev) => onItemClick(ev, e)} title={e.path}>
            {icon(e)}
            <span className="label">
              {e.isDir ? e.name : stripExt(e.name)}
              {results && <small className="muted where"> {dirname(e.path) || "/"}</small>}
            </span>
          </button>
          {!e.isDir && view === "list" && <span className="meta muted">{fmtDate(e.modTime)}</span>}
          <button
            className="icon-btn more"
            aria-label="Actions"
            onClick={() => setMenuFor(menuFor === e.path ? null : e.path)}
          >
            ⋯
          </button>
        </div>
        {view === "list" && menuFor === e.path && actions(e)}
      </li>,
      // En grille, le menu occupe toute la largeur sous la vignette.
      view === "grid" && menuFor === e.path && (
        <li key={`${e.path}#menu`} className="menu-row">
          {actions(e)}
        </li>
      ),
    ];
  };

  return (
    <div className="explorer" onKeyDown={onKeyDown}>
      <div className="toolbar">
        <button onClick={newDrawing} title="Nouveau schéma">＋ Schéma</button>
        <button onClick={newFolder} title="Nouveau dossier">＋ Dossier</button>
        <button onClick={() => fileInput.current?.click()} title="Importer des fichiers .excalidraw">
          Importer
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

      <div className="searchbar">
        <input
          className="search"
          type="search"
          placeholder="Rechercher…"
          value={query}
          onChange={(ev) => setQuery(ev.target.value)}
        />
        <button
          className="icon-btn"
          onClick={() => setView(view === "grid" ? "list" : "grid")}
          title={view === "grid" ? "Affichage en liste" : "Affichage en vignettes"}
          aria-label={view === "grid" ? "Affichage en liste" : "Affichage en vignettes"}
        >
          {view === "grid" ? "☰" : "▦"}
        </button>
        <button className="icon-btn" onClick={refresh} title="Rafraîchir" aria-label="Rafraîchir">
          ⟳
        </button>
      </div>

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

      {selected.size > 0 && (
        <div className="selbar">
          <button className="icon-btn" onClick={clearSelection} aria-label="Annuler la sélection" title="Annuler la sélection (Échap)">
            ✕
          </button>
          <span className="count">{plural(selected.size, "sélectionné")}</span>
          <button
            className="icon-btn"
            onClick={selected.size === list.length ? clearSelection : selectAll}
            title={selected.size === list.length ? "Tout désélectionner" : "Tout sélectionner (Ctrl+A)"}
            aria-label={selected.size === list.length ? "Tout désélectionner" : "Tout sélectionner"}
          >
            {selected.size === list.length ? "☐" : "☑"}
          </button>
          <button onClick={() => setMoving([...selected])}>Déplacer…</button>
          <button className="danger" onClick={() => removeMany([...selected])} title="Mettre à la corbeille (Suppr)" aria-label="Supprimer">
            🗑
          </button>
        </div>
      )}

      {error && (
        <div className="error" onClick={() => setError(null)}>
          {error}
        </div>
      )}

      <ul className={`entries ${view} ${selected.size > 0 ? "selecting" : ""}`}>
        {!results && dir && (
          <li className={`item dir up ${dropTarget === dirname(dir) ? "drop" : ""}`}>
            <div className="row" {...dropProps(dirname(dir))}>
              <button className="name" onClick={() => setDir(dirname(dir))}>
                <span className="icon" aria-hidden>↩</span>
                <span className="label">..</span>
              </button>
            </div>
          </li>
        )}
        {list.flatMap(item)}
        {list.length === 0 && <li className="muted empty">{results ? "Aucun résultat" : "Dossier vide"}</li>}
      </ul>

      {moving && (
        <MoveDialog
          paths={moving}
          startDir={results ? "" : dir}
          onCancel={() => setMoving(null)}
          onConfirm={(target) => moveMany(moving, target)}
        />
      )}
    </div>
  );
}
