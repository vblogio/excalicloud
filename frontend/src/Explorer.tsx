import { useCallback, useEffect, useRef, useState } from "react";
import type { DragEvent, KeyboardEvent, MouseEvent, ReactNode } from "react";
import {
  api,
  ApiError,
  basename,
  dirname,
  displayPath,
  EXT,
  inFavoris,
  inPoubelle,
  join,
  label,
  POUBELLE,
  stripExt,
  withExt,
  type Entry,
  type Hit,
  type TagCount,
} from "./api";
import {
  CloseIcon,
  DeselectIcon,
  CopyIcon,
  DotsIcon,
  DownloadIcon,
  EditIcon,
  FileIcon,
  FilePlusIcon,
  FolderIcon,
  FolderPlusIcon,
  FolderUploadIcon,
  GridIcon,
  HistoryIcon,
  ListIcon,
  MoveIcon,
  RefreshIcon,
  RestoreIcon,
  SearchIcon,
  SelectAllIcon,
  ShareIcon,
  StarFilledIcon,
  StarIcon,
  TagIcon,
  TrashFullIcon,
  TrashIcon,
  UpIcon,
  UploadIcon,
  UserIcon,
} from "./icons";
import { ActionMenu } from "./ActionMenu";
import { FolderGlyph } from "./folderIcons";
import { IconDialog } from "./IconDialog";
import { collectDrop, collectFiles, hasFiles, type Collected } from "./importer";
import { MoveDialog } from "./MoveDialog";
import { TagsDialog } from "./TagsDialog";
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
  /** Affiche l'historique des versions d'un schéma. */
  onHistory: (path: string) => void;
  /** Affiche les liens de partage d'un schéma. */
  onShare: (path: string) => void;
  /** Contenu ajouté au bout de la barre de recherche (bouton d'épinglage du panneau). */
  toolbarEnd?: ReactNode;
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

