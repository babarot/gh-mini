package workspace

import (
	"container/list"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event is what changed in a short burst of file changes.
type Event struct {
	// Paths are the changed paths under the root, slash-separated.
	Paths []string
	// Structure is set when files were added, removed or renamed, or a
	// .gitignore changed.
	Structure bool
	// GitHead is set when HEAD moved, as when switching branches.
	GitHead bool
	// Theme is set when a file in the themes directory changed.
	Theme bool
}

// debounce is how long the watcher waits for a burst of changes to end.
const debounce = 150 * time.Millisecond

// maxViewedCost bounds the directories WatchDir adds, counted as the
// entries in them. On macOS, watching a directory opens a file descriptor
// for every file in it.
const maxViewedCost = 2000

// Watcher follows a workspace's files, its git HEAD and a themes directory.
//
// Directories git ignores, often build output or a tool's state with many
// files, are left out; WatchDir adds one while it is being looked at.
type Watcher struct {
	fw        *fsnotify.Watcher
	ws        *Workspace
	gitDir    string
	themesDir string
	done      chan struct{}

	// mu guards the sets below, and the fsnotify calls that change them.
	// It is never held while building a snapshot.
	mu     sync.Mutex
	closed bool
	// fixed are the directories watched for good: those not ignored.
	fixed map[string]bool
	// viewed are the ignored directories WatchDir added, most recently
	// used first, each costing its number of entries.
	viewed     *list.List
	viewedAt   map[string]*list.Element
	viewedCost int
	maxCost    int
	tooBig     map[string]bool
}

type viewedDir struct {
	path string
	cost int
}

// Watch follows every directory under the workspace's root except skipped
// and ignored ones, the git directory's HEAD, and themesDir when it is not
// empty. After each burst of changes it invalidates the workspace's
// snapshot and then calls the handlers in order, so a handler sees the new
// state.
func Watch(ws *Workspace, themesDir string, handlers ...func(Event)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fw:       fw,
		ws:       ws,
		done:     make(chan struct{}),
		fixed:    map[string]bool{},
		viewed:   list.New(),
		viewedAt: map[string]*list.Element{},
		maxCost:  maxViewedCost,
		tooBig:   map[string]bool{},
	}
	root := ws.opts.Root
	// Mark the workspace watched first, so that the snapshot built here
	// is kept rather than built again on the next request
	ws.watched.Store(true)
	w.addTree(root, ws.Snapshot())
	if gitDir := gitDirOf(root); gitDir != "" {
		// HEAD is replaced by a rename, so follow the directory holding it
		if err := fw.Add(gitDir); err != nil {
			log.Printf("watch %s: %v", gitDir, err)
		} else {
			w.gitDir = gitDir
		}
	}
	if themesDir != "" {
		if _, err := os.Stat(themesDir); err == nil && fw.Add(themesDir) == nil {
			w.themesDir = themesDir
		}
	}
	go w.loop(handlers)
	return w, nil
}

// Close stops watching and waits until no handler is running.
func (w *Watcher) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	err := w.fw.Close()
	<-w.done
	return err
}

// rel returns a path under the root relative to it, slash-separated, or
// false for a path outside the root.
func (w *Watcher) rel(p string) (string, bool) {
	rel, err := filepath.Rel(w.ws.opts.Root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// addTree watches dir and the directories under it, leaving out skipped
// ones and those snap says git ignores.
func (w *Watcher) addTree(dir string, snap *Snapshot) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != dir && w.ws.Skipped(d.Name()) {
			return fs.SkipDir
		}
		rel, ok := w.rel(p)
		if !ok {
			return fs.SkipDir
		}
		if snap.Ignored(rel) {
			return fs.SkipDir
		}
		w.addFixed(p)
		return nil
	})
}

func (w *Watcher) addFixed(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	// A directory that was viewed while ignored is now watched for good
	if e := w.viewedAt[dir]; e != nil {
		w.viewedCost -= e.Value.(*viewedDir).cost
		w.viewed.Remove(e)
		delete(w.viewedAt, dir)
	}
	w.fixed[dir] = true
	if err := w.fw.Add(dir); err != nil {
		log.Printf("watch %s: %v", dir, err)
	}
}

