import { initTree, fetchTree, refresh } from "./tree.js";
import { initSettings, refreshTheme } from "./settings.js";
import { initLangs } from "./langs.js";
import { initToc } from "./toc.js";
import { initReload } from "./reload.js";
import { initCopy } from "./copy.js";
import { initMermaid } from "./mermaid.js";

// Each part starts on its own, so that one failing leaves the rest working
for (const init of [
  initTree,
  initSettings,
  initLangs,
  initToc,
  initCopy,
  initMermaid,
  () => initReload({ onTheme: refreshTheme, onStructure: fetchTree, onFiles: refresh }),
]) {
  try {
    init();
  } catch (e) {
    console.error(e);
  }
}
