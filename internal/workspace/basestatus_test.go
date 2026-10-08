package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What changed since the branch left its base is what a pull request
// shows: the commits since and what is not committed yet, each file told
// by which of the two it changed in.
func TestBaseStatus(t *testing.T) {
	dir := newRepo(t, "", "a.md", "d.md", "f.md")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	ws, err := Open(Options{Root: dir, Name: "x", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	if st := ws.Snapshot().BaseStatus; st != nil {
		t.Fatalf("on main: %+v", st)
	}

	git(t, dir, "checkout", "-q", "-b", "feature")
	commit(t, dir, "a.md", "a\nmore\n")
	commit(t, dir, "b.md", "b\n")
	// Changed, and changed back
	commit(t, dir, "README.md", "y\n")
	commit(t, dir, "README.md", "x\n")
	git(t, dir, "mv", "d.md", "e.md")
	git(t, dir, "rm", "-q", "f.md")
	git(t, dir, "commit", "-q", "-m", "move and remove")
	write(t, filepath.Join(dir, "b.md"), "b\nc\n")
	write(t, filepath.Join(dir, "c.md"), "c\n")

	snap := ws.Snapshot()
	st := snap.BaseStatus
	if st == nil || st.Base != "main" || len(st.Rev()) != 40 {
		t.Fatalf("on feature: %+v", st)
	}
	want := map[string]struct {
		letter              string
		committed, uncommit bool
		added               int
	}{
		"a.md": {"M", true, false, 2},
		"b.md": {"A", true, true, 2},
		"c.md": {"U", false, true, 1},
		"e.md": {"R", true, false, 0},
		"f.md": {"D", true, false, 0},
	}
	if len(st.Files) != len(want) {
		t.Errorf("files = %v", keys(st.Files))
	}
	for p, w := range want {
		f := st.Files[p]
		if f == nil {
			t.Errorf("%s missing", p)
			continue
		}
		if f.Letter != w.letter || f.Committed != w.committed || f.Uncommitted != w.uncommit || f.Added != w.added {
			t.Errorf("%s = %+v, want %+v", p, f, w)
		}
	}
	if f := st.Files["e.md"]; f != nil && f.From != "d.md" {
		t.Errorf("e.md from %q", f.From)
	}
	if f := st.Files["b.md"]; f != nil && (f.X != "." || f.Y != "M") {
		t.Errorf("b.md X, Y = %q, %q: not those of the status since HEAD", f.X, f.Y)
	}
	if st.Added != 5 || st.Deleted != 2 {
		t.Errorf("lines +%d -%d", st.Added, st.Deleted)
	}
	// The status since HEAD is as it was
	if len(snap.Status.Files) != 2 || snap.Status.Base != "" || snap.Status.Rev() != "HEAD" {
		t.Errorf("since HEAD: %v %q", keys(snap.Status.Files), snap.Status.Rev())
	}
	// Saved again as it was, as an editor or a formatter does: the index's
	// stat of it is no longer the file's, and no commit changed it
	git(t, dir, "update-index", "-q", "--refresh")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "README.md"), later, later); err != nil {
		t.Fatal(err)
	}
	if f := ws.Snapshot().BaseStatus.Files["README.md"]; f != nil {
		t.Errorf("README.md saved as it was: %+v", f)
	}
	if snap.BaseStatusETag == "" || snap.BaseStatusETag == snap.StatusETag {
		t.Errorf("ETags %s %s", snap.BaseStatusETag, snap.StatusETag)
	}

	// The diff and the files before are those where the branch left main
	if d := string(ws.Diff(st, DiffAll, false, "a.md")); !strings.Contains(d, "+more") {
		t.Errorf("diff of a.md:\n%s", d)
	}
	if b, ok := ws.Blob(st, st.Rev(), "a.md"); !ok || string(b) != "x\n" {
		t.Errorf("a.md at the base: %q", b)
	}

	// Merging main moves where the branch left it, and leaves out what
	// main changed
	if err := os.Remove(filepath.Join(dir, "c.md")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "checkout", "-q", "b.md")
	git(t, dir, "checkout", "-q", "main")
	commit(t, dir, "g.md", "g\n")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "checkout", "-q", "feature")
	git(t, dir, "merge", "-q", "--no-edit", "refs/remotes/origin/main")
	st = ws.Snapshot().BaseStatus
	if st == nil || st.Files["g.md"] != nil || st.Files["a.md"] == nil {
		t.Errorf("after merging main: %v", keys(st.Files))
	}
}

func keys(files map[string]*FileStatus) []string {
	var out []string
	for p := range files {
		out = append(out, p)
	}
	return out
}

// A file renamed in a commit and then removed is, since the base, a file
// deleted: its old name is listed, as the commit changed it.
func TestBaseStatusRenamedThenRemoved(t *testing.T) {
	dir := newRepo(t, "", "d.md")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "checkout", "-q", "-b", "feature")
	git(t, dir, "mv", "d.md", "e.md")
	git(t, dir, "commit", "-q", "-m", "move")
	if err := os.Remove(filepath.Join(dir, "e.md")); err != nil {
		t.Fatal(err)
	}
	ws, err := Open(Options{Root: dir, Name: "x", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	st := ws.Snapshot().BaseStatus
	if f := st.Files["d.md"]; f == nil || f.Letter != "D" || !f.Committed {
		t.Errorf("d.md = %+v, files %v", f, keys(st.Files))
	}
}
