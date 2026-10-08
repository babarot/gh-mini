# AGENTS.md

gh-mini serves a directory as a small, local GitHub. The README and the [guides](docs/guides) tell what it does and how to use it; the documents below tell how it is meant to be built. Read the one for what you are about to change first.

- [Flags and settings](docs/concepts/flags-and-settings.md): read before adding or changing a command line flag or a setting in the page's dialog. It tells which of the two an option is, and how they meet.

A change users can see goes in the guide for it, and in the README only when it changes what gh-mini is.

## Development

`make dev` serves `internal/markdown/testdata` on port 7419 with air, rebuilding on changes; set `DIR` and `PORT` to serve another.

`internal/server/assets/markdown.css` is generated from the vendored github-markdown-css light and dark files, turning their colors into the variables in [Themes](docs/guides/themes.md). A color without a name in [hack/gencss](hack/gencss/main.go) stops it:

```sh
go run ./hack/gencss internal/server/assets/vendor/github-markdown-light.css \
  internal/server/assets/vendor/github-markdown-dark.css internal/server/assets/markdown.css
```

Settings are listed in [settings.go](internal/server/settings.go); adding one there adds it to the dialog, and the comment at the top of the file tells how its value reaches the page. A new setting goes in the table in [Settings](docs/guides/settings.md) too.
