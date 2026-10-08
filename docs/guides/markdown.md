# Markdown

gh-mini renders Markdown as GitHub renders it, with [goldmark](https://github.com/yuin/goldmark) and the stylesheet of [github-markdown-css](https://github.com/sindresorhus/github-markdown-css): alerts, task lists, footnotes, Mermaid, math, emoji and heading anchors.

## HTML in Markdown

HTML in Markdown is kept as far as GitHub keeps it. `<details>`, `align`, image sizes and the like stay; scripts, styles, frames and forms go. The pages run scripts from gh-mini's own files only.

## Preview and code

Markdown and HTML files have a Preview / Code switch. Code shows the source highlighted, with line numbers you can link to, such as `#L10`. Other source files are shown that way too, but for those over 512 KB, shown as plain text.
