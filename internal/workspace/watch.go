package workspace

import (
	"container/list"
	"errors"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fswatcher/fswatcher"
)

// Event is what changed in a short burst of file changes.
type Event struct {
	// Paths are the changed paths under the root, slash-separated.
	Paths []string
	// Dirs are directories where more changed than Paths lists: under a
	// directory git ignores, such as a build's output, only the first
	// paths are kept, and the rest are told by their directory.
	Dirs []string
	// Structure is set when files were added, removed or renamed, or a
	// .gitignore changed.
	Structure bool
	// GitHead is set when HEAD moved, as when switching branches.
	GitHead bool
	// Theme is set when a file in the themes directory changed.
	Theme bool
	// Resync is set when changes may have been missed, so that pages and
	// the tree are read again.
	Resync bool
}

const (
	// debounce is how long the watcher waits for a burst of changes to
	// end, and maxWait the longest a burst runs before it is told anyway,
	// so that a stream of writes, say a build's, does not hold back the
	// rest.
	debounce = 150 * time.Millisecond
	maxWait  = time.Second
	// maxIgnoredPaths is how many paths under ignored directories one
	// burst lists; the rest go by directory.
	maxIgnoredPaths = 500
)

// recursiveWatch is whether one recursive watch covers the root; tests
// turn it off to run the other way on macOS too.
var recursiveWatch = runtime.GOOS == "darwin"

// maxViewedCost bounds the directories WatchDir adds, counted as the
// entries in them; kqueue opens a file descriptor for every entry.
const maxViewedCost = 2000

// Watcher follows a workspace's files, its git HEAD and a themes directory.
//
// On macOS one recursive watch covers the whole root: FSEvents follows a
// tree without a file descriptor per entry. Elsewhere a watch costs one
// per directory, or per entry with kqueue, so directories git ignores,
// often build output or a tool's state with many files, are left out, and
// WatchDir adds one while it is being looked at.
type Watcher struct {
	fw        *fswatcher.Watcher
	ws        *Workspace
	recursive bool
	gitDir    string
	themesDir string
	// themesOn is set while themesDir is watched; it may be created or
	// removed while the server runs
	themesOn atomic.Bool
	handlers []func(Event)
	stop     chan struct{}
	wg       sync.WaitGroup

	// bmu guards the burst gathered by the reading goroutine and taken by
	// the flushing one, so that reading never waits for a flush
	bmu   sync.Mutex
	burst burst

	// mu guards the sets below, and the watch calls that change them.
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

// burst gathers changes until they are told.
type burst struct {
	first, last time.Time
	paths       []string
	seen        map[string]bool
	gitHead     bool
	theme       bool
	gitignore   bool
	resync      bool
}

func (b *burst) empty() bool { return b.first.IsZero() }

func (b *burst) touch(now time.Time) {
	if b.first.IsZero() {
		b.first = now
	}
	b.last = now
}

// Watch follows the workspace's root, the git directory's HEAD, and
// themesDir when it is not empty. After each burst of changes it
// invalidates the workspace's snapshot and then calls the handlers in
// order, so a handler sees the new state.
func Watch(ws *Workspace, themesDir string, handlers ...func(Event)) (*Watcher, error) {
	fw, err := fswatcher.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fw:        fw,
		ws:        ws,
		recursive: recursiveWatch,
		handlers:  handlers,
		stop:      make(chan struct{}),
		fixed:     map[string]bool{},
		viewed:    list.New(),
		viewedAt:  map[string]*list.Element{},
		maxCost:   maxViewedCost,
		tooBig:    map[string]bool{},
	}
	root := ws.opts.Root
	// Mark the workspace watched first, so that the snapshot built here
	// is kept rather than built again on the next request
	ws.watched.Store(true)
	snap := ws.Snapshot()
	if w.recursive {
		if err := fw.AddRecursive(root, fswatcher.All); err != nil {
			fw.Close()
			return nil, err
		}
	} else {
		w.addTree(root, snap)
	}
	if gitDir := gitDirOf(root); gitDir != "" {
		// A git directory inside a recursively watched root is covered;
		// a worktree's is elsewhere. HEAD is replaced by a rename, so the
		// directory holding it is followed
		inside := w.recursive && strings.HasPrefix(gitDir, root+string(filepath.Separator))
		if inside {
			w.gitDir = gitDir
		} else if err := fw.Add(gitDir, fswatcher.All); err != nil {
			log.Printf("watch %s: %v", gitDir, err)
		} else {
			w.gitDir = gitDir
		}
	}
	if themesDir != "" {
		// Events come with symlinks resolved, /var as /private/var on
		// macOS, so the directory is compared in that form
		w.themesDir = resolve(themesDir)
	}
	w.WatchThemes()
	w.wg.Add(2)
	go w.read()
	go w.flushLoop()
	return w, nil
}

