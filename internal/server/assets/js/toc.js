// Heading anchors and the table of contents of a Markdown page.

import { ICON_LINK } from "./util.js";

export function initToc() {
  const article = document.querySelector(".markdown-body");
  if (!article) return;
  article.querySelectorAll("h1[id], h2[id], h3[id], h4[id]").forEach((h) => {
    const a = document.createElement("a");
    a.className = "anchor";
    a.href = "#" + h.id;
    a.setAttribute("aria-label", "Permalink");
    a.innerHTML = ICON_LINK;
    h.insertBefore(a, h.firstChild);
  });
  // The server renders the table of contents, so that the page does not
  // shift when it appears; each entry is matched to its heading by id
  const toc = document.getElementById("mini.toc");
  if (!toc) return;
  const links = [];
  const heads = [];
  toc.querySelectorAll("a").forEach((a) => {
    const h = document.getElementById(decodeFragment(a.getAttribute("href")));
    if (h) {
      links.push(a);
      heads.push(h);
    }
  });
  if (!heads.length) return;
  // Headings near the end of a page never reach the top of the window,
  // because the page stops scrolling first. At the bottom, the heading
  // the reader jumped to stays active while it is in view; otherwise the
  // last heading is.
  let jumped = heads.findIndex((h) => h.id === decodeFragment(location.hash));
  toc.addEventListener("click", (e) => {
    const a = e.target.closest("a");
    if (a) jumped = links.indexOf(a);
    // A jump that needs no scrolling fires no scroll event.
    requestAnimationFrame(spy);
  });
  // Scrolling by hand ends the jump.
  ["wheel", "touchmove", "keydown"].forEach((type) => {
    window.addEventListener(type, () => { jumped = -1; }, { passive: true });
  });
  // A heading counts as reached once it is where a jump puts it: below
  // the top bar, by the page's scroll padding
  const reached = parseFloat(getComputedStyle(document.documentElement).scrollPaddingTop) + 8 || 120;
  const spy = () => {
    const root = document.documentElement;
    const atBottom = window.innerHeight + window.scrollY >= root.scrollHeight - 2;
    let idx = 0;
    if (atBottom) {
      const top = jumped >= 0 ? heads[jumped].getBoundingClientRect().top : -1;
      idx = top >= 0 && top < window.innerHeight ? jumped : heads.length - 1;
    } else {
      heads.forEach((h, i) => { if (h.getBoundingClientRect().top < reached) idx = i; });
    }
    links.forEach((a, i) => { a.classList.toggle("active", i === idx); });
    // Keep the entry in sight in a long table of contents. Its own
    // scrollTop, not scrollIntoView, which would scroll the page too
    const t = toc.getBoundingClientRect();
    const r = links[idx].getBoundingClientRect();
    if (r.top < t.top + 32) toc.scrollTop -= t.top + 32 - r.top;
    else if (r.bottom > t.bottom - 16) toc.scrollTop += r.bottom - t.bottom + 16;
  };
  document.addEventListener("scroll", spy, { passive: true });
  spy();
}

// decodeFragment turns "#id" into the id it names, decoded when it can be:
// a malformed one is taken as it is.
function decodeFragment(hash) {
  const raw = (hash || "").replace(/^#/, "");
  try {
    return decodeURIComponent(raw);
  } catch (e) {
    return raw;
  }
}
