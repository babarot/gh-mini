// Lines of a source file picked by their numbers, as on GitHub: #L10 is a
// line and #L10-L20 the lines from one to the other. A click on a number
// picks its line, and a click with Shift the lines from the one picked
// before, without scrolling the page.

import { page } from "./util.js";

export function initLines() {
  if (page.kind !== "code") return;
  const code = document.querySelector(".code");
  if (!code) return;
  let picked = null;

  const show = (range, scroll) => {
    for (const el of code.querySelectorAll(".line.selected")) el.classList.remove("selected");
    picked = range;
    if (!range) return;
    for (let n = range[0]; n <= range[1]; n++) {
      document.getElementById("L" + n)?.parentElement.classList.add("selected");
    }
    if (scroll) document.getElementById("L" + range[0])?.scrollIntoView({ block: "start" });
  };

  show(parseRange(location.hash), true);
  window.addEventListener("hashchange", () => show(parseRange(location.hash), true));

  // Shift would otherwise select the text between the clicks
  code.addEventListener("mousedown", (e) => {
    if (e.shiftKey && e.target.closest(".ln")) e.preventDefault();
  });
  code.addEventListener("click", (e) => {
    const a = e.target.closest(".ln a");
    if (!a || e.metaKey || e.ctrlKey || e.altKey) return;
    const n = Number(a.closest(".ln").id.slice(1));
    if (!n) return;
    e.preventDefault();
    // With Shift, from the line picked first toward the one clicked
    const from = e.shiftKey && picked ? picked.from : n;
    const range = [Math.min(from, n), Math.max(from, n)];
    range.from = from;
    history.replaceState(history.state, "", fragment(range));
    show(range, false);
  });
}

// parseRange reads #L10 or #L10-L20, either way round, as [first, last].
function parseRange(hash) {
  const m = /^#L(\d+)(?:-L?(\d+))?$/.exec(hash || "");
  if (!m) return null;
  const a = Number(m[1]);
  const b = m[2] ? Number(m[2]) : a;
  const range = [Math.min(a, b), Math.max(a, b)];
  range.from = a;
  return range;
}

function fragment([first, last]) {
  return first === last ? "#L" + first : `#L${first}-L${last}`;
}
