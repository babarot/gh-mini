// The branch in the top bar, on a branch other than the base: how far it
// is ahead of the base and behind it, its pull request on GitHub, and a
// panel that tells more, where it left the base and what is not pushed.
// The browser opens and closes the panel itself, as a popover; b does
// too.

import { onStatus } from "./status.js";

const button = document.getElementById("mini.branch");
const panel = document.getElementById("mini.branch-panel");

const icons = {
  branch: "M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6A2.5 2.5 0 0 1 10 8.5H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.493 2.493 0 0 1 6 7h4a1 1 0 0 0 1-1v-.628A2.25 2.25 0 0 1 9.5 3.25Zm-6 0a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Zm8.25-.75a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5ZM4.25 12a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Z",
  pr: "M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm5.677-.177L9.573.677A.25.25 0 0 1 10 .854V2.5h1A2.5 2.5 0 0 1 13.5 5v5.628a2.251 2.251 0 1 1-1.5 0V5a1 1 0 0 0-1-1h-1v1.646a.25.25 0 0 1-.427.177L7.177 3.427a.25.25 0 0 1 0-.354ZM3.75 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm0 9.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm8.25.75a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Z",
  merged: "M5.45 5.154A4.25 4.25 0 0 0 9.25 7.5h1.378a2.251 2.251 0 1 1 0 1.5H9.25A5.734 5.734 0 0 1 5 7.123v3.505a2.25 2.25 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.95-.218ZM4.25 13.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Zm8.5-4.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5ZM5 3.25a.75.75 0 1 0 0 .005V3.25Z",
  closed: "M3.25 1A2.25 2.25 0 0 1 4 5.372v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.251 2.251 0 0 1 3.25 1Zm9.5 5.5a.75.75 0 0 1 .75.75v3.378a2.251 2.251 0 1 1-1.5 0V7.25a.75.75 0 0 1 .75-.75Zm-2.03-5.273a.75.75 0 0 1 1.06 0l.97.97.97-.97a.748.748 0 0 1 1.265.332.75.75 0 0 1-.205.729l-.97.97.97.97a.751.751 0 0 1-.018 1.042.751.751 0 0 1-1.042.018l-.97-.97-.97.97a.749.749 0 0 1-1.275-.326.749.749 0 0 1 .215-.734l.97-.97-.97-.97a.75.75 0 0 1 0-1.06ZM2.5 3.25a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0ZM3.25 12a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm9.5 0a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Z",
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

// row is one line of the panel: an icon, what it tells, and a note under,
// leading to a page with href.
function row(iconName, title, note, cls, href) {
  const attrs = { class: "branch-row" + (cls ? " " + cls : "") };
  if (href) attrs.href = href;
  return el(href ? "a" : "div", attrs, icon(iconName),
    el("span", {}, title, note ? el("small", {}, note) : null));
}

// standing tells the commits ahead and behind as words: 2 ahead of main,
// 1 behind.
function standing(b) {
  const base = el("b", {}, b.base);
  if (!b.ahead && !b.behind) return el("span", {}, "Same as ", base);
  const behind = (text) => el("span", { class: "behind" }, text);
  if (!b.ahead) return el("span", {}, behind(b.behind + " behind"), " ", base);
  const out = el("span", {}, b.ahead + " ahead of ", base);
  if (b.behind) out.append(", ", behind(b.behind + " behind"));
  return out;
}

// The branch's standing and its pull request, as last read
let branch = null;
let pr = null;

const prIcon = (state) => ({ merged: "merged", closed: "closed" })[state] || "pr";
const prState = (state) => state[0].toUpperCase() + state.slice(1);

// renderPR puts the pull request's state after the branch, leading to it
// on GitHub, or takes it away.
function renderPR() {
  document.getElementById("mini.pr")?.remove();
  if (!pr) return;
  const chip = el("a", { class: "pr-chip " + pr.state, id: "mini.pr", href: pr.url, target: "_blank", rel: "noopener",
    title: `${pr.title} #${pr.number}: ${prState(pr.state)}` }, icon(prIcon(pr.state)), "#" + pr.number);
  panel.after(chip);
}

function render() {
  const b = branch;
  if (!b) return;
  const ab = button.querySelector("[data-ab]");
  ab.hidden = !b.ahead && !b.behind;
  const ahead = ab.querySelector("[data-ahead]");
  const behind = ab.querySelector("[data-behind]");
  ahead.textContent = "↑" + b.ahead;
  behind.textContent = "↓" + b.behind;
  behind.classList.toggle("behind", b.behind > 0);

  // The changes since the base are on the Changes page, unless changes
  // are not shown
  const changes = document.getElementById("mini.changes") ? "/_mini/changes" : "";
  const note = `Left ${b.base} at ${b.mergeBase}, as of the last fetch` + (changes ? ". Show the changes since" : "");
  const rows = [row("branch", standing(b), note, "", changes)];
  if (!b.detached) {
    if (!b.upstream) rows.push(row("upload", "Not pushed", "No branch on a remote yet"));
    else if (b.unpushed) rows.push(row("upload", plural(b.unpushed, "commit", "commits") + " not pushed", "to " + b.upstream, "attention"));
    else rows.push(row("upload", "Pushed", "to " + b.upstream));
  }
  if (pr) {
    const link = row(prIcon(pr.state), el("span", {}, el("b", {}, pr.title), " #" + pr.number), prState(pr.state) + " · on GitHub", "pr " + pr.state, pr.url);
    link.target = "_blank";
    link.rel = "noopener";
    rows.push(el("hr"), link);
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
    if (b) {
      branch = b;
      render();
    }
  } catch (e) {}
}

// refreshPR asks for the branch's pull request, which the server keeps
// for a minute: it is merged on GitHub, with nothing changed here.
export async function refreshPR() {
  if (!button) return;
  try {
    const r = await fetch("/_mini/api/pr", { cache: "no-store" });
    pr = r.status === 200 ? await r.json() : null;
  } catch (e) {
    return;
  }
  renderPR();
  render();
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
  refreshPR();
  // Coming back from GitHub, where the pull request may have been merged
  window.addEventListener("focus", refreshPR);
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
