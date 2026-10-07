// The settings dialog. The server renders it from settingDefs (settings.go)
// with the values it resolved; a change is saved in the gh-mini-settings
// cookie, so the next page is rendered with it, and applied to this page
// at once.

import { setCookie } from "./util.js";

const COOKIE = "gh-mini-settings";
const root = document.documentElement;
const themeLink = document.getElementById("theme");

// settings holds the value of every setting, as the server resolved them
// and as changed since.
export const settings = JSON.parse(document.getElementById("settings-data").textContent);

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
};

function set(key, value) {
  save({ [key]: value });
  const v = typeof value === "boolean" ? String(value) : value;
  settings[key] = v;
  if (appliers[key]) appliers[key](v);
  else root.setAttribute("data-" + key, v);
}

// refreshTheme loads the theme's CSS again after it was saved.
export function refreshTheme() {
  themeLink.href = themeLink.href.split("?")[0] + "?t=" + Date.now();
}

export function initSettings() {
  migrate();
  const dialog = document.getElementById("settings");

  dialog.addEventListener("change", (e) => {
    const el = e.target.closest("[data-setting]");
    if (!el) return;
    set(el.dataset.setting, el.type === "checkbox" ? el.checked : el.value);
  });
  dialog.addEventListener("click", (e) => {
    const button = e.target.closest("button[data-setting]");
    if (button) {
      for (const b of dialog.querySelectorAll(`button[data-setting="${button.dataset.setting}"]`)) {
        b.classList.toggle("selected", b === button);
        b.setAttribute("aria-checked", String(b === button));
      }
      set(button.dataset.setting, button.dataset.value);
      return;
    }
    // A click on the backdrop lands on the dialog itself, outside its box
    const r = dialog.getBoundingClientRect();
    if (e.target === dialog && (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom)) {
      dialog.close();
    }
  });

  document.getElementById("settings-open").addEventListener("click", () => dialog.showModal());
  document.getElementById("settings-close").addEventListener("click", () => dialog.close());
  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    if (e.metaKey || e.ctrlKey || e.altKey || dialog.open) return;
    if (e.key === ",") {
      e.preventDefault();
      dialog.showModal();
    }
  });
}
