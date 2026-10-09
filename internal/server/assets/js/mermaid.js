// Mermaid diagrams, drawn in the colors of the mode and the theme and drawn
// again when either changes, with buttons to zoom and pan as GitHub has.

import { settings } from "./settings.js";

const root = document.documentElement;

const icon = (d) => '<svg class="octicon" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="' + d + '"></path></svg>';
const ICONS = {
  zoomIn: icon("M3.75 7.5a.75.75 0 0 1 .75-.75h2.25V4.5a.75.75 0 0 1 1.5 0v2.25h2.25a.75.75 0 0 1 0 1.5H8.25v2.25a.75.75 0 0 1-1.5 0V8.25H4.5a.75.75 0 0 1-.75-.75Z M7.5 0a7.5 7.5 0 0 1 5.807 12.247l2.473 2.473a.749.749 0 1 1-1.06 1.06l-2.473-2.473A7.5 7.5 0 1 1 7.5 0Zm-6 7.5a6 6 0 1 0 12 0 6 6 0 0 0-12 0Z"),
  zoomOut: icon("M4.5 6.75h6a.75.75 0 0 1 0 1.5h-6a.75.75 0 0 1 0-1.5Z M0 7.5a7.5 7.5 0 1 1 13.307 4.747l2.473 2.473a.749.749 0 1 1-1.06 1.06l-2.473-2.473A7.5 7.5 0 0 1 0 7.5Zm7.5-6a6 6 0 1 0 0 12 6 6 0 0 0 0-12Z"),
  reset: icon("M1.705 8.005a.75.75 0 0 1 .834.656 5.5 5.5 0 0 0 9.592 2.97l-1.204-1.204a.25.25 0 0 1 .177-.427h3.646a.25.25 0 0 1 .25.25v3.646a.25.25 0 0 1-.427.177l-1.38-1.38A7.002 7.002 0 0 1 1.05 8.84a.75.75 0 0 1 .656-.834ZM8 2.5a5.487 5.487 0 0 0-4.131 1.869l1.204 1.204A.25.25 0 0 1 4.896 6H1.25A.25.25 0 0 1 1 5.75V2.104a.25.25 0 0 1 .427-.177l1.38 1.38A7.002 7.002 0 0 1 14.95 7.16a.75.75 0 0 1-1.49.178A5.5 5.5 0 0 0 8 2.5Z"),
  up: icon("M3.22 10.53a.749.749 0 0 1 0-1.06l4.25-4.25a.749.749 0 0 1 1.06 0l4.25 4.25a.749.749 0 1 1-1.06 1.06L8 6.811 4.28 10.53a.749.749 0 0 1-1.06 0Z"),
  down: icon("M12.78 5.22a.749.749 0 0 1 0 1.06l-4.25 4.25a.749.749 0 0 1-1.06 0L3.22 6.28a.749.749 0 1 1 1.06-1.06L8 8.939l3.72-3.719a.749.749 0 0 1 1.06 0Z"),
  left: icon("M9.78 12.78a.75.75 0 0 1-1.06 0L4.47 8.53a.75.75 0 0 1 0-1.06l4.25-4.25a.751.751 0 0 1 1.042.018.751.751 0 0 1 .018 1.042L6.06 8l3.72 3.72a.75.75 0 0 1 0 1.06Z"),
  right: icon("M6.22 3.22a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L9.94 8 6.22 4.28a.75.75 0 0 1 0-1.06Z"),
};

const ZOOM_STEP = 0.2;
const PAN_STEP = 50;

export function initMermaid() {
  const diagrams = Array.from(document.querySelectorAll(".markdown-body .mermaid"));
  if (!diagrams.length || !window.mermaid) return;
  const sources = diagrams.map((d) => d.textContent.trim());
  let drawing = Promise.resolve();
  const draw = () => {
    drawing = drawing.then(() => drawAll(diagrams, sources));
  };
  draw();
  new MutationObserver(draw).observe(root, { attributes: true, attributeFilter: ["data-mode"] });
  // A theme picked, or saved, loads its stylesheet again
  document.getElementById("mini.theme")?.addEventListener("load", draw);
}

