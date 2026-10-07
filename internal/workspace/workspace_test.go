package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func names(n *Node) []string {
	var out []string
	for _, c := range n.Children {
		out = append(out, c.Name)
	}
	return out
}

func TestSnapshotRebuildsOnlyWhenInvalidated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Open(Options{Root: dir, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.watched.Store(true)

	s1 := w.Snapshot()
	if err := os.WriteFile(filepath.Join(dir, "b.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if s2 := w.Snapshot(); s2 != s1 {
		t.Fatal("rebuilt without being invalidated")
	}

	w.Invalidate(Change{GitHead: true})
	s3 := w.Snapshot()
	if s3.Version != s1.Version+1 || s3.Tree != s1.Tree {
		t.Errorf("GitHead rebuilt the tree: version %d, same tree %v", s3.Version, s3.Tree == s1.Tree)
	}
	if s3.TreeETag != s1.TreeETag {
		t.Error("GitHead changed the tree's ETag")
	}

	w.Invalidate(Change{Structure: true})
	s4 := w.Snapshot()
	if got := names(s4.Tree); len(got) != 2 {
		t.Errorf("tree after Structure = %v, want a.md and b.md", got)
	}
	if s4.TreeETag == s1.TreeETag || !strings.Contains(string(s4.TreeJSON), `"b.md"`) {
		t.Errorf("tree JSON after Structure: ETag %s, %s", s4.TreeETag, s4.TreeJSON)
	}
	if len(names(s1.Tree)) != 1 {
		t.Error("an old snapshot was modified")
	}
}

func TestSnapshotUnwatchedIsAlwaysFresh(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(Options{Root: dir, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Snapshot()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := names(w.Snapshot().Tree); len(got) != 1 {
		t.Errorf("tree = %v, want a.md", got)
	}
}

func TestSnapshotUnwatchedMaxAge(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(Options{Root: dir, Name: "x", MaxAge: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	s1 := w.Snapshot()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if w.Snapshot() != s1 {
		t.Error("rebuilt before MaxAge")
	}
	time.Sleep(250 * time.Millisecond)
	if got := names(w.Snapshot().Tree); len(got) != 1 {
		t.Errorf("tree after MaxAge = %v, want a.md", got)
	}
}

// A directory git ignores is a lazy leaf of the tree, and Subtree reads it.
func TestLazyIgnoredDir(t *testing.T) {
	dir := newRepo(t, "build/\n", "build/a.txt", "build/sub/b.txt", "src/c.go")
	w, err := Open(Options{Root: dir, Name: "x", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var build *Node
	for _, n := range w.Snapshot().Tree.Children {
		if n.Name == "build" {
			build = n
		}
	}
	if build == nil || !build.Lazy || !build.Ignored || build.Children != nil {
		t.Fatalf("build = %+v", build)
	}
	sub, err := w.Subtree("build")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(sub); len(got) != 2 || got[0] != "sub" || got[1] != "a.txt" {
		t.Errorf("build's entries = %v", got)
	}
	if !sub.Children[0].Lazy || !sub.Children[0].Ignored || sub.Children[0].Path != "build/sub" {
		t.Errorf("build/sub = %+v", sub.Children[0])
	}
	for _, bad := range []string{".", "..", "../x", "/etc", "nope", "src/c.go"} {
		if _, err := w.Subtree(bad); err == nil {
			t.Errorf("Subtree(%q) gave no error", bad)
		}
	}
}
