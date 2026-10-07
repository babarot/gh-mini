package server

import (
	"bytes"
	"io/fs"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Node is a file or directory in the sidebar tree.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Dir      bool    `json:"dir,omitempty"`
	Ignored  bool    `json:"ignored,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

// tree returns the cached tree, rebuilding it when the watcher saw files
// added or removed, or every time when there is no watcher.
func (s *Server) tree() *Node {
	s.treeMu.Lock()
	defer s.treeMu.Unlock()
	if s.cachedTree == nil || s.treeDirty || !s.opts.Reload {
		s.ignored = gitIgnored(s.opts.Root)
		s.cachedTree = s.buildTree()
		s.treeDirty = false
	}
	return s.cachedTree
}

func (s *Server) invalidateTree() {
	s.treeMu.Lock()
	s.treeDirty = true
	s.treeMu.Unlock()
}

func (s *Server) isIgnored(rel string) bool {
	s.treeMu.Lock()
	defer s.treeMu.Unlock()
	return s.ignoredLocked(rel)
}

func (s *Server) ignoredLocked(rel string) bool {
	for p := rel; p != "." && p != "/" && p != ""; p = path.Dir(p) {
		if s.ignored[p] {
			return true
		}
	}
	return false
}

func (s *Server) skipped(name string) bool {
	for _, n := range s.opts.Skip {
		if n == name {
			return true
		}
	}
	return false
}

func (s *Server) buildTree() *Node {
	root := &Node{Name: s.opts.Name, Dir: true}
	nodes := map[string]*Node{".": root}
	fsys := s.root.FS()
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return nil
		}
		if s.skipped(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		parent := nodes[path.Dir(p)]
		if parent == nil {
			return nil
		}
		n := &Node{Name: d.Name(), Path: p, Dir: d.IsDir(), Ignored: s.ignoredLocked(p)}
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

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	case d < 365*24*time.Hour:
		return plural(int(d.Hours()/24/30), "month")
	default:
		return plural(int(d.Hours()/24/365), "year")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return strconv.Itoa(n) + " " + unit + "s ago"
}
