// Front matter as a card: the title, the date, the description and the
// tags, as a blog shows them at the top of a post, and the other keys in a
// list below. It comes with gh-mini, off until turned on in the settings,
// and is an example of a plugin: see docs/guides/plugins.md.

const esc = (s) => String(s).replace(/[&<>"']/g, (c) =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

const text = (v) => (v !== null && typeof v === "object" ? JSON.stringify(v) : String(v ?? ""));

const SHOWN = ["title", "date", "description", "tags"];

export default {
  frontMatter(data) {
    if (data === null || typeof data !== "object" || Array.isArray(data)) return null;
    const { title, date, description, tags } = data;
    let html = '<div class="card">';
    if (title) html += `<div class="title">${esc(text(title))}</div>`;
    if (date) html += `<div class="date">${esc(text(date))}</div>`;
    if (description) html += `<p class="description">${esc(text(description))}</p>`;
    if (Array.isArray(tags) && tags.length) {
      html += '<div class="tags">' + tags.map((t) => `<span class="tag">${esc(text(t))}</span>`).join("") + "</div>";
    }
    const rest = Object.keys(data).filter((k) => !SHOWN.includes(k));
    if (rest.length) {
      html += "<dl>" + rest.map((k) => `<dt>${esc(k)}</dt><dd>${esc(text(data[k]))}</dd>`).join("") + "</dl>";
    }
    html += "</div>";
    return { html, css: CSS };
  },
};

const CSS = `
.card {
  border: 1px solid var(--borderColor-default);
  border-radius: 6px;
  padding: 16px;
  color: var(--fgColor-default);
  background: var(--bgColor-muted);
}
.title { font-size: 1.5em; font-weight: 600; line-height: 1.25; }
.date { color: var(--fgColor-muted); font-size: 0.875em; margin-top: 4px; }
.description { margin: 8px 0 0; }
.tags { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 12px; }
.tag {
  font-size: 0.75em;
  padding: 2px 10px;
  border-radius: 999px;
  color: var(--fgColor-accent);
  border: 1px solid var(--fgColor-accent);
}
dl {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 16px;
  margin: 12px 0 0;
  font-size: 0.875em;
}
dt { color: var(--fgColor-muted); }
dd { margin: 0; overflow-wrap: anywhere; }
`;
