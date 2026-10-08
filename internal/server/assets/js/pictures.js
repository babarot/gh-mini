// Pictures with a source for each color scheme, as READMEs give their
// logos: the browser picks by the system's scheme, so a mode picked in
// the settings would show the other logo. Each source's condition on the
// scheme is made true or false by the mode, again when it changes.

const root = document.documentElement;
const SCHEME = /\(\s*prefers-color-scheme\s*:\s*(dark|light)\s*\)/gi;
const YES = "(min-width: 0px)";
const NO = "(not (min-width: 0px))";

export function initPictures() {
  const sources = Array.from(document.querySelectorAll(".markdown-body picture source[media]"))
    .filter((s) => new RegExp(SCHEME.source, "i").test(s.media))
    .map((s) => [s, s.media]);
  if (!sources.length) return;
  const apply = () => {
    const mode = root.dataset.mode;
    for (const [s, media] of sources) {
      s.media = media.replace(SCHEME, (_, scheme) => (scheme.toLowerCase() === mode ? YES : NO));
    }
  };
  apply();
  new MutationObserver(apply).observe(root, { attributes: true, attributeFilter: ["data-mode"] });
}
