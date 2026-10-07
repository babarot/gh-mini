package workspace

import (
	"bytes"
	"os/exec"
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

func gitBranch(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
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
	u := strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
	i := strings.Index(u, "github.com")
	if i < 0 {
		return ""
	}
	parts := strings.FieldsFunc(u[i+len("github.com"):], func(r rune) bool { return r == '/' || r == ':' })
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
