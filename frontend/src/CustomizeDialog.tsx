import { useEffect, useRef, useState } from "react";
import { api, type Settings } from "./api";

export const DEFAULT_TITLE = "Excalidraw";
const MAX_TITLE = 80; // voir storage.MaxTitleLen

interface Props {
  settings: Settings;
  onChange: (s: Settings) => void;
  onClose: () => void;
}

/** Titre et logo de la barre du haut, partagés par tous les utilisateurs. */
export function CustomizeDialog({ settings, onChange, onClose }: Props) {
  const [title, setTitle] = useState(settings.title);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const run = async (op: () => Promise<Settings>, close = false) => {
    setBusy(true);
    setError(null);
    try {
      onChange(await op());
      if (close) onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const onFile = (f: File | undefined) => {
    if (!f) return;
    if (f.type !== "image/png") {
      setError("Seuls les fichiers PNG sont acceptés.");
      return;
    }
    run(() => api.uploadLogo(f));
  };

  const dirty = title.trim() !== settings.title;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form
        className="modal customize"
        role="dialog"
        aria-modal="true"
        aria-label="Personnaliser la barre de titre"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          run(() => api.setTitle(title), true);
        }}
      >
        <h3>Personnaliser</h3>

        <label className="field">
          <span>Titre affiché</span>
          <input
            className="text-input"
            value={title}
            maxLength={MAX_TITLE}
            placeholder={DEFAULT_TITLE}
            autoFocus
            onChange={(e) => setTitle(e.target.value)}
          />
        </label>

        <div className="field">
          <span>Logo</span>
          <div className="logo-edit">
            {settings.logo ? (
              <img className="logo big" src={api.logoUrl(settings.logo)} alt="Logo actuel" />
            ) : (
              <span className="logo big placeholder-logo muted" aria-hidden>
                ?
              </span>
            )}
            <div className="logo-actions">
              <button type="button" disabled={busy} onClick={() => fileInput.current?.click()}>
                {settings.logo ? "Remplacer…" : "Choisir un PNG…"}
              </button>
              {settings.logo && (
                <button type="button" className="danger" disabled={busy} onClick={() => run(api.removeLogo)}>
                  Retirer
                </button>
              )}
              <small className="muted">PNG, recadré au centre et réduit à 64×64 px, affiché en rond.</small>
            </div>
            <input
              ref={fileInput}
              type="file"
              accept="image/png"
              hidden
              onChange={(e) => {
                onFile(e.target.files?.[0]);
                e.target.value = "";
              }}
            />
          </div>
        </div>

        {error && <div className="error">{error}</div>}

        <div className="modal-actions">
          <span className="spacer" />
          <button type="button" onClick={onClose}>
            Fermer
          </button>
          <button type="submit" className="primary" disabled={busy || !dirty}>
            Enregistrer le titre
          </button>
        </div>
      </form>
    </div>
  );
}
