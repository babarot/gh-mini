package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openStatus(t *testing.T, dir string, skip ...string) *Status {
	t.Helper()
	ws, err := Open(Options{Root: dir, Name: "x", Skip: append([]string{".git"}, skip...)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws.Snapshot().Status
}

type want struct {
	x, y, from     string
	added, deleted int
	binary         bool
}

func checkStatus(t *testing.T, st *Status, files map[string]want) {
	t.Helper()
	if !st.Git {
		t.Fatal("not a repository")
	}
	for p, w := range files {
		f := st.Files[p]
		if f == nil {
			t.Errorf("%s: missing", p)
			continue
		}
		got := want{f.X, f.Y, f.From, f.Added, f.Deleted, f.Binary}
		if got != w {
			t.Errorf("%s: got %+v, want %+v", p, got, w)
		}
	}
	if len(st.Files) != len(files) {
		var names []string
		for p := range st.Files {
			names = append(names, p)
		}
		t.Errorf("files = %v, want %d", names, len(files))
	}
}

func TestStatus(t *testing.T) {
	dir := newRepo(t, "*.local\n", "mod.md", "staged.md", "both.md", "gone.md", "old.md", "bin.dat")
	write(t, filepath.Join(dir, "old.md"), "a\nb\nc\nd\n")
	git(t, dir, "commit", "-qam", "lines")

	write(t, filepath.Join(dir, "mod.md"), "y\nz\n")
	write(t, filepath.Join(dir, "staged.md"), "y\n")
	git(t, dir, "add", "staged.md")
	write(t, filepath.Join(dir, "both.md"), "y\n")
	git(t, dir, "add", "both.md")
	write(t, filepath.Join(dir, "both.md"), "y\nz\n")
	os.Remove(filepath.Join(dir, "gone.md"))
	git(t, dir, "mv", "old.md", "new.md")
	write(t, filepath.Join(dir, "bin.dat"), "a\x00b")
	write(t, filepath.Join(dir, "added.md"), "1\n2\n3")
	write(t, filepath.Join(dir, "newdir/a.md"), "1\n")
	write(t, filepath.Join(dir, "newdir/deep/b.md"), "1\n2\n")
	write(t, filepath.Join(dir, "newdir/skip.local"), "1\n")
	write(t, filepath.Join(dir, "skip.local"), "1\n")
	write(t, filepath.Join(dir, "clone/c.md"), "1\n")
	git(t, filepath.Join(dir, "clone"), "init", "-q")

	st := openStatus(t, dir)
	checkStatus(t, st, map[string]want{
		"mod.md":           {x: ".", y: "M", added: 2, deleted: 1},
		"staged.md":        {x: "M", y: ".", added: 1, deleted: 1},
		"both.md":          {x: "M", y: "M", added: 2, deleted: 1},
		"gone.md":          {x: ".", y: "D", deleted: 1},
		"new.md":           {x: "R", y: ".", from: "old.md"},
		"bin.dat":          {x: ".", y: "M", binary: true},
		"added.md":         {x: "?", y: "?", added: 3},
		"newdir/a.md":      {x: "?", y: "?", added: 1},
		"newdir/deep/b.md": {x: "?", y: "?", added: 2},
	})
	for p, l := range map[string]string{"mod.md": "M", "staged.md": "M", "gone.md": "D", "new.md": "R", "added.md": "U"} {
		if got := st.Files[p].Letter; got != l {
			t.Errorf("%s: letter %q, want %q", p, got, l)
		}
	}
	if st.Added != 2+1+2+3+1+2 || st.Deleted != 1+1+1+1 {
		t.Errorf("totals +%d -%d", st.Added, st.Deleted)
	}
}

// Served from a directory in the repository, paths are relative to it and
// what is outside is left out.
func TestStatusSubdir(t *testing.T) {
	dir := newRepo(t, "", "docs/a.md", "top.md")
	write(t, filepath.Join(dir, "docs/a.md"), "y\n")
	write(t, filepath.Join(dir, "top.md"), "y\n")
	write(t, filepath.Join(dir, "docs/new/n.md"), "n\n")
	checkStatus(t, openStatus(t, filepath.Join(dir, "docs")), map[string]want{
		"a.md":     {x: ".", y: "M", added: 1, deleted: 1},
		"new/n.md": {x: "?", y: "?", added: 1},
	})
}

func TestStatusSkip(t *testing.T) {
	dir := newRepo(t, "")
	write(t, filepath.Join(dir, "node_modules/x.js"), "x\n")
	write(t, filepath.Join(dir, "a/node_modules/y.js"), "y\n")
	write(t, filepath.Join(dir, "a/b.md"), "b\n")
	checkStatus(t, openStatus(t, dir, "node_modules"), map[string]want{
		"a/b.md": {x: "?", y: "?", added: 1},
	})
}

func TestStatusNoCommit(t *testing.T) {
	dir := newRepo(t, "")
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	write(t, filepath.Join(dir, "a.md"), "1\n2\n")
	git(t, dir, "add", "a.md")
	checkStatus(t, openStatus(t, dir), map[string]want{
		"a.md":      {x: "A", y: ".", added: 2},
		"README.md": {x: "?", y: "?", added: 1},
	})
}

func TestStatusNotRepository(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.md"), "x\n")
	ws, err := Open(Options{Root: dir, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if st := ws.Snapshot().Status; st.Git || len(st.Files) != 0 {
		t.Errorf("got %+v", st)
	}
}

func TestWatchStatus(t *testing.T) {
	dir := newRepo(t, "", "a.md")
	ws, events := startWatch(t, dir)
	if n := len(ws.Snapshot().Status.Files); n != 0 {
		t.Fatalf("%d files changed at the start", n)
	}

	write(t, filepath.Join(dir, "a.md"), "y\n")
	next(t, events, func(e Event) bool { return e.Status })
	if f := ws.Snapshot().Status.Files["a.md"]; f == nil || f.Y != "M" {
		t.Fatalf("after editing: %+v", f)
	}

	// Staging changes only the index, no file in the tree
	git(t, dir, "add", "a.md")
	next(t, events, func(e Event) bool { return e.Status })
	if f := ws.Snapshot().Status.Files["a.md"]; f == nil || f.X != "M" || f.Y != "." {
		t.Fatalf("after staging: %+v", f)
	}

	git(t, dir, "commit", "-qm", "a")
	next(t, events, func(e Event) bool { return e.Status })
	if n := len(ws.Snapshot().Status.Files); n != 0 {
		t.Fatalf("%d files changed after committing", n)
	}

	// Reading the status writes nothing that the watcher would see, so
	// nothing comes after
	select {
	case e := <-events:
		t.Errorf("event after the status settled: %+v", e)
	case <-time.After(time.Second):
	}
}
