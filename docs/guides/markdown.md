# Markdown

gh-mini renders Markdown as GitHub renders it, with [goldmark](https://github.com/yuin/goldmark) and the stylesheet of [github-markdown-css](https://github.com/sindresorhus/github-markdown-css): alerts, task lists, footnotes, Mermaid, math, emoji and heading anchors.

## HTML in Markdown

HTML in Markdown is kept as far as GitHub keeps it. `<details>`, `align`, image sizes and the like stay; scripts, styles, frames and forms go. Ids and the names of anchors stay with `user-content-` before them, as on GitHub, and a link to `#top` finds `<a name="top">`. `<textarea>`, `<title>` and the like show as text. The pages run scripts from gh-mini's own files only.

A component, a tag whose name starts with a capital as MDX writes one, such as `<Partial name="figure" />` or `<Callout>`…`</Callout>`, shows nothing but what it holds, as on GitHub, unless a [plugin](plugins.md) shows it. A tag of HTML's written so, such as `<Details>`, is a component too, where GitHub would take it as HTML; in capitals alone, as `<DETAILS>`, it is HTML. Front matter shows as a table, unless a plugin shows it otherwise.

## Preview and code

Markdown and HTML files have a Preview / Code switch. Code shows the source highlighted, with line numbers you can link to, such as `#L10`, or `#L10-L20` for the lines picked with a Shift-click. Other source files are shown that way too, but for those over 512 KB, shown as plain text.

A page prints the file alone, without the tree or anything else around it.
