package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// commit writes a file and commits it.
func commit(t *testing.T, dir, name, body string) {
	t.Helper()
	write(t, filepath.Join(dir, name), body)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "change "+name)
}

func TestGitBase(t *testing.T) {
	dir := newRepo(t, "")
	if name, _ := gitBase(dir); name != "" {
		t.Errorf("no remote: base %q", name)
	}
	git(t, dir, "update-ref", "refs/remotes/origin/master", "HEAD")
	if name, ref := gitBase(dir); name != "master" || ref != "refs/remotes/origin/master" {
		t.Errorf("master only: base %q %q", name, ref)
	}
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	if name, _ := gitBase(dir); name != "main" {
		t.Errorf("main and master: base %q, want main", name)
	}
	git(t, dir, "update-ref", "refs/remotes/origin/trunk", "HEAD")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	if name, _ := gitBase(dir); name != "trunk" {
		t.Errorf("origin/HEAD at trunk: base %q", name)
	}
	// Renamed on origin and pruned here: origin/HEAD names a branch gone
	git(t, dir, "update-ref", "-d", "refs/remotes/origin/trunk")
	if name, _ := gitBase(dir); name != "main" {
		t.Errorf("origin/HEAD at a branch gone: base %q, want main", name)
	}
}

func TestGitBranchInfo(t *testing.T) {
	dir := newRepo(t, "")
	if b := gitBranchInfo(dir); b != nil {
		t.Errorf("no base: %+v", b)
	}
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if b := gitBranchInfo(dir); b == nil || !b.OnBase || b.Ahead != 0 || b.Behind != 0 {
		t.Errorf("on main: %+v", b)
	}

	git(t, dir, "checkout", "-q", "-b", "feature")
	commit(t, dir, "a.md", "a\n")
	commit(t, dir, "b.md", "b\n")
	git(t, dir, "checkout", "-q", "main")
	commit(t, dir, "c.md", "c\n")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "checkout", "-q", "feature")
	b := gitBranchInfo(dir)
	if b == nil || b.OnBase || b.Detached || b.Base != "main" || b.Ahead != 2 || b.Behind != 1 || len(b.MergeBase) != 7 {
		t.Fatalf("feature: %+v", b)
	}
	if b.Upstream != "" || b.Unpushed != 0 {
		t.Errorf("feature without upstream: %+v", b)
	}

	// A remote that is never reached, for git to map its branches
	git(t, dir, "remote", "add", "origin", "https://example.com/x.git")
	git(t, dir, "update-ref", "refs/remotes/origin/feature", "HEAD~1")
	git(t, dir, "branch", "-q", "--set-upstream-to=origin/feature")
	if b := gitBranchInfo(dir); b.Upstream != "origin/feature" || b.Unpushed != 1 {
		t.Errorf("feature pushed but one: %+v", b)
	}

	git(t, dir, "checkout", "-q", "--detach")
	if b := gitBranchInfo(dir); b == nil || !b.Detached || b.OnBase || b.Ahead != 2 || b.Upstream != "" {
		t.Errorf("detached: %+v", b)
	}
}

// Branch reads the refs again on every call, and the branch checked out:
// a branch just made is at the same commit as the one it came from.
func TestBranchFollowsRefs(t *testing.T) {
	dir := newRepo(t, "")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	ws, err := Open(Options{Root: dir, Name: "x", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	if b := ws.Branch(); b == nil || !b.OnBase {
		t.Fatalf("on main: %+v", b)
	}
	git(t, dir, "checkout", "-q", "-b", "feature")
	if b := ws.Branch(); b == nil || b.OnBase {
		t.Errorf("on a branch at main's commit: %+v", b)
	}
	commit(t, dir, "a.md", "a\n")
	if b := ws.Branch(); b.Ahead != 1 {
		t.Errorf("after a commit: %+v", b)
	}
	// As a fetch moves it
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	if b := ws.Branch(); b.Ahead != 0 || b.Behind != 0 {
		t.Errorf("after origin/main moved: %+v", b)
	}
	git(t, dir, "checkout", "-q", "main")
	if b := ws.Branch(); !b.OnBase {
		t.Errorf("back on main: %+v", b)
	}

	off, err := Open(Options{Root: dir, Name: "x", NoStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { off.Close() })
	if b := off.Branch(); b != nil {
		t.Errorf("with changes off: %+v", b)
	}
}

// A fetch writes FETCH_HEAD and moves a branch of origin, in the git
// directory a worktree shares with its repository.
func TestWatchRefsOfWorktree(t *testing.T) {
	repo := newRepo(t, "")
	wt := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-wt")
	git(t, repo, "worktree", "add", "-q", "-b", "feature", wt)
	t.Cleanup(func() { os.RemoveAll(wt) })
	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	commit(t, repo, "a.md", "a\n")
	_, events := startWatch(t, wt)

	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	next(t, events, func(e Event) bool { return e.GitHead })
	write(t, filepath.Join(repo, ".git", "FETCH_HEAD"), "x\n")
	next(t, events, func(e Event) bool { return e.GitHead })
	git(t, repo, "pack-refs", "--all")
	next(t, events, func(e Event) bool { return e.GitHead })
}

// A commit moves HEAD without writing HEAD itself, and may leave nothing
// uncommitted changed, as a file written, added and committed at once.
func TestWatchCommitMovesHead(t *testing.T) {
	dir := newRepo(t, "")
	_, events := startWatch(t, dir)
	commit(t, dir, "a.md", "a\n")
	next(t, events, func(e Event) bool { return e.GitHead })
}
