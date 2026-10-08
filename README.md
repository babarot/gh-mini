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
- Markdown rendered as GitHub renders it: alerts, task lists, footnotes, Mermaid, math, emoji, heading anchors. HTML in Markdown is kept as far as GitHub keeps it: `<details>`, `align`, image sizes and the like stay; scripts, styles, frames and forms go, and the pages run scripts from gh-mini's own files only
- source files highlighted with line numbers you can link to (`#L10`), and a Preview / Code switch for Markdown and HTML
- HTML files previewed as a browser shows them, scripts included (see [HTML previews](#html-previews))
- every file on disk, including what git ignores; those are marked `local`
- translations side by side: `guide.md` and `guide.ja.md` get a language switch, and a language picked on a README stays picked. Translations can be named otherwise, such as `ja/guide.md`, or left alone (see [Translations](#translations))
- a menu behind the gear button, with the keyboard shortcuts (or press `?`), an About dialog that tells the version and the directory served, and the settings (or press `,`): built-in `github`, `nord` and `tokyo-night` themes or ones you write as CSS, a light, dark or automatic mode, a full-width page, wrapped lines in source files, whether HTML files open as a preview, whether Markdown files get a language switch, whether directories git ignores are hidden, and whether to show commit authors' pictures from GitHub, which asks GitHub for them by email. They are kept per browser. A setting for something the command line turned off, such as HTML preview when no port could be had for it, is shown off with why
- live reload of the page you are reading when its file changes, and of the tree when files are added or removed. Directories git ignores, such as a `.venv`, show in the tree without their files until you open them. On macOS the whole directory is watched through FSEvents; elsewhere directories git ignores are watched only while you look at a page in them. A new symlink shows in the tree only when its directory changes otherwise too, if it points to another directory

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
| `--theme-dir` | `~/.config/gh-mini/themes` | Directory of themes |
| `--preview-port` | a free one | Port HTML previews are served on, printed at start |
| `--skip` | `.git,node_modules,.DS_Store` | Names left out of the tree |
| `--translations` | `$GH_MINI_TRANSLATIONS`, else `suffix` | How translations are named (see [Translations](#translations)) |

Flags set up what a gh-mini serves and what it can do. The settings in the page are how you like to see it, within that, and are kept per browser for every gh-mini. A flag that gives a setting's default, as `--theme` does, gives way to what you pick in the settings.

## HTML previews

An HTML file's Preview shows it as a browser does, with its styles, images and scripts, module scripts and `fetch` included. Turn on HTML preview in the settings to open HTML files that way; the Preview / Code switch works either way.

The preview comes from a second server on its own port, so the file's scripts run in an origin of their own and cannot reach gh-mini's pages. That server answers only the gh-mini pages that show a preview, through a cookie a page on another site cannot send. A path in the file starting with `/` resolves against the directory gh-mini serves.

What a previewed file's scripts can still do, as with `python -m http.server`:

- read the files under the directory gh-mini serves, and send them anywhere
- read and write the cookies of the host, those of other servers on `localhost` included, but not HttpOnly ones

So preview only files you trust. Through an SSH tunnel, forward the preview port too; set it with `--preview-port`.

Opening an HTML or SVG file raw does not run its scripts.

## Translations

A Markdown file and its translations get a language switch, and a directory's README is shown in the language picked last. How a translation is named is a layout:

| Layout | Translations of `docs/guide.md` |
|---|---|
| `suffix` (the default) | `docs/guide.ja.md`, `docs/guide.zh-TW.md` |
| `dir` | `docs/ja/guide.md`, `docs/zh-TW/guide.md` |

`--translations` picks the layout, as the way a repository names its translations is its own, and takes templates too: the path of a translation from its original's directory, with `{name}` for the original's name and `{lang}` for the language, such as `{name}_{lang}` for `guide_ja.md` or `i18n/{lang}/{name}` for `docs/i18n/ja/guide.md`. Separate layouts with commas to use several; the first that fits a file is taken. `--translations off` leaves translations alone, as files of their own. Turning off Language switch in the settings does the same for you alone, in every gh-mini.

A language is a two-letter code, such as `ja`, or one with a region, such as `zh-TW`. With `dir`, a directory named after a language, such as `id` or `it`, holds translations too, so pick `dir` only for a repository laid out that way.

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

The layout variables (`--mini-sidebar-width`, `--mini-content-width`, ...) and `--mini-folder-color` (folder icons in the tree and the file list, default: `--fgColor-accent`) are at the top of [app.css](internal/server/assets/app.css). See [examples/themes/sepia.css](examples/themes/sepia.css) and the built-in themes in [internal/server/assets/themes](internal/server/assets/themes).

## Development

`internal/server/assets/markdown.css` is generated from the vendored github-markdown-css light and dark files, turning their colors into the variables above. A color without a name in [hack/gencss](hack/gencss/main.go) stops it:

```sh
go run ./hack/gencss internal/server/assets/vendor/github-markdown-light.css \
  internal/server/assets/vendor/github-markdown-dark.css internal/server/assets/markdown.css
```

Settings are listed in [settings.go](internal/server/settings.go); adding one there adds it to the dialog, and the comment at the top of the file tells how its value reaches the page. Whether a new option is a flag or a setting is told in [Flags and settings](docs/concepts/flags-and-settings.md).

## License

MIT
