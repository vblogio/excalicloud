import { useEffect, useState } from "react";
import { api, type Me } from "./api";
import { LogoutIcon } from "./icons";

interface Props {
  /** Appelé avant la déconnexion (enregistrement du schéma ouvert). */
  beforeLogout: () => Promise<unknown>;
}

/** Utilisateur connecté et bouton de déconnexion (mode OIDC uniquement). */
export function UserMenu({ beforeLogout }: Props) {
  const [me, setMe] = useState<Me | null>(null);

  useEffect(() => {
    api
      .me()
      .then(setMe)
      .catch(() => setMe(null));
  }, []);

  if (!me?.user || !me.logout) return null;
  const logout = me.logout;
  const who = me.user.email && me.user.email !== me.user.name ? `${me.user.name} (${me.user.email})` : me.user.name;

  return (
    <button
      className="icon-btn"
      aria-label={`Se déconnecter (${who})`}
      title={`Connecté : ${who}\nCliquer pour se déconnecter`}
      onClick={() => beforeLogout().finally(() => location.assign(logout))}
    >
      <LogoutIcon />
    </button>
  );
}
