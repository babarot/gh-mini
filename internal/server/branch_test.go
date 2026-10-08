package server

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// commitAll commits everything in a test repository.
func commitAll(t *testing.T, root, msg string) {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", msg)
}

// newBranchServer serves the test repository with origin's main at its
// commit, as a clone has it.
func newBranchServer(t *testing.T, opts Options) (root string, srv *Server) {
	t.Helper()
	root, themes := newTestRepo(t)
	git(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	opts.Root, opts.Name, opts.Skip, opts.ThemesDir = root, "repo", []string{".git"}, themes
	srv, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return root, srv
}

// On a branch other than the base, the branch in the top bar opens a panel
// and tells how far it is ahead of the base, and the tab's title names it.
func TestHandlerBranch(t *testing.T) {
	root, srv := newBranchServer(t, Options{})
	h := srv.Handler()
	branch := func() map[string]any {
		t.Helper()
		var b map[string]any
		r := get(t, h, "/_mini/api/branch")
		if err := json.Unmarshal([]byte(r.body), &b); err != nil {
			t.Fatalf("%s: %v", r.body, err)
		}
		return b
	}

	get(t, h, "/").expect(t, 200, `<span class="branch" title="Current branch: main">`, "<title>example/repo</title>")
	if b := branch(); b != nil {
		t.Errorf("on main: %v", b)
	}

	git(t, root, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(root, "new.md"), []byte("# New\n"))
	commitAll(t, root, "new")
	// Without a watcher, the branch's name is read again only after a while
	srv, err := New(Options{Root: root, Name: "repo", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	h = srv.Handler()
	r := get(t, h, "/docs/guide.md")
	r.expect(t, 200, `id="mini.branch" popovertarget="mini.branch-panel"`, `<span data-ahead title="Commits ahead of main">↑1</span>`,
		"<title>docs/guide.md · example/repo · feature</title>")
	r.reject(t, `<span class="ab" data-ab hidden>`, `class="behind"`)
	if b := branch(); b["base"] != "main" || b["ahead"] != 1.0 || b["behind"] != 0.0 || b["onBase"] != false {
		t.Errorf("on feature: %v", b)
	}

	_, off := newBranchServer(t, Options{NoChanges: true})
	get(t, off.Handler(), "/").expect(t, 200, `<span class="branch" title="Current branch: main">`, "<title>example/repo</title>")
}

// A branch of origin moving, as a fetch moves it, is told to the pages,
// for where the branch stands.
func TestHandlerEventsHead(t *testing.T) {
	root, _, url := newReloadServer(t)
	ch := events(t, url)
	git(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	waitFor(t, ch, func(c change) bool { return c.Head })
}

// On a branch, the pages count what changed since it left the base, and
// the Changes page shows it, with the commits since, or what changed since
// the last commit.
func TestHandlerChangesSinceBase(t *testing.T) {
	root, _ := newBranchServer(t, Options{})
	git(t, root, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(root, "docs", "guide.md"), []byte("# Guide\n\n## Install\n\nCommitted.\n"))
	commitAll(t, root, "Explain the install")
	writeFile(t, filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println() }\n"))
	// With reload on, for the status a page is rendered with
	srv, err := New(Options{Root: root, Name: "repo", Skip: []string{".git"}, Reload: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	h := srv.Handler()

	status := get(t, h, "/_mini/api/status")
	status.expect(t, 200, `"base":"main"`, `"docs/guide.md":{`, `"committed":true`, `"main.go":{`, `"uncommitted":true`)
	etag := strings.Trim(status.header.Get("ETag"), `"`)

	r := get(t, h, "/_mini/changes")
	r.expect(t, 200, `title="Changes since main"`, "the working tree against where the branch left origin/main",
		`Since main <span class="counter">2</span>`, `Uncommitted <span class="counter">1</span>`,
		"<strong>1 commit</strong>", "Explain the install", `<span class="unpushed"`,
		`<span class="stage stage-committed">committed</span>`, `<span class="stage stage-uncommitted">uncommitted</span>`,
		"Committed.", `data-status="`+etag+`"`)
	r.reject(t, `aria-label="Which changes"`)

	// The page's status is the same in either scope, so that it is not
	// reloaded for the status the page is told
	r = get(t, h, "/_mini/changes?scope=uncommitted")
	r.expect(t, 200, "the working tree against the last commit", `aria-label="Which changes"`,
		`href="/_mini/changes?scope=uncommitted&amp;show=staged"`, "println", `data-status="`+etag+`"`)
	r.reject(t, "Committed.", "Explain the install")

	r = get(t, h, "/docs/guide.md?diff=1")
	r.expect(t, 200, "committed on this branch", "Committed.")

	// On the base, as before
	git(t, root, "checkout", "-q", "main.go")
	git(t, root, "checkout", "-q", "main")
	srv, err = New(Options{Root: root, Name: "repo", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	get(t, srv.Handler(), "/_mini/changes").reject(t, `aria-label="Since when"`, "Since main")
}
