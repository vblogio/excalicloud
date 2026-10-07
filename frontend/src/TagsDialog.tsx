import { useEffect, useState } from "react";
import { api, basename, stripExt, type Entry, type TagCount } from "./api";
import { CloseIcon, TagIcon } from "./icons";

interface Props {
  /** Éléments concernés, avec leurs tags actuels. */
  items: Entry[];
  /** Tags déjà utilisés (suggestions). */
  known: TagCount[];
  /** Appelé après chaque modification pour rafraîchir l'explorateur. */
  onChanged: () => Promise<void>;
  onClose: () => void;
}

const key = (t: string) => t.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();

/** Ajout et retrait de tags sur un ou plusieurs éléments. */
export function TagsDialog({ items, known, onChanged, onClose }: Props) {
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const paths = items.map((e) => e.path);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Tags présents et nombre d'éléments qui les portent.
  const present = new Map<string, { name: string; count: number }>();
  for (const e of items)
    for (const t of e.tags ?? []) {
      const p = present.get(key(t));
      if (p) p.count++;
      else present.set(key(t), { name: t, count: 1 });
    }
  const suggestions = known.filter((t) => !present.has(key(t.name)) || present.get(key(t.name))!.count < items.length);

  const run = async (add: string[], remove: string[]) => {
    setBusy(true);
    setError(null);
    try {
      await api.editTags(paths, add, remove);
      await onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const add = (raw: string) => {
    const tags = raw
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
    if (tags.length === 0) return;
    setInput("");
    run(tags, []);
  };

  const title =
    items.length === 1
      ? `Tags · ${items[0].isDir ? items[0].name : stripExt(basename(items[0].path))}`
      : `Tags · ${items.length} éléments`;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal tags-dialog" role="dialog" aria-modal="true" aria-label={title} onClick={(e) => e.stopPropagation()}>
        <h3>{title}</h3>

        <div className="chips">
          {[...present.values()].map((t) => (
            <span key={t.name} className="chip">
              {t.name}
              {items.length > 1 && t.count < items.length && (
                <small className="muted">
                  {" "}
                  {t.count}/{items.length}
                </small>
              )}
              <button
                className="icon-btn"
                disabled={busy}
                onClick={() => run([], [t.name])}
                title={`Retirer « ${t.name} »`}
                aria-label={`Retirer ${t.name}`}
              >
                <CloseIcon size={12} />
              </button>
            </span>
          ))}
          {present.size === 0 && <span className="muted">Aucun tag.</span>}
        </div>

        <form
          className="tag-add"
          onSubmit={(e) => {
            e.preventDefault();
            add(input);
          }}
        >
          <label className="search">
            <TagIcon />
            <input
              autoFocus
              value={input}
              list="known-tags"
              maxLength={200}
              placeholder="Ajouter des tags (séparés par des virgules)"
              onChange={(e) => setInput(e.target.value)}
            />
          </label>
          <datalist id="known-tags">
            {known.map((t) => (
              <option key={t.name} value={t.name} />
            ))}
          </datalist>
          <button type="submit" className="primary" disabled={busy || !input.trim()}>
            Ajouter
          </button>
        </form>

        {suggestions.length > 0 && (
          <div className="chips suggestions">
            {suggestions.slice(0, 30).map((t) => (
              <button key={t.name} className="chip" disabled={busy} onClick={() => run([t.name], [])} title={`Ajouter « ${t.name} »`}>
                + {t.name}
              </button>
            ))}
          </div>
        )}

        {error && <div className="error">{error}</div>}

        <div className="modal-actions">
          <span className="spacer" />
          <button onClick={onClose}>Fermer</button>
        </div>
      </div>
    </div>
  );
}
