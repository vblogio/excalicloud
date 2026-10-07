// Icônes au trait, dans le style d'Excalidraw (jeu Tabler, trait fin, couleur héritée).
import type { ReactNode } from "react";

const icon = (paths: ReactNode) =>
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
    <path d="M4 20h4L18.5 9.5a2.83 2.83 0 0 0-4-4L4 16v4" />
    <path d="M13.5 6.5l4 4" />
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
