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
	run(t, ctx, chromedp.Click(`[id="mini.tree-filters-open"]`, chromedp.ByQuery))
	run(t, ctx, chromedp.Click(`#mini\.changed-only`, chromedp.ByQuery))
	// The button tells that files are left out
	waitFor(t, ctx, `!document.querySelector('[id="mini.tree-filters-open"] .filter-on').hidden`)
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

// The Changes page follows git: a file staged moves to the staged ones,
// and a file edited again shows its new lines, though no count changed.
func TestChangesPage(t *testing.T) {
	a := newRepoApp(t, map[string]string{"a.md": "# A\n", "b.md": "# B\n"})
	a.write("a.md", "# A\n\nOne.\n")
	ctx := tab(t)
	open(t, ctx, a.URL("/_mini/changes"))
	subscribed(t, ctx)
	text := `document.querySelector(".diff-file").textContent`
	stages := `Array.from(document.querySelectorAll(".diff-file .stage"), (e) => e.textContent).join(",")`
	waitFor(t, ctx, text+`.includes("One.") && `+stages+` === "unstaged"`)

	a.git("add", "a.md")
	waitFor(t, ctx, stages+` === "staged"`)

	a.write("a.md", "# A\n\nTwo.\n")
	waitFor(t, ctx, text+`.includes("Two.") && `+stages+` === "staged,unstaged"`)

	// The count in the top bar leads here
	open(t, ctx, a.URL("/"))
	run(t, ctx, chromedp.Click(`#mini\.changes`, chromedp.ByQuery))
	waitFor(t, ctx, `document.body.dataset.kind === "changes"`)
}

// A changed file's page tells how it changed, as git has it now, and a
// file deleted, found in the tree, shows its deletion.
func TestChangedFilePage(t *testing.T) {
	a := newRepoApp(t, map[string]string{"a.md": "# A\n", "b.md": "# B\n"})
	a.write("a.md", "# A\n\nOne.\n")
	if err := os.Remove(filepath.Join(a.root, "b.md")); err != nil {
		t.Fatal(err)
	}
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md?diff=1"))
	subscribed(t, ctx)
	banner := `document.querySelector(".uncommitted")?.textContent`
	waitFor(t, ctx, banner+`.includes("not staged") && document.body.dataset.kind === "diff"`)
	a.git("add", "a.md")
	waitFor(t, ctx, banner+`.includes(", staged")`)

	run(t, ctx, chromedp.Click(`.tree .row.gone[data-path="b.md"]`, chromedp.ByQuery))
	waitFor(t, ctx, `document.body.dataset.path === "b.md" && document.querySelector(".diff .text.del")?.textContent.includes("# B")`)
}

// The settings of Changes: the marks in the tree change at once, and
// turning Changes off takes it all away, and the settings under it.
func TestChangesSettings(t *testing.T) {
	a := newRepoApp(t, map[string]string{"a.md": "# A\n"})
	a.write("a.md", "# A\n\nOne.\n")
	ctx := tab(t)
	open(t, ctx, a.URL("/"))
	waitFor(t, ctx, `(`+letter+`)("a.md") === "M"`)
	visible := `(sel) => { const el = document.querySelector(sel); return !!el && getComputedStyle(el).display !== "none" }`

	run(t, ctx, chromedp.KeyEvent(","))
	waitFor(t, ctx, `document.getElementById("mini.settings").open`)
	run(t, ctx, chromedp.Click(`[id="mini.settings-tab-changes"]`, chromedp.ByQuery))
	run(t, ctx, chromedp.Click(`button[data-setting="treeMarks"][data-value="none"]`, chromedp.ByQuery))
	waitFor(t, ctx, `!(`+visible+`)('.tree .row[data-path="a.md"] .status-letter')`)

	run(t, ctx, chromedp.Click(`input[data-setting="changes"]`, chromedp.ByQuery))
	waitFor(t, ctx, count+` === null && document.querySelector(".tree .status-letter") === null`)
	// The page came back with the dialog open, the settings under it off
	waitFor(t, ctx, `document.getElementById("mini.settings").open && document.querySelector('input[data-setting="untracked"]').disabled`)
}
