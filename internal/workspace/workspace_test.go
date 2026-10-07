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
