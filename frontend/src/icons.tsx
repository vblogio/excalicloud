// Icônes de l'interface : jeu Papicons (formes pleines, couleur héritée via currentColor).
// Les tracés Papicons sont dans papicons.ts ; les quelques icônes absentes du jeu sont
// dessinées ici dans le même esprit (formes pleines arrondies, ou trait épais à bouts ronds).
import { useId, type ReactNode } from "react";
import * as P from "./papicons";
import type { PapPath } from "./papicons";

type IconProps = { size?: number };

/** Identifiant de masque SVG unique et sûr dans url(#…). */
function useMaskId() {
  return "m" + useId().replace(/[^a-zA-Z0-9_-]/g, "");
}

function Svg({ size = 16, children }: { size?: number; children: ReactNode }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      {children}
    </svg>
  );
}

const paths = (p: PapPath[]) =>
  p.map((x, i) => <path key={i} d={x.d} fillRule={x.evenodd ? "evenodd" : undefined} clipRule={x.evenodd ? "evenodd" : undefined} />);

/** Icône Papicons telle quelle. */
export const pap = (p: PapPath[], transform?: string) =>
  function Icon({ size }: IconProps) {
    return (
      <Svg size={size}>
        <g transform={transform}>{paths(p)}</g>
      </Svg>
    );
  };

/** Icône au trait, pour les icônes de dossiers sans équivalent Papicons (trait 2, plus appuyé que Tabler). */
export const icon = (children: ReactNode) =>
  function Icon({ size = 16 }: IconProps) {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
        {children}
      </svg>
    );
  };

/** Icône pleine dessinée ici. */
const solid = (children: ReactNode) =>
  function Icon({ size }: IconProps) {
    return <Svg size={size}>{children}</Svg>;
  };

/** Icône au trait épais (graisse des flèches Papicons), pour les symboles absents du jeu. */
const line = (children: ReactNode) =>
  function Icon({ size = 16 }: IconProps) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={2.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden
      >
        {children}
      </svg>
    );
  };

/** Icône Papicons avec une pastille en bas à droite (le fond est évidé autour de la pastille). */
const badged = (base: PapPath[], badge: PapPath[]) =>
  function Icon({ size }: IconProps) {
    const id = useMaskId();
    return (
      <Svg size={size}>
        <mask id={id}>
          <rect width="24" height="24" fill="#fff" />
          <circle cx="18" cy="18" r="7" fill="#000" />
        </mask>
        <g mask={`url(#${id})`}>{paths(base)}</g>
        <g transform="translate(18 18) scale(0.5) translate(-12 -12)">{paths(badge)}</g>
      </Svg>
    );
  };

// Thème, barre du haut, panneaux.
/** Soleil dessiné ici : celui de Papicons ressemble trop à la roue dentée des Paramètres. */
export const SunIcon = solid(
  <>
    <circle cx="12" cy="12" r="4.5" />
    <path
      d="M12 2.5v1.5M12 20v1.5M2.5 12H4M20 12h1.5M5.3 5.3l1.05 1.05M17.65 17.65l1.05 1.05M5.3 18.7l1.05-1.05M17.65 6.35l1.05-1.05"
      stroke="currentColor"
      strokeWidth={2.5}
      strokeLinecap="round"
    />
  </>,
);
export const MoonIcon = pap(P.Moon);
export const SettingsIcon = pap(P.Gears);
export const MenuIcon = pap(P.Menu);
export const CloseIcon = pap(P.Cross);
export const UpdateIcon = pap(P.Sparkles);
export const UserIcon = pap(P.User);
export const LogoutIcon = pap(P.Logout);
export const InfoIcon = pap(P.Info);
export const LinkIcon = pap(P.Link);
export const SearchIcon = pap(P.Search);

// Fichiers et dossiers.
export const FolderIcon = pap(P.Folder);
export const FileIcon = pap(P.Paper);
export const FilePlusIcon = badged(P.Paper, P.Add);
export const FolderPlusIcon = badged(P.Folder, P.Add);
export const FolderUploadIcon = badged(P.Folder, P.ArrowUpBox);
export const MoveIcon = badged(P.Folder, P.ArrowRightBox);
export const UploadIcon = pap(P.ArrowUpBox);
export const DownloadIcon = pap(P.ArrowDownBox);
/** Dossier parent (« .. »). */
export const UpIcon = pap(P.ArrowLeftUp);
export const EditIcon = pap(P.PenAlt);
export const SelectAllIcon = pap(P.Tasks);
export const DeselectIcon = pap(P.Placeholder);
/** Menu d'actions « … » : points à l'horizontale. */
export const DotsIcon = pap(P.Dots, "rotate(90 12 12)");

// Vues de l'explorateur.
export const ListIcon = pap(P.List);
export const GridIcon = pap(P.Grid);
export const GalleryIcon = pap(P.Gallery);

// Poubelle.
export const TrashIcon = pap(P.Trash);
/** Poubelle qui contient des éléments : pastille pleine sur le couvercle. */
export function TrashFullIcon({ size }: IconProps) {
  const id = useMaskId();
  return (
    <Svg size={size}>
      <mask id={id}>
        <rect width="24" height="24" fill="#fff" />
        <circle cx="19.5" cy="4.5" r="4.5" fill="#000" />
      </mask>
      <g mask={`url(#${id})`}>{paths(P.Trash)}</g>
      <circle cx="19.5" cy="4.5" r="3" />
    </Svg>
  );
}

