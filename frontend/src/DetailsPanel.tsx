import { useEffect, useState } from "react";
import { api, dirname, displayPath, stripExt, type Hit, type Share, type Version } from "./api";
import { fmtDate, fmtRelative, fmtSize, plural, who } from "./format";
import { CloseIcon, FolderIcon, HistoryIcon, ShareIcon, StarFilledIcon, StarIcon } from "./icons";
import { Thumb } from "./Thumb";
import { expectedThumb } from "./thumbs";

interface Props {
  entry: Hit;
  onOpen: (path: string) => void;
  onLocate: (dir: string) => void;
  onHistory: (path: string) => void;
  onShare: (path: string) => void;
  onFavorite: (e: Hit) => void;
  onTag: (tag: string) => void;
  onClose: () => void;
}

const LAST_VERSIONS = 5;

/** Panneau Détails de la vue Galerie : métadonnées complètes d'un schéma. */
export function DetailsPanel({ entry: e, onOpen, onLocate, onHistory, onShare, onFavorite, onTag, onClose }: Props) {
  const [versions, setVersions] = useState<Version[] | null>(null);
  const [shares, setShares] = useState<Share[] | null>(null);
  const d = e.details ?? {};

  // Versions et liens : relus quand le schéma change (modTime) ou qu'on en change.
  useEffect(() => {
    let live = true;
    setVersions(null);
    setShares(null);
    api.history(e.path).then((v) => live && setVersions(v), () => live && setVersions([]));
    api.shares(e.path).then((s) => live && setShares(s), () => live && setShares([]));
    return () => {
      live = false;
    };
  }, [e.path, e.modTime]);

  const content = [
    d.elements ? plural(d.elements, "élément") : "vide",
    d.texts ? plural(d.texts, "texte") : "",
    d.arrows ? plural(d.arrows, "flèche") : "",
    d.images ? plural(d.images, "image") : "",
    d.frames ? plural(d.frames, "cadre") : "",
  ].filter(Boolean);
  const location = displayPath(dirname(e.path)) || "Racine";

  return (
    <aside className="details-panel" aria-label="Détails du schéma">
      <div className="details-head">
        <h3 title={e.name}>{stripExt(e.name)}</h3>
        <button className="icon-btn" onClick={onClose} aria-label="Fermer les détails" title="Fermer (Échap)">
          <CloseIcon />
        </button>
      </div>
      <button className="details-shot" onClick={() => onOpen(e.path)} title="Ouvrir le schéma">
        <Thumb path={e.path} version={e.modTime} minSize={expectedThumb(d.width, d.height)} />
      </button>
      <button className="details-where" onClick={() => onLocate(dirname(e.path))} title="Ouvrir l'emplacement">
        <FolderIcon size={14} />
        {location}
      </button>
      <div className="details-actions">
        <button className="primary" onClick={() => onOpen(e.path)}>
          Ouvrir
        </button>
        <button onClick={() => onShare(e.path)}>
          <ShareIcon />
          Partager
        </button>
        <button onClick={() => onHistory(e.path)}>
          <HistoryIcon />
          Historique
        </button>
        <button
          className="square"
          onClick={() => onFavorite(e)}
          title={e.favorite ? "Retirer des favoris" : "Ajouter aux favoris"}
          aria-label={e.favorite ? "Retirer des favoris" : "Ajouter aux favoris"}
          aria-pressed={!!e.favorite}
        >
          {e.favorite ? <StarFilledIcon /> : <StarIcon />}
        </button>
      </div>

      <dl>
        <dt>Modifié</dt>
        <dd>
          {fmtDate(e.modTime)} <span className="muted">({fmtRelative(e.modTime)})</span>
          {who(e.editedBy) && <span className="muted"> par {e.editedBy}</span>}
        </dd>
        {d.created && (
          <>
            <dt>Créé</dt>
            <dd>
              {fmtDate(d.created)}
              {who(d.createdBy) ? (
                <span className="muted"> par {d.createdBy}</span>
              ) : (
                <span className="muted" title="Date de la plus ancienne version conservée">
                  {" "}
                  (plus ancienne version)
                </span>
              )}
            </dd>
          </>
        )}
        <dt>Taille</dt>
        <dd>{fmtSize(e.size)}</dd>
        <dt>Contenu</dt>
        <dd>{content.join(", ")}</dd>
        {!!d.width && !!d.height && (
          <>
            <dt>Dimensions</dt>
            <dd>
              {d.width.toLocaleString()} × {d.height.toLocaleString()} px
            </dd>
          </>
        )}
        {d.background && (
          <>
            <dt>Fond</dt>
            <dd className="swatch-line">
              <span className="swatch" style={{ background: d.background }} />
              {d.background}
            </dd>
          </>
        )}
        {!!e.tags?.length && (
          <>
            <dt>Tags</dt>
            <dd className="tags">
              {e.tags.map((t) => (
                <button key={t} className="chip" onClick={() => onTag(t)} title={`Filtrer sur « ${t} »`}>
                  {t}
                </button>
              ))}
            </dd>
          </>
        )}
      </dl>

      {!!d.frameNames?.length && (
        <section>
          <h4>Cadres</h4>
          <ul>
            {d.frameNames.map((f, i) => (
              <li key={i}>{f}</li>
            ))}
            {(d.frames ?? 0) > d.frameNames.length && (
              <li className="muted">et {d.frames! - d.frameNames.length} autres</li>
            )}
          </ul>
        </section>
      )}

      <section>
        <h4>Versions{versions && versions.length > 0 ? ` (${versions.length})` : ""}</h4>
        {versions === null ? (
          <p className="muted">Chargement…</p>
        ) : versions.length === 0 ? (
          <p className="muted">Aucune version enregistrée</p>
        ) : (
          <ul>
            {versions.slice(0, LAST_VERSIONS).map((v) => (
              <li key={v.id}>
                <span>{fmtDate(v.time)}</span>
                <span className="muted">{fmtSize(v.size)}</span>
              </li>
            ))}
            {versions.length > LAST_VERSIONS && (
              <li>
                <button className="link" onClick={() => onHistory(e.path)}>
                  Tout l'historique…
                </button>
              </li>
            )}
          </ul>
        )}
      </section>

      <section>
        <h4>Partages actifs{shares && shares.length > 0 ? ` (${shares.length})` : ""}</h4>
        {shares === null ? (
          <p className="muted">Chargement…</p>
        ) : shares.length === 0 ? (
          <p className="muted">Aucun lien de partage</p>
        ) : (
          <ul>
            {shares.map((s) => (
              <li key={s.id}>
                <span title={api.shareUrl(s.token)}>Créé le {fmtDate(s.created)}</span>
                <span className="muted">{s.expires ? `expire le ${fmtDate(s.expires)}` : "sans expiration"}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </aside>
  );
}
