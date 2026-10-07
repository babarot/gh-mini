// The file tree in the sidebar, its filter and its toggle.

import { page, load, save, href, dirname, isMarkdown, esc, ICON_DIR, ICON_FILE, ICON_CHEVRON, setCookie } from "./util.js";

const body = document.body;
const treeEl = document.getElementById("mini.tree");
const filterEl = document.getElementById("mini.tree-filter");
const mdOnlyEl = document.getElementById("mini.md-only");
const current = page.path;
let tree = null;
const open = new Set(load("open", []));

// loaded are the lazy directories whose entries were read: directories
// git ignores come without them, and are read when opened.
const loaded = new Set();

// prune drops files that are not Markdown, and directories left empty. A
// directory not read yet stays: it may hold some.
function prune(node) {
  if (!node.dir) return isMarkdown(node.name) ? node : null;
  if (node.lazy && !node.children) return node;
  const kids = (node.children || []).map(prune).filter(Boolean);
  if (!kids.length && node.path) return null;
  return Object.assign({}, node, { children: kids });
}

function rowHTML(node) {
  const cls = "row" + (node.path === current ? " current" : "");
  const icon = node.dir ? ICON_CHEVRON + ICON_DIR : '<span class="indent"></span>' + ICON_FILE;
  return '<a class="' + cls + '" href="' + href(node.path, node.dir) + '" data-path="' + esc(node.path) + '"' +
    (node.dir ? ' data-dir="1"' : "") + ">" + icon + '<span class="label">' + esc(node.name) + "</span></a>";
}

function listHTML(nodes) {
  let out = "<ul>";
  nodes.forEach((n) => {
    const cls = [];
    if (n.dir && open.has(n.path)) cls.push("open");
    if (n.ignored) cls.push("ignored");
    out += '<li class="' + cls.join(" ") + '"' + (n.ignored ? ' title="Ignored by git"' : "") + ">" + rowHTML(n);
    // Only what is open is drawn: a large tree costs nothing folded
    if (n.dir && open.has(n.path) && n.children && n.children.length) out += listHTML(n.children);
    out += "</li>";
  });
  return out + "</ul>";
}

function flatten(node, out) {
  (node.children || []).forEach((n) => {
    if (!n.dir) out.push(n);
    else flatten(n, out);
  });
  return out;
}

function render() {
  if (!tree) return;
  const data = mdOnlyEl.checked ? prune(tree) || { children: [] } : tree;
  const q = filterEl.value.trim().toLowerCase();
  if (q) {
    const words = q.split(/\s+/);
    const hits = flatten(data, []).filter((n) => {
      const p = n.path.toLowerCase();
      return words.every((w) => p.indexOf(w) >= 0);
    }).slice(0, 300);
    treeEl.innerHTML = hits.length ? "<ul>" + hits.map((n) => {
      const d = dirname(n.path);
      return '<li class="' + (n.ignored ? "ignored" : "") + '"><a class="row' + (n.path === current ? " current" : "") +
        '" href="' + href(n.path) + '">' + ICON_FILE + '<span class="label">' +
        (d === "." ? "" : '<span class="dir-path">' + esc(d) + "/</span>") + esc(n.name) + "</span></a></li>";
    }).join("") + "</ul>" : '<div class="empty">No matching files</div>';
    return;
  }
  treeEl.innerHTML = listHTML(data.children || []);
}

// Every page builds the tree anew, so where it was scrolled is kept for the
// next page of this tab: a file picked in the tree stays where it was
// clicked. Only when the current file is out of sight, as after following
// a link or the file finder, does the tree scroll to it.
const scrollKey = "gh-mini-tree-scroll:" + (body.dataset.name || "");

function keepScroll() {
  try {
    // A filtered list is gone on the next page, and so is its position
    if (filterEl.value.trim()) sessionStorage.removeItem(scrollKey);
    else sessionStorage.setItem(scrollKey, String(treeEl.scrollTop));
  } catch (e) {}
}

function restoreScroll() {
  try {
    const top = sessionStorage.getItem(scrollKey);
    if (top !== null) treeEl.scrollTop = Number(top);
  } catch (e) {}
}

