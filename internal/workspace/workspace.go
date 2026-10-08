// Package workspace holds the state of the directory being served: its
// file tree, what git ignores in it, what changed since the last commit,
// its branch and its GitHub repository.
// The state is read as immutable snapshots, rebuilt only for what changed.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures a Workspace.
type Options struct {
	// Root is the absolute path of the directory, with symlinks resolved.
	Root string
	// Name is the name of the tree's root node.
	Name string
	// Skip lists file and directory names left out of the tree.
	Skip []string
	// MaxAge is how long a snapshot is used when no Watcher invalidates
	// it. Zero rebuilds it every time.
	MaxAge time.Duration
	// NoStatus leaves out what changed since the last commit, which is
	// then never read from git.
	NoStatus bool
}

// Change tells which parts of a snapshot are out of date.
type Change struct {
	// Structure is set when files were added, removed or renamed, or a
	// .gitignore changed: the tree and what git ignores.
	Structure bool
	// GitHead is set when HEAD moved: the branch.
	GitHead bool
	// Status is set when files or git's index may have changed: what
	// changed since the last commit.
	Status bool
}

const (
	dirtyStructure uint32 = 1 << iota
	dirtyGitHead
	dirtyStatus
	dirtyAll = dirtyStructure | dirtyGitHead | dirtyStatus
)

// Workspace is one directory being served.
type Workspace struct {
	opts Options
	root *os.Root
	repo string
	// git is set in a git repository, where prefix is where the root is
	// in it, such as "docs/"
	git    bool
	prefix string

	mu    sync.Mutex // serializes rebuilds
	snap  atomic.Pointer[Snapshot]
	dirty atomic.Uint32
	// gen counts invalidations. A snapshot records the one it was built
	// at, and is current while that is still the count: dirty alone is
	// cleared when a rebuild starts, and a request then would take the
	// old snapshot instead of waiting for the new one
	gen atomic.Uint64
	// watched is set once a Watcher invalidates the snapshot on changes.
	// Until then, every Snapshot is rebuilt from scratch.
	watched atomic.Bool
	commits commitCache
}

// Snapshot is the state of a workspace at one time. It is never modified,
// so it can be read from many requests at once.
type Snapshot struct {
	// Version grows every time a snapshot is rebuilt.
	Version uint64
	Tree    *Node
	// TreeJSON is Tree encoded once, and TreeETag a hash of it, the same
	// for the same tree even across restarts.
	TreeJSON []byte
	TreeETag string
	Branch   string
	// Repo is the GitHub repository of the origin remote, "owner/name", or
	// "" when there is none.
	Repo string
	// Status is what changed since the last commit, StatusJSON it encoded
	// once, and StatusETag a hash of that.
	Status     *Status
	StatusJSON []byte
	StatusETag string
	ignored    map[string]bool
	gen        uint64
	// dirs are the tree's directories by path, "." for the root
	dirs  map[string]*Node
	built time.Time
}

// Open opens the directory.
func Open(opts Options) (*Workspace, error) {
	root, err := os.OpenRoot(opts.Root)
	if err != nil {
		return nil, err
	}
	w := &Workspace{opts: opts, root: root, repo: gitHubRepo(opts.Root)}
	if !opts.NoStatus {
		w.prefix, w.git = gitPrefix(opts.Root)
	}
	w.dirty.Store(dirtyAll)
	return w, nil
}

// Close closes the directory.
func (w *Workspace) Close() error {
	return w.root.Close()
}

// FS reads files under the directory without leaving it.
func (w *Workspace) FS() *os.Root {
	return w.root
}

// Root is the absolute path of the directory.
func (w *Workspace) Root() string {
	return w.opts.Root
}

// Repo is the GitHub repository of the origin remote, read when the
// workspace was opened.
func (w *Workspace) Repo() string {
	return w.repo
}

// Skipped tells whether a file or directory name is left out of the tree.
func (w *Workspace) Skipped(name string) bool {
	for _, n := range w.opts.Skip {
		if n == name {
			return true
		}
	}
	return false
}

