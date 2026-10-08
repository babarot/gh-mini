// Live reload: the server tells what changed, and the page reloads when it
// shows one of the changed files, keeping its scroll position, which of
// its <details> are open, and a dialog open on it, such as the settings
// when one of them changes what the server renders.

import { page, dirname, isImage } from "./util.js";

const scrollKey = "gh-mini-scroll:" + location.pathname + location.search;
const detailsKey = "gh-mini-details:" + location.pathname + location.search;
const dialogKey = "gh-mini-dialog:" + location.pathname + location.search;

const allDetails = () => Array.from(document.querySelectorAll(".markdown-body details"));

export function reload() {
  try {
    sessionStorage.setItem(scrollKey, String(window.scrollY));
    sessionStorage.setItem(detailsKey, JSON.stringify(allDetails().map((d) => d.open)));
    const dialog = document.querySelector("dialog[open]");
    if (dialog) {
      // The setting just changed keeps the focus too
      const focused = dialog.contains(document.activeElement) ? document.activeElement.dataset.setting : undefined;
      sessionStorage.setItem(dialogKey, JSON.stringify({ id: dialog.id, setting: focused }));
    }
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
    const shown = sessionStorage.getItem(dialogKey);
    if (shown !== null) {
      sessionStorage.removeItem(dialogKey);
      const { id, setting } = JSON.parse(shown);
      const dialog = document.getElementById(id);
      if (dialog && !dialog.open) {
        dialog.showModal();
        if (setting) dialog.querySelector(`[data-setting="${CSS.escape(setting)}"]`)?.focus();
      }
    }
    const y = sessionStorage.getItem(scrollKey);
    if (y !== null) {
      sessionStorage.removeItem(scrollKey);
      window.addEventListener("load", () => { window.scrollTo(0, Number(y)); });
    }
  } catch (e) {}
}

// initReload listens for changes. onTheme runs when a theme was saved,
// onStructure when files were added or removed, and onFiles with the
// changed paths otherwise.
export function initReload({ onTheme, onStructure, onFiles }) {
  restore();
  if (!page.reload) return;
  // The latest change the page has been told, or rendered with: a change
  // told twice, by the stream and by catching up, is taken once
  let last = page.seq;
  const onChange = (c) => {
    if (!c.resync && c.seq <= last) return;
    last = Math.max(last, c.seq || 0);
    if (!c.theme && !c.structure && !c.resync && !c.paths?.length && !c.dirs?.length) return;
    if (c.theme) onTheme();
    if (c.structure || c.resync) onStructure();
    else onFiles(c.paths, c.dirs);
    if (c.resync) {
      reload();
      return;
    }
    const paths = c.paths || [];
    // dirs stand for changes too many to list: any file directly in them
    const dirs = c.dirs || [];
    const inDirs = dirs.some((d) => d === page.path || d === dirname(page.path) ||
      (page.kind === "html" && underDir(d + "/x")));
    const hit = inDirs || paths.some((p) => p === page.path || (page.kind === "dir" && dirname(p) === page.path) ||
      (page.kind === "html" && underDir(p)) ||
      // A Markdown page shows the images next to it
      ((page.kind === "markdown" || page.kind === "dir") && isImage(p) && underDir(p)));
    if (hit || (page.kind === "notfound" && c.structure)) reload();
  };
  // Changes may be missed before the page subscribes, while it is in the
  // back/forward cache and while the stream is lost: ask for them then
  const catchUp = async () => {
    try {
      const r = await fetch(`/_mini/api/changes?since=${last}&boot=${encodeURIComponent(page.boot)}`, { cache: "no-store" });
      if (r.ok) onChange(await r.json());
    } catch (e) {}
  };
  // Every stream starts by telling the server's boot ID. Another server,
  // perhaps with other code, means reading it all again; the same one, a
  // stream opened again, may have missed changes meanwhile
  const onBoot = (boot) => {
    if (boot !== page.boot) reload();
    else catchUp();
  };
  subscribe(onChange, onBoot, catchUp);
}

// underDir tells whether a path is in the directory of the page's file or
// under it: an HTML preview loads its styles and scripts from around it.
function underDir(p) {
  const dir = page.kind === "dir" ? page.path : dirname(page.path);
  return dir === "." || p.startsWith(dir + "/");
}

// subscribe calls onChange with every change the server tells, onBoot with
// the boot ID a stream starts with, and catchUp once subscribed and when
// the page comes back from the back/forward cache, as changes may have
// been missed then. The tabs share one connection through a shared worker
// where there is one.
function subscribe(onChange, onBoot, catchUp) {
  window.addEventListener("pageshow", (e) => { if (e.persisted) catchUp(); });
  if (!window.SharedWorker) {
    const es = new EventSource("/_mini/events");
    es.onmessage = (e) => onChange(JSON.parse(e.data));
    es.addEventListener("boot", (e) => onBoot(e.data));
    catchUp();
    return;
  }
  const worker = new SharedWorker(new URL("./events-worker.js", import.meta.url), { name: "gh-mini-events" });
  worker.port.onmessage = (e) => {
    const c = JSON.parse(e.data);
    if (c.boot !== undefined) onBoot(c.boot);
    else onChange(c);
  };
  worker.port.start();
  // The worker may have its stream open already, and tells no boot ID to
  // a tab that comes later
  catchUp();
  window.addEventListener("pagehide", () => worker.port.postMessage("close"));
  window.addEventListener("pageshow", (e) => { if (e.persisted) worker.port.postMessage("open"); });
}