export function Explorer({ current, refreshKey, onOpen, onMoved, onDeleted, onHistory, onShare, toolbarEnd }: Props) {
  const [dir, setDir] = useState(() => (current ? dirname(current) : ""));
  const [entries, setEntries] = useState<Entry[]>([]);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Hit[] | null>(null);
  const [view, setView] = useState<View>(initialView);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [anchor, setAnchor] = useState<string | null>(null); // origine d'une sélection Maj+clic
  const [menu, setMenu] = useState<{ entry: Hit; anchor: HTMLElement } | null>(null); // menu « … » ouvert
  const [moving, setMoving] = useState<string[] | null>(null); // éléments du dialogue « Déplacer vers »
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<{ text: string; detail?: string } | null>(null);
  const [busy, setBusy] = useState<string | null>(null); // progression d'un import
  const [fileDrag, setFileDrag] = useState(false); // fichiers du système survolant l'explorateur
  const [tags, setTags] = useState<TagCount[]>([]);
  const [tagFilter, setTagFilter] = useState<string | null>(null);
  const [tagging, setTagging] = useState<Entry[] | null>(null); // éléments du dialogue « Tags »
  const [iconing, setIconing] = useState<Entry[] | null>(null); // dossiers du dialogue « Icône »
  const [trashDays, setTrashDays] = useState<number | null>(null); // purge automatique (0 : jamais)
  const fileInput = useRef<HTMLInputElement>(null);
  const dirInput = useRef<HTMLInputElement>(null);
  const stay = useRef<string | null>(null); // schéma ouvert depuis Favoris : on reste dans Favoris

  const list = results ?? entries;
  const filter = { tag: tagFilter };
  const filtering = query.trim() !== "" || tagFilter !== null;
  // Dossier Favoris : des liens vers les originaux, ni création ni déplacement.
  const favView = !results && inFavoris(dir);
  // Poubelle : éléments à restaurer ou à supprimer définitivement, rien d'autre.
  const trashView = !results && inPoubelle(dir);
  const locked = favView || trashView; // ni création, ni import, ni déplacement

  const closeMenu = useCallback(() => setMenu(null), []);
  // La liste change de forme : le bouton « … » d'origine disparaît.
  useEffect(closeMenu, [view, dir, results, closeMenu]);

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
        const [next, tagList] = await Promise.all([api.list(dir), api.tags()]);
        setEntries(next);
        setTags(tagList);
        // Tag filtré qui n'existe plus : filtre retiré.
        setTagFilter((t) => (t && tagList.some((x) => x.name === t) ? t : null));
        setMenu(null);
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
    if (current && current !== stay.current) setDir(dirname(current));
    stay.current = null;
  }, [current]);

  // Recherche (texte, tag, favoris) avec anti-rebond.
  useEffect(() => {
    if (!filtering) {
      setResults(null);
      return;
    }
    const t = setTimeout(() => guard(async () => setResults(await api.search(query, filter))), 250);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, tagFilter, guard, refreshKey]);

  /** Recharge le dossier et, si une recherche est affichée, ses résultats. */
  const reload = async () => {
    await refresh();
    if (filtering) setResults(await api.search(query, filter));
  };

  // Délai de purge, affiché dans la poubelle (réglé dans les paramètres).
  useEffect(() => {
    if (!trashView) return;
    api
      .settings()
      .then((s) => setTrashDays(s.trashDays))
      .catch(() => setTrashDays(null));
  }, [trashView, refreshKey]);

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
    const paths = selectable.map((e) => e.path);
    const a = anchor ? paths.indexOf(anchor) : -1;
    const b = paths.indexOf(path);
    if (a < 0 || b < 0) return toggle(path);
    const [lo, hi] = a < b ? [a, b] : [b, a];
    setSelected((s) => new Set([...s, ...paths.slice(lo, hi + 1)]));
  };

  const selectable = list.filter((e) => !e.special);
  const selectAll = () => setSelected(new Set(selectable.map((e) => e.path)));

  const onItemClick = (ev: MouseEvent, e: Entry) => {
    if (!e.special) {
      if (ev.shiftKey) return selectRange(e.path);
      if (trashView) return toggle(e.path); // un élément de la poubelle ne s'ouvre pas
      // Ctrl/Cmd+clic, ou clic quand une sélection est en cours (mode sélection tactile).
      if (ev.ctrlKey || ev.metaKey || selected.size > 0) return toggle(e.path);
    }
    if (e.broken) return;
    if (e.isDir) {
      setQuery("");
      setDir(e.path);
    } else {
      if (favView && e.path !== current) stay.current = e.path;
      onOpen(e.path);
    }
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
      await reload();
      if (failed.length) {
        setError(`${plural(failed.length, "élément")} non déplacé${failed.length > 1 ? "s" : ""} — ${failed.join(" ; ")}`);
      }
    });

  /** Favoris : la sélection devient favorite, sauf si elle l'est déjà entièrement. */
  const toggleFavorite = (items: Entry[]) =>
    guard(async () => {
      const files = items.filter((e) => !e.isDir);
      if (files.length === 0) return;
      const fav = favView ? false : !files.every((e) => e.favorite);
      await api.setFavorite(
        files.map((e) => e.path),
        fav,
      );
      setMenu(null);
      if (favView) clearSelection();
      await reload();
    });

  /** Dépôt sur Favoris : les schémas déposés y sont ajoutés (lien, pas déplacement). */
  const addFavorites = (paths: string[]) =>
    toggleFavorite(
      paths.map((p) => list.find((e) => e.path === p) ?? { name: basename(p), path: p, isDir: false, size: 0, modTime: "" }),
    );

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
      // Dans Favoris, « Supprimer » retire le lien ; l'original reste en place.
      if (favView) {
        await api.setFavorite(paths, false);
        clearSelection();
        return reload();
      }
      const one = list.find((e) => e.path === paths[0]);
      const what =
        paths.length > 1
          ? `${plural(paths.length, "élément")} (dossiers compris avec leur contenu)`
          : one?.isDir
            ? `le dossier « ${one.name} » et tout son contenu`
            : `« ${stripExt(one?.name ?? basename(paths[0]))} »`;
      if (trashView) {
        if (!confirm(`Supprimer définitivement ${what} ? Cette action est irréversible.`)) return;
        for (const p of paths) await api.remove(p, true);
        clearSelection();
        return reload();
      }
      if (!confirm(`Mettre ${what} à la poubelle ?`)) return;
      for (const p of paths) {
        await api.remove(p, true);
        onDeleted(p);
      }
      clearSelection();
      await reload();
    });

  /** Remet des éléments de la poubelle à leur emplacement d'origine. */
  const restoreMany = (paths: string[]) =>
    guard(async () => {
      const res = await api.restoreTrash(paths);
      const failed = res.filter((r) => r.error);
      const done = res.filter((r) => r.to);
      clearSelection();
      await reload();
      if (done.length) {
        setNotice({
          text: `${plural(done.length, "élément")} restauré${done.length > 1 ? "s" : ""}`,
          detail: done.map((r) => displayPath(r.to!)).join("\n"),
        });
      }
      if (failed.length) {
        const name = (p: string) => stripExt(list.find((e) => e.path === p)?.name ?? basename(p));
        setError(
          `${plural(failed.length, "élément")} non restauré${failed.length > 1 ? "s" : ""} — ${failed.map((r) => `${name(r.from)} : ${r.error}`).join(" ; ")}`,
        );
      }
    });

  const emptyTrash = () =>
    guard(async () => {
      if (!confirm(`Vider la poubelle (${plural(list.length, "élément")}) ? Cette action est irréversible.`)) return;
      await api.remove(POUBELLE, true);
      clearSelection();
      await reload();
    });

  /**
   * Importe fichiers et arborescences dans targetDir : les dossiers sont recréés,
   * seuls les .excalidraw sont importés, les schémas déjà présents sont conservés.
   */
  const importTree = (pending: Collected | Promise<Collected>, targetDir: string) =>
    guard(async () => {
      setNotice(null);
      setBusy("Lecture…");
      try {
        const { files, dirs, ignored } = await pending;
        for (const d of dirs) {
          try {
            await api.mkdir(join(targetDir, d));
          } catch (err) {
            if (!(err instanceof ApiError && err.status === 409)) throw err; // déjà présent
          }
        }
        let done = 0;
        const existing: string[] = [];
        const failed: string[] = [];
        for (const { rel, file } of files) {
          setBusy(`Import ${done + existing.length + failed.length + 1}/${files.length}…`);
          try {
            await api.write(join(targetDir, rel), await file.text(), "*");
            done++;
          } catch (err) {
            if (err instanceof ApiError && err.status === 409) existing.push(rel);
            else failed.push(`${rel} : ${err instanceof Error ? err.message : err}`);
          }
        }
        await refresh();
        const parts = [`${plural(done, "schéma")} importé${done > 1 ? "s" : ""}`];
        if (dirs.length) parts.push(plural(dirs.length, "dossier"));
        if (existing.length) parts.push(`${existing.length} déjà présent${existing.length > 1 ? "s" : ""} (conservé${existing.length > 1 ? "s" : ""})`);
        if (ignored.length) parts.push(`${plural(ignored.length, "fichier")} ignoré${ignored.length > 1 ? "s" : ""} (pas .excalidraw)`);
        const detail = [
          ...failed.map((f) => `Échec — ${f}`),
          ...existing.map((f) => `Déjà présent — ${f}`),
          ...ignored.map((f) => `Ignoré — ${f}`),
        ].join("\n");
        setNotice({ text: parts.join(" · "), detail: detail || undefined });
        if (failed.length) setError(`${plural(failed.length, "fichier")} en échec : ${failed.join(" ; ")}`);
      } finally {
        setBusy(null);
      }
    });

  // --- Glisser-déposer (souris) ; sur mobile : sélection + « Déplacer vers… » -

  const onDragStart = (ev: DragEvent, e: Entry) => {
    // Glisser un élément sélectionné emporte toute la sélection.
    const paths = selected.has(e.path) ? [...selected] : [e.path];
    ev.dataTransfer.setData(DND_TYPE, JSON.stringify(paths));
    ev.dataTransfer.effectAllowed = "linkMove";
    if (paths.length > 1) {
      const ghost = document.createElement("div");
      ghost.className = "drag-ghost";
      ghost.textContent = plural(paths.length, "élément");
      document.body.appendChild(ghost);
      ev.dataTransfer.setDragImage(ghost, 12, 12);
      setTimeout(() => ghost.remove());
    }
  };

  // Cibles de dépôt : dossiers, fil d'Ariane et « .. ». Accepte les éléments de
  // l'explorateur (déplacement) et les fichiers/dossiers du système (import).
  const dropProps = (targetDir: string) => ({
    onDragOver: (ev: DragEvent) => {
      const external = hasFiles(ev.dataTransfer);
      if (!external && !ev.dataTransfer.types.includes(DND_TYPE)) return;
      if (external && (inFavoris(targetDir) || inPoubelle(targetDir))) return; // pas d'import dans Favoris ni la poubelle
      ev.preventDefault();
      ev.dataTransfer.dropEffect = external ? "copy" : inFavoris(targetDir) ? "link" : "move";
      setDropTarget(targetDir);
    },
    onDragLeave: () => setDropTarget(null),
    onDrop: (ev: DragEvent) => {
      ev.preventDefault();
      ev.stopPropagation(); // l'explorateur n'importe pas une seconde fois
      setDropTarget(null);
      setFileDrag(false);
      if (hasFiles(ev.dataTransfer)) return importTree(collectDrop(ev.dataTransfer), targetDir);
      let paths: string[] = [];
      try {
        paths = JSON.parse(ev.dataTransfer.getData(DND_TYPE));
      } catch {
        return;
      }
      if (inFavoris(targetDir)) addFavorites(paths);
      else if (inPoubelle(targetDir)) removeMany(paths);
      else moveMany(paths, targetDir);
    },
  });

  // --- Rendu ---------------------------------------------------------------

  const crumbs = dir ? dir.split("/") : [];
  const importDir = results || locked ? "" : dir; // pendant une recherche, le dossier courant n'est pas affiché
  const where = (p: string) => displayPath(dirname(p)) || "Racine";
  const deletedInfo = (e: Entry) =>
    `Supprimé le ${fmtDate(e.modTime)}${e.by ? ` par ${e.by}` : ""}\nEmplacement d'origine : ${e.origin ? where(e.origin) : "inconnu"}`;

  // Dépôt n'importe où dans l'explorateur : import dans le dossier courant.
  const zoneProps = {
    onDragOver: (ev: DragEvent) => {
      if (locked || !hasFiles(ev.dataTransfer)) return;
      ev.preventDefault();
      ev.dataTransfer.dropEffect = "copy";
      setFileDrag(true);
    },
    onDragLeave: (ev: DragEvent) => {
      if (!ev.currentTarget.contains(ev.relatedTarget as Node | null)) setFileDrag(false);
    },
    onDrop: (ev: DragEvent) => {
      if (locked || !hasFiles(ev.dataTransfer)) return;
      ev.preventDefault();
      setFileDrag(false);
      importTree(collectDrop(ev.dataTransfer), importDir);
    },
  };

  const big = view === "grid" ? 40 : 20;
  const icon = (e: Entry) =>
    e.special ? (
      <span className={`icon special ${e.special} ${e.count ? "full" : ""}`} aria-hidden>
        {e.special === "favoris" ? (
          <StarFilledIcon size={big} />
        ) : e.special === "poubelle" ? (
          e.count ? (
            <TrashFullIcon size={big} />
          ) : (
            <TrashIcon size={big} />
          )
        ) : (
          <UserIcon size={big} />
        )}
      </span>
    ) : trashView ? (
      <span className="icon" aria-hidden>
        {e.isDir ? <FolderIcon size={big} /> : <FileIcon size={big} />}
      </span>
    ) : e.broken ? (
      <span className="icon folder" aria-hidden>
        <StarIcon size={big} />
      </span>
    ) : e.isDir ? (
      <span className="icon folder" aria-hidden>
        <FolderGlyph name={e.icon} size={big} />
      </span>
    ) : (
      <Thumb path={e.path} version={e.modTime} />
    );

  // Dans Favoris : actions sur l'original, retrait du lien à la place de la suppression.
  const favActions = (e: Entry) => (
    <>
      {!e.broken && (
        <>
          <button role="menuitem" onClick={() => setDir(dirname(e.path))}>
            <FolderIcon />
            Ouvrir l'emplacement
          </button>
          <button role="menuitem" onClick={() => onHistory(e.path)}>
            <HistoryIcon />
            Historique…
          </button>
          <button role="menuitem" onClick={() => onShare(e.path)}>
            <ShareIcon />
            Partager…
          </button>
          <button role="menuitem" onClick={() => setTagging([e])}>
            <TagIcon />
            Tags…
          </button>
          <a role="menuitem" href={api.downloadUrl(e.path)} download={e.name}>
            <DownloadIcon />
            Télécharger
          </a>
          <hr />
        </>
      )}
      <button role="menuitem" className="danger" onClick={() => removeMany([e.path])}>
        <StarIcon />
        Retirer des favoris
      </button>
    </>
  );

  // Dans la poubelle : restauration ou suppression définitive.
  const trashActions = (e: Entry) => (
    <>
      <button role="menuitem" onClick={() => restoreMany([e.path])}>
        <RestoreIcon />
        Restaurer
      </button>
      <hr />
      <button role="menuitem" className="danger" onClick={() => removeMany([e.path])}>
        <TrashIcon />
        Supprimer définitivement
      </button>
      <span className="muted size deleted">{deletedInfo(e)}</span>
    </>
  );

  const actions = (e: Entry) => (
    <>
      <button role="menuitem" onClick={() => rename(e)}>
        <EditIcon />
        Renommer
      </button>
      <button role="menuitem" onClick={() => setMoving([e.path])}>
        <MoveIcon />
        Déplacer…
      </button>
      {!e.isDir && (
        <button role="menuitem" onClick={() => duplicate(e)}>
          <CopyIcon />
          Dupliquer
        </button>
      )}
      {!e.isDir && (
        <button role="menuitem" onClick={() => onHistory(e.path)}>
          <HistoryIcon />
          Historique…
        </button>
      )}
      {!e.isDir && (
        <button role="menuitem" onClick={() => onShare(e.path)}>
          <ShareIcon />
          Partager…
        </button>
      )}
      {!e.isDir && (
        <button role="menuitem" onClick={() => toggleFavorite([e])}>
          {e.favorite ? <StarFilledIcon /> : <StarIcon />}
          {e.favorite ? "Retirer des favoris" : "Favori"}
        </button>
      )}
      <button role="menuitem" onClick={() => setTagging([e])}>
        <TagIcon />
        Tags…
      </button>
      {e.isDir && (
        <button role="menuitem" onClick={() => setIconing([e])}>
          <FolderGlyph name={e.icon} />
          Icône…
        </button>
      )}
      {!e.isDir && (
        <a role="menuitem" href={api.downloadUrl(e.path)} download={e.name}>
          <DownloadIcon />
          Télécharger
        </a>
      )}
      <hr />
      <button role="menuitem" className="danger" onClick={() => removeMany([e.path])}>
        <TrashIcon />
        Supprimer
      </button>
      {!e.isDir && <span className="muted size">{fmtSize(e.size)}</span>}
    </>
  );

  const item = (e: Hit) => {
    const isSel = selected.has(e.path);
    const cls = [
      "item",
      e.isDir ? "dir" : "file",
      e.path === current ? "active" : "",
      isSel ? "selected" : "",
      e.special ? "special" : "",
      e.broken ? "broken" : "",
      dropTarget === e.path ? "drop" : "",
    ].join(" ");
    return (
      <li key={e.path} className={cls}>
        <div
          className="row"
          draggable={!e.special && !locked}
          onDragStart={(ev) => onDragStart(ev, e)}
          {...(e.isDir ? dropProps(e.path) : {})}
        >
          {!e.special && (
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
          )}
          <button
            className="name"
            onClick={(ev) => onItemClick(ev, e)}
            title={
              e.broken
                ? "Original introuvable : retirer ce favori"
                : trashView
                  ? deletedInfo(e)
                  : e.special === "poubelle"
                    ? e.count
                      ? `Poubelle : ${plural(e.count, "élément")}`
                      : "Poubelle vide"
                    : displayPath(e.path)
            }
          >
            {icon(e)}
            <span className="label">
              {e.favorite && (
                <span className="fav" title="Favori">
                  <StarFilledIcon size={12} />
                </span>
              )}
              {e.isDir ? e.name : stripExt(e.name)}
              {e.special === "poubelle" && !!e.count && <small className="badge">{e.count}</small>}
              {(results || (favView && !e.broken)) && <small className="muted where"> {where(e.path)}</small>}
              {trashView && <small className="muted where"> {e.origin ? where(e.origin) : ""}</small>}
              {e.snippet && <small className="snippet">{e.snippet}</small>}
            </span>
          </button>
          {!!e.tags?.length && (
            <span className="tags">
              {e.tags.map((t) => (
                <button
                  key={t}
                  className={`chip ${t === tagFilter ? "on" : ""}`}
                  title={`Filtrer sur « ${t} »`}
                  onClick={() => setTagFilter(t === tagFilter ? null : t)}
                >
                  {t}
                </button>
              ))}
            </span>
          )}
          {(!e.isDir || trashView) && view === "list" && <span className="meta muted">{fmtDate(e.modTime)}</span>}
          {!e.special && (
          <button
            className={`icon-btn more ${menu?.entry.path === e.path ? "open" : ""}`}
            aria-label="Actions"
            aria-haspopup="menu"
            aria-expanded={menu?.entry.path === e.path}
            title="Actions"
            onClick={(ev) => {
              const anchor = ev.currentTarget;
              setMenu((m) => (m?.entry.path === e.path ? null : { entry: e, anchor }));
            }}
          >
            <DotsIcon />
          </button>
          )}
        </div>
      </li>
    );
  };

  return (
    <div className={`explorer ${fileDrag ? "file-drag" : ""}`} onKeyDown={onKeyDown} {...zoneProps}>
      <div className="toolbar">
        <button className="ex-btn" onClick={newDrawing} title="Nouveau schéma" disabled={locked}>
          <FilePlusIcon />
          <span>Schéma</span>
        </button>
        <button className="ex-btn" onClick={newFolder} title="Nouveau dossier" disabled={locked}>
          <FolderPlusIcon />
          <span>Dossier</span>
        </button>
        <button
          className="ex-btn"
          onClick={() => fileInput.current?.click()}
          disabled={locked}
          title="Importer des fichiers .excalidraw (ou glisser-déposer fichiers et dossiers ici)"
        >
          <UploadIcon />
          <span>Importer</span>
        </button>
        <button
          className="ex-btn square"
          onClick={() => dirInput.current?.click()}
          disabled={locked}
          title="Importer un dossier complet (arborescence recréée, seuls les .excalidraw sont importés)"
          aria-label="Importer un dossier"
        >
          <FolderUploadIcon />
        </button>
        <input
          ref={fileInput}
          type="file"
          accept=".excalidraw"
          multiple
          hidden
          onChange={(ev) => {
            if (ev.target.files) importTree(collectFiles(ev.target.files), importDir);
            ev.target.value = "";
          }}
        />
        <input
          ref={dirInput}
          type="file"
          multiple
          hidden
          {...{ webkitdirectory: "" }}
          onChange={(ev) => {
            if (ev.target.files) importTree(collectFiles(ev.target.files), importDir);
            ev.target.value = "";
          }}
        />
      </div>

      <div className="searchbar">
        <label className="search">
          <SearchIcon />
          <input
            type="search"
            placeholder="Rechercher (noms, textes, IP…)"
            value={query}
            onChange={(ev) => setQuery(ev.target.value)}
          />
        </label>
        <div className="segmented" role="group" aria-label="Affichage">
          <button
            className={`icon-btn ${view === "grid" ? "on" : ""}`}
            onClick={() => setView("grid")}
            title="Affichage en vignettes"
            aria-label="Affichage en vignettes"
            aria-pressed={view === "grid"}
          >
            <GridIcon />
          </button>
          <button
            className={`icon-btn ${view === "list" ? "on" : ""}`}
            onClick={() => setView("list")}
            title="Affichage en liste"
            aria-label="Affichage en liste"
            aria-pressed={view === "list"}
          >
            <ListIcon />
          </button>
        </div>
        <button className="icon-btn" onClick={refresh} title="Rafraîchir" aria-label="Rafraîchir">
          <RefreshIcon />
        </button>
        {toolbarEnd}
      </div>

      <div className="filters" role="group" aria-label="Filtres">
        {tags.map((t) => (
          <button
            key={t.name}
            className={`chip ${t.name === tagFilter ? "on" : ""}`}
            aria-pressed={t.name === tagFilter}
            onClick={() => setTagFilter(t.name === tagFilter ? null : t.name)}
            title={`${plural(t.count, "élément")} avec ce tag`}
          >
            {t.name}
            <small>{t.count}</small>
          </button>
        ))}
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
                  {label(c)}
                </button>
              </span>
            );
          })}
        </nav>
      )}

      {trashView && (
        <div className="trashbar">
          <small className="muted">
            {trashDays === null
              ? ""
              : trashDays === 0
                ? "Conservés jusqu'à suppression définitive"
                : `Supprimés définitivement après ${plural(trashDays, "jour")}`}
          </small>
          <button className="ex-btn danger" onClick={emptyTrash} disabled={list.length === 0}>
            <TrashIcon />
            <span>Vider la poubelle</span>
          </button>
        </div>
      )}

      {selected.size > 0 && (
        <div className="selbar">
          <button className="icon-btn" onClick={clearSelection} aria-label="Annuler la sélection" title="Annuler la sélection (Échap)">
            <CloseIcon />
          </button>
          <span className="count">{plural(selected.size, "sélectionné")}</span>
          <button
            className="icon-btn"
            onClick={selected.size === selectable.length ? clearSelection : selectAll}
            title={selected.size === selectable.length ? "Tout désélectionner" : "Tout sélectionner (Ctrl+A)"}
            aria-label={selected.size === selectable.length ? "Tout désélectionner" : "Tout sélectionner"}
          >
            {selected.size === selectable.length ? <DeselectIcon /> : <SelectAllIcon />}
          </button>
          {trashView && (
            <button className="ex-btn" onClick={() => restoreMany([...selected])}>
              <RestoreIcon />
              <span>Restaurer</span>
            </button>
          )}
          {!trashView && (
            <button
              className="icon-btn"
              onClick={() => toggleFavorite(list.filter((e) => selected.has(e.path)))}
              title={favView ? "Retirer des favoris" : "Favoris (ajouter ou retirer)"}
              aria-label="Favoris"
            >
              <StarIcon />
            </button>
          )}
          {!trashView && (
            <button
              className="icon-btn"
              onClick={() => setTagging(list.filter((e) => selected.has(e.path)))}
              title="Tags…"
              aria-label="Tags"
            >
              <TagIcon />
            </button>
          )}
          {!locked && (
            <button className="ex-btn" onClick={() => setMoving([...selected])}>
              <MoveIcon />
              <span>Déplacer…</span>
            </button>
          )}
          <button
            className="icon-btn danger"
            onClick={() => removeMany([...selected])}
            title={
              favView ? "Retirer des favoris (Suppr)" : trashView ? "Supprimer définitivement (Suppr)" : "Mettre à la poubelle (Suppr)"
            }
            aria-label={favView ? "Retirer des favoris" : trashView ? "Supprimer définitivement" : "Supprimer"}
          >
            <TrashIcon />
          </button>
        </div>
      )}

      {error && (
        <div className="error" onClick={() => setError(null)}>
          {error}
        </div>
      )}
      {busy && <div className="notice busy">{busy}</div>}
      {notice && !busy && (
        <div className="notice" onClick={() => setNotice(null)} title={notice.detail ? `${notice.detail}\n\n(clic pour masquer)` : "Clic pour masquer"}>
          {notice.text}
        </div>
      )}

      <ul className={`entries ${view} ${selected.size > 0 ? "selecting" : ""}`}>
        {!results && dir && (
          <li className={`item dir up ${dropTarget === dirname(dir) ? "drop" : ""}`}>
            <div className="row" {...dropProps(dirname(dir))}>
              <button className="name" onClick={() => setDir(dirname(dir))}>
                <span className="icon" aria-hidden>
                  <UpIcon size={view === "grid" ? 32 : 20} />
                </span>
                <span className="label">..</span>
              </button>
            </div>
          </li>
        )}
        {list.map(item)}
        {list.length === 0 && (
          <li className="muted empty">
            {results
              ? "Aucun résultat"
              : favView
                ? "Aucun favori : étoile ou glisser-déposer un schéma ici"
                : trashView
                  ? "La poubelle est vide"
                  : "Dossier vide"}
          </li>
        )}
      </ul>

      {fileDrag && (
        <div className="drop-hint" aria-hidden>
          <FolderUploadIcon size={28} />
          <span>Déposer pour importer dans « {importDir ? label(basename(importDir)) : "Racine"} »</span>
          <small>Les dossiers sont recréés · seuls les .excalidraw sont importés</small>
        </div>
      )}

      {tagging && (
        <TagsDialog
          items={tagging.map((t) => list.find((e) => e.path === t.path) ?? t)}
          known={tags}
          onChanged={reload}
          onClose={() => setTagging(null)}
        />
      )}

      {iconing && <IconDialog items={iconing} onChanged={reload} onClose={() => setIconing(null)} />}

      {menu && (
        <ActionMenu anchor={menu.anchor} onClose={closeMenu}>
          {favView ? favActions(menu.entry) : trashView ? trashActions(menu.entry) : actions(menu.entry)}
        </ActionMenu>
      )}

      {moving && (
        <MoveDialog
          paths={moving}
          startDir={importDir}
          onCancel={() => setMoving(null)}
          onConfirm={(target) => moveMany(moving, target)}
        />
      )}
    </div>
  );
}
