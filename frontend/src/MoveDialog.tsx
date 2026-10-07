import { useEffect, useState } from "react";
import { api, basename, dirname, join, type Entry } from "./api";

interface Props {
  /** Éléments à déplacer. */
  paths: string[];
  startDir: string;
  onCancel: () => void;
  onConfirm: (dir: string) => void;
}

/** Sélecteur de dossier de destination (utilisable au doigt comme à la souris). */
export function MoveDialog({ paths, startDir, onCancel, onConfirm }: Props) {
  const [dir, setDir] = useState(startDir);
  const [folders, setFolders] = useState<Entry[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    setError(null);
    api
      .list(dir)
      .then((es) => live && setFolders(es.filter((e) => e.isDir)))
      .catch((e) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, [dir]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onCancel();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);

  // Un dossier déplacé ne peut pas recevoir son propre contenu.
  const blocked = (p: string) => paths.some((m) => p === m || p.startsWith(m + "/"));
  const crumbs = dir ? dir.split("/") : [];

  const newFolder = async () => {
    const name = prompt("Nom du dossier :");
    if (!name?.trim()) return;
    try {
      const e = await api.mkdir(join(dir, name.trim()));
      setDir(e.path);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const title =
    paths.length === 1 ? `Déplacer « ${basename(paths[0])} »` : `Déplacer ${paths.length} éléments`;

  return (
    <div className="modal-backdrop" onClick={onCancel}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={title} onClick={(e) => e.stopPropagation()}>
        <h3>{title}</h3>
        <nav className="crumbs">
          <button onClick={() => setDir("")}>Racine</button>
          {crumbs.map((c, i) => {
            const p = crumbs.slice(0, i + 1).join("/");
            return (
              <span key={p}>
                <span className="muted"> / </span>
                <button onClick={() => setDir(p)}>{c}</button>
              </span>
            );
          })}
        </nav>
        {error && <div className="error">{error}</div>}
        <ul className="folders">
          {folders.map((f) => (
            <li key={f.path}>
              <button disabled={blocked(f.path)} onClick={() => setDir(f.path)}>
                <span aria-hidden>📁</span> {f.name}
              </button>
            </li>
          ))}
          {folders.length === 0 && <li className="muted empty">Aucun sous-dossier</li>}
        </ul>
        <div className="modal-actions">
          <button onClick={newFolder}>＋ Dossier</button>
          <span className="spacer" />
          <button onClick={onCancel}>Annuler</button>
          <button
            className="primary"
            disabled={blocked(dir) || paths.every((p) => dirname(p) === dir)}
            onClick={() => onConfirm(dir)}
          >
            Déplacer ici
          </button>
        </div>
      </div>
    </div>
  );
}
