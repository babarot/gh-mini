import { initTree, fetchTree, refresh } from "./tree.js";
import { initSettings, refreshTheme } from "./settings.js";
import { initMenu } from "./menu.js";
import { initBranch, refreshBranch, refreshPR } from "./branch.js";
import { initLangs } from "./langs.js";
import { initToc } from "./toc.js";
import { initReload, checkStatus } from "./reload.js";
import { fetchStatus } from "./status.js";
import { initCopy } from "./copy.js";
import { initMermaid } from "./mermaid.js";
import { initPictures } from "./pictures.js";
import { initLines } from "./lines.js";

// Each part starts on its own, so that one failing leaves the rest working
for (const init of [
  initTree,
  initSettings,
  initMenu,
  initBranch,
  initLangs,
  initToc,
  initCopy,
  initMermaid,
  initPictures,
  initLines,
  () => initReload({ onTheme: refreshTheme, onStructure: fetchTree, onFiles: refresh, onStatus: fetchStatus, onHead: () => { refreshBranch(); refreshPR(); } }),
  () => fetchStatus().then(({ etag }) => checkStatus(etag)),
]) {
  try {
    init();
  } catch (e) {
    console.error(e);
  }
}
