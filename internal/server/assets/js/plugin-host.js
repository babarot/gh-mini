// The document a plugin runs in, sandboxed to an origin of its own (see
// plugins.go). It talks to the page on a channel the page hands it, and
// only then imports the plugin: the plugin is the only code here that could
// take the frame elsewhere, and once the channel is handed over, a document
// it took the frame to cannot ask for it again. A classic script, as a
// module from here would need the page to allow this origin to read it.

(() => {
  // Absolute: import() from a script of another origin, as this one is
  // to the sandbox, resolves against no base
  const src = new URL(document.currentScript.dataset.plugin, location.href).href;
  let port;
  window.addEventListener("message", (e) => {
    if (port || e.source !== window.parent || !e.ports[0]) return;
    port = e.ports[0];
    start();
  });
  window.parent.postMessage("ready", "*");

  function start() {
    const plugin = import(src).then((m) => m.default || {});
    // Reads the plugin asked for, by ID, waiting for the page's answer
    const reads = new Map();
    let readID = 0;
    // ctx is what a hook is handed for one request: the file shown, and
    // reading the files next to it
    const ctx = (m) => ({
      path: m.path,
      read(rel) {
        return new Promise((resolve, reject) => {
          const id = ++readID;
          reads.set(id, { resolve, reject });
          port.postMessage({ read: id, for: m.id, path: rel });
        });
      },
    });
    port.onmessage = async (e) => {
      const m = e.data;
      if (m.readID !== undefined) {
        const r = reads.get(m.readID);
        reads.delete(m.readID);
        if (!r) return;
        if (m.error !== undefined) r.reject(new Error(m.error));
        else r.resolve(m.text);
        return;
      }
      try {
        const p = await plugin;
        let result;
        if (m.hook === "frontMatter") {
          if (typeof p.frontMatter !== "function") throw new Error("no frontMatter");
          result = await p.frontMatter(m.data, ctx(m));
        } else {
          const fn = p.elements?.[m.tag];
          if (typeof fn !== "function") throw new Error("no element " + m.tag);
          result = await fn(m.attrs, ctx(m));
        }
        port.postMessage({ id: m.id, result: normalize(result) });
      } catch (err) {
        port.postMessage({ id: m.id, error: String(err?.message || err) });
      }
    };
  }

  // normalize is what a hook gave back as the page takes it: null to show
  // nothing of the plugin's, else HTML and CSS as text.
  function normalize(r) {
    if (r === null || r === undefined) return null;
    if (typeof r === "string") return { html: r, css: "" };
    return { html: String(r.html ?? ""), css: String(r.css ?? "") };
  }
})();
