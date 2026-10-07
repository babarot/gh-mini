# gh-mini

A small, local GitHub for a directory of Markdown.

```sh
gh-mini            # serve the current directory
gh-mini docs/      # serve another directory
gh-mini notes.md   # serve the current directory and open notes.md
```

It opens `http://localhost:6419/` with:

- a file tree in the sidebar, with a file finder (press `t`) and a "Markdown only" filter
- directory pages that list their files and show their README, as GitHub does
- Markdown rendered as GitHub renders it: alerts, task lists, footnotes, Mermaid, math, emoji, heading anchors, `#123` links to the repository's issues
- source files highlighted with line numbers you can link to (`#L10`), and a Preview / Code switch for Markdown and HTML
- HTML files previewed as a browser shows them, scripts included (see [HTML previews](#html-previews))
- every file on disk, including what git ignores; those are marked `local`
- translations side by side: `guide.md` and `guide.ja.md` get a language switch, and a language picked on a README stays picked
- settings behind the gear button (or press `,`): built-in `github`, `nord` and `tokyo-night` themes or ones you write as CSS, a light, dark or automatic mode, and whether HTML files open as a preview. They are kept per browser
- live reload of the page you are reading when its file changes. Directories git ignores are watched only while you look at a page in them, so files added there show in the tree later

Rendering uses [goldmark](https://github.com/yuin/goldmark) and the stylesheet of [github-markdown-css](https://github.com/sindresorhus/github-markdown-css).

## Install

```sh
go install github.com/babarot/gh-mini@latest
```

## Flags

| Flag | Default | |
|---|---|---|
| `-p`, `--port` | `6419` | Port; the next free one is used when it is taken |
| `--host` | `localhost` | Address to listen on |
| `--no-open` | | Do not open the browser |
| `--no-reload` | | Do not reload pages when files change |
| `--theme` | `$GH_MINI_THEME` | Theme until one is picked in the settings |
| `--themes` | `~/.config/gh-mini/themes` | Directory of themes |
| `--preview-port` | a free one | Port HTML previews are served on, printed at start |
| `--skip` | `.git,node_modules,.DS_Store` | Names left out of the tree |

## HTML previews

An HTML file's Preview shows it as a browser does, with its styles, images and scripts, module scripts and `fetch` included. Turn on HTML preview in the settings to open HTML files that way; the Preview / Code switch works either way.

The preview comes from a second server on its own port, so the file's scripts run in an origin of their own and cannot reach gh-mini's pages. That server answers only the gh-mini pages that show a preview, through a cookie a page on another site cannot send. A path in the file starting with `/` resolves against the directory gh-mini serves.

What a previewed file's scripts can still do, as with `python -m http.server`:

- read the files under the directory gh-mini serves, and send them anywhere
- read and write the cookies of the host, those of other servers on `localhost` included, but not HttpOnly ones

So preview only files you trust. Through an SSH tunnel, forward the preview port too; set it with `--preview-port`.

Opening an HTML or SVG file raw does not run its scripts.

## Themes

The built-in themes are `github` (the default), `nord` and `tokyo-night`; each has a light and a dark version.

Your own theme is a CSS file in `~/.config/gh-mini/themes/` (`$XDG_CONFIG_HOME/gh-mini/themes/`). Its file name is the theme's name in the settings, and a file named after a built-in theme replaces it. It is read on every page load, and saving it updates open pages.

Colors are CSS variables, so a theme usually only sets them. Set them under `:root[data-mode="light"]` and `:root[data-mode="dark"]`, or `:root[data-mode]` for both:

```css
:root[data-mode="dark"] {
  --bgColor-default: #1f1b16;
  --fgColor-default: #e8dcc8;
  --fgColor-accent: #e0a46f;
}

.markdown-body {
  font-family: Georgia, serif;
}
```

The variables come in three groups. A theme mostly sets the first; the other two follow it unless set too.

| Group | Variables |
|---|---|
| Base | `--bgColor-default`, `--bgColor-muted`, `--bgColor-neutral-muted` (inline code and the selected file), `--fgColor-default`, `--fgColor-muted`, `--fgColor-accent`, `--borderColor-default`, `--borderColor-muted`, `--focus-outlineColor` (the ring around a focused or linked-to element), and `--fgColor-success`, `--fgColor-attention`, `--fgColor-danger`, `--fgColor-done` for alerts and states |
| Derived | `--borderColor-success-emphasis`, `--borderColor-attention-emphasis`, `--borderColor-danger-emphasis`, `--borderColor-done-emphasis` (alert borders, default: the `--fgColor-*` of the same name) and `--bgColor-attention-muted` (`<mark>` and linked-to lines, default: `--fgColor-attention` at 20%) |
| Syntax | `--syntax-comment`, `--syntax-keyword`, `--syntax-string`, `--syntax-number`, `--syntax-function`, `--syntax-type`, `--syntax-constant`, `--syntax-tag`, `--syntax-operator`, `--syntax-variable`, `--syntax-inserted`, `--syntax-deleted`, and `--syntax-inserted-bg`, `--syntax-deleted-bg` for the lines' backgrounds (default: GitHub's colors) |

The layout variables (`--mini-sidebar-width`, `--mini-content-width`, ...) are at the top of [app.css](internal/server/assets/app.css). See [examples/themes/sepia.css](examples/themes/sepia.css) and the built-in themes in [internal/server/assets/themes](internal/server/assets/themes).

## Development

`internal/server/assets/markdown.css` is generated from the vendored github-markdown-css light and dark files, turning their colors into the variables above. A color without a name in [hack/gencss](hack/gencss/main.go) stops it:

```sh
go run ./hack/gencss internal/server/assets/vendor/github-markdown-light.css \
  internal/server/assets/vendor/github-markdown-dark.css internal/server/assets/markdown.css
```

Settings are listed in [settings.go](internal/server/settings.go); adding one there adds it to the dialog, and the comment at the top of the file tells how its value reaches the page.

## License

MIT