async function drawAll(diagrams, sources) {
  window.mermaid.initialize({ startOnLoad: false, logLevel: "error", ...themeConfig() });
  for (let i = 0; i < diagrams.length; i++) {
    const node = diagrams[i];
    try {
      const { svg } = await window.mermaid.render("mermaid-" + i, sources[i]);
      node.innerHTML = "";
      node.classList.remove("mermaid-error");
      const viewport = document.createElement("div");
      viewport.className = "mermaid-viewport";
      viewport.innerHTML = svg;
      node.append(controls(viewport.querySelector("svg")), viewport);
    } catch (e) {
      node.classList.add("mermaid-error");
      node.textContent = "Mermaid error:\n" + (e.message || String(e));
    }
    node.classList.add("rendered");
  }
}

// themeConfig is Mermaid's theme for the page's. The github theme draws
// diagrams as GitHub does, in Mermaid's own colors; another draws them in
// its colors, read from the page.
function themeConfig() {
  const dark = root.dataset.mode === "dark";
  const name = settings.theme || "github";
  if (name === "github") return { theme: dark ? "dark" : "default" };
  const bg = color("--bgColor-default");
  const muted = color("--bgColor-muted");
  const fg = color("--fgColor-default");
  const border = color("--borderColor-default");
  // The block is drawn in the muted background, so a node stands out
  // from it in the page's
  return {
    theme: "base",
    themeVariables: {
      darkMode: dark,
      background: muted,
      primaryColor: bg,
      primaryTextColor: fg,
      primaryBorderColor: border,
      secondaryColor: bg,
      tertiaryColor: muted,
      lineColor: color("--fgColor-muted"),
      textColor: fg,
      mainBkg: bg,
      nodeBorder: border,
      clusterBkg: muted,
      clusterBorder: border,
      edgeLabelBackground: muted,
      noteBkgColor: bg,
      noteTextColor: fg,
      noteBorderColor: border,
      fontFamily: getComputedStyle(document.body).fontFamily,
    },
  };
}

// color reads a color variable of the page as #rrggbb, the form Mermaid
// computes its shades from, whatever form the theme gives it in.
const canvas = document.createElement("canvas");
canvas.width = canvas.height = 1;
function color(name) {
  const probe = document.createElement("span");
  probe.style.color = `var(${name})`;
  document.body.append(probe);
  const c = getComputedStyle(probe).color;
  probe.remove();
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  ctx.clearRect(0, 0, 1, 1);
  ctx.fillStyle = c;
  ctx.fillRect(0, 0, 1, 1);
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
  return "#" + [r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("");
}

function controls(svg) {
  let scale = 1, x = 0, y = 0;
  const apply = () => { svg.style.transform = `translate(${x}px, ${y}px) scale(${scale})`; };
  const zoom = (d) => { scale = Math.min(10, Math.max(0.1, scale + d)); apply(); };
  svg.style.transformOrigin = "50% 0";

  const panel = document.createElement("div");
  panel.className = "mermaid-controls";
  const buttons = [
    ["zoomIn", "Zoom in", () => zoom(ZOOM_STEP)],
    ["zoomOut", "Zoom out", () => zoom(-ZOOM_STEP)],
    ["reset", "Reset view", () => { scale = 1; x = 0; y = 0; apply(); }],
    ["up", "Pan up", () => { y += PAN_STEP; apply(); }],
    ["down", "Pan down", () => { y -= PAN_STEP; apply(); }],
    ["left", "Pan left", () => { x += PAN_STEP; apply(); }],
    ["right", "Pan right", () => { x -= PAN_STEP; apply(); }],
  ];
  for (const [name, label, fn] of buttons) {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "icon-btn " + name;
    b.title = label;
    b.setAttribute("aria-label", label);
    b.innerHTML = ICONS[name];
    b.addEventListener("click", fn);
    panel.appendChild(b);
  }
  return panel;
}
