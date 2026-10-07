// The file tree in the sidebar, its filter and its toggle.

import { page, load, save, href, dirname, isMarkdown, esc, ICON_DIR, ICON_FILE, ICON_CHEVRON } from "./util.js";

const body = document.body;
const treeEl = document.getElementById("tree");
const filterEl = document.getElementById("tree-filter");
const mdOnlyEl = document.getElementById("md-only");
const current = page.path;
let tree = null;
const open = new Set(load("open", []));

// prune drops files that are not Markdown, and directories left empty
function prune(node) {
  if (!node.dir) return isMarkdown(node.name) ? node : null;
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
    if (n.dir && n.children && n.children.length) out += listHTML(n.children);
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

function scrollToCurrent() {
  const el = treeEl.querySelector(".row.current");
  if (el) el.scrollIntoView({ block: "center" });
}

// fetchTree loads the tree, again when files were added or removed.
export function fetchTree() {
  return fetch("/_mini/api/tree", { cache: "no-store" })
    .then((r) => r.json())
    .then((t) => { tree = t; render(); });
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
    const li = row.parentElement;
    const path = row.dataset.path;
    if (e.target.closest(".chevron")) {
      e.preventDefault();
      li.classList.toggle("open");
      if (li.classList.contains("open")) open.add(path); else open.delete(path);
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

  const toggle = document.getElementById("sidebar-toggle");
  if (load("sidebarHidden", false)) body.classList.add("sidebar-hidden");
  toggle.addEventListener("click", () => {
    if (window.matchMedia("(max-width: 767px)").matches) {
      body.classList.toggle("sidebar-shown");
      return;
    }
    body.classList.toggle("sidebar-hidden");
    save("sidebarHidden", body.classList.contains("sidebar-hidden"));
  });

  fetchTree().then(scrollToCurrent);
}
