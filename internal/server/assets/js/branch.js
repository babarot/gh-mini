// The branch in the top bar, on a branch other than the base: how far it
// is ahead of the base and behind it, and a panel that tells more, where
// it left the base and what is not pushed. The browser opens and closes
// the panel itself, as a popover; b does too.

import { onStatus } from "./status.js";

const button = document.getElementById("mini.branch");
const panel = document.getElementById("mini.branch-panel");

const icons = {
  branch: "M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6A2.5 2.5 0 0 1 10 8.5H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.493 2.493 0 0 1 6 7h4a1 1 0 0 0 1-1v-.628A2.25 2.25 0 0 1 9.5 3.25Zm-6 0a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Zm8.25-.75a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5ZM4.25 12a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Z",
  upload: "M2.75 14A1.75 1.75 0 0 1 1 12.25v-2.5a.75.75 0 0 1 1.5 0v2.5c0 .138.112.25.25.25h10.5a.25.25 0 0 0 .25-.25v-2.5a.75.75 0 0 1 1.5 0v2.5A1.75 1.75 0 0 1 13.25 14ZM11.78 4.72a.749.749 0 1 1-1.06 1.06L8.75 3.811V9.5a.75.75 0 0 1-1.5 0V3.811L5.28 5.78a.749.749 0 1 1-1.06-1.06l3.25-3.25a.749.749 0 0 1 1.06 0l3.25 3.25Z",
};

function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  e.append(...children.filter((c) => c !== null && c !== undefined));
  return e;
}

function icon(name) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "octicon");
  svg.setAttribute("viewBox", "0 0 16 16");
  svg.setAttribute("width", "16");
  svg.setAttribute("height", "16");
  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", icons[name]);
  svg.append(path);
  return svg;
}

const plural = (n, one, many) => n + " " + (n === 1 ? one : many);

// row is one line of the panel: an icon, what it tells, and a note under.
function row(iconName, title, note, cls) {
  return el("div", { class: "branch-row" + (cls ? " " + cls : "") }, icon(iconName),
    el("span", {}, title, note ? el("small", {}, note) : null));
}

// standing tells the commits ahead and behind as words.
function standing(b) {
  if (!b.ahead && !b.behind) return el("span", {}, "Same as ", el("b", {}, b.base));
  const parts = [];
  if (b.ahead) parts.push(b.ahead + " ahead");
  if (b.behind) parts.push(el("span", { class: "behind" }, b.behind + " behind"));
  const out = el("span", {});
  parts.forEach((p, i) => out.append(...(i ? [", ", p] : [p])));
  out.append(" ", el("b", {}, b.base));
  return out;
}

function render(b) {
  const ab = button.querySelector("[data-ab]");
  ab.hidden = !b.ahead && !b.behind;
  const ahead = ab.querySelector("[data-ahead]");
  const behind = ab.querySelector("[data-behind]");
  ahead.textContent = "↑" + b.ahead;
  behind.textContent = "↓" + b.behind;
  behind.classList.toggle("behind", b.behind > 0);

  const rows = [row("branch", standing(b), `Left ${b.base} at ${b.mergeBase}, as of the last fetch`)];
  if (!b.detached) {
    if (!b.upstream) rows.push(row("upload", "Not pushed", "No branch on a remote yet"));
    else if (b.unpushed) rows.push(row("upload", plural(b.unpushed, "commit", "commits") + " not pushed", "to " + b.upstream, "attention"));
    else rows.push(row("upload", "Pushed", "to " + b.upstream));
  }
  panel.replaceChildren(...rows);
}

// refreshBranch reads where the branch stands again. On the base, or
// without one, there is no button to update: the page shows that once
// reloaded.
export async function refreshBranch() {
  if (!button) return;
  try {
    const r = await fetch("/_mini/api/branch", { cache: "no-store" });
    const b = r.ok ? await r.json() : null;
    if (b) render(b);
  } catch (e) {}
}

export function initBranch() {
  if (!button) return;
  // Under the button, as wide as it lets on a narrow screen
  panel.addEventListener("beforetoggle", (e) => {
    if (e.newState !== "open") return;
    const r = button.getBoundingClientRect();
    panel.style.top = r.bottom + 4 + "px";
    // The panel is not laid out yet: its width is the one app.css gives
    const width = Math.min(340, window.innerWidth - 16);
    panel.style.left = Math.max(8, Math.min(r.left, window.innerWidth - width - 8)) + "px";
  });
  // A commit moves the branch without a change of HEAD's file
  onStatus(refreshBranch);
  refreshBranch();
  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    if (e.metaKey || e.ctrlKey || e.altKey || document.querySelector("dialog[open]")) return;
    if (e.key === "b") {
      e.preventDefault();
      panel.togglePopover();
    }
  });
}
