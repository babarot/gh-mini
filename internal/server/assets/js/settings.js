// The settings dialog. The server renders it from settingDefs (settings.go)
// with the values it resolved; a change is saved in the gh-mini-settings
// cookie, so the next page is rendered with it, and applied to this page
// at once.

import { page, setCookie, closeOnBackdrop, load, save as keep } from "./util.js";
import { reload } from "./reload.js";

const COOKIE = "gh-mini-settings";
const root = document.documentElement;
const themeLink = document.getElementById("mini.theme");

// settings holds the value of every setting, as the server resolved them
// and as changed since.
export const settings = JSON.parse(document.getElementById("mini.settings-data").textContent);

function readCookie(name) {
  const prefix = name + "=";
  for (const part of document.cookie.split("; ")) {
    if (part.startsWith(prefix)) return part.slice(prefix.length);
  }
  return null;
}

function deleteCookie(name) {
  document.cookie = name + "=; path=/; max-age=0; samesite=lax";
}

// stored returns what the cookie holds: only the keys picked, maybe by
// another gh-mini on localhost, which shares the cookie whatever its port.
function stored() {
  const raw = readCookie(COOKIE);
  if (raw === null) return {};
  try {
    const v = JSON.parse(decodeURIComponent(raw));
    return v && typeof v === "object" && !Array.isArray(v) ? v : {};
  } catch (e) {
    return {};
  }
}

// save sets the given keys and keeps the rest of the cookie as it is, even
// keys and values this server does not know.
function save(patch) {
  setCookie(COOKIE, JSON.stringify({ ...stored(), ...patch }));
}

// Settings were kept in a cookie each before. Move the ones there are,
// without overwriting what the new cookie already holds.
const legacy = { theme: "gh-mini-theme", mode: "gh-mini-mode" };

function migrate() {
  const current = stored();
  const patch = {};
  const found = [];
  for (const [key, name] of Object.entries(legacy)) {
    const v = readCookie(name);
    if (v === null) continue;
    found.push(name);
    if (!(key in current)) patch[key] = decodeURIComponent(v);
  }
  if (Object.keys(patch).length) save(patch);
  found.forEach(deleteCookie);
}

