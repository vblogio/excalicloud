import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Le build est embarqué dans le binaire Go (web/dist).
  build: { outDir: "../web/dist", emptyOutDir: true, chunkSizeWarningLimit: 4096 },
  // `npm run dev` : l'API Go tourne sur :8080.
  server: { proxy: { "/api": "http://localhost:8080" } },
  define: { "process.env.IS_PREACT": JSON.stringify("false") },
});
