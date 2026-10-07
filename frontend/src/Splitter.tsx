import { useRef } from "react";

interface Props {
  width: number;
  min: number;
  max: number;
  onResize: (width: number) => void;
  /** Début / fin de glissement (désactive l'animation de la grille). */
  onDragging: (dragging: boolean) => void;
  onReset: () => void;
}

/** Poignée verticale entre l'explorateur et le canevas : souris, doigt ou clavier. */
export function Splitter({ width, min, max, onResize, onDragging, onReset }: Props) {
  const start = useRef<{ x: number; w: number } | null>(null);

  return (
    <div
      className="splitter"
      role="separator"
      aria-orientation="vertical"
      aria-label="Largeur de l'explorateur"
      aria-valuenow={width}
      aria-valuemin={min}
      aria-valuemax={max}
      tabIndex={0}
      title="Glisser pour redimensionner, double-clic pour la largeur par défaut"
      onPointerDown={(e) => {
        if (e.button !== 0) return;
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        start.current = { x: e.clientX, w: width };
        onDragging(true);
      }}
      onPointerMove={(e) => {
        if (start.current) onResize(start.current.w + e.clientX - start.current.x);
      }}
      onPointerUp={(e) => {
        e.currentTarget.releasePointerCapture(e.pointerId);
        start.current = null;
        onDragging(false);
      }}
      onLostPointerCapture={() => {
        if (start.current) {
          start.current = null;
          onDragging(false);
        }
      }}
      onDoubleClick={onReset}
      onKeyDown={(e) => {
        const step = e.shiftKey ? 64 : 16;
        if (e.key === "ArrowLeft") onResize(width - step);
        else if (e.key === "ArrowRight") onResize(width + step);
        else if (e.key === "Home") onResize(min);
        else if (e.key === "End") onResize(max);
        else return;
        e.preventDefault();
      }}
    />
  );
}
