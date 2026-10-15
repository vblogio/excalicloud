// Icônes au trait, dans le style d'Excalidraw (jeu Tabler, trait fin, couleur héritée).
import type { ReactNode } from "react";

export const icon = (paths: ReactNode) =>
  function Icon({ size = 16 }: { size?: number }) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden
      >
        {paths}
      </svg>
    );
  };

export const SunIcon = icon(
  <>
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
  </>,
);

export const MoonIcon = icon(<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9z" />);

export const SettingsIcon = icon(
  <>
    <path d="M10.33 4.32c.43-1.76 2.93-1.76 3.35 0a1.72 1.72 0 0 0 2.57 1.07c1.54-.94 3.31.83 2.37 2.37a1.72 1.72 0 0 0 1.07 2.57c1.76.43 1.76 2.93 0 3.35a1.72 1.72 0 0 0-1.07 2.57c.94 1.54-.83 3.31-2.37 2.37a1.72 1.72 0 0 0-2.57 1.07c-.43 1.76-2.93 1.76-3.35 0a1.72 1.72 0 0 0-2.57-1.07c-1.54.94-3.31-.83-2.37-2.37a1.72 1.72 0 0 0-1.07-2.57c-1.76-.43-1.76-2.93 0-3.35a1.72 1.72 0 0 0 1.07-2.57c-.94-1.54.83-3.31 2.37-2.37 1 .61 2.3.07 2.57-1.07z" />
    <circle cx="12" cy="12" r="3" />
  </>,
);

export const MenuIcon = icon(<path d="M4 6h16M4 12h16M4 18h16" />);

export const CloseIcon = icon(<path d="M18 6 6 18M6 6l12 12" />);

export const FilePlusIcon = icon(
  <>
    <path d="M14 3v4a1 1 0 0 0 1 1h4" />
    <path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z" />
    <path d="M12 11v6M9 14h6" />
  </>,
);

export const FolderPlusIcon = icon(
  <>
    <path d="M12 19H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h4l3 3h7a2 2 0 0 1 2 2v3.5" />
    <path d="M16 19h6M19 16v6" />
  </>,
);

export const UploadIcon = icon(
  <>
    <path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
    <path d="M7 9l5-5 5 5M12 4v12" />
  </>,
);

export const FolderUploadIcon = icon(
  <>
    <path d="M12 19H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h4l3 3h7a2 2 0 0 1 2 2v3" />
    <path d="M19 22v-6M22 19l-3-3-3 3" />
  </>,
);

export const FolderIcon = icon(
  <path d="M5 4h4l3 3h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2" />,
);

export const UpIcon = icon(<path d="M18 18h-6a3 3 0 0 1-3-3V5M13 10 9 6 5 10" />);

export const SearchIcon = icon(
  <>
    <circle cx="10" cy="10" r="7" />
    <path d="m21 21-6-6" />
  </>,
);

export const ListIcon = icon(<path d="M9 6h11M9 12h11M9 18h11M5 6v.01M5 12v.01M5 18v.01" />);

export const GridIcon = icon(
  <>
    <rect x="4" y="4" width="6" height="6" rx="1" />
    <rect x="14" y="4" width="6" height="6" rx="1" />
    <rect x="4" y="14" width="6" height="6" rx="1" />
    <rect x="14" y="14" width="6" height="6" rx="1" />
  </>,
);

export const RefreshIcon = icon(
  <>
    <path d="M20 11A8.1 8.1 0 0 0 4.5 9M4 5v4h4" />
    <path d="M4 13a8.1 8.1 0 0 0 15.5 2m.5 4v-4h-4" />
  </>,
);

export const DotsIcon = icon(
  <>
    <circle cx="5" cy="12" r="1" />
    <circle cx="12" cy="12" r="1" />
    <circle cx="19" cy="12" r="1" />
  </>,
);

export const TrashIcon = icon(
  <path d="M4 7h16M10 11v6M14 11v6M5 7l1 12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2l1-12M9 7V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v3" />,
);

export const SelectAllIcon = icon(
  <>
    <rect x="4" y="4" width="16" height="16" rx="2" />
    <path d="m9 12 2 2 4-4" />
  </>,
);

export const DeselectIcon = icon(<rect x="4" y="4" width="16" height="16" rx="2" />);

export const MoveIcon = icon(
  <>
    <path d="M12 19H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h4l3 3h7a2 2 0 0 1 2 2v3" />
    <path d="M16 19h6M19 16l3 3-3 3" />
  </>,
);

export const SaveIcon = icon(
  <>
    <path d="M6 4h10l4 4v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2" />
    <circle cx="12" cy="14" r="2" />
    <path d="M14 4v4H8V4" />
  </>,
);

export const UpdateIcon = icon(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 16V8M8.5 11.5 12 8l3.5 3.5" />
  </>,
);

export const CopyIcon = icon(
  <>
    <rect x="8" y="8" width="12" height="12" rx="2" />
    <path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" />
  </>,
);

export const EditIcon = icon(<path d="M4 20h4L18.5 9.5a2.83 2.83 0 0 0-4-4L4 16v4M13.5 6.5l4 4" />);

export const HistoryIcon = icon(
  <>
    <path d="M12 8v4l2 2" />
    <path d="M3.05 11a9 9 0 1 1 .5 4m-.5 5v-5h5" />
  </>,
);

export const DownloadIcon = icon(
  <>
    <path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
    <path d="M7 11l5 5 5-5M12 4v12" />
  </>,
);

export const StarIcon = icon(
  <path d="M12 17.75l-6.17 3.24 1.18-6.87-5-4.86 6.9-1 3.09-6.26 3.09 6.26 6.9 1-5 4.86 1.18 6.87z" />,
);

/** Étoile pleine (favori). */
export function StarFilledIcon({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" strokeWidth={1.5} strokeLinejoin="round" aria-hidden>
      <path d="M12 17.75l-6.17 3.24 1.18-6.87-5-4.86 6.9-1 3.09-6.26 3.09 6.26 6.9 1-5 4.86 1.18 6.87z" />
    </svg>
  );
}

export const TagIcon = icon(
  <>
    <circle cx="8.5" cy="8.5" r="1" />
    <path d="M4 7v3.86a2 2 0 0 0 .59 1.41l7.71 7.71a2.4 2.4 0 0 0 3.4 0l4.27-4.27a2.4 2.4 0 0 0 0-3.4L12.26 4.59A2 2 0 0 0 10.85 4H7a3 3 0 0 0-3 3z" />
  </>,
);

export const ShareIcon = icon(
  <>
    <circle cx="6" cy="12" r="3" />
    <circle cx="18" cy="6" r="3" />
    <circle cx="18" cy="18" r="3" />
    <path d="M8.7 10.7l6.6-3.4M8.7 13.3l6.6 3.4" />
  </>,
);

export const EyeIcon = icon(
  <>
    <circle cx="12" cy="12" r="2" />
    <path d="M22 12c-2.67 4.67-6 7-10 7s-7.33-2.33-10-7c2.67-4.67 6-7 10-7s7.33 2.33 10 7" />
  </>,
);

/** Punaise : panneau épinglé ou flottant (comme le bouton d'ancrage des panneaux d'Excalidraw). */
export const PinIcon = icon(<path d="M15 4.5l-4 4-4 1.5-1.5 1.5 7 7 1.5-1.5 1.5-4 4-4M9 15l-4.5 4.5M14.5 4 20 9.5" />);

/** Espace personnel de l'utilisateur. */
export const UserIcon = icon(
  <>
    <circle cx="12" cy="8" r="4" />
    <path d="M6 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2" />
  </>,
);

export const LogoutIcon = icon(
  <>
    <path d="M14 8V6a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h7a2 2 0 0 0 2-2v-2" />
    <path d="M9 12h12l-3-3M18 15l3-3" />
  </>,
);