// Invalidate marks parts of the snapshot out of date. The next Snapshot
// rebuilds them.
func (w *Workspace) Invalidate(c Change) {
	var bits uint32
	if c.Structure {
		bits |= dirtyStructure
	}
	if c.GitHead {
		bits |= dirtyGitHead
	}
	if c.Status {
		bits |= dirtyStatus
	}
	w.dirty.Or(bits)
	w.gen.Add(1)
}

// Snapshot returns the current state, rebuilding the parts that changed.
func (w *Workspace) Snapshot() *Snapshot {
	if !w.watched.Load() {
		if s := w.snap.Load(); s == nil || time.Since(s.built) >= w.opts.MaxAge {
			w.Invalidate(Change{Structure: true, GitHead: true, Status: true})
		}
	}
	if s := w.snap.Load(); s != nil && s.gen == w.gen.Load() {
		return s
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	prev := w.snap.Load()
	// The count is read before the parts to rebuild: an invalidation in
	// between leaves the new snapshot out of date, to be rebuilt again
	gen := w.gen.Load()
	// Another request may have rebuilt it while this one waited
	if prev != nil && prev.gen == gen {
		return prev
	}
	bits := w.dirty.Swap(0)
	next := &Snapshot{Repo: w.repo}
	if prev != nil {
		*next = *prev
	}
	next.gen = gen
	if prev != nil && bits == 0 {
		w.snap.Store(next)
		return next
	}
	next.Version++
	next.built = time.Now()
	if prev == nil || bits&dirtyStructure != 0 {
		next.ignored, next.Tree, next.dirs = w.ignoredTree()
		next.TreeJSON, _ = json.Marshal(next.Tree)
		sum := sha256.Sum256(next.TreeJSON)
		next.TreeETag = `"` + hex.EncodeToString(sum[:8]) + `"`
	}
	if prev == nil || bits&dirtyGitHead != 0 {
		next.Branch = gitBranch(w.opts.Root)
	}
	// Files added or removed change it too, and the tree it reads
	if prev == nil || bits&(dirtyStatus|dirtyStructure|dirtyGitHead) != 0 {
		next.Status = w.status(next)
		next.StatusJSON, _ = json.Marshal(next.Status)
		sum := sha256.Sum256(next.StatusJSON)
		next.StatusETag = `"` + hex.EncodeToString(sum[:8]) + `"`
	}
	w.snap.Store(next)
	return next
}

// ignoredTree builds the tree with what git ignores in it. git tells what
// one repository ignores, and not what those in it do, such as the build
// output of a repository cloned into another: so each found in the tree
// is asked too, and the tree built again with what they ignore left out.
func (w *Workspace) ignoredTree() (map[string]bool, *Node, map[string]*Node) {
	ignored := gitIgnored(w.opts.Root)
	asked := map[string]bool{}
	for {
		tree, dirs, repos := w.buildTree(ignored)
		grown := false
		for _, r := range repos {
			if asked[r] {
				continue
			}
			asked[r] = true
			for p := range gitIgnored(filepath.Join(w.opts.Root, filepath.FromSlash(r))) {
				ignored[r+"/"+p] = true
				grown = true
			}
		}
		if !grown {
			return ignored, tree, dirs
		}
	}
}

func (w *Workspace) status(s *Snapshot) *Status {
	if !w.git {
		return &Status{Files: map[string]*FileStatus{}}
	}
	return gitStatus(w.opts.Root, w.prefix, w.opts.Skip, s.untrackedFiles)
}

// Ignored tells whether git ignores a path relative to the root, or one
// of its parent directories.
func (s *Snapshot) Ignored(rel string) bool {
	for p := rel; p != "." && p != "/" && p != ""; p = path.Dir(p) {
		if s.ignored[p] {
			return true
		}
	}
	return false
}

// childNames returns the names in a directory of the tree, and false when
// the tree has no such directory.
func (s *Snapshot) childNames(rel string) (map[string]bool, bool) {
	n, ok := s.dirs[rel]
	if !ok {
		return nil, false
	}
	names := make(map[string]bool, len(n.Children))
	for _, c := range n.Children {
		names[c.Name] = true
	}
	return names, true
}
