import { useEffect, useRef, useState } from "react";
import { api, type Channel, type Settings, type Share, type UpdateStatus } from "./api";
import { CopyIcon, TrashIcon } from "./icons";
import { fmtExpiry } from "./ShareDialog";
import { INSTALLED, pendingVersion, rebuildCommand } from "./update";

export const DEFAULT_TITLE = "Excalidraw";
const MAX_TITLE = 80; // voir storage.MaxTitleLen

interface Props {
  settings: Settings;
  update: UpdateStatus | null;
  /** Ouvert depuis l'icône de mise à jour : affiche directement la section Excalidraw. */
  focusExcalidraw?: boolean;
  onChange: (s: Settings) => void;
  onClose: () => void;
}

const CHANNELS: { id: Channel; label: string; hint: string }[] = [
  { id: "latest", label: "Stable", hint: "latest : versions publiées officiellement (recommandé)." },
  {
    id: "next",
    label: "Préversion",
    hint: "next : construite à chaque modification de la branche principale d'Excalidraw. Nouveautés en avance, stabilité non garantie. Son numéro (0.18.0-<commit>) est inférieur à la dernière stable alors que le code est plus récent.",
  },
];

/** Paramètres partagés par tous les utilisateurs : titre, logo, version d'Excalidraw. */
export function SettingsDialog({ settings, update, focusExcalidraw, onChange, onClose }: Props) {
  const [title, setTitle] = useState(settings.title);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [shares, setShares] = useState<Share[] | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const exSection = useRef<HTMLElement>(null);

  useEffect(() => {
    if (focusExcalidraw) exSection.current?.scrollIntoView({ block: "start" });
  }, [focusExcalidraw]);

  const loadShares = () =>
    api
      .shares()
      .then(setShares)
      .catch((e) => setError(e.message));

  useEffect(() => {
    loadShares();
  }, []);

  const revoke = async (op: () => Promise<void>, question: string) => {
    if (!confirm(question)) return;
    setError(null);
    try {
      await op();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
    loadShares();
  };

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
  const pending = pendingVersion(update, settings.channel);
  const cmd = pending ? rebuildCommand(pending) : null;
  const copy = () =>
    cmd &&
    navigator.clipboard?.writeText(cmd).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form
        className="modal customize settings"
        role="dialog"
        aria-modal="true"
        aria-label="Paramètres"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          run(() => api.setTitle(title), true);
        }}
      >
        <h3>Paramètres</h3>

        <h4>Apparence</h4>
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

        <section className="field" ref={exSection}>
          <h4>Excalidraw</h4>
          <span>Version à suivre</span>
          <div className="radio-group" role="radiogroup" aria-label="Canal d'Excalidraw">
            {CHANNELS.map((c) => (
              <button
                key={c.id}
                type="button"
                role="radio"
                aria-checked={settings.channel === c.id}
                className={settings.channel === c.id ? "on" : ""}
                disabled={busy}
                title={c.hint}
                onClick={() => settings.channel !== c.id && run(() => api.setChannel(c.id))}
              >
                {c.label}
                <small>{update?.tags?.[c.id] ?? "?"}</small>
              </button>
            ))}
          </div>
          <small className="muted">{CHANNELS.find((c) => c.id === settings.channel)?.hint}</small>
          <p className="versions">
            Version installée : <b>{INSTALLED}</b>
            <br />
            {!update?.enabled
              ? "Vérification des mises à jour désactivée (UPDATE_CHECK=false)."
              : !update.tags && !update.latest
                ? "Registre npm injoignable pour l'instant : versions publiées inconnues."
                : pending
                  ? <>À installer : <b>{pending}</b></>
                  : "À jour pour ce canal."}
          </p>
          {cmd && (
            <>
              <small className="muted">
                Excalidraw est compilé dans l'image : changer de canal ou de version ne prend effet
                qu'après reconstruction, depuis le dossier du docker-compose :
              </small>
              <div className="cmd">
                <code>{cmd}</code>
                <button type="button" className="icon-btn" onClick={copy} title="Copier" aria-label="Copier la commande">
                  <CopyIcon />
                </button>
              </div>
              <small className="muted">
                {copied
                  ? "Commande copiée."
                  : <>Pour conserver ce choix aux reconstructions suivantes, mettre <code>EXCALIDRAW_VERSION={pending}</code> dans <code>.env</code>. Tester l'image avant la mise en service : les versions 0.x peuvent changer d'API.</>}
              </small>
            </>
          )}
        </section>

        <section className="field">
          <h4>Liens de partage</h4>
          {shares?.length ? (
            <ul className="share-list all">
              {shares.map((s) => (
                <li key={s.id}>
                  <div className="cmd">
                    <code title={api.shareUrl(s.token)}>{s.path}</code>
                    <button
                      type="button"
                      className="icon-btn danger"
                      onClick={() => revoke(() => api.revokeShare(s.id), `Révoquer le lien vers « ${s.path} » ?`)}
                      title="Révoquer"
                      aria-label={`Révoquer le lien vers ${s.path}`}
                    >
                      <TrashIcon />
                    </button>
                  </div>
                  <small className="muted">
                    Créé le {new Date(s.created).toLocaleDateString(undefined, { dateStyle: "short" })} · {fmtExpiry(s)}
                  </small>
                </li>
              ))}
            </ul>
          ) : (
            <small className="muted">
              Aucun lien actif. Pour partager un schéma en lecture seule : icône de partage de la barre du haut, ou menu
              « … » de l'explorateur.
            </small>
          )}
          {!!shares?.length && (
            <button
              type="button"
              className="danger"
              style={{ alignSelf: "flex-start" }}
              onClick={() =>
                revoke(
                  api.revokeAllShares,
                  "Révoquer tous les liens de partage ? La clé de signature est renouvelée : aucun lien existant ne fonctionnera plus.",
                )
              }
            >
              Tout révoquer
            </button>
          )}
        </section>

        {error && <div className="error">{error}</div>}

        <div className="modal-actions">
          <a className="link" href="https://github.com/excalidraw/excalidraw/releases" target="_blank" rel="noreferrer">
            Notes de version
          </a>
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
