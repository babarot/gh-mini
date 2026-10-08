package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubGH puts a gh first on PATH that answers with out, or fails when out
// is empty, and writes its arguments to the file it returns.
func stubGH(t *testing.T, out string) (calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script for gh")
	}
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	answer := filepath.Join(dir, "answer")
	writeFile(t, answer, []byte(out))
	script := "#!/bin/sh\necho \"$@\" >> '" + calls + "'\n[ -s '" + answer + "' ] || exit 1\ncat '" + answer + "'\n"
	writeFile(t, filepath.Join(dir, "gh"), []byte(script))
	if err := os.Chmod(filepath.Join(dir, "gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func callsOf(t *testing.T, calls string) []string {
	t.Helper()
	b, err := os.ReadFile(calls)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// The branch's pull request comes from gh, asked once for a while, and
// only for a branch pushed.
func TestHandlerPR(t *testing.T) {
	calls := stubGH(t, `{"number":3,"title":"Add a page","state":"OPEN","isDraft":false,"url":"https://github.com/example/repo/pull/3"}`)
	root, srv := newBranchServer(t, Options{})
	h := srv.Handler()
	get(t, h, "/_mini/api/pr").expect(t, 204)

	git(t, root, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(root, "new.md"), []byte("# New\n"))
	commitAll(t, root, "new")
	get(t, h, "/_mini/api/pr").expect(t, 204)
	if c := callsOf(t, calls); c != nil {
		t.Errorf("asked gh for a branch not pushed: %v", c)
	}

	git(t, root, "update-ref", "refs/remotes/origin/feature", "HEAD")
	git(t, root, "branch", "-q", "--set-upstream-to=origin/feature")
	get(t, h, "/_mini/api/pr").expect(t, 200, `"number":3`, `"state":"open"`, `"url":"https://github.com/example/repo/pull/3"`)
	get(t, h, "/_mini/api/pr").expect(t, 200, `"number":3`)
	if c := callsOf(t, calls); len(c) != 1 || c[0] != "pr view feature --json number,title,state,isDraft,url" {
		t.Errorf("gh asked %v", c)
	}
}

func TestGHPullRequest(t *testing.T) {
	for out, want := range map[string]string{
		`{"number":3,"title":"t","state":"OPEN","isDraft":true,"url":"https://github.com/o/r/pull/3"}`:    "draft",
		`{"number":3,"title":"t","state":"MERGED","isDraft":false,"url":"https://github.com/o/r/pull/3"}`: "merged",
		`{"number":3,"title":"t","state":"CLOSED","isDraft":false,"url":"https://github.com/o/r/pull/3"}`: "closed",
		`{"number":3,"title":"t","state":"OPEN","url":"javascript:alert(1)"}`:                             "",
		"": "",
	} {
		stubGH(t, out)
		pr := ghPullRequest("gh", t.TempDir(), "feature")
		if got := ""; pr != nil {
			got = pr.State
			if got != want {
				t.Errorf("%s: state %q, want %q", out, got, want)
			}
		} else if want != "" {
			t.Errorf("%s: none, want %q", out, want)
		}
	}
}
