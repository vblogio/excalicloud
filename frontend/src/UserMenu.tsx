import { useEffect, useState } from "react";
import { api, type Me } from "./api";
import { LogoutIcon } from "./icons";

interface Props {
  /** Appelé avant la déconnexion (enregistrement du schéma ouvert). */
  beforeLogout: () => Promise<unknown>;
}

/**
 * Utilisateur connecté : cartouche du mode (« oidc » ou « local ») suivi de
 * l'identifiant, puis bouton de déconnexion (mode OIDC uniquement).
 */
export function UserMenu({ beforeLogout }: Props) {
  const [me, setMe] = useState<Me | null>(null);

  useEffect(() => {
    api
      .me()
      .then(setMe)
      .catch(() => setMe(null));
  }, []);

  if (!me) return null;
  const oidc = me.mode === "oidc";
  // OIDC : l'identifiant de l'espace personnel (e-mail) ; local : le nom du compte.
  const id = oidc ? me.id : (me.user?.name ?? me.id);
  const who = me.user
    ? me.user.email && me.user.email !== me.user.name
      ? `${me.user.name} (${me.user.email})`
      : me.user.name
    : id;
  const logout = me.logout;

  return (
    <>
      <span className="whoami" title={`Connecté : ${who}\nEspace personnel : ${me.id}`}>
        <span className={`auth-tag ${oidc ? "oidc" : "local"}`}>{oidc ? "oidc" : "local"}</span>
        <span className="user-id">{id}</span>
      </span>
      {logout && (
        <button
          className="icon-btn"
          aria-label={`Se déconnecter (${who})`}
          title={`Connecté : ${who}\nCliquer pour se déconnecter`}
          onClick={() => beforeLogout().finally(() => location.assign(logout))}
        >
          <LogoutIcon />
        </button>
      )}
    </>
  );
}
