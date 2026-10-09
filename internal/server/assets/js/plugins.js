// Plugins show what a Markdown file leaves to them: its front matter, which
// the server also shows as a table, and components such as
// <Partial name="figure" />, which the server keeps as <mini-element>s,
// empty or holding their children. Each plugin runs in a sandbox of its own (see
// plugins.go), handed what it shows and the files it reads; what it gives
// back is sanitized by the server and shown in a shadow root, where the
// page's styles and the plugin's keep apart.

import { page, href, dirname } from "./util.js";

// TIMEOUT is how long a plugin has to start, and to show one thing.
const TIMEOUT = 10000;
// MAX_READ is the largest file a plugin may read.
const MAX_READ = 1 << 20;

// reads are the files plugins read for this page, which reload it when
// they change, as the file itself does.
export const reads = new Set();

export function initPlugins() {
  const plugins = JSON.parse(document.getElementById("mini.plugins")?.textContent || "[]");
  if (!plugins.length) return;
  const jobs = [];
  for (const article of document.querySelectorAll(".markdown-body")) {
    // A directory's page shows its README, which may be in another
    const file = article.dataset.file || page.path;
    for (const el of article.querySelectorAll(".mini-frontmatter[data-front-matter]")) {
      const p = plugins.find((p) => p.frontMatter);
      if (p) jobs.push({ p, el, file, msg: { hook: "frontMatter", data: JSON.parse(el.dataset.frontMatter) } });
    }
    for (const el of article.querySelectorAll("mini-element[data-tag]")) {
      const p = plugins.find((p) => p.elements.includes(el.dataset.tag));
      if (!p) continue;
      const msg = { hook: "element", tag: el.dataset.tag, attrs: JSON.parse(el.dataset.attrs || "{}"), block: el.hasAttribute("data-block") };
      if (el.dataset.children !== undefined) msg.children = JSON.parse(el.dataset.children);
      jobs.push({ p, el, file, msg });
    }
  }
  const sandboxes = new Map();
  for (const job of jobs) {
    if (!sandboxes.has(job.p.name)) sandboxes.set(job.p.name, new Sandbox(job.p));
    run(sandboxes.get(job.p.name), job);
  }
}

async function run(sandbox, { p, el, file, msg }) {
  try {
    const result = await sandbox.ask({ ...msg, path: file }, file);
    if (result) await show(el, p, result);
  } catch (e) {
    console.error(`gh-mini: plugin ${p.name}:`, e);
    // The front matter keeps its table
    if (msg.hook !== "element") return;
    const err = document.createElement("span");
    err.className = "mini-plugin-error";
    err.textContent = `plugin ${p.name}: ${e.message || e}`;
    // A component's children stay
    if (msg.children) el.prepend(err);
    else el.replaceChildren(err);
  }
}

// show puts what a plugin gave back in the place of el, in a shadow root
// on a box inside it: the box, kept in by el's paint containment, holds
// even a fixed position the plugin's styles give it. A component's
// children move into the box, where the plugin's <slot> shows them: still
// the page's, its styles theirs.
async function show(el, p, { html, css }) {
  const r = await fetch("/_mini/api/sanitize", { method: "POST", headers: { "Content-Type": "text/plain" }, body: html });
  if (!r.ok) throw new Error("could not sanitize: " + r.status);
  const box = document.createElement("div");
  box.append(...el.childNodes);
  const root = box.attachShadow({ mode: "open" });
  root.innerHTML = await r.text();
  if (css) {
    const style = document.createElement("style");
    style.textContent = css;
    root.prepend(style);
  }
  el.dataset.plugin = p.name;
  el.replaceChildren(box);
}

// Sandbox is a plugin's iframe, and the channel to it.
class Sandbox {
  constructor(p) {
    this.p = p;
    this.pending = new Map();
    this.nextID = 0;
    this.port = this.start();
  }

