package workspace

import (
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// Watcher follows a workspace's files, its git HEAD and a themes directory.
type Watcher struct {
	fw   *fsnotify.Watcher
	done chan struct{}
}

// Watch follows every directory under the workspace's root except skipped
// ones, the git directory's HEAD, and themesDir when it is not empty.
// After each burst of changes it invalidates the workspace's snapshot and
// then calls the handlers in order, so a handler sees the new state.
func Watch(ws *Workspace, themesDir string, handlers ...func(Event)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{fw: fw, done: make(chan struct{})}
	root := ws.opts.Root
	w.addTree(ws, root)
	gitDir := gitDirOf(root)
	if gitDir != "" {
		// HEAD is replaced by a rename, so follow the directory holding it
		if err := fw.Add(gitDir); err != nil {
			log.Printf("watch %s: %v", gitDir, err)
			gitDir = ""
		}
	}
	if themesDir != "" {
		if _, err := os.Stat(themesDir); err == nil {
			_ = fw.Add(themesDir)
		} else {
			themesDir = ""
		}
	}
	ws.watched.Store(true)
	go w.loop(ws, gitDir, themesDir, handlers)
	return w, nil
}

// Close stops watching and waits until no handler is running.
func (w *Watcher) Close() error {
	err := w.fw.Close()
	<-w.done
	return err
}

func (w *Watcher) addTree(ws *Workspace, dir string) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != dir && ws.Skipped(d.Name()) {
			return fs.SkipDir
		}
		if err := w.fw.Add(p); err != nil {
			log.Printf("watch %s: %v", p, err)
		}
		return nil
	})
}

func (w *Watcher) loop(ws *Workspace, gitDir, themesDir string, handlers []func(Event)) {
	defer close(w.done)
	var (
		pending Event
		seen    = map[string]bool{}
		timer   = time.NewTimer(time.Hour)
	)
	timer.Stop()
	defer timer.Stop()
	root := ws.opts.Root
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
			case gitDir != "" && dir == gitDir:
				// Only HEAD matters; git writes its index and locks here
				// all the time
				if filepath.Base(e.Name) != "HEAD" {
					continue
				}
				pending.GitHead = true
			case themesDir != "" && dir == themesDir:
				pending.Theme = true
			default:
				rel, err := filepath.Rel(root, e.Name)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					continue
				}
				rel = filepath.ToSlash(rel)
				if ws.Skipped(filepath.Base(rel)) {
					continue
				}
				if !seen[rel] {
					seen[rel] = true
					pending.Paths = append(pending.Paths, rel)
				}
				if e.Has(fsnotify.Create) || e.Has(fsnotify.Remove) || e.Has(fsnotify.Rename) ||
					filepath.Base(rel) == ".gitignore" {
					pending.Structure = true
				}
				if e.Has(fsnotify.Create) {
					if fi, err := os.Stat(e.Name); err == nil && fi.IsDir() {
						w.addTree(ws, e.Name)
					}
				}
			}
			timer.Reset(debounce)
		case <-timer.C:
			ws.Invalidate(Change{Structure: pending.Structure, GitHead: pending.GitHead})
			for _, h := range handlers {
				h(pending)
			}
			pending = Event{}
			seen = map[string]bool{}
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
