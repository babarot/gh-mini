# Themes

The built-in themes are `github` (the default), `nord` and `tokyo-night`; each has a light and a dark version. Pick one in the settings, or give the default with `--theme` (or `$GH_MINI_THEME`). Mermaid diagrams are drawn in the theme's colors, but under `github`, which draws them as GitHub does.

## Writing a theme

Your own theme is a CSS file in `~/.config/gh-mini/themes/` (`$XDG_CONFIG_HOME/gh-mini/themes/`, or the directory given to `--theme-dir`). Its file name is the theme's name in the settings, and a file named after a built-in theme replaces it. It is read on every page load, and saving it updates open pages.

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

See [examples/themes/sepia.css](../../examples/themes/sepia.css) and the built-in themes in [internal/server/assets/themes](../../internal/server/assets/themes).

## Variables

The variables come in three groups. A theme mostly sets the first; the other two follow it unless set too.

| Group | Variables |
|---|---|
| Base | `--bgColor-default`, `--bgColor-muted`, `--bgColor-neutral-muted` (inline code and the selected file), `--fgColor-default`, `--fgColor-muted`, `--fgColor-accent`, `--borderColor-default`, `--borderColor-muted`, `--focus-outlineColor` (the ring around a focused or linked-to element), and `--fgColor-success`, `--fgColor-attention`, `--fgColor-danger`, `--fgColor-done` for alerts and states |
| Derived | `--borderColor-success-emphasis`, `--borderColor-attention-emphasis`, `--borderColor-danger-emphasis`, `--borderColor-done-emphasis` (alert borders, default: the `--fgColor-*` of the same name) and `--bgColor-attention-muted` (`<mark>` and linked-to lines, default: `--fgColor-attention` at 20%) |
| Syntax | `--syntax-comment`, `--syntax-keyword`, `--syntax-string`, `--syntax-number`, `--syntax-function`, `--syntax-type`, `--syntax-constant`, `--syntax-tag`, `--syntax-operator`, `--syntax-variable`, `--syntax-inserted`, `--syntax-deleted`, and `--syntax-inserted-bg`, `--syntax-deleted-bg` for the lines' backgrounds (default: GitHub's colors) |

The layout variables (`--mini-sidebar-width`, `--mini-content-width`, ...) and `--mini-folder-color` (folder icons in the tree and the file list, default: `--fgColor-accent`) are at the top of [app.css](../../internal/server/assets/app.css).
