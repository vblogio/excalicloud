import { useEffect, useState } from "react";
import { api } from "./api";
import { CopyIcon, UpdateIcon } from "./icons";

const CURRENT = __EXCALIDRAW_VERSION__;
const POLL_MS = 60 * 60 * 1000; // le serveur, lui, interroge npm toutes les 12 h par défaut

/** Compare deux versions x.y.z (suffixe de pré-version ignoré) : > 0 si a est plus récente. */
export function compareVersions(a: string, b: string): number {
  const parse = (v: string) => v.split("-")[0].split(".").map((n) => parseInt(n, 10) || 0);
  const [pa, pb] = [parse(a), parse(b)];
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d) return d;
  }
  return 0;
}

/**
 * Icône affichée dans la barre du haut quand une version d'Excalidraw plus récente
 * que celle embarquée est publiée ; un clic donne la commande de reconstruction.
 */
export function UpdateBadge() {
  const [latest, setLatest] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const check = () =>
      api
        .update()
        .then((s) => setLatest(s.latest && compareVersions(s.latest, CURRENT) > 0 ? s.latest : null))
        .catch(() => {}); // silencieux : vérification facultative
    check();
    const t = setInterval(check, POLL_MS);
    return () => clearInterval(t);
  }, []);

  if (!latest) return null;

  const cmd = `EXCALIDRAW_VERSION=${latest} docker compose up -d --build`;
  const copy = () =>
    navigator.clipboard?.writeText(cmd).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });

  return (
    <>
      <button
        className="icon-btn update-btn"
        title={`Excalidraw ${latest} disponible (installée : ${CURRENT}) — reconstruire l'image`}
        aria-label={`Mise à jour d'Excalidraw disponible : ${latest}`}
        onClick={() => setOpen(true)}
      >
        <UpdateIcon />
      </button>
      {open && (
        <div className="modal-backdrop" onClick={() => setOpen(false)}>
          <div
            className="modal update"
            role="dialog"
            aria-modal="true"
            aria-label="Mise à jour d'Excalidraw"
            onClick={(e) => e.stopPropagation()}
          >
            <h3>Mise à jour d'Excalidraw disponible</h3>
            <p>
              Version installée : <b>{CURRENT}</b>
              <br />
              Dernière version publiée : <b>{latest}</b>
            </p>
            <p>Reconstruire et relancer le conteneur depuis le dossier du docker-compose :</p>
            <div className="cmd">
              <code>{cmd}</code>
              <button className="icon-btn" onClick={copy} title="Copier" aria-label="Copier la commande">
                <CopyIcon />
              </button>
            </div>
            <p className="muted small">
              {copied
                ? "Commande copiée."
                : "Les versions 0.x d'Excalidraw peuvent changer d'API : tester l'image avant la mise en service. Pour figer la version dans le dépôt, voir « Mettre à jour Excalidraw » dans le README."}
            </p>
            <div className="modal-actions">
              <span className="spacer" />
              <a className="link" href={`https://github.com/excalidraw/excalidraw/releases`} target="_blank" rel="noreferrer">
                Notes de version
              </a>
              <button className="primary" onClick={() => setOpen(false)}>
                Fermer
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