// Favoris : étoile pleine (favori) ou évidée (pas favori), même silhouette.
export const StarFilledIcon = pap(P.Star);
export function StarIcon({ size }: IconProps) {
  const id = useMaskId();
  return (
    <Svg size={size}>
      <mask id={id}>
        <rect width="24" height="24" fill="#fff" />
        <g fill="#000" transform="translate(12 13) scale(0.45) translate(-12 -13)">
          {paths(P.Star)}
        </g>
      </mask>
      <g mask={`url(#${id})`}>{paths(P.Star)}</g>
    </Svg>
  );
}

// Actions sans équivalent dans Papicons.
export const RefreshIcon = line(
  <>
    <path d="M19.5 10.5A7.8 7.8 0 0 0 5 8.5M4.5 4.5v4h4" />
    <path d="M4.5 13.5A7.8 7.8 0 0 0 19 15.5m.5 4v-4h-4" />
  </>,
);

export const RestoreIcon = line(<path d="M9 14.5 4.5 10 9 5.5M4.5 10h11a4.5 4.5 0 1 1 0 9H14" />);

export const HistoryIcon = line(
  <>
    <path d="M12 8v4l2.5 2" />
    <path d="M3.5 11a8.6 8.6 0 1 1 .6 4M3.5 20v-5h5" />
  </>,
);

export const SortIcon = line(<path d="M4 6h9M4 12h7M4 18h5M17.5 4.5v15M14.5 16.5l3 3 3-3" />);

export const CopyIcon = function CopyIcon({ size }: IconProps) {
  const id = useMaskId();
  return (
    <Svg size={size}>
      <mask id={id}>
        <rect width="24" height="24" fill="#fff" />
        <rect x="7" y="7" width="16" height="16" rx="5" fill="#000" />
      </mask>
      <rect x="3" y="3" width="12.5" height="12.5" rx="3.5" mask={`url(#${id})`} />
      <rect x="9" y="9" width="12" height="12" rx="3.5" />
    </Svg>
  );
};

export const SaveIcon = function SaveIcon({ size }: IconProps) {
  const id = useMaskId();
  return (
    <Svg size={size}>
      <mask id={id}>
        <rect width="24" height="24" fill="#fff" />
        <rect x="7.5" y="1" width="7.5" height="6.5" rx="1.5" fill="#000" />
        <circle cx="12" cy="15" r="2.75" fill="#000" />
      </mask>
      <path mask={`url(#${id})`} d="M7 3h8.2c.8 0 1.56.32 2.12.88l2.8 2.8c.56.56.88 1.32.88 2.12V17a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V7a4 4 0 0 1 4-4z" />
    </Svg>
  );
};

export const TagIcon = solid(
  <path
    fillRule="evenodd"
    clipRule="evenodd"
    d="M3 6a3 3 0 0 1 3-3h4.76a3 3 0 0 1 2.12.88l7.9 7.9a3 3 0 0 1 0 4.24l-4.76 4.76a3 3 0 0 1-4.24 0l-7.9-7.9A3 3 0 0 1 3 10.76zm5 .25a1.75 1.75 0 1 0 0 3.5 1.75 1.75 0 0 0 0-3.5z"
  />,
);

export const ShareIcon = solid(
  <>
    <path d="M6 12 17.5 5.5M6 12l11.5 6.5" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" />
    <circle cx="6" cy="12" r="3.25" />
    <circle cx="17.5" cy="5.5" r="3.25" />
    <circle cx="17.5" cy="18.5" r="3.25" />
  </>,
);

export const EyeIcon = solid(
  <path
    fillRule="evenodd"
    clipRule="evenodd"
    d="M12 5c4.3 0 7.6 2.7 9.5 6.1.3.56.3 1.24 0 1.8C19.6 16.3 16.3 19 12 19s-7.6-2.7-9.5-6.1a1.9 1.9 0 0 1 0-1.8C4.4 7.7 7.7 5 12 5m0 3.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7m0 2a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3"
  />,
);

/** Punaise : panneau épinglé ou flottant. */
export const PinIcon = solid(
  <g transform="rotate(35 12 12)">
    <path d="M8.5 3h7a1.5 1.5 0 0 1 0 3H15v4l2.6 2.4c.95.88.33 2.6-1 2.6H7.4c-1.33 0-1.95-1.72-1-2.6L9 10V6h-.5a1.5 1.5 0 0 1 0-3z" />
    <path d="M12 15v6" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" />
  </g>,
);

// Métadonnées des schémas (vue Galerie, panneau Détails).
export const ClockIcon = pap(P.Clock);
export const TextIcon = pap(P.Font);
export const PhotoIcon = pap(P.Camera);
export const ArrowIcon = pap(P.ArrowRightUp);
/** Taille du fichier. */
export const SizeIcon = pap(P.Archive);

export const ShapesIcon = solid(
  <>
    <rect x="3" y="3" width="8.5" height="8.5" rx="2.5" />
    <circle cx="17" cy="17" r="4.25" />
    <path d="M15.7 3.75a1.5 1.5 0 0 1 2.6 0l3.2 5.5A1.5 1.5 0 0 1 20.2 11.5h-6.4a1.5 1.5 0 0 1-1.3-2.25z" />
  </>,
);

export const FrameIcon = line(<path d="M4 7.5h16M4 16.5h16M7.5 4v16M16.5 4v16" />);
