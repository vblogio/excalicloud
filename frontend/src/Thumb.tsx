import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { ensureThumb, knownThumb } from "./thumbs";

type State = "idle" | "server" | "generating" | "retry" | "empty" | "error";

/**
 * Miniature d'un schéma, chargée quand elle devient visible. Si le serveur
 * n'en a pas (ou qu'elle est périmée), elle est générée dans le navigateur
 * puis envoyée au serveur pour les visites suivantes.
 */
export function Thumb({ path, version }: { path: string; version: string }) {
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

  const onError = () => {
    if (state !== "server") return setState("error");
    setState("generating");
    ensureThumb(path, version).then((r) => setState(r === "ok" ? "retry" : r));
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
        <img src={src} alt="" loading="lazy" decoding="async" draggable={false} onError={onError} />
      ) : (
        <span className="ph">{state === "empty" ? "∅" : state === "error" ? "📄" : ""}</span>
      )}
    </span>
  );
}
