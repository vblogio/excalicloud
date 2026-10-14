import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";

interface Props {
  /** Bouton « … » qui a ouvert le menu : le menu s'aligne sur lui. */
  anchor: HTMLElement;
  onClose: () => void;
  children: ReactNode;
}

const MARGIN = 8; // écart minimal avec les bords de la fenêtre

/**
 * Menu déroulant des actions d'un élément, ouvert sous le bouton « … » et aligné
 * sur son bord droit (au-dessus s'il manque de place en bas). Rendu dans <body>
 * pour ne pas être rogné par le défilement de la liste ; refermé par un clic
 * ailleurs, Échap, un défilement, un redimensionnement ou le choix d'une action.
 */
export function ActionMenu({ anchor, onClose, children }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);

  useLayoutEffect(() => {
    const menu = ref.current;
    if (!menu) return;
    const a = anchor.getBoundingClientRect();
    const { offsetWidth: w, offsetHeight: h } = menu;
    let top = a.bottom + 4;
    if (top + h > innerHeight - MARGIN) top = Math.max(MARGIN, a.top - 4 - h);
    const left = Math.min(Math.max(MARGIN, a.right - w), innerWidth - w - MARGIN);
    setPos({ top, left });
  }, [anchor]);

  // Focus sur la première action une fois le menu placé (masqué, il ne le prend pas).
  useEffect(() => {
    if (pos) ref.current?.querySelector<HTMLElement>("button, a")?.focus({ preventScroll: true });
  }, [pos]);

  useEffect(() => {
    const inside = (t: EventTarget | null) => t instanceof Node && (ref.current?.contains(t) || anchor.contains(t));
    const onDown = (ev: PointerEvent) => {
      if (!inside(ev.target)) onClose();
    };
    const onKey = (ev: KeyboardEvent) => {
      if (ev.key === "Escape") {
        // Ni désélection, ni fermeture du panneau flottant : seulement le menu.
        ev.preventDefault();
        ev.stopPropagation();
        onClose();
        anchor.focus();
      } else if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
        const items = [...(ref.current?.querySelectorAll<HTMLElement>("button, a") ?? [])];
        if (!items.length) return;
        ev.preventDefault();
        const i = items.indexOf(document.activeElement as HTMLElement);
        const next = ev.key === "ArrowDown" ? (i + 1) % items.length : (i - 1 + items.length) % items.length;
        items[next].focus();
      }
    };
    const onScroll = (ev: Event) => {
      if (!(ev.target instanceof Node && ref.current?.contains(ev.target))) onClose();
    };
    document.addEventListener("pointerdown", onDown, true);
    document.addEventListener("keydown", onKey, true);
    document.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onClose);
    return () => {
      document.removeEventListener("pointerdown", onDown, true);
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onClose);
    };
  }, [anchor, onClose]);

  return createPortal(
    <div
      ref={ref}
      className="actions menu-popup"
      role="menu"
      style={pos ?? { top: 0, left: 0, visibility: "hidden" }}
      // Une action choisie referme le menu (après coup, pour laisser un lien de
      // téléchargement aboutir).
      onClick={(ev) => {
        if ((ev.target as HTMLElement).closest("button, a")) setTimeout(onClose);
      }}
    >
      {children}
    </div>,
    document.body,
  );
}
