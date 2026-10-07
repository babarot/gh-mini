// Live reload: the server tells what changed, and the page reloads when it
// shows one of the changed files, keeping its scroll position and which of
// its <details> are open.

import { page, dirname } from "./util.js";

const scrollKey = "gh-mini-scroll:" + location.pathname + location.search;
const detailsKey = "gh-mini-details:" + location.pathname + location.search;

const allDetails = () => Array.from(document.querySelectorAll(".markdown-body details"));

export function reload() {
  try {
    sessionStorage.setItem(scrollKey, String(window.scrollY));
    sessionStorage.setItem(detailsKey, JSON.stringify(allDetails().map((d) => d.open)));
  } catch (e) {}
  location.reload();
}

function restore() {
  try {
    const open = sessionStorage.getItem(detailsKey);
    if (open !== null) {
      sessionStorage.removeItem(detailsKey);
      const states = JSON.parse(open);
      allDetails().forEach((d, i) => { if (i < states.length) d.open = states[i]; });
    }
    const y = sessionStorage.getItem(scrollKey);
    if (y !== null) {
      sessionStorage.removeItem(scrollKey);
      window.addEventListener("load", () => { window.scrollTo(0, Number(y)); });
    }
  } catch (e) {}
}

// initReload listens for changes. onTheme runs when a theme was saved, and
// onStructure when files were added or removed.
export function initReload({ onTheme, onStructure }) {
  restore();
  if (!page.reload) return;
  subscribe((c) => {
    if (c.theme) onTheme();
    if (c.structure) onStructure();
    const paths = c.paths || [];
    const hit = paths.some((p) => p === page.path || (page.kind === "dir" && dirname(p) === page.path));
    if (hit || (page.kind === "notfound" && c.structure)) reload();
  });
}

// subscribe calls onChange with every change the server tells. The tabs
// share one connection through a shared worker where there is one.
function subscribe(onChange) {
  if (!window.SharedWorker) {
    const es = new EventSource("/_mini/events");
    es.onmessage = (e) => onChange(JSON.parse(e.data));
    return;
  }
  const worker = new SharedWorker(new URL("./events-worker.js", import.meta.url), { name: "gh-mini-events" });
  worker.port.onmessage = (e) => onChange(JSON.parse(e.data));
  worker.port.start();
  window.addEventListener("pagehide", () => worker.port.postMessage("close"));
  window.addEventListener("pageshow", (e) => { if (e.persisted) worker.port.postMessage("open"); });
}
