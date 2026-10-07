// Copie les polices d'Excalidraw dans public/fonts pour un usage hors ligne.
import { cpSync, existsSync } from "node:fs";

const src = "node_modules/@excalidraw/excalidraw/dist/prod/fonts";
if (!existsSync(src)) {
  console.error(`polices introuvables : ${src}`);
  process.exit(1);
}
cpSync(src, "public/fonts", { recursive: true });
console.log("polices Excalidraw copiées dans public/fonts");