  // start loads the iframe, and hands it a channel when it tells it is
  // ready, once: what tells so later, or from elsewhere, gets nothing.
  start() {
    const iframe = document.createElement("iframe");
    iframe.setAttribute("sandbox", "allow-scripts");
    iframe.hidden = true;
    iframe.title = "Plugin " + this.p.name;
    iframe.src = this.p.host;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        window.removeEventListener("message", onMessage);
        reject(new Error("did not start"));
      }, TIMEOUT);
      const onMessage = (e) => {
        if (e.source !== iframe.contentWindow || e.data !== "ready") return;
        window.removeEventListener("message", onMessage);
        clearTimeout(timer);
        const channel = new MessageChannel();
        channel.port1.onmessage = (e) => this.receive(e.data);
        iframe.contentWindow.postMessage("port", "*", [channel.port2]);
        resolve(channel.port1);
      };
      window.addEventListener("message", onMessage);
      document.body.append(iframe);
    });
  }

  // ask sends the plugin one thing to show, from the file at path.
  async ask(msg, path) {
    const port = await this.port;
    const id = ++this.nextID;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error("took too long"));
      }, TIMEOUT);
      this.pending.set(id, { path, resolve, reject, timer });
      port.postMessage({ ...msg, id });
    });
  }

  async receive(m) {
    const port = await this.port;
    if (m.read !== undefined) {
      // A read is relative to the file of the request it is made for
      try {
        const asked = this.pending.get(m.for);
        if (!asked) throw new Error("read for no request");
        port.postMessage({ readID: m.read, text: await this.read(asked.path, m.path) });
      } catch (e) {
        port.postMessage({ readID: m.read, error: e.message || String(e) });
      }
      return;
    }
    const p = this.pending.get(m.id);
    if (!p) return;
    this.pending.delete(m.id);
    clearTimeout(p.timer);
    if (m.error !== undefined) p.reject(new Error(m.error));
    else p.resolve(m.result);
  }

  // read reads a file for the plugin, when the plugin's read patterns
  // name it: relative to the directory of the file shown, or with a /
  // before it, to the directory served. The server opens it under the
  // root, which a symlink out of it does not escape.
  async read(file, rel) {
    if (typeof rel !== "string") throw new Error("read takes a path");
    const fromRoot = rel.startsWith("/");
    const clean = normalize(fromRoot ? rel.slice(1) : rel);
    const allowed = this.p.read.filter((g) => g.startsWith("/") === fromRoot).map((g) => glob(fromRoot ? g.slice(1) : g));
    if (clean === null || !allowed.some((re) => re.test(clean))) {
      throw new Error(`not allowed to read ${rel}: plugin.json names the files a plugin reads`);
    }
    const dir = fromRoot ? "." : dirname(file);
    const full = dir === "." ? clean : dir + "/" + clean;
    reads.add(full);
    const r = await fetch(href(full) + "?raw", { cache: "no-store" });
    if (!r.ok) throw new Error(`could not read ${rel}: ${r.status}`);
    if (Number(r.headers.get("Content-Length")) > MAX_READ) throw new Error(`${rel} is too large`);
    const text = await r.text();
    if (text.length > MAX_READ) throw new Error(`${rel} is too large`);
    return text;
  }
}

// normalize is a relative path with . and .. resolved, or null when it
// leaves the directory it is relative to.
function normalize(rel) {
  if (rel.startsWith("/") || rel.includes("\\")) return null;
  const out = [];
  for (const part of rel.split("/")) {
    if (part === "" || part === ".") continue;
    if (part === "..") {
      if (!out.length) return null;
      out.pop();
    } else {
      out.push(part);
    }
  }
  return out.length ? out.join("/") : null;
}

// glob is a pattern of plugin.json as a regular expression, as Go's
// path.Match reads it: * and ? within a name, [...] a set of characters,
// [^...] one out of it. The server takes only patterns path.Match does.
function glob(g) {
  let re = "";
  for (let i = 0; i < g.length; i++) {
    const c = g[i];
    if (c === "*") re += "[^/]*";
    else if (c === "?") re += "[^/]";
    else if (c === "[") {
      const end = g.indexOf("]", i + 2);
      if (end < 0) return /(?!)/;
      re += "[" + g.slice(i + 1, end) + "]";
      i = end;
    } else re += c.replace(/[.+^${}()|\\]/g, "\\$&");
  }
  return new RegExp("^" + re + "$");
}
