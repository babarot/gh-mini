<p align="center">
  <img src="internal/server/assets/favicon.svg" alt="gh-mini" width="160">
</p>

# gh-mini

A small, local GitHub for a directory of Markdown.

```sh
gh-mini            # serve the current directory
gh-mini docs/      # serve another directory
gh-mini notes.md   # serve the current directory and open notes.md
```

It opens `http://localhost:6419/` with:

- a file tree with a file finder (press `t`), and directory pages that show their README, as GitHub does
- Markdown rendered as GitHub renders it: alerts, task lists, footnotes, Mermaid, math, emoji
- source files highlighted, with line numbers you can link to
- HTML files previewed as a browser shows them, scripts included
- translations side by side, such as `guide.md` and `guide.ja.md`
- themes, built in or written as CSS, each in light and dark
- live reload when files change

## Install

As a [gh](https://cli.github.com/) extension, run as `gh mini`:

```sh
gh extension install babarot/gh-mini
```

Or with Go, run as `gh-mini`:

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
| `--theme-dir` | `~/.config/gh-mini/themes` | Directory of themes |
| `--preview-port` | a free one | Port HTML previews are served on, printed at start |
| `--skip` | `.git,node_modules,.DS_Store` | Names left out of the tree |
| `--translations` | `$GH_MINI_TRANSLATIONS`, else `suffix` | How translations are named |
| `--version` | | Print the version |

How you like to see the pages, such as the theme, is picked in the settings in the page (press `,`).

## HTML previews

An HTML file's preview runs its scripts. They run in an origin of their own and cannot reach gh-mini's pages, but, as with `python -m http.server`, they can read the files gh-mini serves and send them anywhere. Preview only files you trust.

## Guides

- [Markdown](docs/guides/markdown.md): what is rendered as GitHub renders it, and which HTML is kept
- [Files and live reload](docs/guides/files.md): what the tree shows, and what is watched
- [Settings](docs/guides/settings.md): the settings and keyboard shortcuts, and how they meet the flags
- [HTML previews](docs/guides/html-previews.md): how previews are served, and what their scripts can do
- [Translations](docs/guides/translations.md): how translations are named, and how to name yours otherwise
- [Themes](docs/guides/themes.md): writing a theme of your own

## License

MIT
