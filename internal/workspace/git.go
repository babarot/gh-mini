package workspace

import (
	"bytes"
	"os/exec"
	"regexp"
	"strings"
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

// gitBranch is the branch checked out; on a detached HEAD, the tag there,
// or the commit.
func gitBranch(dir string) string {
	branch := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "HEAD" {
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

func parseGitHubRepo(url string) string {
	m := gitHubRemote.FindStringSubmatch(url)
	if m == nil {
		return ""
	}
	return m[1] + "/" + m[2]
}
