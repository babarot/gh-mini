package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// newRepo makes a committed git repository with git's configuration kept
// apart from the machine's, holding README.md and the files given. A
// non-empty gitignore is written as .gitignore before the commit, so the
// files it matches stay untracked.
func newRepo(t *testing.T, gitignore string, files ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"README.md"}, files...) {
		write(t, filepath.Join(dir, name), "x\n")
	}
	if gitignore != "" {
		write(t, filepath.Join(dir, ".gitignore"), gitignore)
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init")
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func startWatch(t *testing.T, dir string) (*Workspace, <-chan Event) {
	t.Helper()
	ws, w, events := startWatcher(t, dir)
	_ = w
	return ws, events
}

func startWatcher(t *testing.T, dir string) (*Workspace, *Watcher, <-chan Event) {
	t.Helper()
	ws, err := Open(Options{Root: dir, Name: "x", Skip: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 64)
	w, err := Watch(ws, "", func(e Event) { events <- e })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		w.Close()
		ws.Close()
	})
	return ws, w, events
}

func hasPath(p string) func(Event) bool {
	return func(e Event) bool {
		for _, q := range e.Paths {
			if q == p {
				return true
			}
		}
		return false
	}
}

// quiet fails when an event for p comes within a second.
func quiet(t *testing.T, events <-chan Event, p string) {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case e := <-events:
			if hasPath(p)(e) {
				t.Errorf("event for %s: %+v", p, e)
				return
			}
		case <-timeout:
			return
		}
	}
}

func next(t *testing.T, events <-chan Event, ok func(Event) bool) Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e := <-events:
			if ok(e) {
				return e
			}
		case <-timeout:
			t.Fatal("no matching event")
			return Event{}
		}
	}
}

func TestWatchFileChanges(t *testing.T) {
	dir := newRepo(t, "")
	ws, events := startWatch(t, dir)
	if got := names(ws.Snapshot().Tree); len(got) != 1 {
		t.Fatalf("tree = %v", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "new.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e := next(t, events, func(e Event) bool { return e.Structure })
	if len(e.Paths) != 1 || e.Paths[0] != "new.md" {
		t.Errorf("paths = %v", e.Paths)
	}
	// The handler runs after the snapshot was invalidated
	if got := names(ws.Snapshot().Tree); len(got) != 2 {
		t.Errorf("tree after the event = %v", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	next(t, events, func(e Event) bool {
		return len(e.Paths) == 1 && e.Paths[0] == "README.md" && !e.Structure
	})
}

func TestWatchGitHead(t *testing.T) {
	dir := newRepo(t, "")
	ws, events := startWatch(t, dir)
	if b := ws.Snapshot().Branch; b != "main" {
		t.Fatalf("branch = %q", b)
	}
	git(t, dir, "checkout", "-q", "-b", "feature")
	next(t, events, func(e Event) bool { return e.GitHead })
	if b := ws.Snapshot().Branch; b != "feature" {
		t.Errorf("branch after checkout = %q", b)
	}
}

// git writes its index and lock files all the time; they are not changes
// to the tree.
func TestWatchIgnoresGitInternals(t *testing.T) {
	dir := newRepo(t, "")
	_, events := startWatch(t, dir)
	git(t, dir, "update-index", "--refresh")
	git(t, dir, "reset", "-q")
	select {
	case e := <-events:
		t.Errorf("event from the git directory: %+v", e)
	case <-time.After(time.Second):
	}
}

func TestWatcherCloseStopsGoroutines(t *testing.T) {
	dir := t.TempDir()
	before := runtime.NumGoroutine()
	ws, err := Open(Options{Root: dir, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := Watch(ws, "", func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	ws.Close()
	for i := 0; i < 50 && runtime.NumGoroutine() > before; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("goroutines: %d before, %d after Close", before, n)
	}
}

func TestWatchIgnoredDirOnlyWhenViewed(t *testing.T) {
	dir := newRepo(t, "build/\n", "build/out.md")
	_, w, events := startWatcher(t, dir)

	write(t, filepath.Join(dir, "build", "out.md"), "1\n")
	quiet(t, events, "build/out.md")

	w.WatchDir("build")
	write(t, filepath.Join(dir, "build", "out.md"), "2\n")
	next(t, events, hasPath("build/out.md"))
}

func TestWatchDirEvictsLeastRecentlyViewed(t *testing.T) {
	dir := newRepo(t, "build/\n", "build/a/1.md", "build/b/1.md", "build/c/1.md")
	_, w, events := startWatcher(t, dir)
	w.mu.Lock()
	w.maxCost = 4 // two of the directories, at 2 each
	w.mu.Unlock()

	w.WatchDir("build/a")
	w.WatchDir("build/b")
	w.WatchDir("build/a") // a is now the most recent
	w.WatchDir("build/c") // so b goes

	write(t, filepath.Join(dir, "build", "b", "1.md"), "2\n")
	quiet(t, events, "build/b/1.md")
	write(t, filepath.Join(dir, "build", "a", "1.md"), "2\n")
	next(t, events, hasPath("build/a/1.md"))
	write(t, filepath.Join(dir, "build", "c", "1.md"), "2\n")
	next(t, events, hasPath("build/c/1.md"))
}

func TestWatchFollowsDirsNoLongerIgnored(t *testing.T) {
	dir := newRepo(t, "build/\n", "build/out.md")
	_, events := startWatch(t, dir)

	write(t, filepath.Join(dir, ".gitignore"), "\n")
	next(t, events, hasPath(".gitignore"))
	write(t, filepath.Join(dir, "build", "out.md"), "2\n")
	next(t, events, hasPath("build/out.md"))
}

func TestWatchNewDir(t *testing.T) {
	dir := newRepo(t, "")
	_, events := startWatch(t, dir)

	if err := os.Mkdir(filepath.Join(dir, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	next(t, events, hasPath("new"))
	write(t, filepath.Join(dir, "new", "a.md"), "1\n")
	next(t, events, hasPath("new/a.md"))
}

func TestWatchDirAfterClose(t *testing.T) {
	dir := newRepo(t, "build/\n", "build/out.md")
	ws, err := Open(Options{Root: dir, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	w, err := Watch(ws, "")
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	w.WatchDir("build")
}

func TestWatchThemesCreatedLater(t *testing.T) {
	dir := newRepo(t, "")
	themes := filepath.Join(t.TempDir(), "themes")
	ws, err := Open(Options{Root: dir, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	events := make(chan Event, 16)
	w, err := Watch(ws, themes, func(e Event) { events <- e })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	write(t, filepath.Join(themes, "a.css"), ":root {}\n")
	w.WatchThemes()
	write(t, filepath.Join(themes, "a.css"), ":root { --x: 1; }\n")
	next(t, events, func(e Event) bool { return e.Theme })

	// Removed and created again, it is watched again
	if err := os.RemoveAll(themes); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	write(t, filepath.Join(themes, "b.css"), ":root {}\n")
	w.WatchThemes()
	for len(events) > 0 {
		<-events
	}
	write(t, filepath.Join(themes, "b.css"), ":root { --y: 1; }\n")
	next(t, events, func(e Event) bool { return e.Theme })
}
