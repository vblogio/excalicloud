// Icônes proposées pour les dossiers (jeu Tabler, comme icons.tsx). Le nom est
// enregistré côté serveur (.meta.json) : ne pas renommer une clé existante.
import { FolderIcon, icon } from "./icons";

type Icon = ReturnType<typeof icon>;

export const FOLDER_ICONS: { name: string; label: string; Icon: Icon }[] = [
  {
    name: "briefcase",
    label: "Projet",
    Icon: icon(
      <>
        <rect x="3" y="7" width="18" height="13" rx="2" />
        <path d="M8 7V5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M12 12v.01M3 13a20 20 0 0 0 18 0" />
      </>,
    ),
  },
  {
    name: "rocket",
    label: "Production",
    Icon: icon(
      <>
        <path d="M4 13a8 8 0 0 1 7 7 6 6 0 0 0 3-5 9 9 0 0 0 6-8 3 3 0 0 0-3-3 9 9 0 0 0-8 6 6 6 0 0 0-5 3" />
        <path d="M7 14a6 6 0 0 0-3 6 6 6 0 0 0 6-3" />
        <circle cx="15" cy="9" r="1" />
      </>,
    ),
  },
  {
    name: "server",
    label: "Serveurs",
    Icon: icon(
      <>
        <rect x="3" y="4" width="18" height="8" rx="3" />
        <rect x="3" y="12" width="18" height="8" rx="3" />
        <path d="M7 8v.01M7 16v.01" />
      </>,
    ),
  },
  {
    name: "cpu",
    label: "Calcul",
    Icon: icon(
      <>
        <rect x="5" y="5" width="14" height="14" rx="1" />
        <rect x="9" y="9" width="6" height="6" />
        <path d="M3 10h2M3 14h2M10 3v2M14 3v2M21 10h-2M21 14h-2M14 21v-2M10 21v-2" />
      </>,
    ),
  },
  {
    name: "database",
    label: "Stockage",
    Icon: icon(
      <>
        <ellipse cx="12" cy="6" rx="8" ry="3" />
        <path d="M4 6v6a8 3 0 0 0 16 0V6M4 12v6a8 3 0 0 0 16 0v-6" />
      </>,
    ),
  },
  {
    name: "box",
    label: "Conteneurs",
    Icon: icon(<path d="M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3M12 12l8-4.5M12 12v9M12 12 4 7.5" />),
  },
  {
    name: "cloud",
    label: "Cloud",
    Icon: icon(
      <path d="M6.66 18C4.08 18 2 15.99 2 13.52s2.09-4.49 4.66-4.49c.39-1.76 1.79-3.2 3.67-3.77 1.88-.57 3.96-.19 5.44 1 1.49 1.19 2.16 3.01 1.77 4.77h.99c1.91 0 3.46 1.56 3.46 3.49S20.45 18 18.54 18H6.66" />,
    ),
  },
  {
    name: "network",
    label: "Réseau",
    Icon: icon(
      <>
        <circle cx="12" cy="9" r="6" />
        <path d="M12 3c1.33.33 2 2.33 2 6s-.67 5.67-2 6M12 3c-1.33.33-2 2.33-2 6s.67 5.67 2 6M6 9h12M3 20h7M14 20h7M12 15v3" />
        <circle cx="12" cy="20" r="2" />
      </>,
    ),
  },
  {
    name: "sitemap",
    label: "Topologie",
    Icon: icon(
      <>
        <rect x="3" y="15" width="6" height="6" rx="2" />
        <rect x="15" y="15" width="6" height="6" rx="2" />
        <rect x="9" y="3" width="6" height="6" rx="2" />
        <path d="M6 15v-1a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1M12 9v3" />
      </>,
    ),
  },
  {
    name: "router",
    label: "Routeur",
    Icon: icon(
      <>
        <rect x="3" y="13" width="18" height="8" rx="2" />
        <path d="M17 17v.01M13 17v.01M15 13v-2M11.75 8.75a4 4 0 0 1 6.5 0M8.5 6.5a8 8 0 0 1 13 0" />
      </>,
    ),
  },
  {
    name: "world",
    label: "Internet",
    Icon: icon(
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M3.6 9h16.8M3.6 15h16.8M11.5 3a17 17 0 0 0 0 18M12.5 3a17 17 0 0 1 0 18" />
      </>,
    ),
  },
  {
    name: "shield",
    label: "Sécurité",
    Icon: icon(<path d="M12 3a12 12 0 0 0 8.5 3A12 12 0 0 1 12 21 12 12 0 0 1 3.5 6 12 12 0 0 0 12 3" />),
  },
  {
    name: "lock",
    label: "Confidentiel",
    Icon: icon(
      <>
        <rect x="5" y="11" width="14" height="10" rx="2" />
        <circle cx="12" cy="16" r="1" />
        <path d="M8 11V7a4 4 0 1 1 8 0v4" />
      </>,
    ),
  },
  {
    name: "desktop",
    label: "Postes",
    Icon: icon(
      <>
        <rect x="3" y="4" width="18" height="12" rx="1" />
        <path d="M7 20h10M9 16v4M15 16v4" />
      </>,
    ),
  },
  {
    name: "code",
    label: "Code",
    Icon: icon(<path d="M7 8l-4 4 4 4M17 8l4 4-4 4M14 4l-4 16" />),
  },
  {
    name: "tool",
    label: "Outils",
    Icon: icon(<path d="M7 10h3V7L6.5 3.5a6 6 0 0 1 8 8l6 6a2 2 0 0 1-3 3l-6-6a6 6 0 0 1-8-8L7 10" />),
  },
  {
    name: "file-text",
    label: "Documents",
    Icon: icon(
      <>
        <path d="M14 3v4a1 1 0 0 0 1 1h4" />
        <path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2zM9 9h1M9 13h6M9 17h6" />
      </>,
    ),
  },
  {
    name: "book",
    label: "Documentation",
    Icon: icon(<path d="M3 19a9 9 0 0 1 9 0 9 9 0 0 1 9 0M3 6a9 9 0 0 1 9 0 9 9 0 0 1 9 0M3 6v13M12 6v13M21 6v13" />),
  },
  {
    name: "presentation",
    label: "Présentations",
    Icon: icon(<path d="M3 4h18M4 4v10a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V4M12 16v4M9 20h6M8 12l3-3 2 2 3-3" />),
  },
  {
    name: "chart",
    label: "Indicateurs",
    Icon: icon(
      <>
        <rect x="3" y="12" width="6" height="8" rx="1" />
        <rect x="9" y="8" width="6" height="12" rx="1" />
        <rect x="15" y="4" width="6" height="16" rx="1" />
        <path d="M4 20h14" />
      </>,
    ),
  },
  {
    name: "bulb",
    label: "Idées",
    Icon: icon(
      <path d="M3 12h1m8-9v1m8 8h1M5.6 5.6l.7.7m12.1-.7-.7.7M9 16a5 5 0 1 1 6 0 3.5 3.5 0 0 0-1 3 2 2 0 0 1-4 0 3.5 3.5 0 0 0-1-3M9.7 17h4.6" />,
    ),
  },
  {
    name: "users",
    label: "Équipe",
    Icon: icon(
      <>
        <circle cx="9" cy="7" r="4" />
        <path d="M3 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2M16 3.13a4 4 0 0 1 0 7.75M21 21v-2a4 4 0 0 0-3-3.85" />
      </>,
    ),
  },
  {
    name: "building",
    label: "Client / site",
    Icon: icon(
      <path d="M3 21h18M9 8h1M9 12h1M9 16h1M14 8h1M14 12h1M14 16h1M5 21V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16" />,
    ),
  },
  {
    name: "calendar",
    label: "Planning",
    Icon: icon(
      <>
        <rect x="4" y="5" width="16" height="16" rx="2" />
        <path d="M16 3v4M8 3v4M4 11h16" />
      </>,
    ),
  },
  {
    name: "photo",
    label: "Images",
    Icon: icon(
      <>
        <rect x="3" y="3" width="18" height="18" rx="3" />
        <path d="M15 8h.01M3 16l5-5c.93-.89 2.07-.89 3 0l5 5M14 14l1-1c.93-.89 2.07-.89 3 0l3 3" />
      </>,
    ),
  },
  {
    name: "flag",
    label: "Jalons",
    Icon: icon(<path d="M5 5a5 5 0 0 1 7 0 5 5 0 0 0 7 0v9a5 5 0 0 1-7 0 5 5 0 0 0-7 0V5M5 21v-7" />),
  },
  {
    name: "archive",
    label: "Archives",
    Icon: icon(
      <>
        <rect x="3" y="4" width="18" height="4" rx="2" />
        <path d="M5 8v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8M10 12h4" />
      </>,
    ),
  },
];

const BY_NAME = new Map(FOLDER_ICONS.map((i) => [i.name, i.Icon]));

/** Icône d'un dossier : celle choisie, ou le dossier classique (nom absent ou inconnu). */
export function FolderGlyph({ name, size }: { name?: string; size?: number }) {
  const Icon = (name && BY_NAME.get(name)) || FolderIcon;
  return <Icon size={size} />;
}
