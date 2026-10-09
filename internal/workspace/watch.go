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
	"slices"
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
	// GitHead is set when HEAD moved, as when switching branches or
	// committing, or a branch of origin did, as a fetch or a push moves
	// one.
	GitHead bool
	// Status is set when what changed since the last commit is not what
	// it was: a file edited, staged, committed or restored.
	Status bool
	// Theme is set when a file in the themes directory changed.
	Theme bool
	// Plugin is set when a file of the viewer's plugins changed, or one
	// was added or removed.
	Plugin bool
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

// Watcher follows a workspace's files, its git HEAD and index, and a themes
// directory.
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
	// commonDir is the git directory a worktree shares with its
	// repository, where the refs are; the git directory itself outside a
	// worktree
	commonDir string
	// ignores are the directories outside the root holding what tells git
	// what to ignore in it, each with the file names that do: the
	// .gitignore of the directories between the repository's top and the
	// root, and info/exclude
	ignores   map[string]string
	themesDir string
	// themesOn is set while themesDir is watched; it may be created or
	// removed while the server runs
	themesOn atomic.Bool
	// pmu guards pluginDirs, the directories of the viewer's plugins
	// watched, as events name them
	pmu        sync.Mutex
	pluginDirs map[string]bool
	handlers   []func(Event)
	stop       chan struct{}
	wg         sync.WaitGroup
	// head is HEAD's commit as last told, so that a commit, which moves
	// HEAD without writing HEAD itself, is told as HEAD moving; only the
	// flushing goroutine uses it
	head string

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
	gitIndex    bool
	// headLog is set when logs/HEAD was written, as every move of HEAD
	// writes it, and some writes that move nothing
	headLog   bool
	theme     bool
	plugin    bool
	gitignore bool
	resync    bool
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

		pluginDirs: map[string]bool{},
	}
	root := ws.opts.Root
	// Mark the workspace watched first, so that the snapshot built here
	// is kept rather than built again on the next request
	ws.watched.Store(true)
	snap := ws.Snapshot()
	w.head = gitOutput(root, "rev-parse", "-q", "--verify", "HEAD")
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
		// a worktree's is elsewhere. HEAD and the index are replaced by a
		// rename, so the directory holding them is followed, and logs,
		// where every commit is added to logs/HEAD
		inside := w.recursive && strings.HasPrefix(gitDir, root+string(filepath.Separator))
		if inside {
			w.gitDir = gitDir
		} else if err := fw.Add(gitDir, fswatcher.All); err != nil {
			log.Printf("watch %s: %v", gitDir, err)
		} else {
			w.gitDir = gitDir
			if err := fw.Add(filepath.Join(gitDir, "logs"), fswatcher.All); err != nil && !errors.Is(err, fs.ErrNotExist) {
				log.Printf("watch %s: %v", filepath.Join(gitDir, "logs"), err)
			}
		}
	}
	if w.gitDir != "" {
		w.watchRefs(root)
	}
	w.ignores = ignoreDirs(root)
	for dir := range w.ignores {
		// The git directory may be in a root watched whole
		if w.recursive && strings.HasPrefix(dir, root+string(filepath.Separator)) {
			continue
		}
		if err := fw.Add(dir, fswatcher.All); err != nil {
			log.Printf("watch %s: %v", dir, err)
			delete(w.ignores, dir)
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

// WatchPlugins watches the directories of the viewer's plugins, those
// given and the ones in them, those not watched yet. The server calls it on
// every page with the plugins directory and each plugin's, so one added is
// followed from the next page on. A plugin linked from elsewhere is
// watched where it is; hidden directories and node_modules are not.
func (w *Watcher) WatchPlugins(dirs []string) {
	for _, top := range dirs {
		top = resolve(top)
		_ = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if p != top && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return fs.SkipDir
			}
			w.watchPluginDir(p)
			return nil
		})
	}
}

func (w *Watcher) watchPluginDir(dir string) {
	w.pmu.Lock()
	known := w.pluginDirs[dir]
	w.pmu.Unlock()
	if known {
		return
	}
	root := w.ws.opts.Root
	// A directory in a root watched whole is watched already, and one the
	// root's watch holds for good too
	covered := w.recursive && (dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)))
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	if !covered && !w.fixed[dir] {
		// A directory removed and made again is a new one to the watcher
		_ = w.fw.Remove(dir)
		if err := w.fw.Add(dir, fswatcher.All); err != nil && !errors.Is(err, fswatcher.ErrAlreadyAdded) {
			w.mu.Unlock()
			return
		}
	}
	w.mu.Unlock()
	w.pmu.Lock()
	w.pluginDirs[dir] = true
	w.pmu.Unlock()
}

// pluginChanged tells whether an event is of a plugin's file or
// directory. A directory gone is forgotten, to be watched again if made
// again.
func (w *Watcher) pluginChanged(name, dir string) bool {
	w.pmu.Lock()
	defer w.pmu.Unlock()
	if w.pluginDirs[name] {
		if _, err := os.Stat(name); err != nil {
			delete(w.pluginDirs, name)
		}
		return true
	}
	return w.pluginDirs[dir]
}

