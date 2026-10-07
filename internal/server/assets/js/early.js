// Loaded in <head>, before anything is painted: follow the system's color
// scheme until a mode is picked. A classic script rather than inline, so
// that the page's CSP allows scripts from gh-mini's own files only.
(function () {
  var root = document.documentElement;
  if (root.dataset.mode) return;
  var mq = window.matchMedia("(prefers-color-scheme: dark)");
  var apply = function () { root.dataset.auto = "1"; root.dataset.mode = mq.matches ? "dark" : "light"; };
  apply();
  mq.addEventListener("change", function () { if (root.dataset.auto) apply(); });
})();