// Close stops watching and waits until no handler is running.
func (w *Watcher) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	err := w.fw.Close()
	close(w.stop)
	w.wg.Wait()
	return err
}

// WatchThemes watches the themes directory if it exists and is not watched
// yet. The server calls it on every page, so a directory created after
// starting is followed from the next page on.
func (w *Watcher) WatchThemes() {
	if w.themesDir == "" || w.themesOn.Load() {
		return
	}
	if fi, err := os.Stat(w.themesDir); err != nil || !fi.IsDir() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.themesOn.Load() {
		return
	}
	// A directory removed and made again is a new one to the watcher
	_ = w.fw.Remove(w.themesDir)
	if err := w.fw.Add(w.themesDir, fswatcher.All); err == nil {
		w.themesOn.Store(true)
	}
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

// skipped tells whether any name in a path under the root is skipped.
func (w *Watcher) skipped(rel string) bool {
	for _, name := range strings.Split(rel, "/") {
		if w.ws.Skipped(name) {
			return true
		}
	}
	return false
}

// read takes events off the watcher into the burst, never waiting for a
// burst to be told: FSEvents drops events that are not taken.
func (w *Watcher) read() {
	defer w.wg.Done()
	events, errs := w.fw.Events, w.fw.Errors
	for events != nil || errs != nil {
		select {
		case e, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			w.add(e)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			// Events may have been dropped: read everything again
			log.Printf("watch: %v", err)
			w.markResync()
		}
	}
}

func (w *Watcher) markResync() {
	w.bmu.Lock()
	w.burst.resync = true
	w.burst.touch(time.Now())
	w.bmu.Unlock()
}

func (w *Watcher) add(e fswatcher.Event) {
	if e.Op == fswatcher.Chmod {
		return
	}
	w.bmu.Lock()
	defer w.bmu.Unlock()
	b := &w.burst
	dir := filepath.Dir(e.Name)
	switch {
	case w.gitDir != "" && (e.Name == w.gitDir || strings.HasPrefix(e.Name, w.gitDir+string(filepath.Separator))):
		// Only HEAD matters; git writes its index and locks here all
		// the time
		if dir != w.gitDir || filepath.Base(e.Name) != "HEAD" {
			return
		}
		b.gitHead = true
	case w.themesDir != "" && e.Name == w.themesDir:
		// The watch ends with the directory; WatchThemes starts it again
		if _, err := os.Stat(w.themesDir); err != nil {
			w.themesOn.Store(false)
		}
		return
	case w.themesDir != "" && dir == w.themesDir:
		b.theme = true
	default:
		rel, ok := w.rel(e.Name)
		if !ok || rel == "." || w.skipped(rel) {
			return
		}
		if b.seen == nil {
			b.seen = map[string]bool{}
		}
		if !b.seen[rel] {
			b.seen[rel] = true
			b.paths = append(b.paths, rel)
		}
		if path.Base(rel) == ".gitignore" {
			b.gitignore = true
		}
		if !w.recursive && (e.Op.Has(fswatcher.Remove) || e.Op.Has(fswatcher.Rename)) {
			if _, err := os.Lstat(e.Name); err != nil {
				w.forget(e.Name)
			}
		}
	}
	b.touch(time.Now())
}

// flushLoop tells a burst once it has been quiet for debounce, or has run
// for maxWait.
func (w *Watcher) flushLoop() {
	defer w.wg.Done()
	tick := time.NewTicker(debounce / 3)
	defer tick.Stop()
	for {
		select {
		case <-w.stop:
			return
		case now := <-tick.C:
			w.bmu.Lock()
			due := !w.burst.empty() &&
				(now.Sub(w.burst.last) >= debounce || now.Sub(w.burst.first) >= maxWait)
			var b burst
			if due {
				b = w.burst
				w.burst = burst{}
			}
			w.bmu.Unlock()
			if due {
				w.flush(b)
			}
		}
	}
}

func (w *Watcher) flush(b burst) {
	snap := w.ws.Snapshot()
	structure := b.gitignore || b.resync || w.structureChanged(snap, b.paths)
	w.ws.Invalidate(Change{Structure: structure, GitHead: b.gitHead || b.resync})
	if structure && !w.recursive {
		// Watch new directories, and those no longer ignored, by what git
		// ignores now
		w.addTree(w.ws.opts.Root, w.ws.Snapshot())
	}
	e := Event{Structure: structure, GitHead: b.gitHead, Theme: b.theme, Resync: b.resync}
	e.Paths, e.Dirs = limitPaths(snap, b.paths)
	for _, h := range w.handlers {
		h(e)
	}
}

// structureChanged tells whether files were added or removed, comparing
// the directories the paths are in with the tree. The kind of an event is
// no guide: FSEvents reports what happened to a path over a while, so a
// plain save comes as a creation too, and an editor's save by rename is a
// removal and a creation of a file that stays. Event paths are also those
// of symlinks' targets, so a new link shows only in its directory.
func (w *Watcher) structureChanged(snap *Snapshot, paths []string) bool {
	dirs := map[string]bool{}
	for _, rel := range paths {
		dirs[path.Dir(rel)] = true
		if fi, err := os.Lstat(filepath.Join(w.ws.opts.Root, filepath.FromSlash(rel))); err == nil && fi.IsDir() {
			dirs[rel] = true
		}
	}
	for dir := range dirs {
		if w.dirChanged(snap, dir) {
			return true
		}
	}
	return false
}

// dirChanged tells whether a directory's entries, but for skipped ones,
// differ from the tree's.
func (w *Watcher) dirChanged(snap *Snapshot, dir string) bool {
	inTree, ok := snap.childNames(dir)
	entries, err := os.ReadDir(filepath.Join(w.ws.opts.Root, filepath.FromSlash(dir)))
	if err != nil {
		// Gone: changed if the tree has it
		return ok
	}
	if !ok {
		return true
	}
	n := 0
	for _, e := range entries {
		if w.ws.Skipped(e.Name()) {
			continue
		}
		if !inTree[e.Name()] {
			return true
		}
		n++
	}
	return n != len(inTree)
}

// limitPaths keeps every path git tracks, and the first maxIgnoredPaths
// of those it ignores; the directories of the rest stand for them.
func limitPaths(snap *Snapshot, paths []string) (kept, dirs []string) {
	ignored := 0
	seenDir := map[string]bool{}
	for _, rel := range paths {
		if !snap.Ignored(rel) || ignored < maxIgnoredPaths {
			if snap.Ignored(rel) {
				ignored++
			}
			kept = append(kept, rel)
			continue
		}
		if d := path.Dir(rel); !seenDir[d] {
			seenDir[d] = true
			dirs = append(dirs, d)
		}
	}
	return kept, dirs
}

// addTree watches dir and the directories under it, leaving out skipped
// ones and those snap says git ignores. It is not used with a recursive
// watch.
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
	if w.closed || w.fixed[dir] {
		return
	}
	w.fixed[dir] = true
	// A directory that was viewed while ignored is now watched for good
	if e := w.viewedAt[dir]; e != nil {
		w.viewedCost -= e.Value.(*viewedDir).cost
		w.viewed.Remove(e)
		delete(w.viewedAt, dir)
		return
	}
	if err := w.fw.Add(dir, fswatcher.All); err != nil && !errors.Is(err, fswatcher.ErrAlreadyAdded) {
		log.Printf("watch %s: %v", dir, err)
	}
}

