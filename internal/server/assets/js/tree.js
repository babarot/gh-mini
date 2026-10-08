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

// hideable tells a directory git ignores that the setting to hide them
// leaves out: the outermost one, as what is in it is ignored too, unless
// the page shown is in it, so that the tree still leads to the page.
function hideable(node, parentIgnored) {
  if (!node.dir || !node.ignored || parentIgnored) return false;
  return current !== node.path && !current.startsWith(node.path + "/");
}

const hidingIgnored = () => document.documentElement.dataset.hideignoreddirs === "true";

function rowHTML(node) {
  const cls = "row" + (node.path === current ? " current" : "");
  const icon = node.dir ? ICON_CHEVRON + ICON_DIR : '<span class="indent"></span>' + ICON_FILE;
  return '<a class="' + cls + '" href="' + href(node.path, node.dir) + '" data-path="' + esc(node.path) + '"' +
    (node.dir ? ' data-dir="1" aria-expanded="' + open.has(node.path) + '"' : "") +
    (node.path === current ? ' aria-current="page"' : "") +
    ' title="' + esc(node.name) + '">' + icon + '<span class="label">' + esc(node.name) + "</span></a>";
}

function listHTML(nodes, parentIgnored) {
  let out = "<ul>";
  nodes.forEach((n) => {
    const cls = [];
    if (n.dir && open.has(n.path)) cls.push("open");
    if (n.ignored) cls.push("ignored");
    // Hidden by CSS, so that the setting applies at once
    if (hideable(n, parentIgnored)) cls.push("hideable");
    out += '<li class="' + cls.join(" ") + '"' + (n.ignored ? ' title="Ignored by git"' : "") + ">" + rowHTML(n);
    // Only what is open is drawn: a large tree costs nothing folded
    if (n.dir && open.has(n.path) && n.children && n.children.length) out += listHTML(n.children, n.ignored);
    out += "</li>";
  });
  return out + "</ul>";
}

function flatten(node, out, hide) {
  (node.children || []).forEach((n) => {
    if (!n.dir) out.push(n);
    else if (!(hide && hideable(n, node.ignored))) flatten(n, out, hide);
  });
  return out;
}

// rank orders a file found by the words typed: the name equal to them,
// starting with them, holding them all, then the path holding them; then
// shallower paths, then earlier matches. A lower rank comes first.
function rank(n, q, words) {
  const name = n.name.toLowerCase();
  const p = n.path.toLowerCase();
  const kind = name === q ? 0 : name.startsWith(words[0]) ? 1 : words.every((w) => name.includes(w)) ? 2 : 3;
  return [kind, n.path.split("/").length, p.indexOf(words[0]), p.length, p];
}

function before(a, b) {
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return 0;
}

// fold opens or folds a directory in the tree, reading it first if it
// came without its entries.
function fold(path, opening) {
  if (opening) open.add(path); else open.delete(path);
  save("open", Array.from(open));
  const n = find(path);
  (n && n.lazy && !n.children && opening ? readDir(path) : Promise.resolve()).then(render);
}

// unpeek puts away the tree shown for the moment, once initTree has run.
let unpeek = () => {};

// hits are the files found, and selected the one Enter opens.
let hits = [];
let selected = 0;

// render draws the tree, or the files found, keeping the focus on the row
// it was on.
function render() {
  const focused = treeEl.contains(document.activeElement) ? document.activeElement.dataset.path : undefined;
  draw();
  if (focused === undefined) return;
  const row = Array.from(treeEl.querySelectorAll(".row")).find((r) => r.dataset.path === focused);
  if (row) row.focus({ preventScroll: true });
}

function draw() {
  if (!tree) return;
  const data = mdOnlyEl.checked ? prune(tree) || { children: [] } : tree;
  const q = filterEl.value.trim().toLowerCase();
  if (q) {
    const words = q.split(/\s+/);
    hits = flatten(data, [], hidingIgnored())
      .filter((n) => words.every((w) => n.path.toLowerCase().includes(w)))
      .map((n) => [rank(n, q, words), n])
      .sort((a, b) => before(a[0], b[0]))
      .slice(0, 300)
      .map((x) => x[1]);
    selected = Math.min(selected, Math.max(hits.length - 1, 0));
    // The name first, so that a deep path cuts the directory, not it
    treeEl.innerHTML = hits.length ? "<ul>" + hits.map((n, i) => {
      const d = dirname(n.path);
      const cls = "row" + (n.path === current ? " current" : "") + (i === selected ? " selected" : "");
      return '<li class="' + (n.ignored ? "ignored" : "") + '"><a class="' + cls + '" href="' + href(n.path) +
        '" data-path="' + esc(n.path) + '" title="' + esc(n.path) + '"' + (n.path === current ? ' aria-current="page"' : "") + '>' + ICON_FILE + '<span class="label">' + esc(n.name) +
        (d === "." ? "" : ' <span class="dir-path">' + esc(d) + "</span>") + "</span></a></li>";
    }).join("") + "</ul>" : '<div class="empty">No matching files</div>';
    return;
  }
  hits = [];
  treeEl.innerHTML = listHTML(data.children || []);
}

// select moves the choice among the files found, keeping it in sight.
function select(i) {
  if (!hits.length) return;
  selected = (i + hits.length) % hits.length;
  const rows = treeEl.querySelectorAll(".row");
  rows.forEach((r, k) => r.classList.toggle("selected", k === selected));
  const row = rows[selected];
  const t = treeEl.getBoundingClientRect();
  const r = row.getBoundingClientRect();
  if (r.top < t.top) treeEl.scrollTop -= t.top - r.top;
  else if (r.bottom > t.bottom) treeEl.scrollTop += r.bottom - t.bottom;
}

