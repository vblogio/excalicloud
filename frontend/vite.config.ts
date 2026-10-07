import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { readFileSync } from "node:fs";

// Version d'Excalidraw réellement installée (et non la plage de package.json),
// comparée par l'interface à la dernière version publiée sur npm.
const excalidrawVersion: string = JSON.parse(
  readFileSync(new URL("./node_modules/@excalidraw/excalidraw/package.json", import.meta.url), "utf8"),
).version;

export default defineConfig({
  plugins: [react()],
  // Le build est embarqué dans le binaire Go (web/dist).
  build: { outDir: "../web/dist", emptyOutDir: true, chunkSizeWarningLimit: 4096 },
  // `npm run dev` : l'API Go tourne sur :8080.
  server: { proxy: { "/api": "http://localhost:8080" } },
  define: {
    "process.env.IS_PREACT": JSON.stringify("false"),
    __EXCALIDRAW_VERSION__: JSON.stringify(excalidrawVersion),
  },
});
