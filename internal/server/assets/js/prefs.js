// The theme, the color mode and the language picked on a README, kept in
// cookies so that the server renders the next page with them.

import { setCookie } from "./util.js";
import { reload } from "./reload.js";

const root = document.documentElement;
const themeLink = document.getElementById("theme");

// refreshTheme loads the theme's CSS again after it was saved.
export function refreshTheme() {
  themeLink.href = themeLink.href.split("?")[0] + "?t=" + Date.now();
}

export function initPrefs() {
  document.getElementById("theme-select").addEventListener("change", (e) => {
    setCookie("gh-mini-theme", e.target.value);
    themeLink.href = "/_mini/theme/" + encodeURIComponent(e.target.value) + ".css";
  });
  document.getElementById("mode-select").addEventListener("change", (e) => {
    const mode = e.target.value;
    setCookie("gh-mini-mode", mode);
    if (mode) {
      delete root.dataset.auto;
      root.dataset.mode = mode;
    } else {
      root.dataset.auto = "1";
      root.dataset.mode = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    document.body.dataset.theme = root.dataset.mode;
    // Mermaid picks its colors when it draws
    if (document.querySelector(".mermaid")) reload();
  });

  // A language picked on a README stays picked
  document.querySelectorAll(".langs a").forEach((a) => {
    a.addEventListener("click", () => {
      setCookie("gh-mini-lang", a.dataset.lang === "Default" ? "default" : a.dataset.lang.toLowerCase());
    });
  });
}
