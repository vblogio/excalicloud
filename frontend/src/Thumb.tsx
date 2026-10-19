import { useEffect, useRef, useState } from "react";
import type { SyntheticEvent } from "react";
import { api } from "./api";
import { ensureThumb, knownThumb } from "./thumbs";

type State = "idle" | "server" | "generating" | "retry" | "empty" | "error";

/**
 * Miniature d'un schéma, chargée quand elle devient visible. Si le serveur
 * n'en a pas (ou qu'elle est périmée), elle est générée dans le navigateur
 * puis envoyée au serveur pour les visites suivantes. minSize : une miniature
 * du serveur plus petite (rendue à une ancienne taille) est régénérée.
 */
export function Thumb({ path, version, minSize }: { path: string; version: string; minSize?: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [visible, setVisible] = useState(false);
  const [state, setState] = useState<State>("idle");

  useEffect(() => {
    const k = knownThumb(path, version);
    setState(k === "empty" || k === "error" ? k : "idle");
  }, [path, version]);

  useEffect(() => {
    const el = ref.current;
    if (!el || visible) return;
    const io = new IntersectionObserver(
      (es) => {
        if (es.some((e) => e.isIntersecting)) {
          setVisible(true);
          io.disconnect();
        }
      },
      { rootMargin: "200px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [visible]);

  useEffect(() => {
    if (visible && state === "idle") setState("server");
  }, [visible, state]);

  const regenerate = () => {
    setState("generating");
    ensureThumb(path, version).then((r) => setState(r === "ok" ? "retry" : r));
  };

  const onError = () => (state === "server" ? regenerate() : setState("error"));

  const onLoad = (ev: SyntheticEvent<HTMLImageElement>) => {
    const img = ev.currentTarget;
    if (minSize && state === "server" && Math.max(img.naturalWidth, img.naturalHeight) < minSize) regenerate();
  };

  const src =
    state === "server"
      ? api.thumbUrl(path, version)
      : state === "retry"
        ? `${api.thumbUrl(path, version)}&r=1` // la réponse 404 précédente n'est pas réutilisée
        : null;

  return (
    <span ref={ref} className={`thumb ${state}`} aria-hidden>
      {src ? (
        <img src={src} alt="" loading="lazy" decoding="async" draggable={false} onError={onError} onLoad={onLoad} />
      ) : (
        <span className="ph">{state === "empty" ? "∅" : state === "error" ? "📄" : ""}</span>
      )}
    </span>
  );
}
