package workspace

import (
	"path/filepath"
	"testing"
)

func TestGitLastCommit(t *testing.T) {
	dir := newRepo(t, "*.local\n", "a.md", "b*.md", "skip.local")
	write(t, filepath.Join(dir, "a.md"), "y\n")
	git(t, dir, "-c", "user.name=Ann", "-c", "user.email=a@example.com", "commit", "-q", "-am", "Change a\n\nbody")

	c := gitLastCommit(dir, "a.md")
	if c == nil || c.Author != "Ann" || c.Email != "a@example.com" || c.Subject != "Change a" || len(c.SHA) != 40 {
		t.Fatalf("a.md: got %+v", c)
	}
	if c.Pushed {
		t.Error("a.md: pushed without a remote")
	}
	if c := gitLastCommit(dir, "README.md"); c == nil || c.Subject != "init" {
		t.Errorf("README.md: got %+v, want init", c)
	}
	// Names are taken literally: as a pattern, a.md* would match a.md
	if c := gitLastCommit(dir, "a.md*"); c != nil {
		t.Errorf("a.md*: got %+v, want none", c)
	}
	if c := gitLastCommit(dir, "b*.md"); c == nil || c.Subject != "init" {
		t.Errorf("b*.md: got %+v, want init", c)
	}
	write(t, filepath.Join(dir, "new.md"), "x\n")
	for _, rel := range []string{"skip.local", "new.md"} {
		if c := gitLastCommit(dir, rel); c != nil {
			t.Errorf("%s: got %+v, want none", rel, c)
		}
	}
	if c := gitLastCommit(t.TempDir(), "a.md"); c != nil {
		t.Errorf("not a repository: got %+v", c)
	}

	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD~1")
	if c := gitLastCommit(dir, "a.md"); c.Pushed {
		t.Error("a.md: pushed before its commit was")
	}
	if c := gitLastCommit(dir, "README.md"); !c.Pushed {
		t.Error("README.md: not pushed")
	}
}

func TestParseGitHubRepo(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/owner/name.git":           "owner/name",
		"https://github.com/owner/name":               "owner/name",
		"https://github.com/owner/name/":              "owner/name",
		"git@github.com:owner/name.git":               "owner/name",
		"ssh://git@github.com/owner/name.git":         "owner/name",
		"ssh://git@ssh.github.com:443/owner/name.git": "owner/name",
		"git@ssh.github.com:owner/name.git":           "owner/name",
		"https://user@github.com/owner/name.git":      "owner/name",
		"https://github.com/owner/name/tree/main.git": "owner/name",
		"https://gitlab.com/owner/name.git":           "",
		"https://github.company.com/owner/name.git":   "",
		"git@notgithub.com:owner/name.git":            "",
		"https://www.github.com/owner/name":           "owner/name",
		"https://github.com/owner":                    "",
		"https://github.com/":                         "",
		"":                                            "",
	} {
		if got := parseGitHubRepo(url); got != want {
			t.Errorf("parseGitHubRepo(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestGitHubRepo(t *testing.T) {
	dir := newRepo(t, "")
	if got := gitHubRepo(dir); got != "" {
		t.Errorf("no origin: got %q, want none", got)
	}
	if got := gitHubRepo(t.TempDir()); got != "" {
		t.Errorf("not a repository: got %q, want none", got)
	}
	git(t, dir, "remote", "add", "origin", "git@github.com:owner/name.git")
	if got := gitHubRepo(dir); got != "owner/name" {
		t.Errorf("got %q, want owner/name", got)
	}
}

func TestGitBranch(t *testing.T) {
	dir := newRepo(t, "")
	if got := gitBranch(dir); got != "main" {
		t.Errorf("got %q, want main", got)
	}
	git(t, dir, "switch", "-q", "-c", "feature/x")
	if got := gitBranch(dir); got != "feature/x" {
		t.Errorf("got %q, want feature/x", got)
	}
	git(t, dir, "switch", "-q", "--detach")
	if got, want := gitBranch(dir), gitOutput(dir, "rev-parse", "--short", "HEAD"); got != want || got == "" {
		t.Errorf("detached: got %q, want the commit %q", got, want)
	}
	if got := gitBranch(t.TempDir()); got != "" {
		t.Errorf("not a repository: got %q, want none", got)
	}
}

// A new repository is on a branch before its first commit.
func TestGitBranchUnborn(t *testing.T) {
	dir := newRepo(t, "")
	unborn := t.TempDir()
	git(t, unborn, "init", "-q", "-b", "trunk")
	if b := gitBranch(unborn); b != "trunk" {
		t.Errorf("no commits: %q, want trunk", b)
	}
	git(t, dir, "switch", "-q", "--orphan", "fresh")
	if b := gitBranch(dir); b != "fresh" {
		t.Errorf("orphan branch: %q, want fresh", b)
	}
}

func TestGitBranchDetached(t *testing.T) {
	dir := newRepo(t, "")
	if b := gitBranch(dir); b != "main" {
		t.Fatalf("branch = %q", b)
	}
	git(t, dir, "tag", "v1.0.0")
	git(t, dir, "checkout", "-q", "--detach")
	if b := gitBranch(dir); b != "v1.0.0" {
		t.Errorf("detached at a tag: %q", b)
	}
	git(t, dir, "tag", "-d", "v1.0.0")
	sha := gitOutput(dir, "rev-parse", "--short", "HEAD")
	if b := gitBranch(dir); b != sha || sha == "" {
		t.Errorf("detached: %q, want %q", b, sha)
	}
}
