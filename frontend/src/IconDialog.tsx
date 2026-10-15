import { useEffect, useState } from "react";
import { api, type Entry } from "./api";
import { FOLDER_ICONS, FolderGlyph } from "./folderIcons";

interface Props {
  /** Dossiers concernés. */
  items: Entry[];
  /** Appelé après le changement pour rafraîchir l'explorateur. */
  onChanged: () => Promise<void>;
  onClose: () => void;
}

/** Choix de l'icône d'un ou plusieurs dossiers (partagée par tous les utilisateurs). */
export function IconDialog({ items, onChanged, onClose }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Icône commune aux dossiers (sinon aucune n'est marquée).
  const current = items.every((e) => (e.icon ?? "") === (items[0].icon ?? "")) ? (items[0].icon ?? "") : null;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const choose = async (name: string) => {
    setBusy(true);
    setError(null);
    try {
      await api.setIcon(
        items.map((e) => e.path),
        name,
      );
      await onChanged();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };

  const title = items.length === 1 ? `Icône · ${items[0].name}` : `Icône · ${items.length} dossiers`;
  const choice = (name: string, label: string) => (
    <button
      key={name || "default"}
      className={`folder-icon-choice${current === name ? " current" : ""}`}
      disabled={busy}
      onClick={() => choose(name)}
      title={label}
      aria-label={label}
      aria-pressed={current === name}
    >
      <span className="folder">
        <FolderGlyph name={name} size={24} />
      </span>
      <small>{label}</small>
    </button>
  );

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal icon-dialog" role="dialog" aria-modal="true" aria-label={title} onClick={(e) => e.stopPropagation()}>
        <h3>{title}</h3>
        <div className="folder-icon-grid">
          {choice("", "Par défaut")}
          {FOLDER_ICONS.map((i) => choice(i.name, i.label))}
        </div>
        {error && <div className="error">{error}</div>}
        <div className="modal-actions">
          <span className="spacer" />
          <button onClick={onClose}>Fermer</button>
        </div>
      </div>
    </div>
  );
}
