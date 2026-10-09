# Plugins

A plugin shows a part of a Markdown file its own way: the front matter, a component such as `<Partial name="figure" />` or `<Callout>`, which GitHub, and gh-mini without a plugin, leave out, showing what a component holds as if it were not there, or code blocks of a language, such as ` ```csv ` as a table; and it may bring themes. It is for what gh-mini does not do for everyone, such as how one blog lays out its figures.

## Plugins and the settings

Plugins are directories in `~/.config/gh-mini/plugins/` (`$XDG_CONFIG_HOME/gh-mini/plugins/`, or the directory given to `--plugin-dir`). Each is listed under Plugins in the settings, on until you turn it off. A plugin that cannot run, such as one whose `plugin.json` does not parse, is shown off there with why.

gh-mini comes with `front-matter-card`, which shows front matter as a card: the title, date, description and tags, and the other keys below. It is off until you turn it on. A plugin of yours with the same name takes its place.

Plugins are read on every page load, and a page of Markdown, or of a directory, loads again when a file of yours changes: a plugin added or edited shows at once.

## Writing one

A plugin is a directory named after the plugin in lower case, of `plugin.json`, and `main.js` unless it brings themes alone:

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
| `elements` | The components it shows, by name (see [Components](#components)) |
| `codeBlocks` | The languages of the code blocks it shows, in lower case, as `["csv"]`. The fence's language matches in any case. `mermaid` and `math` are gh-mini's |
| `themes` | Themes, by name, each a CSS file of the plugin's, as `{"paper": "themes/paper.css"}`, written as a theme file is (see [Themes](themes.md)). A plugin of themes alone needs no `main.js` |
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
  codeBlocks: {
    // code is the block's text; a language such as c++ is quoted, as "c++"
    csv(code, ctx) {
      const rows = code.trim().split("\n").map((line) => `<tr>${line.split(",").map((c) => `<td>${escape(c)}</td>`).join("")}</tr>`);
      return `<table>${rows.join("")}</table>`;
    },
  },
};
```

`main.js` may import other modules of the plugin's directory, as `import { draw } from "./lib/draw.js"`, and fetch its other files, as `fetch("./data.json")`, both relative to the directory; WebAssembly compiles, so a renderer built to it can draw a diagram. Files whose names start with `.`, and those of a symlink out of the directory, are not served. Workers are not available.

[examples/plugins/partial](../../examples/plugins/partial) is this plugin, and [front-matter-card](../../internal/server/assets/plugins/front-matter-card) the one that comes with gh-mini.

A function gives back HTML and CSS, as an object, or HTML alone as a string, or `null` to show what gh-mini shows without it: the front matter's table, for a component what it holds and nothing else, and the code for a code block. It may be async. What it throws is shown in the component's or the code's place, before what it holds, and logged in the console.

A function is called again for the same thing when the mode or the theme changes, to show it in them, and may be at other times; write it to give back what it is given, and to keep nothing from one call to the next. Called again, what it gives back takes the place of what it showed; `null`, or what it throws, leaves that.

`ctx.path` is the file shown, from the directory served. `ctx.mode` is `"light"` or `"dark"`, as the page shows, and `ctx.theme` the theme's name. For a code block, `ctx.lang` is its language as the fence writes it, and `ctx.meta` the rest of the fence's line, as `title="x"` of ` ```csv title="x" `. For a component, `ctx.block` tells it is on a line of its own, and `ctx.children` lists the components it holds, as `[{ tag, attrs }]`, without those in them: `[]` when it holds none, and undefined for a self-closing one. `ctx.read(path)` reads a file that `read` names, as text. A path is relative to the directory of the file shown, or, starting with `/`, to the directory served. Files larger than 1 MB are not read.

The HTML is kept as far as HTML in Markdown is (see [Markdown](markdown.md)), and classes, `style` attributes, roles and `aria-` attributes, `<slot>`, and SVG that draws: shapes, paths, text, markers, gradients, patterns, clip paths, masks and the common filters. Scripts, `<style>` elements, event handlers, forms and frames go, and in SVG `<foreignObject>`, the animations, `<use>` and `<image>`. A `style` or SVG attribute that would load something goes too: `url()` may name an element of the plugin's own, as `fill="url(#gradient)"` does, and nothing else. To be sure of that, such a value may use these functions alone, none in another: `rgb()`, `rgba()`, `hsl()`, `hsla()`, `var()`, `calc()`, `translate()`, `rotate()`, `scale()`, `matrix()`, `skewX()`, `skewY()` and `url(#id)`. Anything else, a gradient or `clamp()`, goes in the plugin's CSS, which is kept as it is. Ids are kept as written, and are the plugin's own, as is the rest of what it shows. The CSS is the plugin's own, and applies to its HTML alone; the page's styles do not apply to it either. The theme's variables, such as `--fgColor-default` and `--bgColor-muted` (see [Themes](themes.md)), and the fonts, reach it, and `:host-context([data-mode="dark"])` matches in the dark mode.

## Components

A component is a tag whose name starts with a capital and is of letters, digits and hyphens, as MDX writes one. `Image` is a component too, but a name of HTML's in capitals alone, such as `BR` or `IMG`, is HTML, as old READMEs write it.

It is self-closing, as `<Partial name="x" />`, inline or on a line of its own, or holds Markdown between an open tag and a close tag:

```mdx
<Callout type="warn">

Some **Markdown**.

</Callout>
```

Inline, it holds what is between the tags, as `<Kbd>Ctrl</Kbd>`. On lines of their own, the tags are best with a blank line after the open one and before the close one, as above: Markdown takes the lines right after a tag as HTML, up to a blank line, so they show as text; a close tag right after a paragraph, with no blank line before it, closes it too. A close tag in the middle of a paragraph, or the two tags in different lists or quotes, are not paired: the tags are dropped, and what is between them shows plainly.

What a component holds stays the page's, and so does a code block's code: gh-mini renders it, the page's styles apply to it, and its headings are in the outline. A plugin places it with `<slot></slot>` in its HTML, as a diagram's source in `<details><summary>Source</summary><slot></slot></details>`; without a slot, it does not show. The plugin's CSS reaches it with `::slotted(...)`, for the elements at its top, and gives way to the page's styles but with `!important`:

```js
Callout({ type }) {
  return {
    html: `<div class="callout ${type}"><slot></slot></div>`,
    css: ".callout { border-left: 4px solid var(--borderColor-attention-emphasis); padding: 0 1em; } ::slotted(:last-child) { margin-bottom: 0 !important; }",
  };
},
```

A plugin that fails shows why before what the component holds.

## What a plugin can do

A plugin runs in a sandbox of its own, apart from the page. It is given what it shows and the files its `read` names, and nothing else: it cannot reach the page, gh-mini's other files or its API, and loads nothing from the network but its own files. What it gives back is shown in a box of its own, with its scripts and the like taken out, so a file it shows, even from someone else's repository, runs nothing and cannot cover the page.

A plugin's theme is CSS for the whole page, as a theme file is, outside the sandbox. A plugin can still send what it is given, the files it reads among them, elsewhere: through an image in what it shows, for one. Use plugins you trust, as you would any program you run. gh-mini reads plugins only from the plugins directory and from itself, never from the directory served.
