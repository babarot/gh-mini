// What changed since the last commit, as the server reads it from git: the
// count in the top bar, and the letters the file tree shows.

const countEl = document.getElementById("mini.changes");

// files are the changed files by path, each with its letter and lines.
let files = {};
const listeners = [];

export const changedFiles = () => files;

// onStatus calls f whenever the status is read again.
export function onStatus(f) {
  listeners.push(f);
}

// fetchStatus reads the status and tells the paths whose status is not
// what it was, and the ETag of what it read, without its quotes. Outside
// a git repository there is nothing to read.
export async function fetchStatus() {
  if (!countEl) return { changed: [], etag: "" };
  const r = await fetch("/_mini/api/status", { cache: "no-cache" });
  if (!r.ok) return { changed: [], etag: "" };
  const st = await r.json();
  const before = files;
  files = st.files || {};
  const changed = Array.from(new Set([...Object.keys(before), ...Object.keys(files)]))
    .filter((p) => JSON.stringify(before[p]) !== JSON.stringify(files[p]));
  const n = Object.keys(files).length;
  countEl.hidden = n === 0;
  countEl.querySelector("[data-files]").textContent = n + (n === 1 ? " change" : " changes");
  countEl.querySelector("[data-added]").textContent = "+" + st.added;
  countEl.querySelector("[data-deleted]").textContent = "−" + st.deleted;
  listeners.forEach((f) => f());
  return { changed, etag: (r.headers.get("ETag") || "").replace(/"/g, "") };
}

// changedDirs are the directories with a changed file under them.
export function changedDirs() {
  const dirs = new Set();
  for (const p of Object.keys(files)) {
    for (let i = p.lastIndexOf("/"); i > 0; i = p.lastIndexOf("/", i - 1)) dirs.add(p.slice(0, i));
  }
  return dirs;
}
