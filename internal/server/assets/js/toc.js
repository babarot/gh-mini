// Heading anchors and the table of contents of a Markdown page.

import { esc, ICON_LINK } from "./util.js";

export function initToc() {
  const article = document.querySelector(".markdown-body");
  if (!article) return;
  const heads = Array.from(article.querySelectorAll("h1[id], h2[id], h3[id], h4[id]"));
  heads.forEach((h) => {
    const a = document.createElement("a");
    a.className = "anchor";
    a.href = "#" + h.id;
    a.setAttribute("aria-label", "Permalink");
    a.innerHTML = ICON_LINK;
    h.insertBefore(a, h.firstChild);
  });
  if (heads.length < 2) return;
  const toc = document.getElementById("toc");
  toc.innerHTML = "<h2>On this page</h2>" + heads.map((h) =>
    '<a class="l' + h.tagName.slice(1) + '" href="#' + esc(h.id) + '">' + esc(h.textContent) + "</a>"
  ).join("");
  toc.hidden = false;
  const links = Array.from(toc.querySelectorAll("a"));
  // Headings near the end of a page never reach the top of the window,
  // because the page stops scrolling first. At the bottom, the heading
  // the reader jumped to stays active while it is in view; otherwise the
  // last heading is.
  let jumped = heads.findIndex((h) => "#" + h.id === decodeURIComponent(location.hash));
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
  const spy = () => {
    const root = document.documentElement;
    const atBottom = window.innerHeight + window.scrollY >= root.scrollHeight - 2;
    let idx = 0;
    if (atBottom) {
      const top = jumped >= 0 ? heads[jumped].getBoundingClientRect().top : -1;
      idx = top >= 0 && top < window.innerHeight ? jumped : heads.length - 1;
    } else {
      heads.forEach((h, i) => { if (h.getBoundingClientRect().top < 120) idx = i; });
    }
    links.forEach((a, i) => { a.classList.toggle("active", i === idx); });
  };
  document.addEventListener("scroll", spy, { passive: true });
  spy();
}
