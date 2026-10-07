// MathJax's settings, read when it loads. A file of its own rather than
// inline, for the page's CSP.
//
// MathJax looks for \( and $$ in the whole page by default, so text the
// renderer did not take as math, like "$$10 or $$20", was typeset too. It
// now skips the page and typesets only the renderer's math elements.
window.MathJax = {
  options: {
    a11y: { backgroundOpacity: 0 },
    enableMenu: false,
    ignoreHtmlClass: "markdown-body|tree|toc|topbar",
    processHtmlClass: "math-inline|math-display",
  },
  output: { fontPath: "[mathjax]/../mathjax-newcm-font" },
};
