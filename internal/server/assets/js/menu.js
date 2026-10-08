// The menu behind the gear button, and the dialogs it opens besides the
// settings: About and the keyboard shortcuts. The browser opens and closes
// the menu itself, as a popover.

import { closeOnBackdrop } from "./util.js";

export function initMenu() {
  const menu = document.getElementById("mini.menu");
  const open = (dialog) => {
    // Closed first, so that closing it does not take the focus back to
    // the gear button from the dialog
    menu.hidePopover();
    dialog.showModal();
  };
  menu.addEventListener("click", (e) => {
    const item = e.target.closest("[data-open]");
    if (item) open(document.getElementById(item.dataset.open));
  });
  // As a menu does: the focus goes to its first item, and the arrow keys
  // move it through them
  const items = () => Array.from(menu.querySelectorAll("button"));
  menu.addEventListener("toggle", (e) => {
    if (e.newState === "open") items()[0]?.focus();
  });
  menu.addEventListener("keydown", (e) => {
    const all = items();
    const i = all.indexOf(document.activeElement);
    const to = { ArrowDown: i + 1, ArrowUp: i - 1, Home: 0, End: all.length - 1 }[e.key];
    if (to === undefined) return;
    e.preventDefault();
    all[(to + all.length) % all.length].focus();
  });

  for (const id of ["mini.about", "mini.shortcuts"]) {
    const dialog = document.getElementById(id);
    dialog.querySelector("[data-close]").addEventListener("click", () => dialog.close());
    closeOnBackdrop(dialog);
  }

  const shortcuts = document.getElementById("mini.shortcuts");
  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (t.matches && t.matches("input, textarea, select, [contenteditable]")) return;
    if (e.metaKey || e.ctrlKey || e.altKey || document.querySelector("dialog[open]")) return;
    if (e.key === "?") {
      e.preventDefault();
      open(shortcuts);
    }
  });
}
