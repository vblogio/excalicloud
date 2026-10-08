import type { ExcalidrawImperativeAPI } from "@excalidraw/excalidraw/types";

const isMobile = () => window.matchMedia("(max-width: 768px)").matches;

/**
 * Cadre tout le schéma dans la zone visible, sans zoomer au-delà de 100 %.
 *
 * Remplace `initialData.scrollToContent` : quand le contenu déborde de la
 * fenêtre, Excalidraw centre alors un seul élément (le plus proche de l'origine)
 * au lieu de l'ensemble, et le schéma s'affiche à partir du milieu de l'écran.
 */
export function fitScene(ex: ExcalidrawImperativeAPI) {
  const elements = ex.getSceneElements();
  if (elements.length === 0) return;
  ex.scrollToContent(elements, {
    fitToContent: true,
    viewportZoomFactor: 0.9,
    animate: false,
    // Barres d'outils d'Excalidraw posées sur le canevas (en haut, et en bas sur mobile).
    canvasOffsets: { top: 64, bottom: isMobile() ? 64 : 0 },
  });
}
