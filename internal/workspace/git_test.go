package workspace

import "testing"

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