// Every page builds the tree anew, so where it was scrolled is kept for the
// next page of this tab: a file picked in the tree stays where it was
// clicked. Only when the current file is out of sight, as after following
// a link or the file finder, does the tree scroll to it.
const scrollKey = "gh-mini-tree-scroll:" + (body.dataset.name || "");

// A reload keeps what was typed in the filter, and the focus on it; the
// next page does not, as one opens a file found there.
const filterKey = "gh-mini-filter:" + location.pathname + location.search;

function keepFilter() {
  try {
    const q = filterEl.value;
    if (q) sessionStorage.setItem(filterKey, JSON.stringify({ q, focused: document.activeElement === filterEl, selected }));
    else sessionStorage.removeItem(filterKey);
  } catch (e) {}
}

function restoreFilter() {
  try {
    const kept = sessionStorage.getItem(filterKey);
    if (kept === null) return;
    sessionStorage.removeItem(filterKey);
    const f = JSON.parse(kept);
    filterEl.value = f.q;
    selected = f.selected || 0;
    render();
    if (f.focused) filterEl.focus();
  } catch (e) {}
}

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
      fold(path, !open.has(path));
    } else {
      open.add(path);
      save("open", Array.from(open));
    }
  });
  // The keys of a tree view: up and down move between the rows, right
  // opens a directory or moves into it, left folds it or moves out of it
  treeEl.addEventListener("keydown", (e) => {
    const row = e.target.closest(".row");
    if (!row || e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return;
    const rows = Array.from(treeEl.querySelectorAll(".row")).filter((r) => r.offsetParent !== null);
    const i = rows.indexOf(row);
    const path = row.dataset.path;
    const go = (r) => { if (r) r.focus(); };
    switch (e.key) {
      case "ArrowDown": go(rows[i + 1]); break;
      case "ArrowUp": go(rows[i - 1]); break;
      case "Home": go(rows[0]); break;
      case "End": go(rows[rows.length - 1]); break;
      case "ArrowRight":
        if (!row.dataset.dir) return;
        if (!open.has(path)) fold(path, true);
        else go(row.parentElement.querySelector(":scope > ul .row"));
        break;
      case "ArrowLeft":
        if (row.dataset.dir && open.has(path)) fold(path, false);
        else go(row.parentElement.parentElement.closest("li")?.querySelector(":scope > .row"));
        break;
      default:
        return;
    }
    e.preventDefault();
  });

  filterEl.addEventListener("input", () => {
    selected = 0;
    render();
  });
  filterEl.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      select(selected + (e.key === "ArrowDown" ? 1 : -1));
    } else if (e.key === "Enter") {
      // Nothing typed, nothing to open
      if (hits.length) location.href = href(hits[selected].path);
    } else if (e.key === "Escape") {
      filterEl.value = "";
      render();
      filterEl.blur();
      unpeek();
    }
  });
  mdOnlyEl.addEventListener("change", () => {
    save("mdOnly", mdOnlyEl.checked);
    render();
  });

  // The server renders the tree closed from a cookie. It was kept in
  // localStorage before: move it over once
  const toggle = document.getElementById("mini.sidebar-toggle");
  const sidebar = document.getElementById("mini.sidebar");
  if (!document.cookie.split("; ").some((c) => c.startsWith("gh-mini-sidebar=")) && load("sidebarHidden", false)) {
    setCookie("gh-mini-sidebar", "hidden");
    body.classList.add("sidebar-hidden");
  }
  const narrow = () => window.matchMedia("(max-width: 767px)").matches;
  const expanded = () => {
    const shown = narrow() ? body.classList.contains("sidebar-shown") : !body.classList.contains("sidebar-hidden");
    toggle.setAttribute("aria-expanded", String(shown));
  };
  // A narrow screen shows the tree over the page, and a wide one with the
  // tree closed shows it for the moment only: either is put away again by
  // Escape or a click elsewhere, and neither is kept for the next page
  let peeking = false;
  const peek = () => {
    if (narrow()) body.classList.add("sidebar-shown");
    else if (body.classList.contains("sidebar-hidden")) {
      body.classList.remove("sidebar-hidden");
      peeking = true;
    }
    expanded();
  };
  unpeek = () => {
    body.classList.remove("sidebar-shown");
    if (peeking) body.classList.add("sidebar-hidden");
    peeking = false;
    expanded();
  };
  toggle.addEventListener("click", () => {
    if (narrow()) {
      body.classList.toggle("sidebar-shown");
    } else {
      const hidden = body.classList.toggle("sidebar-hidden");
      peeking = false;
      setCookie("gh-mini-sidebar", hidden ? "hidden" : "shown");
      save("sidebarHidden", hidden);
    }
    expanded();
  });
  window.matchMedia("(max-width: 767px)").addEventListener("change", expanded);
  expanded();

  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (e.key === "Escape" && !document.querySelector("dialog[open]")) {
      if (body.classList.contains("sidebar-shown") || peeking) unpeek();
      return;
    }
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (document.querySelector("dialog[open]")) return;
    if (e.key === "t" || e.key === "/") {
      e.preventDefault();
      peek();
      filterEl.focus();
      filterEl.select();
    }
  });
  document.addEventListener("pointerdown", (e) => {
    if (!body.classList.contains("sidebar-shown") && !peeking) return;
    if (sidebar.contains(e.target) || toggle.contains(e.target)) return;
    unpeek();
  });

  window.addEventListener("pagehide", keepScroll);
  window.addEventListener("pagehide", keepFilter);
  fetchTree().then(() => {
    restoreFilter();
    restoreScroll();
    revealCurrent();
  });
}