// appliers apply a setting to this page. A setting without one is set as
// <html data-<key>>, which is what settings marked Attr need. A setting
// that changes what the server renders can call reload() from reload.js.
const appliers = {
  theme(value) {
    themeLink.href = "/_mini/theme/" + encodeURIComponent(value) + ".css";
  },
  mode(value) {
    if (value) {
      delete root.dataset.auto;
      root.dataset.mode = value;
    } else {
      root.dataset.auto = "1";
      root.dataset.mode = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
  },
  avatars() {
    // The server leaves the picture out; only a file's page shows one
    if (document.querySelector(".commit")) reload();
  },
  htmlPreview() {
    // The server picks the view of an HTML file, unless the URL does
    const q = new URLSearchParams(location.search);
    const html = page.kind === "html" || (page.kind === "code" && /\.html?$/i.test(page.path));
    if (html && !q.has("plain") && !q.has("preview")) reload();
  },
  languageSwitch() {
    // The server picks the README and the language switch
    if (page.kind === "dir" || /\.(md|markdown|mdx)$/i.test(page.path)) reload();
  },
  // What changed since the last commit is in what the server renders, all
  // over the page
  changes() {
    reload();
  },
  untracked() {
    reload();
  },
  ignoreWhitespace() {
    if (page.kind === "changes" || page.kind === "diff") reload();
  },
  openChanged() {
    // The server picks the view of a changed file, unless the URL does
    const q = new URLSearchParams(location.search);
    if (page.kind !== "dir" && page.kind !== "changes" && !q.has("diff") && !q.has("plain") && !q.has("preview")) reload();
  },
};

// enableChildren shows the settings under a toggle as doing something or
// not, as it is on or off.
function enableChildren(dialog, key, on) {
  for (const row of dialog.querySelectorAll(`.setting[data-parent="${key}"]`)) {
    row.classList.toggle("disabled", !on);
    row.querySelectorAll("[data-setting]").forEach((el) => { el.disabled = !on; });
  }
}

function set(key, value) {
  save({ [key]: value });
  const v = typeof value === "boolean" ? String(value) : value;
  settings[key] = v;
  if (appliers[key]) appliers[key](v);
  // A plugin shows parts of Markdown, on a file's page or a README, and
  // may bring the theme shown, which is the built-in one with it off
  else if (key.startsWith("plugin.")) {
    if (page.kind === "dir" || page.kind === "markdown") reload();
    else refreshTheme();
  } else root.setAttribute("data-" + key, v);
}

// initSections switches the page of the dialog, one per section, from the
// list of sections, which is one stop of Tab chosen with the arrow keys as
// the dialog's tabs. The page last shown is shown again.
function initSections(dialog) {
  const tabs = Array.from(dialog.querySelectorAll("button[role=tab]"));
  const show = (tab) => {
    for (const t of tabs) {
      const on = t === tab;
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
      document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
    }
    keep("settingsSection", tab.dataset.section);
  };
  const last = tabs.find((t) => t.dataset.section === load("settingsSection", ""));
  if (last) show(last);
  dialog.addEventListener("click", (e) => {
    const tab = e.target.closest("button[role=tab]");
    if (tab) show(tab);
  });
  dialog.addEventListener("keydown", (e) => {
    const tab = e.target.closest("button[role=tab]");
    if (!tab) return;
    const step = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 }[e.key];
    if (!step) return;
    e.preventDefault();
    const next = tabs[(tabs.indexOf(tab) + step + tabs.length) % tabs.length];
    show(next);
    next.focus();
  });
}

// refreshTheme loads the theme's CSS again after it was saved.
export function refreshTheme() {
  themeLink.href = themeLink.href.split("?")[0] + "?t=" + Date.now();
}

export function initSettings() {
  migrate();
  const dialog = document.getElementById("mini.settings");

  dialog.addEventListener("change", (e) => {
    const el = e.target.closest("[data-setting]");
    if (!el) return;
    set(el.dataset.setting, el.type === "checkbox" ? el.checked : el.value);
    if (el.type === "checkbox") enableChildren(dialog, el.dataset.setting, el.checked);
  });
  // A group of buttons is one stop of Tab, chosen with the arrow keys, as
  // radio buttons are
  const choose = (button) => {
    for (const b of dialog.querySelectorAll(`button[data-setting="${button.dataset.setting}"]`)) {
      b.classList.toggle("selected", b === button);
      b.setAttribute("aria-checked", String(b === button));
      b.tabIndex = b === button ? 0 : -1;
    }
    set(button.dataset.setting, button.dataset.value);
  };
  initSections(dialog);
  dialog.addEventListener("keydown", (e) => {
    const button = e.target.closest("button[role=radio]");
    if (!button) return;
    const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
    if (!step) return;
    e.preventDefault();
    const group = Array.from(dialog.querySelectorAll(`button[data-setting="${button.dataset.setting}"]`));
    const next = group[(group.indexOf(button) + step + group.length) % group.length];
    choose(next);
    next.focus();
  });
  dialog.addEventListener("click", (e) => {
    const button = e.target.closest("button[data-setting]");
    if (button) choose(button);
  });
  closeOnBackdrop(dialog);

  document.getElementById("mini.settings-close").addEventListener("click", () => dialog.close());
  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    // Not over a dialog already open, the About one included
    if (e.metaKey || e.ctrlKey || e.altKey || document.querySelector("dialog[open]")) return;
    if (e.key === ",") {
      e.preventDefault();
      // The menu may be open; it would stay above the dialog
      document.getElementById("mini.menu").hidePopover();
      dialog.showModal();
    }
  });
}
