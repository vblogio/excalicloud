import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@excalidraw/excalidraw/index.css";
import "./styles.css";
import App from "./App";
import { ShareView } from "./ShareView";

// /s/<jeton> : schéma partagé en lecture seule (sans authentification).
const share = location.pathname.match(/^\/s\/([^/]+)$/);

createRoot(document.getElementById("root")!).render(
  <StrictMode>{share ? <ShareView token={decodeURIComponent(share[1])} /> : <App />}</StrictMode>,
);
