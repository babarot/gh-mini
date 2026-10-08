package workspace

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// gitIgnored lists what git ignores under dir, so that local-only files
// such as untracked translations can be marked. Outside a git repository
// nothing is marked.
func gitIgnored(dir string) map[string]bool {
	out, err := exec.Command("git", "-C", dir, "ls-files",
		"--others", "--ignored", "--exclude-standard", "--directory", "-z").Output()
	set := map[string]bool{}
	if err != nil {
		return set
	}
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			set[strings.TrimSuffix(string(p), "/")] = true
		}
	}
	return set
}

// gitBranch is the branch checked out, even one with no commits yet; on a
// detached HEAD, the tag there, or the commit.
func gitBranch(dir string) string {
	if branch := gitOutput(dir, "symbolic-ref", "-q", "--short", "HEAD"); branch != "" {
		return branch
	}
	if tag := gitOutput(dir, "describe", "--tags", "--exact-match"); tag != "" {
		return tag
	}
	return gitOutput(dir, "rev-parse", "--short", "HEAD")
}

func gitOutput(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitHubRepo(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return parseGitHubRepo(strings.TrimSpace(string(out)))
}

// parseGitHubRepo returns "owner/name" from a GitHub remote URL, HTTPS or
// SSH, or "" when the URL is not GitHub's.
// gitHubRemote finds owner and name in a remote on github.com, over
// HTTPS or SSH, and nowhere else: github.company.com is another host.
var gitHubRemote = regexp.MustCompile(`(?:^|[@/.])github\.com[:/]+([^/:]+)/([^/]+?)(?:\.git)?(?:/.*)?$`)

// gitHubPort is a port after the host, as in
// ssh://git@ssh.github.com:443/owner/name. Only a URL with a scheme has
// one; the scp-like form puts the owner after the colon.
var gitHubPort = regexp.MustCompile(`github\.com:\d+/`)

func parseGitHubRepo(url string) string {
	if strings.Contains(url, "://") {
		url = gitHubPort.ReplaceAllString(url, "github.com/")
	}
	m := gitHubRemote.FindStringSubmatch(url)
	if m == nil {
		return ""
	}
	return m[1] + "/" + m[2]
}

// Commit is the last commit that changed a file.
type Commit struct {
	SHA     string
	Author  string
	Email   string
	Subject string
	Time    time.Time
	// Pushed tells whether a remote branch has the commit, so that a link
	// to it on GitHub finds it
	Pushed bool
}

// LastCommit returns the last commit that changed a file or directory
// under the root, or nil outside a repository and for what is not
// committed.
func (w *Workspace) LastCommit(rel string) *Commit {
	return gitLastCommit(w.opts.Root, rel)
}

func gitLastCommit(dir, rel string) *Commit {
	// Literal, so that a name with * or : is not taken for a pattern
	out, err := exec.Command("git", "--literal-pathspecs", "-C", dir, "log", "-1",
		"--format=%H%x00%an%x00%ae%x00%at%x00%s", "--", rel).Output()
	if err != nil {
		return nil
	}
	f := strings.SplitN(strings.TrimSuffix(string(out), "\n"), "\x00", 5)
	if len(f) != 5 {
		return nil
	}
	sec, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return nil
	}
	c := &Commit{SHA: f[0], Author: f[1], Email: f[2], Time: time.Unix(sec, 0), Subject: f[4]}
	out, err = exec.Command("git", "-C", dir, "for-each-ref", "--contains", c.SHA,
		"--count=1", "--format=%(refname)", "refs/remotes").Output()
	c.Pushed = err == nil && len(bytes.TrimSpace(out)) > 0
	return c
}
