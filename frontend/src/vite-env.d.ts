/// <reference types="vite/client" />

interface Window {
  EXCALIDRAW_ASSET_PATH?: string | string[];
}

/** Version de @excalidraw/excalidraw embarquée au build (voir vite.config.ts). */
declare const __EXCALIDRAW_VERSION__: string;
