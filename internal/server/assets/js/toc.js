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
  const links = toc.querySelectorAll("a");
  const spy = () => {
    let idx = 0;
    heads.forEach((h, i) => { if (h.getBoundingClientRect().top < 120) idx = i; });
    links.forEach((a, i) => { a.classList.toggle("active", i === idx); });
  };
  document.addEventListener("scroll", spy, { passive: true });
  spy();
}
