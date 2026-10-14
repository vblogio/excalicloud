import { useEffect, useState } from "react";
import { api, type ApiToken } from "./api";
import { CopyIcon, TrashIcon } from "./icons";

const fmtDate = (d: string) => new Date(d).toLocaleDateString(undefined, { dateStyle: "short" });

/** Section « Accès API et MCP » des paramètres : adresse du serveur MCP et jetons de l'utilisateur. */
export function ApiAccess({ onError }: { onError: (msg: string | null) => void }) {
  const [tokens, setTokens] = useState<ApiToken[] | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const mcpUrl = `${location.origin}/mcp`;
  const cmd = created
    ? `claude mcp add --transport http excalicloud ${mcpUrl} --header "Authorization: Bearer ${created}"`
    : null;

  const load = () =>
    api
      .tokens()
      .then(setTokens)
      .catch((e) => onError(e.message));

  useEffect(() => {
    load();
  }, []);

  const copy = (what: string, text: string) =>
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(what);
      setTimeout(() => setCopied(null), 1500);
    });

  const create = async () => {
    if (!name.trim()) return;
    setBusy(true);
    onError(null);
    try {
      const t = await api.createToken(name);
      setCreated(t.token);
      setName("");
      load();
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (t: ApiToken) => {
    const what = t.kind === "oauth" ? `l'accès de « ${t.name} » (${t.client})` : `le jeton « ${t.name} »`;
    if (!confirm(`Révoquer ${what} ? Les clients qui l'utilisent n'auront plus accès.`)) return;
    onError(null);
    try {
      await api.revokeToken(t.id);
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e));
    }
    load();
  };

  return (
    <section className="field api-access">
      <h4>Accès API et MCP</h4>
      <span>Adresse du serveur MCP</span>
      <div className="cmd">
        <code>{mcpUrl}</code>
        <button type="button" className="icon-btn" onClick={() => copy("url", mcpUrl)} title="Copier" aria-label="Copier l'adresse">
          <CopyIcon />
        </button>
      </div>
      <small className="muted">
        {copied === "url" ? (
          "Adresse copiée."
        ) : (
          <>
            claude.ai, Claude Desktop ou mobile : <i>Paramètres › Connecteurs › Ajouter un connecteur personnalisé</i>{" "}
            avec cette adresse, sans identifiant OAuth ; la connexion ouvre ici une page d'autorisation. Claude Code et
            scripts : créer un jeton personnel.
          </>
        )}
      </small>

      <div className="token-create">
        <input
          className="text-input"
          value={name}
          maxLength={80}
          placeholder="Nom du jeton, ex. Claude Code portable"
          aria-label="Nom du nouveau jeton"
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              create();
            }
          }}
        />
        <button type="button" disabled={busy || !name.trim()} onClick={create}>
          Créer un jeton
        </button>
      </div>

      {created && cmd && (
        <div className="token-new">
          <small>Jeton créé : le copier maintenant, il ne sera plus affiché.</small>
          <div className="cmd">
            <code>{created}</code>
            <button type="button" className="icon-btn" onClick={() => copy("token", created)} title="Copier" aria-label="Copier le jeton">
              <CopyIcon />
            </button>
          </div>
          <small className="muted">{copied === "token" ? "Jeton copié." : "Commande Claude Code :"}</small>
          <div className="cmd">
            <code>{cmd}</code>
            <button type="button" className="icon-btn" onClick={() => copy("cmd", cmd)} title="Copier" aria-label="Copier la commande">
              <CopyIcon />
            </button>
          </div>
          {copied === "cmd" && <small className="muted">Commande copiée.</small>}
        </div>
      )}

      {tokens?.length ? (
        <ul className="share-list all">
          {tokens.map((t) => (
            <li key={t.id}>
              <div className="cmd">
                <code title={t.name}>{t.name}</code>
                <button
                  type="button"
                  className="icon-btn danger"
                  onClick={() => revoke(t)}
                  title="Révoquer"
                  aria-label={`Révoquer ${t.name}`}
                >
                  <TrashIcon />
                </button>
              </div>
              <small className="muted">
                {t.kind === "oauth" ? `Connecteur autorisé (${t.client})` : "Jeton personnel"} · créé le {fmtDate(t.created)}
                {t.lastUsed ? ` · utilisé le ${fmtDate(t.lastUsed)}` : " · jamais utilisé"}
              </small>
            </li>
          ))}
        </ul>
      ) : (
        tokens && <small className="muted">Aucun jeton ni connecteur autorisé.</small>
      )}
    </section>
  );
}