// watchRefs follows the refs a fetch or a push moves, origin's branches,
// which tell where the branch stands against its base. They are in the
// common git directory, which a worktree shares with its repository. A
// fetch writes FETCH_HEAD there and a pull or a push may write
// packed-refs or refs/remotes/origin/<name>; elsewhere than on macOS a
// watch is not recursive, so a branch of origin named with a slash, as
// origin/feature/x, is not followed, and is read again on the next page.
func (w *Watcher) watchRefs(root string) {
	common := gitOutput(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if common == "" {
		return
	}
	if real, err := filepath.EvalSymlinks(common); err == nil {
		common = real
	}
	w.commonDir = common
	// The git directory itself is watched already, and the root on macOS
	// with all under it
	covered := func(dir string) bool {
		return dir == w.gitDir || w.recursive && strings.HasPrefix(dir, root+string(filepath.Separator))
	}
	for _, dir := range []string{common, filepath.Join(common, "refs", "remotes", "origin")} {
		if covered(dir) {
			continue
		}
		if err := w.fw.Add(dir, fswatcher.All); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("watch %s: %v", dir, err)
		}
	}
}

// refMoved tells whether a path in the common git directory is one a
// fetch, a pull or a push writes when a branch of origin moves.
func (w *Watcher) refMoved(name string) bool {
	switch name {
	case filepath.Join(w.commonDir, "FETCH_HEAD"), filepath.Join(w.commonDir, "packed-refs"):
		return true
	}
	return strings.HasPrefix(name, filepath.Join(w.commonDir, "refs", "remotes")+string(filepath.Separator))
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
	// The plugins directory may be in the root, where what changed in it
	// is a change of the root's too
	if w.pluginChanged(e.Name, dir) {
		b.plugin = true
		b.touch(time.Now())
	}
	switch {
	case w.ignores[dir] != "":
		// What git ignores in the root changes with these files
		if filepath.Base(e.Name) != w.ignores[dir] {
			return
		}
		b.gitignore = true
	case w.gitDir != "" && (e.Name == w.gitDir || strings.HasPrefix(e.Name, w.gitDir+string(filepath.Separator))):
		// Only HEAD, the index and the log of HEAD matter; git writes
		// locks and objects here all the time
		switch {
		case e.Name == filepath.Join(w.gitDir, "HEAD"):
			b.gitHead = true
		case e.Name == filepath.Join(w.gitDir, "index"):
			b.gitIndex = true
		case e.Name == filepath.Join(w.gitDir, "logs", "HEAD"):
			b.gitIndex, b.headLog = true, true
		case w.refMoved(e.Name):
			b.gitHead = true
		default:
			return
		}
	case w.commonDir != w.gitDir && (e.Name == w.commonDir || strings.HasPrefix(e.Name, w.commonDir+string(filepath.Separator))):
		if !w.refMoved(e.Name) {
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
	if b.headLog || b.gitHead || b.resync {
		if head := gitOutput(w.ws.opts.Root, "rev-parse", "-q", "--verify", "HEAD"); head != w.head {
			w.head = head
			b.gitHead = true
		}
	}
	snap := w.ws.Snapshot()
	structure := b.gitignore || b.resync || w.structureChanged(snap, b.paths)
	// A change under a directory git ignores changes no file git sees
	status := structure || b.gitHead || b.gitIndex || b.resync || slices.ContainsFunc(b.paths, func(p string) bool { return !snap.Ignored(p) })
	w.ws.Invalidate(Change{Structure: structure, GitHead: b.gitHead || b.resync, Status: status})
	if structure && !w.recursive {
		// Watch new directories, and those no longer ignored, by what git
		// ignores now
		w.addTree(w.ws.opts.Root, w.ws.Snapshot())
	}
	e := Event{Structure: structure, GitHead: b.gitHead, Theme: b.theme, Plugin: b.plugin, Resync: b.resync}
	// git writes its index for other reasons too, such as a status run
	// by an editor: only a change of status is told
	if status {
		now := w.ws.Snapshot()
		// What changed since the base changes alone when the base is
		// merged, or a commit is amended with nothing left uncommitted
		e.Status = now.StatusETag != snap.StatusETag || now.BaseStatusETag != snap.BaseStatusETag
	}
	e.Paths, e.Dirs = limitPaths(snap, b.paths)
	if len(e.Paths) == 0 && len(e.Dirs) == 0 && !e.Structure && !e.GitHead && !e.Status && !e.Theme && !e.Plugin && !e.Resync {
		return
	}
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
		// The tree lists no entries of an ignored directory
		if parent := path.Dir(rel); parent == "." || !snap.Ignored(parent) {
			dirs[parent] = true
		}
		if fi, err := os.Lstat(filepath.Join(w.ws.opts.Root, filepath.FromSlash(rel))); err == nil && fi.IsDir() && !snap.Ignored(rel) {
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

// ignoreDirs finds the directories outside root whose files tell git what
// to ignore in it, each with the name of that file: the directories from
// the repository's top to root's parent, with their .gitignore, and that
// of info/exclude. Paths have symlinks resolved, as events give them.
func ignoreDirs(root string) map[string]string {
	dirs := map[string]string{}
	top := gitOutput(root, "rev-parse", "--show-toplevel")
	if top == "" {
		return dirs
	}
	if real, err := filepath.EvalSymlinks(top); err == nil {
		top = real
	}
	if strings.HasPrefix(root, top+string(filepath.Separator)) {
		for d := filepath.Dir(root); ; d = filepath.Dir(d) {
			dirs[d] = ".gitignore"
			if d == top {
				break
			}
		}
	}
	// A worktree shares the exclude file of its repository's git directory
	if exclude := gitOutput(root, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude"); exclude != "" {
		dir := filepath.Dir(exclude)
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		if _, err := os.Stat(dir); err == nil {
			dirs[dir] = filepath.Base(exclude)
		}
	}
	return dirs
}
