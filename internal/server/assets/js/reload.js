// Live reload: the server tells what changed, and the page reloads when it
// shows one of the changed files, keeping its scroll position.

import { page, dirname } from "./util.js";

const scrollKey = "gh-mini-scroll:" + location.pathname + location.search;

export function reload() {
  try { sessionStorage.setItem(scrollKey, String(window.scrollY)); } catch (e) {}
  location.reload();
}

function restoreScroll() {
  try {
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
  restoreScroll();
  if (!page.reload) return;
  const es = new EventSource("/_mini/events");
  es.onmessage = (e) => {
    const c = JSON.parse(e.data);
    if (c.theme) onTheme();
    if (c.structure) onStructure();
    const paths = c.paths || [];
    const hit = paths.some((p) => p === page.path || (page.kind === "dir" && dirname(p) === page.path));
    if (hit || (page.kind === "notfound" && c.structure)) reload();
  };
}
