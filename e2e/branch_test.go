package e2e

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// newBranchApp serves a repository whose origin has main at its first
// commit, as a clone has it, on a branch made from there, after change.
func newBranchApp(t *testing.T, files map[string]string, change func(a *app)) *app {
	t.Helper()
	return newRepoApp(t, files, func(a *app) {
		a.git("update-ref", "refs/remotes/origin/main", "HEAD")
		a.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
		a.git("checkout", "-q", "-b", "feature")
		change(a)
	})
}

const (
	ahead  = `document.querySelector("#mini\\.branch [data-ahead]")?.textContent`
	abHid  = `document.querySelector("#mini\\.branch [data-ab]").hidden`
	panelQ = `document.getElementById("mini.branch-panel")`
)

// The branch tells how far it is ahead of main, following commits and a
// fetch without a reload, and b opens the panel that tells more.
func TestBranchStanding(t *testing.T) {
	a := newBranchApp(t, map[string]string{"a.md": "# A\n"}, func(a *app) {
		a.write("b.md", "# B\n")
		a.git("add", "-A")
		a.git("commit", "-q", "-m", "b")
	})
	ctx := tab(t)
	// A page none of the commits below changes: it is never reloaded
	open(t, ctx, a.URL("/a.md"))
	subscribed(t, ctx)
	run(t, ctx, chromedp.Evaluate(`window.notReloaded = true`, nil))
	waitFor(t, ctx, ahead+` === "↑1" && !`+abHid)
	waitFor(t, ctx, `document.title.endsWith(" · feature")`)

	run(t, ctx, chromedp.KeyEvent("b"))
	waitFor(t, ctx, panelQ+`.matches(":popover-open") && `+panelQ+`.textContent.includes("1 ahead main") && `+panelQ+`.textContent.includes("Not pushed")`)
	run(t, ctx, chromedp.KeyEvent("b"))
	waitFor(t, ctx, `!`+panelQ+`.matches(":popover-open")`)

	// A commit moves the branch, and no file of HEAD's
	a.write("c.md", "# C\n")
	a.git("add", "-A")
	a.git("commit", "-q", "-m", "c")
	waitFor(t, ctx, ahead+` === "↑2"`)

	// As a fetch moves origin's main: here, up to the branch
	a.git("update-ref", "refs/remotes/origin/main", "HEAD")
	waitFor(t, ctx, abHid)
	if !eval[bool](t, ctx, `window.notReloaded === true`) {
		t.Error("the page reloaded")
	}
}

// On main, the branch is as it always was.
func TestBranchOnBase(t *testing.T) {
	a := newRepoApp(t, map[string]string{"a.md": "# A\n"}, func(a *app) {
		a.git("update-ref", "refs/remotes/origin/main", "HEAD")
	})
	ctx := tab(t)
	open(t, ctx, a.URL("/"))
	waitFor(t, ctx, `document.querySelector("span.branch") !== null && document.getElementById("mini.branch") === null`)
}
