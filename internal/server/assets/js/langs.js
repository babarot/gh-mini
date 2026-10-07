// A language picked on a README stays picked: the server reads the cookie
// to choose the README of the next directory.

import { setCookie } from "./util.js";

export function initLangs() {
  document.querySelectorAll(".langs a").forEach((a) => {
    a.addEventListener("click", () => {
      setCookie("gh-mini-lang", a.dataset.lang === "Default" ? "default" : a.dataset.lang.toLowerCase());
    });
  });
}
