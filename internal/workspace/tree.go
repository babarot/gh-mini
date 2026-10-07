package workspace

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Node is a file or directory in the tree.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Dir      bool    `json:"dir,omitempty"`
	Ignored  bool    `json:"ignored,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

func (w *Workspace) buildTree(ignored map[string]bool) *Node {
	snap := &Snapshot{ignored: ignored}
	root := &Node{Name: w.opts.Name, Dir: true}
	nodes := map[string]*Node{".": root}
	_ = fs.WalkDir(w.root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
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
		if d.IsDir() {
			nodes[p] = n
		}
		return nil
	})
	sortTree(root)
	return root
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