// forget drops what was known about a removed directory and the ones under
// it; fsnotify already stopped watching them.
func (w *Watcher) forget(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	under := func(p string) bool {
		return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
	}
	for p := range w.fixed {
		if under(p) {
			delete(w.fixed, p)
		}
	}
	for p, e := range w.viewedAt {
		if under(p) {
			w.viewedCost -= e.Value.(*viewedDir).cost
			w.viewed.Remove(e)
			delete(w.viewedAt, p)
		}
	}
}

// WatchDir watches one directory under the root, not the ones under it,
// for as long as the directories it added since stay within a budget. The
// server calls it for a page under a directory git ignores, so that the
// page still reloads. It returns once the directory is watched.
func (w *Watcher) WatchDir(rel string) {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return
	}
	for _, name := range strings.Split(rel, "/") {
		if w.ws.Skipped(name) {
			return
		}
	}
	dir := filepath.Join(w.ws.opts.Root, filepath.FromSlash(rel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cost := len(entries) + 1

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.fixed[dir] {
		return
	}
	if e := w.viewedAt[dir]; e != nil {
		w.viewed.MoveToFront(e)
		return
	}
	if cost > w.maxCost {
		if !w.tooBig[dir] {
			w.tooBig[dir] = true
			log.Printf("watch %s: too many files (%d) to follow changes", rel, len(entries))
		}
		return
	}
	for w.viewedCost+cost > w.maxCost {
		e := w.viewed.Back()
		v := e.Value.(*viewedDir)
		_ = w.fw.Remove(v.path)
		w.viewedCost -= v.cost
		w.viewed.Remove(e)
		delete(w.viewedAt, v.path)
	}
	if err := w.fw.Add(dir); err != nil {
		if err != fsnotify.ErrClosed {
			log.Printf("watch %s: %v", dir, err)
		}
		return
	}
	w.viewedAt[dir] = w.viewed.PushFront(&viewedDir{path: dir, cost: cost})
	w.viewedCost += cost
}

func (w *Watcher) loop(handlers []func(Event)) {
	defer close(w.done)
	var (
		pending   Event
		seen      = map[string]bool{}
		newDirs   []string
		gitignore bool
		timer     = time.NewTimer(time.Hour)
	)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Printf("watch: %v", err)
		case e, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if e.Has(fsnotify.Chmod) && !e.Has(fsnotify.Write) {
				continue
			}
			dir := filepath.Dir(e.Name)
			switch {
			case w.gitDir != "" && dir == w.gitDir:
				// Only HEAD matters; git writes its index and locks here
				// all the time
				if filepath.Base(e.Name) != "HEAD" {
					continue
				}
				pending.GitHead = true
			case w.themesDir != "" && dir == w.themesDir:
				pending.Theme = true
			default:
				rel, ok := w.rel(e.Name)
				if !ok || w.ws.Skipped(filepath.Base(rel)) {
					continue
				}
				if !seen[rel] {
					seen[rel] = true
					pending.Paths = append(pending.Paths, rel)
				}
				if e.Has(fsnotify.Create) || e.Has(fsnotify.Remove) || e.Has(fsnotify.Rename) {
					pending.Structure = true
				}
				if filepath.Base(rel) == ".gitignore" {
					pending.Structure = true
					gitignore = true
				}
				if e.Has(fsnotify.Remove) || e.Has(fsnotify.Rename) {
					w.forget(e.Name)
				}
				if e.Has(fsnotify.Create) {
					newDirs = append(newDirs, e.Name)
				}
			}
			timer.Reset(debounce)
		case <-timer.C:
			w.ws.Invalidate(Change{Structure: pending.Structure, GitHead: pending.GitHead})
			// Watch new directories, and those no longer ignored, by what
			// git ignores now
			if gitignore || len(newDirs) > 0 {
				snap := w.ws.Snapshot()
				if gitignore {
					w.addTree(w.ws.opts.Root, snap)
				} else {
					for _, d := range newDirs {
						if fi, err := os.Stat(d); err == nil && fi.IsDir() {
							w.addTree(d, snap)
						}
					}
				}
			}
			for _, h := range handlers {
				h(pending)
			}
			pending = Event{}
			seen = map[string]bool{}
			newDirs = nil
			gitignore = false
		}
	}
}

// gitDirOf returns the git directory of the repository dir is in, with
// symlinks resolved, or "" outside a repository. A worktree's git
// directory is outside the worktree, so ask git rather than join ".git".
func gitDirOf(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return ""
	}
	p, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return ""
	}
	return p
}