// revealCurrent centers the current file in the tree if it is out of
// sight. It scrolls the tree alone: scrollIntoView would scroll the page
// too, by a little more on every load.
function revealCurrent() {
  const el = treeEl.querySelector(".row.current");
  if (!el) return;
  const t = treeEl.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  if (r.top >= t.top && r.bottom <= t.bottom) return;
  treeEl.scrollTop += r.top - t.top - (treeEl.clientHeight - el.offsetHeight) / 2;
}

// fetchTree loads the tree, again when files were added or removed.
// find returns the node at a path in the tree as read so far.
function find(path) {
  let n = tree;
  if (!n || path === ".") return n;
  for (const name of path.split("/")) {
    n = (n.children || []).find((c) => c.name === name);
    if (!n) return null;
  }
  return n;
}

// readDir reads the entries of a lazy directory into its node.
async function readDir(path) {
  const r = await fetch("/_mini/api/tree?path=" + encodeURIComponent(path), { cache: "no-store" });
  const n = find(path);
  if (!n) return;
  if (!r.ok) {
    n.children = [];
    return;
  }
  n.children = (await r.json()).children || [];
  loaded.add(path);
}

// readOpen reads the lazy directories that are open, outer ones first, as
// the inner ones are only known once those are read.
async function readOpen() {
  const paths = Array.from(open).sort((a, b) => a.split("/").length - b.split("/").length);
  for (const p of paths) {
    const n = find(p);
    if (n && n.lazy && !n.children) await readDir(p);
  }
}

export async function fetchTree() {
  const r = await fetch("/_mini/api/tree", { cache: "no-cache" });
  tree = await r.json();
  loaded.clear();
  await readOpen();
  render();
}

// refresh reads again the read directories where files changed: changes
// there are no change of the tree's structure.
export async function refresh(paths, dirs) {
  const stale = new Set();
  for (const p of paths || []) if (loaded.has(dirname(p))) stale.add(dirname(p));
  for (const d of dirs || []) if (loaded.has(d)) stale.add(d);
  if (!stale.size) return;
  for (const d of stale) await readDir(d);
  render();
}

export function initTree() {
  // Open the directories down to the current page
  for (let p = page.kind === "dir" ? current : dirname(current); p !== "."; p = dirname(p)) open.add(p);
  if (page.kind === "dir" && current !== ".") open.add(current);
  mdOnlyEl.checked = load("mdOnly", false);

  treeEl.addEventListener("click", (e) => {
    const row = e.target.closest(".row");
    if (!row || !row.dataset.dir) return;
    // A click on the chevron only folds; a click on the name also opens the
    // directory's page
    const path = row.dataset.path;
    if (e.target.closest(".chevron")) {
      e.preventDefault();
      if (open.has(path)) open.delete(path); else open.add(path);
      const n = find(path);
      (n && n.lazy && !n.children && open.has(path) ? readDir(path) : Promise.resolve()).then(render);
    } else {
      open.add(path);
    }
    save("open", Array.from(open));
  });

  filterEl.addEventListener("input", render);
  filterEl.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      const first = treeEl.querySelector(".row");
      if (first) location.href = first.href;
    } else if (e.key === "Escape") {
      filterEl.value = "";
      render();
      filterEl.blur();
    }
  });
  mdOnlyEl.addEventListener("change", () => {
    save("mdOnly", mdOnlyEl.checked);
    render();
  });

  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "t" || e.key === "/") {
      e.preventDefault();
      body.classList.remove("sidebar-hidden");
      filterEl.focus();
      filterEl.select();
    }
  });

  // The server renders the tree closed from a cookie. It was kept in
  // localStorage before: move it over once
  const toggle = document.getElementById("mini.sidebar-toggle");
  if (!document.cookie.split("; ").some((c) => c.startsWith("gh-mini-sidebar=")) && load("sidebarHidden", false)) {
    setCookie("gh-mini-sidebar", "hidden");
    body.classList.add("sidebar-hidden");
  }
  toggle.addEventListener("click", () => {
    if (window.matchMedia("(max-width: 767px)").matches) {
      body.classList.toggle("sidebar-shown");
      return;
    }
    const hidden = body.classList.toggle("sidebar-hidden");
    setCookie("gh-mini-sidebar", hidden ? "hidden" : "shown");
    save("sidebarHidden", hidden);
  });

  window.addEventListener("pagehide", keepScroll);
  fetchTree().then(() => {
    restoreScroll();
    revealCurrent();
  });
}
