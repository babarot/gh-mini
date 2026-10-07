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
- source files highlighted with line numbers you can link to (`#L10`), and a Preview / Code switch for Markdown
- every file on disk, including what git ignores; those are marked `local`
- translations side by side: `guide.md` and `guide.ja.md` get a language switch, and a language picked on a README stays picked
- settings behind the gear button (or press `,`): themes you write as CSS, and a light, dark or automatic mode. They are kept per browser
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
| `--skip` | `.git,node_modules,.DS_Store` | Names left out of the tree |

## Themes

A theme is a CSS file in `~/.config/gh-mini/themes/` (`$XDG_CONFIG_HOME/gh-mini/themes/`). Its file name is the theme's name in the settings. It is read on every page load, and saving it updates open pages.

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

The main variables are `--bgColor-default`, `--bgColor-muted`, `--fgColor-default`, `--fgColor-muted`, `--fgColor-accent`, `--focus-outlineColor` (the ring around a focused or linked-to element) and `--borderColor-default`; the full list is at the top of [markdown.css](internal/server/assets/markdown.css), and the layout ones (`--mini-sidebar-width`, `--mini-content-width`, ...) at the top of [app.css](internal/server/assets/app.css). See [examples/themes/sepia.css](examples/themes/sepia.css).

## Development

`internal/server/assets/markdown.css` is generated from the vendored github-markdown-css light and dark files, turning their colors into variables:

```sh
go run ./hack/gencss internal/server/assets/vendor/github-markdown-light.css \
  internal/server/assets/vendor/github-markdown-dark.css internal/server/assets/markdown.css
```

Settings are listed in [settings.go](internal/server/settings.go); adding one there adds it to the dialog, and the comment at the top of the file tells how its value reaches the page.

## License

MIT
