import { useEffect, useState } from "react";
import { api, basename, stripExt, type Share } from "./api";
import { CopyIcon, ShareIcon, TrashIcon } from "./icons";

interface Props {
  path: string;
  onClose: () => void;
}

const EXPIRY: { days: number; label: string }[] = [
  { days: 1, label: "1 jour" },
  { days: 7, label: "7 jours" },
  { days: 30, label: "30 jours" },
  { days: 0, label: "Sans expiration" },
];

export const fmtExpiry = (s: Share) =>
  s.expires
    ? `expire le ${new Date(s.expires).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" })}`
    : "sans expiration";

/** Liens de partage en lecture seule d'un schéma. */
export function ShareDialog({ path, onClose }: Props) {
  const [links, setLinks] = useState<Share[] | null>(null);
  const [days, setDays] = useState(7);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const name = stripExt(basename(path));

  const run = async (op: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await op();
      setLinks(await api.shares(path));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    run(async () => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const copy = (s: Share) =>
    navigator.clipboard?.writeText(api.shareUrl(s.token)).then(() => {
      setCopied(s.id);
      setTimeout(() => setCopied(null), 1500);
    });

  const create = () =>
    run(async () => {
      const s = await api.createShare(path, days);
      await copy(s)?.catch(() => {});
    });

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal share-dialog" role="dialog" aria-modal="true" aria-label={`Partager ${name}`} onClick={(e) => e.stopPropagation()}>
        <h3>Partager · {name}</h3>
        <small className="muted">
          Toute personne disposant du lien voit le schéma en lecture seule, sans compte, dans sa version enregistrée
          la plus récente. Le reste des schémas reste inaccessible.
        </small>

        <div className="share-create">
          <div className="radio-group" role="radiogroup" aria-label="Durée de validité">
            {EXPIRY.map((e) => (
              <button
                key={e.days}
                type="button"
                role="radio"
                aria-checked={days === e.days}
                className={days === e.days ? "on" : ""}
                onClick={() => setDays(e.days)}
              >
                {e.label}
              </button>
            ))}
          </div>
          <button className="primary" disabled={busy} onClick={create}>
            <ShareIcon />
            <span>Créer un lien</span>
          </button>
        </div>

        <ul className="share-list">
          {links?.map((s) => (
            <li key={s.id}>
              <div className="cmd">
                <code>{api.shareUrl(s.token)}</code>
                <button className="icon-btn" onClick={() => copy(s)} title="Copier le lien" aria-label="Copier le lien">
                  <CopyIcon />
                </button>
                <button
                  className="icon-btn danger"
                  disabled={busy}
                  onClick={() => confirm("Révoquer ce lien ? Il cessera de fonctionner immédiatement.") && run(() => api.revokeShare(s.id))}
                  title="Révoquer"
                  aria-label="Révoquer le lien"
                >
                  <TrashIcon />
                </button>
              </div>
              <small className="muted">
                {copied === s.id ? "Lien copié." : `Créé le ${new Date(s.created).toLocaleDateString(undefined, { dateStyle: "short" })} · ${fmtExpiry(s)}`}
              </small>
            </li>
          ))}
          {links?.length === 0 && <li className="muted empty">Aucun lien actif pour ce schéma.</li>}
        </ul>

        {error && <div className="error">{error}</div>}

        <div className="modal-actions">
          <span className="spacer" />
          <button onClick={onClose}>Fermer</button>
        </div>
      </div>
    </div>
  );
}
