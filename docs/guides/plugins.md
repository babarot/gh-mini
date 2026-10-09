# Plugins

A plugin shows a part of a Markdown file its own way: the front matter, or a component such as `<Partial name="figure" />`, which GitHub, and gh-mini without a plugin, leave out. It is for what gh-mini does not do for everyone, such as how one blog lays out its figures.

## Plugins and the settings

Plugins are directories in `~/.config/gh-mini/plugins/` (`$XDG_CONFIG_HOME/gh-mini/plugins/`, or the directory given to `--plugin-dir`). Each is listed under Plugins in the settings, on until you turn it off. A plugin that cannot run, such as one whose `plugin.json` does not parse, is shown off there with why.

gh-mini comes with `front-matter-card`, which shows front matter as a card: the title, date, description and tags, and the other keys below. It is off until you turn it on. A plugin of yours with the same name takes its place.

Plugins are read on every page load: one added or changed shows on the next.

## Writing one

A plugin is a directory of two files, named after the plugin in lower case:

```
~/.config/gh-mini/plugins/partial/
  plugin.json
  main.js
```

`plugin.json` tells what it shows, and which files it reads:

```json
{
  "description": "Show <Partial name=\"x\" /> as figures/x.part.html next to the file",
  "elements": ["Partial"],
  "read": ["figures/*.part.html", "/src/styles/figures/*.css"]
}
```

| Key | |
|---|---|
| `description` | Shown in the settings |
| `frontMatter` | `true` to show the front matter |
| `elements` | The components it shows, by name. A component is a tag alone, self-closing, whose name starts with a capital and is of letters, digits and hyphens, as MDX writes one: `<Partial name="x" />`, inline or on a line of its own. `Image` is a component too, but a name of HTML's in capitals alone, such as `BR` or `IMG`, is HTML, as old READMEs write it |
| `read` | Patterns of the files it may read, relative to the directory of the file shown, or, starting with `/`, to the directory served. `*` and `?` match within a name, `[...]` a character; `..` is not allowed |

`main.js` is an ES module whose default export has a function for each:

```js
export default {
  // data is the front matter, as JSON has it
  frontMatter(data, ctx) {
    return { html: `<h1 class="title">${escape(data.title)}</h1>`, css: ".title { color: var(--fgColor-accent); }" };
  },
  elements: {
    // attrs are the component's attributes, by name; one with no value is true
    async Partial({ name }, ctx) {
      const src = await ctx.read(`figures/${name}.part.html`);
      const css = [];
      const html = src.replace(/<style>([\s\S]*?)<\/style>/g, (_, c) => (css.push(c), ""));
      return { html, css: css.join("\n") };
    },
  },
};
```

[examples/plugins/partial](../../examples/plugins/partial) is this plugin, and [front-matter-card](../../internal/server/assets/plugins/front-matter-card) the one that comes with gh-mini.

A function gives back HTML and CSS, as an object, or HTML alone as a string, or `null` to show what gh-mini shows without it: the front matter's table, or nothing for a component. It may be async. What it throws is shown in the component's place, and logged in the console.

`ctx.path` is the file shown, from the directory served, and `ctx.read(path)` reads a file that `read` names, as text. A path is relative to the directory of the file shown, or, starting with `/`, to the directory served. Files larger than 1 MB are not read.

The HTML is kept as far as HTML in Markdown is (see [Markdown](markdown.md)), and classes too: scripts, styles, event handlers, forms, frames and SVG go. The CSS is the plugin's own, and applies to its HTML alone; the page's styles do not apply to it either. The theme's variables, such as `--fgColor-default` and `--bgColor-muted` (see [Themes](themes.md)), and the fonts, reach it, and `:host-context([data-mode="dark"])` matches in the dark mode.

## What a plugin can do

A plugin runs in a sandbox of its own, apart from the page. It is given what it shows and the files its `read` names, and nothing else: it cannot reach the page, gh-mini's other files or its API, and loads nothing from the network. What it gives back is shown in a box of its own, with its scripts and the like taken out, so a file it shows, even from someone else's repository, runs nothing and cannot cover the page.

A plugin can still send what it is given, the files it reads among them, elsewhere: through an image in what it shows, for one. Use plugins you trust, as you would any program you run. gh-mini reads plugins only from the plugins directory and from itself, never from the directory served.
