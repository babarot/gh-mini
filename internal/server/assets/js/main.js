import { initTree, fetchTree } from "./tree.js";
import { initPrefs, refreshTheme } from "./prefs.js";
import { initToc } from "./toc.js";
import { initReload } from "./reload.js";

initTree();
initPrefs();
initToc();
initReload({ onTheme: refreshTheme, onStructure: fetchTree });
