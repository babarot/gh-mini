package workspace

import (
	"os"
	"path/filepath"
	"testing"
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

	w.Invalidate(Change{Structure: true})
	s4 := w.Snapshot()
	if got := names(s4.Tree); len(got) != 2 {
		t.Errorf("tree after Structure = %v, want a.md and b.md", got)
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
