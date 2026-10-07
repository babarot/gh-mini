// Package workspace holds the state of the directory being served: its
// file tree, what git ignores in it, its branch and its GitHub repository.
// The state is read as immutable snapshots, rebuilt only for what changed.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
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
}

// Change tells which parts of a snapshot are out of date.
type Change struct {
	// Structure is set when files were added, removed or renamed, or a
	// .gitignore changed: the tree and what git ignores.
	Structure bool
	// GitHead is set when HEAD moved: the branch.
	GitHead bool
}

const (
	dirtyStructure uint32 = 1 << iota
	dirtyGitHead
	dirtyAll = dirtyStructure | dirtyGitHead
)

// Workspace is one directory being served.
type Workspace struct {
	opts Options
	root *os.Root
	repo string

	mu    sync.Mutex // serializes rebuilds
	snap  atomic.Pointer[Snapshot]
	dirty atomic.Uint32
	// watched is set once a Watcher invalidates the snapshot on changes.
	// Until then, every Snapshot is rebuilt from scratch.
	watched atomic.Bool
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
	Repo    string
	ignored map[string]bool
	built   time.Time
}

// Open opens the directory.
func Open(opts Options) (*Workspace, error) {
	root, err := os.OpenRoot(opts.Root)
	if err != nil {
		return nil, err
	}
	w := &Workspace{opts: opts, root: root, repo: gitHubRepo(opts.Root)}
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
	w.dirty.Or(bits)
}

// Snapshot returns the current state, rebuilding the parts that changed.
func (w *Workspace) Snapshot() *Snapshot {
	if !w.watched.Load() {
		if s := w.snap.Load(); s == nil || time.Since(s.built) >= w.opts.MaxAge {
			w.dirty.Store(dirtyAll)
		}
	}
	if s := w.snap.Load(); s != nil && w.dirty.Load() == 0 {
		return s
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	prev := w.snap.Load()
	// Another request may have rebuilt it while this one waited
	bits := w.dirty.Swap(0)
	if prev != nil && bits == 0 {
		return prev
	}
	next := &Snapshot{Repo: w.repo}
	if prev != nil {
		*next = *prev
	}
	next.Version++
	next.built = time.Now()
	if prev == nil || bits&dirtyStructure != 0 {
		next.ignored = gitIgnored(w.opts.Root)
		next.Tree = w.buildTree(next.ignored)
		next.TreeJSON, _ = json.Marshal(next.Tree)
		sum := sha256.Sum256(next.TreeJSON)
		next.TreeETag = `"` + hex.EncodeToString(sum[:8]) + `"`
	}
	if prev == nil || bits&dirtyGitHead != 0 {
		next.Branch = gitBranch(w.opts.Root)
	}
	w.snap.Store(next)
	return next
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