// forget drops a removed directory and the ones under it.
func (w *Watcher) forget(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	under := func(p string) bool {
		return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
	}
	for p := range w.fixed {
		if under(p) {
			delete(w.fixed, p)
			_ = w.fw.Remove(p)
		}
	}
	for p, e := range w.viewedAt {
		if under(p) {
			w.viewedCost -= e.Value.(*viewedDir).cost
			w.viewed.Remove(e)
			delete(w.viewedAt, p)
			_ = w.fw.Remove(p)
		}
	}
}

// WatchDir watches one directory under the root, not the ones under it,
// for as long as the directories it added since stay within a budget. The
// server calls it for a page under a directory git ignores, so that the
// page still reloads. It returns once the directory is watched. With a
// recursive watch, every directory is watched already.
func (w *Watcher) WatchDir(rel string) {
	if w.recursive {
		return
	}
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
	if err := w.fw.Add(dir, fswatcher.All); err != nil && !errors.Is(err, fswatcher.ErrAlreadyAdded) {
		if !errors.Is(err, fswatcher.ErrClosed) {
			log.Printf("watch %s: %v", dir, err)
		}
		return
	}
	w.viewedAt[dir] = w.viewed.PushFront(&viewedDir{path: dir, cost: cost})
	w.viewedCost += cost
}

// resolve resolves the symlinks in p as far as p exists: the directory
// may be created later.
func resolve(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for dir := p; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				real = filepath.Join(real, rest[i])
			}
			return real
		}
		if filepath.Dir(dir) == dir {
			return p
		}
		rest = append(rest, filepath.Base(dir))
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
