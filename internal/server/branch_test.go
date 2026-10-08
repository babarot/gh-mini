package server

import (
	"encoding/json"
	"path/filepath"
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
