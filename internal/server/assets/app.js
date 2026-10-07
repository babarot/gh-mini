(function () {
  "use strict";

  var body = document.body;
  var root = document.documentElement;
  var current = body.dataset.path || ".";
  var kind = body.dataset.kind;
  var storeKey = "gh-mini:" + (body.dataset.name || "");

  function load(key, fallback) {
    try {
      var v = localStorage.getItem(storeKey + ":" + key);
      return v === null ? fallback : JSON.parse(v);
    } catch (e) {
      return fallback;
    }
  }
  function save(key, value) {
    try { localStorage.setItem(storeKey + ":" + key, JSON.stringify(value)); } catch (e) {}
  }
  function setCookie(name, value) {
    document.cookie = name + "=" + encodeURIComponent(value) + "; path=/; max-age=31536000; samesite=lax";
  }

  function href(path, dir) {
    if (!path || path === ".") return "/";
    var u = "/" + path.split("/").map(encodeURIComponent).join("/");
    return dir ? u + "/" : u;
  }
  function dirname(p) {
    var i = p.lastIndexOf("/");
    return i < 0 ? "." : p.slice(0, i);
  }
  function isMarkdown(name) {
    return /\.(md|markdown|mdx)$/i.test(name);
  }

  var ICON_DIR = '<svg class="octicon dir" viewBox="0 0 16 16" width="16" height="16"><path d="M1.75 1A1.75 1.75 0 0 0 0 2.75v10.5C0 14.216.784 15 1.75 15h12.5A1.75 1.75 0 0 0 16 13.25v-8.5A1.75 1.75 0 0 0 14.25 3H7.5a.25.25 0 0 1-.2-.1l-.9-1.2C6.07 1.26 5.55 1 5 1H1.75Z"></path></svg>';
  var ICON_FILE = '<svg class="octicon" viewBox="0 0 16 16" width="16" height="16"><path d="M2 1.75C2 .784 2.784 0 3.75 0h6.586c.464 0 .909.184 1.237.513l2.914 2.914c.329.328.513.773.513 1.237v9.586A1.75 1.75 0 0 1 13.25 16h-9.5A1.75 1.75 0 0 1 2 14.25Zm1.75-.25a.25.25 0 0 0-.25.25v12.5c0 .138.112.25.25.25h9.5a.25.25 0 0 0 .25-.25V6h-2.75A1.75 1.75 0 0 1 9 4.25V1.5Zm6.75.062V4.25c0 .138.112.25.25.25h2.688l-.011-.013-2.914-2.914-.013-.011Z"></path></svg>';
  var ICON_CHEVRON = '<svg class="octicon chevron" viewBox="0 0 16 16" width="12" height="12"><path d="M6.22 3.22a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L9.94 8 6.22 4.28a.75.75 0 0 1 0-1.06Z"></path></svg>';
  var ICON_LINK = '<svg class="octicon octicon-link" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="m7.775 3.275 1.25-1.25a3.5 3.5 0 1 1 4.95 4.95l-2.5 2.5a3.5 3.5 0 0 1-4.95 0 .751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018 1.998 1.998 0 0 0 2.83 0l2.5-2.5a2.002 2.002 0 0 0-2.83-2.83l-1.25 1.25a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042Zm-4.69 9.64a1.998 1.998 0 0 0 2.83 0l1.25-1.25a.751.751 0 0 1 1.042.018.751.751 0 0 1 .018 1.042l-1.25 1.25a3.5 3.5 0 1 1-4.95-4.95l2.5-2.5a3.5 3.5 0 0 1 4.95 0 .751.751 0 0 1-.018 1.042.751.751 0 0 1-1.042.018 1.998 1.998 0 0 0-2.83 0l-2.5 2.5a1.998 1.998 0 0 0 0 2.83Z"></path></svg>';

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  /* Sidebar tree */

  var treeEl = document.getElementById("tree");
  var filterEl = document.getElementById("tree-filter");
  var mdOnlyEl = document.getElementById("md-only");
  var tree = null;
  var open = new Set(load("open", []));
  // Open the directories down to the current page
  for (var p = kind === "dir" ? current : dirname(current); p !== "."; p = dirname(p)) open.add(p);
  if (kind === "dir" && current !== ".") open.add(current);
  mdOnlyEl.checked = load("mdOnly", false);

  // prune drops files that are not Markdown, and directories left empty
  function prune(node) {
    if (!node.dir) return isMarkdown(node.name) ? node : null;
    var kids = (node.children || []).map(prune).filter(Boolean);
    if (!kids.length && node.path) return null;
    return Object.assign({}, node, { children: kids });
  }

  function rowHTML(node, depthPad) {
    var cls = "row" + (node.path === current ? " current" : "");
    var icon = node.dir ? ICON_CHEVRON + ICON_DIR : '<span class="indent"></span>' + ICON_FILE;
    return '<a class="' + cls + '" href="' + href(node.path, node.dir) + '" data-path="' + esc(node.path) + '"' +
      (node.dir ? ' data-dir="1"' : "") + ">" + icon + '<span class="label">' + esc(node.name) + "</span></a>";
  }

  function listHTML(nodes) {
    var out = "<ul>";
    nodes.forEach(function (n) {
      var cls = [];
      if (n.dir && open.has(n.path)) cls.push("open");
      if (n.ignored) cls.push("ignored");
      out += '<li class="' + cls.join(" ") + '"' + (n.ignored ? ' title="Ignored by git"' : "") + ">" + rowHTML(n);
      if (n.dir && n.children && n.children.length) out += listHTML(n.children);
      out += "</li>";
    });
    return out + "</ul>";
  }

  function flatten(node, out) {
    (node.children || []).forEach(function (n) {
      if (!n.dir) out.push(n);
      else flatten(n, out);
    });
    return out;
  }

  function renderTree() {
    if (!tree) return;
    var data = mdOnlyEl.checked ? prune(tree) || { children: [] } : tree;
    var q = filterEl.value.trim().toLowerCase();
    if (q) {
      var words = q.split(/\s+/);
      var hits = flatten(data, []).filter(function (n) {
        var p = n.path.toLowerCase();
        return words.every(function (w) { return p.indexOf(w) >= 0; });
      }).slice(0, 300);
      treeEl.innerHTML = hits.length ? "<ul>" + hits.map(function (n) {
        var d = dirname(n.path);
        return '<li class="' + (n.ignored ? "ignored" : "") + '"><a class="row' + (n.path === current ? " current" : "") +
          '" href="' + href(n.path) + '">' + ICON_FILE + '<span class="label">' +
          (d === "." ? "" : '<span class="dir-path">' + esc(d) + "/</span>") + esc(n.name) + "</span></a></li>";
      }).join("") + "</ul>" : '<div class="empty">No matching files</div>';
      return;
    }
    treeEl.innerHTML = listHTML(data.children || []);
  }

  function scrollToCurrent() {
    var el = treeEl.querySelector(".row.current");
    if (el) el.scrollIntoView({ block: "center" });
  }

  function fetchTree() {
    return fetch("/_mini/api/tree", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (t) { tree = t; renderTree(); });
  }

  treeEl.addEventListener("click", function (e) {
    var row = e.target.closest(".row");
    if (!row || !row.dataset.dir) return;
    // A click on the chevron only folds; a click on the name also opens the
    // directory's page
    var li = row.parentElement;
    var path = row.dataset.path;
    if (e.target.closest(".chevron")) {
      e.preventDefault();
      li.classList.toggle("open");
      if (li.classList.contains("open")) open.add(path); else open.delete(path);
    } else {
      open.add(path);
    }
    save("open", Array.from(open));
  });

  filterEl.addEventListener("input", renderTree);
  filterEl.addEventListener("keydown", function (e) {
    if (e.key === "Enter") {
      var first = treeEl.querySelector(".row");
      if (first) location.href = first.href;
    } else if (e.key === "Escape") {
      filterEl.value = "";
      renderTree();
      filterEl.blur();
    }
  });
  mdOnlyEl.addEventListener("change", function () {
    save("mdOnly", mdOnlyEl.checked);
    renderTree();
  });

  document.addEventListener("keydown", function (e) {
    var t = e.target;
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "t" || e.key === "/") {
      e.preventDefault();
      body.classList.remove("sidebar-hidden");
      filterEl.focus();
      filterEl.select();
    }
  });

  var toggle = document.getElementById("sidebar-toggle");
  if (load("sidebarHidden", false)) body.classList.add("sidebar-hidden");
  toggle.addEventListener("click", function () {
    if (window.matchMedia("(max-width: 767px)").matches) {
      body.classList.toggle("sidebar-shown");
      return;
    }
    body.classList.toggle("sidebar-hidden");
    save("sidebarHidden", body.classList.contains("sidebar-hidden"));
  });

  fetchTree().then(scrollToCurrent);

  /* Theme and mode */

  var themeLink = document.getElementById("theme");
  document.getElementById("theme-select").addEventListener("change", function (e) {
    setCookie("gh-mini-theme", e.target.value);
    themeLink.href = "/_mini/theme/" + encodeURIComponent(e.target.value) + ".css";
  });
  document.getElementById("mode-select").addEventListener("change", function (e) {
    var mode = e.target.value;
    setCookie("gh-mini-mode", mode);
    if (mode) {
      delete root.dataset.auto;
      root.dataset.mode = mode;
    } else {
      root.dataset.auto = "1";
      root.dataset.mode = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    body.dataset.theme = root.dataset.mode;
    // Mermaid picks its colors when it draws
    if (document.querySelector(".mermaid")) reload();
  });

  /* Translations: a language picked on a README stays picked */

  document.querySelectorAll(".langs a").forEach(function (a) {
    a.addEventListener("click", function () {
      setCookie("gh-mini-lang", a.dataset.lang === "Default" ? "default" : a.dataset.lang.toLowerCase());
    });
  });

  /* Heading anchors and table of contents */

  var article = document.querySelector(".markdown-body");
  if (article) {
    var heads = Array.prototype.slice.call(article.querySelectorAll("h1[id], h2[id], h3[id], h4[id]"));
    heads.forEach(function (h) {
      var a = document.createElement("a");
      a.className = "anchor";
      a.href = "#" + h.id;
      a.setAttribute("aria-label", "Permalink");
      a.innerHTML = ICON_LINK;
      h.insertBefore(a, h.firstChild);
    });
    var toc = document.getElementById("toc");
    if (heads.length > 1) {
      toc.innerHTML = "<h2>On this page</h2>" + heads.map(function (h) {
        return '<a class="l' + h.tagName.slice(1) + '" href="#' + esc(h.id) + '">' + esc(h.textContent) + "</a>";
      }).join("");
      toc.hidden = false;
      var links = toc.querySelectorAll("a");
      var spy = function () {
        var idx = 0;
        heads.forEach(function (h, i) { if (h.getBoundingClientRect().top < 120) idx = i; });
        links.forEach(function (a, i) { a.classList.toggle("active", i === idx); });
      };
      document.addEventListener("scroll", spy, { passive: true });
      spy();
    }
  }

  /* Live reload */

  var scrollKey = "gh-mini-scroll:" + location.pathname + location.search;
  function reload() {
    try { sessionStorage.setItem(scrollKey, String(window.scrollY)); } catch (e) {}
    location.reload();
  }
  try {
    var y = sessionStorage.getItem(scrollKey);
    if (y !== null) {
      sessionStorage.removeItem(scrollKey);
      window.addEventListener("load", function () { window.scrollTo(0, Number(y)); });
    }
  } catch (e) {}

  if (body.dataset.reload) {
    var es = new EventSource("/_mini/events");
    es.onmessage = function (e) {
      var c = JSON.parse(e.data);
      if (c.theme) themeLink.href = themeLink.href.split("?")[0] + "?t=" + Date.now();
      if (c.structure) fetchTree();
      var paths = c.paths || [];
      var hit = paths.some(function (p) {
        return p === current || (kind === "dir" && dirname(p) === current);
      });
      if (hit || (kind === "notfound" && c.structure)) reload();
    };
  }
})();
