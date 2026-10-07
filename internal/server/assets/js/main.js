import { initTree, fetchTree } from "./tree.js";
import { initSettings, refreshTheme } from "./settings.js";
import { initLangs } from "./langs.js";
import { initToc } from "./toc.js";
import { initReload } from "./reload.js";

initTree();
initSettings();
initLangs();
initToc();
initReload({ onTheme: refreshTheme, onStructure: fetchTree });
