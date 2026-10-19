// Mise en forme des dates, tailles et auteurs (explorateur, panneau Détails).

export const fmtDate = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });

export const fmtSize = (n: number) =>
  n < 1024 ? `${n} o` : n < 1 << 20 ? `${(n / 1024).toFixed(1)} Ko` : `${(n / (1 << 20)).toFixed(1)} Mo`;

export const plural = (n: number, word: string) => `${n} ${word}${n > 1 ? "s" : ""}`;

const relative = new Intl.RelativeTimeFormat("fr", { numeric: "auto" });

/** « à l'instant », « il y a 5 minutes », « hier »… puis la date au-delà d'un mois. */
export const fmtRelative = (iso: string) => {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return "à l'instant";
  if (s < 3600) return relative.format(-Math.round(s / 60), "minute");
  if (s < 86400) return relative.format(-Math.round(s / 3600), "hour");
  if (s < 30 * 86400) return relative.format(-Math.round(s / 86400), "day");
  return new Date(iso).toLocaleDateString(undefined, { dateStyle: "medium" });
};

/** Auteur affichable : rien pour l'utilisateur unique sans authentification. */
export const who = (id?: string) => (id && id !== "local" ? id : "");
