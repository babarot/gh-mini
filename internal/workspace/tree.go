package workspace

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// Node is a file or directory in the tree.
type Node struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Dir     bool   `json:"dir,omitempty"`
	Ignored bool   `json:"ignored,omitempty"`
	// Lazy is set on a directory git ignores: its entries are left out,
	// as such directories are often a build's output or a tool's state
	// with many files, and Subtree gives them when it is opened.
	Lazy     bool    `json:"lazy,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

// buildTree returns the tree and its directories by path.
func (w *Workspace) buildTree(ignored map[string]bool) (*Node, map[string]*Node) {
	snap := &Snapshot{ignored: ignored}
	root := &Node{Name: w.opts.Name, Dir: true}
	nodes := map[string]*Node{".": root}
	// Only names are read, and a walk does not follow symlinks, so the
	// plain directory is enough; walking through os.Root, which checks
	// every path it opens, took five times as long
	_ = fs.WalkDir(os.DirFS(w.opts.Root), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return nil
		}
		if w.Skipped(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		parent := nodes[path.Dir(p)]
		if parent == nil {
			return nil
		}
		n := &Node{Name: d.Name(), Path: p, Dir: d.IsDir(), Ignored: snap.Ignored(p)}
		parent.Children = append(parent.Children, n)
		if d.IsDir() && n.Ignored {
			n.Lazy = true
			return fs.SkipDir
		}
		if d.IsDir() {
			nodes[p] = n
		}
		return nil
	})
	sortTree(root)
	return root, nodes
}

// sortTree orders directories first, then by name ignoring case, as GitHub
// does.
func sortTree(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		sortTree(c)
	}
}

// Subtree returns a directory under the root with its entries, but not
// theirs: a directory among them is Lazy. It is how the entries of a
// directory left out of the tree are read.
func (w *Workspace) Subtree(rel string) (*Node, error) {
	rel = path.Clean(rel)
	if rel == "." || !fs.ValidPath(rel) {
		return nil, fs.ErrNotExist
	}
	entries, err := fs.ReadDir(w.root.FS(), rel)
	if err != nil {
		return nil, err
	}
	snap := w.Snapshot()
	n := &Node{Name: path.Base(rel), Path: rel, Dir: true, Ignored: snap.Ignored(rel)}
	for _, e := range entries {
		if w.Skipped(e.Name()) {
			continue
		}
		p := path.Join(rel, e.Name())
		n.Children = append(n.Children, &Node{
			Name: e.Name(), Path: p, Dir: e.IsDir(), Ignored: snap.Ignored(p), Lazy: e.IsDir(),
		})
	}
	sortTree(n)
	return n, nil
}
