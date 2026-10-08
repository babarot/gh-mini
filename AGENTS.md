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

Mermaid and MathJax are vendored from npm, at the versions in [hack/vendorjs](hack/vendorjs/main.go), which checks each package's integrity. To update one, change its version and integrity there and run:

```sh
go run ./hack/vendorjs internal/server/assets/vendor
```

The tests in [e2e](e2e) drive the pages in a headless Chrome, for what the browser does with them: live reload, the tree, the keyboard. They run with `go test ./...` when Chrome is found and are skipped otherwise, or with `-short`. Wait for what a test expects, with `waitFor`, rather than for a time: changes to files reach a page a second or so later. For the same reason, make the files a test starts from before the server starts, as `newApp` and `newRepoApp` do: a change made after reaches a page opened meanwhile late, and reloads it under what the test does to it, though what the test waits for may already show.

A browser test that fails now and then is fixed, not run again until it passes: Test runs it once for a pull request, and passing it once does not tell it passes on main. Flaky runs the browser tests a few times over, each night and for a pull request that changes them or the page's assets.

Settings are listed in [settings.go](internal/server/settings.go); adding one there adds it to the dialog, and the comment at the top of the file tells how its value reaches the page. A new setting goes in the table in [Settings](docs/guides/settings.md) too.
