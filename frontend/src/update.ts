import { useEffect, useState } from "react";
import { api, type Channel, type UpdateStatus } from "./api";

/** Version d'Excalidraw compilée dans cette image. */
export const INSTALLED = __EXCALIDRAW_VERSION__;

const POLL_MS = 60 * 60 * 1000; // le serveur, lui, interroge npm toutes les 12 h par défaut

export const rebuildCommand = (version: string) => `EXCALIDRAW_VERSION=${version} docker compose up -d --build`;

/**
 * Version publiée sur le canal choisi, si elle diffère de celle installée.
 * Simple comparaison d'égalité : sur le canal next, les numéros (0.18.0-<commit>)
 * sont inférieurs à la dernière stable alors que le code est plus récent, et
 * revenir de next à latest doit aussi proposer une reconstruction.
 */
export const pendingVersion = (status: UpdateStatus | null, channel: Channel) => {
  const v = status?.tags?.[channel] ?? (channel === "latest" ? status?.latest : undefined);
  return v && v !== INSTALLED ? v : null;
};

/** Versions publiées sur npm, relues périodiquement (silencieux en cas d'erreur). */
export function useUpdateStatus() {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  useEffect(() => {
    const check = () =>
      api
        .update()
        .then(setStatus)
        .catch(() => {}); // vérification facultative
    check();
    const t = setInterval(check, POLL_MS);
    return () => clearInterval(t);
  }, []);
  return status;
}
