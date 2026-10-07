import { initTree, fetchTree } from "./tree.js";
import { initSettings, refreshTheme } from "./settings.js";
import { initLangs } from "./langs.js";
import { initToc } from "./toc.js";
import { initReload } from "./reload.js";
import { initCopy } from "./copy.js";
import { initMermaid } from "./mermaid.js";

initTree();
initSettings();
initLangs();
initToc();
initCopy();
initMermaid();
initReload({ onTheme: refreshTheme, onStructure: fetchTree });
