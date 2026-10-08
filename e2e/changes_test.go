package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// newRepoApp serves a git repository holding files, committed, with git's
// configuration kept apart from the machine's.
func newRepoApp(t *testing.T, files map[string]string) *app {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "t")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.com")
	}
	a := newApp(t, files)
	a.git("init", "-q", "-b", "main")
	a.git("add", "-A")
	a.git("commit", "-q", "-m", "init")
	// The server learns it serves a repository as it starts
	a.skip = []string{".git"}
	a.restart()
	return a
}

func (a *app) git(args ...string) {
	a.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", a.root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		a.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

const (
	count  = `document.getElementById("mini.changes")`
	letter = `(p) => document.querySelector('.tree .row[data-path="' + p + '"] .status-letter')?.textContent`
)

// The tree marks how files changed since the last commit, the top bar
// counts them, and both follow git as files are edited, staged and
// committed.
func TestChangesInTree(t *testing.T) {
	a := newRepoApp(t, map[string]string{"a.md": "# A\n", "docs/c.md": "# C\n", "docs/d.md": "# D\n"})
	a.write("a.md", "# A\n\nMore.\n")
	a.write("new.md", "# New\n")
	if err := os.Remove(filepath.Join(a.root, "docs", "c.md")); err != nil {
		t.Fatal(err)
	}
	ctx := tab(t)
	open(t, ctx, a.URL("/"))
	subscribed(t, ctx)
	waitFor(t, ctx, `(`+letter+`)("a.md") === "M" && (`+letter+`)("new.md") === "U"`)
	waitFor(t, ctx, `!`+count+`.hidden && `+count+`.textContent.includes("3 changes")`)
	if !eval[bool](t, ctx, `document.querySelector('.tree .row[data-path="docs"] .status-dot') !== null`) {
		t.Error("no dot on docs")
	}
	// The directory's page lists them too, the deleted file under docs
	// in docs' sum
	if !eval[bool](t, ctx, `document.querySelector(".files tr.changed.s-M a[href='/a.md']") !== null`) {
		t.Error("a.md is not listed as modified")
	}

	// Only changed files, with the directories holding them opened, and
	// the deleted file in its place
	run(t, ctx, chromedp.Click(`#mini\.changed-only`, chromedp.ByQuery))
	waitFor(t, ctx, `document.querySelector('.tree .row.gone[data-path="docs/c.md"]') !== null`)
	if eval[bool](t, ctx, `document.querySelector('.tree .row[data-path="docs/d.md"]') !== null`) {
		t.Error("an unchanged file is shown")
	}

	// Staging changes no file, but the page that lists the status
	eval[any](t, ctx, `window.marker = 1`)
	a.git("add", "a.md")
	waitFor(t, ctx, `window.marker === undefined`)
	open(t, ctx, a.URL("/"))
	subscribed(t, ctx)
	a.git("add", "-A")
	a.git("commit", "-q", "-m", "all")
	waitFor(t, ctx, count+`.hidden`)
	waitFor(t, ctx, `document.querySelector(".tree .status-letter") === null`)
}
