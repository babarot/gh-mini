package e2e

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

var treeFiles = map[string]string{
	"README.md":       "# Readme\n",
	"a.md":            "# A\n",
	"docs/guide.md":   "# Guide\n",
	"docs/deep/x.md":  "# X\n",
	"src/main.go":     "package main\n",
	"src/lib/util.go": "package lib\n",
}

// openDirs lists the directories open in the tree.
const openDirs = `[...document.querySelectorAll(".tree li.open > .row")].map((r) => r.dataset.path).join(",")`

// row is the tree's row for a path.
func row(path string) string {
	return `.tree .row[data-path=` + q(path) + `]`
}

// Directories opened in the tree, by the chevron or by the name, stay open
// from page to page; the way to a page reached by a link does not.
func TestTreeKeepsOpen(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))

	run(t, ctx, chromedp.Click(row("docs")+" .label", chromedp.ByQuery))
	waitFor(t, ctx, `location.pathname === "/docs/" && document.querySelector(".tree .row") !== null`)
	run(t, ctx, chromedp.Click(row("src")+" .chevron", chromedp.ByQuery))
	waitFor(t, ctx, openDirs+` === "docs,src"`)
	run(t, ctx, chromedp.Click(row("src/main.go"), chromedp.ByQuery))
	waitFor(t, ctx, `location.pathname === "/src/main.go" && document.querySelector(".tree .row") !== null`)
	if got := eval[string](t, ctx, openDirs); got != "docs,src" {
		t.Errorf("after opening a file: open %q, want docs,src", got)
	}

	// A link, as in a page or the finder, opens the way to its page there
	open(t, ctx, a.URL("/docs/deep/x.md"))
	if got := eval[string](t, ctx, openDirs); got != "docs,docs/deep,src" {
		t.Errorf("on docs/deep/x.md: open %q", got)
	}
	open(t, ctx, a.URL("/a.md"))
	if got := eval[string](t, ctx, openDirs); got != "docs,src" {
		t.Errorf("back on a.md: open %q, want docs,src", got)
	}

	run(t, ctx, chromedp.Click(`[id="mini.tree-options-open"]`, chromedp.ByQuery))
	run(t, ctx, chromedp.Click(`[id="mini.collapse-all"]`, chromedp.ByQuery))
	waitFor(t, ctx, openDirs+` === ""`)
	open(t, ctx, a.URL("/README.md"))
	if got := eval[string](t, ctx, openDirs); got != "" {
		t.Errorf("after Collapse all: open %q", got)
	}
}

// The arrow keys move through the tree and open and fold directories
// without leaving the page, which the tree tells.
func TestTreeKeyboard(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	if got := eval[string](t, ctx, `document.querySelector('.tree [aria-current="page"]')?.dataset.path`); got != "a.md" {
		t.Errorf("current row %q", got)
	}
	focused := `document.activeElement.dataset.path`
	run(t, ctx, chromedp.Focus(row("docs"), chromedp.ByQuery))
	for _, step := range []struct{ key, focus, open string }{
		{kb.ArrowRight, "docs", "docs"},
		{kb.ArrowRight, "docs/deep", "docs"},
		{kb.ArrowDown, "docs/guide.md", "docs"},
		{kb.ArrowUp, "docs/deep", "docs"},
		{kb.ArrowLeft, "docs", "docs"},
		{kb.ArrowLeft, "docs", ""},
		{kb.ArrowDown, "src", ""},
		{kb.End, "README.md", ""},
		{kb.Home, "docs", ""},
	} {
		run(t, ctx, chromedp.KeyEvent(step.key))
		waitFor(t, ctx, focused+` === `+q(step.focus)+` && `+openDirs+` === `+q(step.open))
	}
	if got := eval[string](t, ctx, `document.querySelector(`+q(row("docs"))+`).getAttribute("aria-expanded")`); got != "false" {
		t.Errorf("docs aria-expanded %q", got)
	}
	if got := eval[string](t, ctx, `location.pathname`); got != "/a.md" {
		t.Errorf("the keys left the page for %s", got)
	}

	// Drawn again for a file added, the tree keeps the focus on its row
	run(t, ctx, chromedp.Focus(row("src"), chromedp.ByQuery))
	a.write("b.md", "# B\n")
	waitFor(t, ctx, `document.querySelector(`+q(row("b.md"))+`) !== null`)
	if got := eval[string](t, ctx, focused); got != "src" {
		t.Errorf("focus after the tree was drawn again: %q", got)
	}
}

// The finder takes the letters typed in their order, and Enter opens the
// first file found.
func TestFinder(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	run(t, ctx, chromedp.KeyEvent("t"))
	waitFor(t, ctx, `document.activeElement.id === "mini.tree-filter"`)
	// KeyEvent sends the character apart from the key, so the page's
	// preventDefault on t leaves it typed, as it does not in a browser
	eval[any](t, ctx, `document.getElementById("mini.tree-filter").value = ""`)
	run(t, ctx, chromedp.SendKeys(`[id="mini.tree-filter"]`, "rdme", chromedp.ByQuery))
	waitFor(t, ctx, `document.querySelector(".tree .row.selected")?.dataset.path === "README.md"`)
	run(t, ctx, chromedp.KeyEvent(kb.Enter))
	waitFor(t, ctx, `location.pathname === "/README.md"`)
}

// Expand all opens every directory, and they stay open; a tree too large
// to draw whole opens only as deep as it fits.
func TestTreeExpandAll(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/README.md"))
	expand := func() {
		run(t, ctx, chromedp.Click(`[id="mini.tree-options-open"]`, chromedp.ByQuery))
		run(t, ctx, chromedp.Click(`[id="mini.expand-all"]`, chromedp.ByQuery))
	}
	expand()
	waitFor(t, ctx, openDirs+` === "docs,docs/deep,src,src/lib"`)
	if eval[bool](t, ctx, `document.getElementById("mini.tree-options").matches(":popover-open")`) {
		t.Error("the menu stayed open")
	}
	open(t, ctx, a.URL("/a.md"))
	if got := eval[string](t, ctx, openDirs); got != "docs,docs/deep,src,src/lib" {
		t.Errorf("on the next page: open %q", got)
	}

	// Five directories, each with one of 1000 files: the first level fits,
	// the files under it do not, and the tree opens down to them only
	files := map[string]string{}
	for d := range 5 {
		for f := range 1000 {
			files[fmt.Sprintf("d%d/sub/f%04d.md", d, f)] = "x\n"
		}
	}
	big := newApp(t, files)
	open(t, ctx, big.URL("/"))
	expand()
	waitFor(t, ctx, openDirs+` === "d0,d1,d2,d3,d4"`)
}
