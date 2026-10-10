package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-mini/internal/version"
)

func TestSplitList(t *testing.T) {
	for s, want := range map[string][]string{
		"":                            nil,
		".git":                        {".git"},
		".git,node_modules,.DS_Store": {".git", "node_modules", ".DS_Store"},
		" .git , node_modules ,":      {".git", "node_modules"},
		",,":                          nil,
		"dir with space,x":            {"dir with space", "x"},
	} {
		if got := splitList(s); !slices.Equal(got, want) {
			t.Errorf("splitList(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, want := configDir("themes"), filepath.Join("/xdg", "gh-mini", "themes"); got != want {
		t.Errorf("with XDG_CONFIG_HOME: got %q, want %q", got, want)
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	if got, want := configDir("themes"), filepath.Join(home, ".config", "gh-mini", "themes"); got != want {
		t.Errorf("without XDG_CONFIG_HOME: got %q, want %q", got, want)
	}
}

// mkdirs makes a directory holding a/b/c.md and d.md, and returns its path
// with symlinks resolved, as resolve returns paths.
func mkdirs(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a/b/c.md", "d.md"} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolve(t *testing.T) {
	dir := mkdirs(t)
	other := mkdirs(t)
	t.Chdir(dir)
	for _, tt := range []struct {
		arg, root, open string
	}{
		// A directory is served itself
		{"", dir, "/"},
		{".", dir, "/"},
		{"a", filepath.Join(dir, "a"), "/"},
		{filepath.Join(other, "a"), filepath.Join(other, "a"), "/"},
		// A file under the current directory: the current directory is
		// served, so the files around it are in the tree
		{"d.md", dir, "/d.md"},
		{"a/b/c.md", dir, "/a/b/c.md"},
		{filepath.Join(dir, "a/b/c.md"), dir, "/a/b/c.md"},
		// A file elsewhere: its own directory is served
		{filepath.Join(other, "a/b/c.md"), filepath.Join(other, "a/b"), "/c.md"},
	} {
		root, open, err := resolve(tt.arg)
		if err != nil {
			t.Errorf("resolve(%q): %v", tt.arg, err)
			continue
		}
		if root != tt.root || open != tt.open {
			t.Errorf("resolve(%q) = %q, %q, want %q, %q", tt.arg, root, open, tt.root, tt.open)
		}
	}
	if _, _, err := resolve("nope"); err == nil {
		t.Error("resolve(nope): no error")
	}
}

// From inside a subdirectory, a file above it is served from its own
// directory, not from the current one.
func TestResolveFileAboveCwd(t *testing.T) {
	dir := mkdirs(t)
	t.Chdir(filepath.Join(dir, "a"))
	root, open, err := resolve("../d.md")
	if err != nil {
		t.Fatal(err)
	}
	if root != dir || open != "/d.md" {
		t.Errorf("got %q, %q, want %q, /d.md", root, open, dir)
	}
}

// Paths are resolved through symlinks, so that they agree with the
// watcher's events.
func TestResolveSymlink(t *testing.T) {
	dir := mkdirs(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	root, open, err := resolve(link)
	if err != nil {
		t.Fatal(err)
	}
	if root != dir || open != "/" {
		t.Errorf("got %q, %q, want %q, /", root, open, dir)
	}
}

func TestListenTakesNextFreePort(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := taken.Addr().(*net.TCPAddr).Port
	if port > 65535-20 {
		t.Skipf("port %d leaves no room for the next ones", port)
	}

	ln, err := listen("127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := ln.Addr().(*net.TCPAddr).Port
	if got <= port || got >= port+20 {
		t.Errorf("listened on %d, want one of the next ports after %d", got, port)
	}
}

func TestListenGivesUp(t *testing.T) {
	if _, err := listen("256.0.0.1", 6419); err == nil {
		t.Error("no error for an address that cannot be listened on")
	}
}

func TestParseArgsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	t.Setenv("GH_MINI_THEME", "")
	c, err := parseArgs(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := config{
		port:       6419,
		host:       "localhost",
		skip:       []string{".git", "node_modules", ".DS_Store"},
		themesDir:  filepath.Join("/xdg", "gh-mini", "themes"),
		pluginsDir: filepath.Join("/xdg", "gh-mini", "plugins"),
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v, want %+v", c, want)
	}
}

func TestParseArgs(t *testing.T) {
	t.Setenv("GH_MINI_THEME", "sepia")
	c, err := parseArgs([]string{
		"-p", "8000", "--host", "0.0.0.0", "--no-open", "--no-reload",
		"--skip", "dist, .cache", "--theme-dir", "/t", "--plugin-dir", "/p", "docs",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := config{
		port:       8000,
		host:       "0.0.0.0",
		noOpen:     true,
		noReload:   true,
		theme:      "sepia",
		skip:       []string{"dist", ".cache"},
		themesDir:  "/t",
		pluginsDir: "/p",
		target:     "docs",
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v, want %+v", c, want)
	}

	// --port is the long form of -p, and --theme wins over $GH_MINI_THEME
	if c, _ := parseArgs([]string{"--port=9000", "--theme=dark"}, io.Discard); c.port != 9000 || c.theme != "dark" {
		t.Errorf("--port=9000 --theme=dark: got port %d, theme %q", c.port, c.theme)
	}
	// An empty --skip leaves nothing out
	if c, _ := parseArgs([]string{"--skip="}, io.Discard); c.skip != nil {
		t.Errorf("--skip=: got %q", c.skip)
	}
}

func TestParseArgsHelp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	var stderr strings.Builder
	if _, err := parseArgs([]string{"-h"}, &stderr); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("err = %v, want flag.ErrHelp", err)
	}
	out := stderr.String()
	for _, s := range []string{
		"Usage:\n  gh-mini [flags] [DIR | FILE]",
		"Themes are CSS files in " + filepath.Join("/xdg", "gh-mini", "themes") + ", and plugins directories in " + filepath.Join("/xdg", "gh-mini", "plugins") + ".",
		`  -p, --port     port to listen on; the next free one is used when it is taken (default "6419")`,
		"  --no-open      do not open the browser\n",
		`  --skip         comma-separated names left out of the tree (default ".git,node_modules,.DS_Store")`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("usage does not contain %q:\n%s", s, out)
		}
	}
	// -p is listed only as part of --port
	if strings.Contains(out, "  --p ") {
		t.Errorf("usage lists --p on its own:\n%s", out)
	}
}

func TestParseArgsErrors(t *testing.T) {
	var stderr strings.Builder
	_, err := parseArgs([]string{"--bogus"}, &stderr)
	var ue usageError
	if !errors.As(err, &ue) {
		t.Errorf("--bogus: err = %v, want a usageError", err)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -bogus") {
		t.Errorf("--bogus: stderr does not tell what is wrong:\n%s", stderr.String())
	}

	stderr.Reset()
	_, err = parseArgs([]string{"a", "b"}, &stderr)
	if err == nil || err.Error() != "too many arguments" || errors.As(err, &ue) {
		t.Errorf("a b: err = %v, want too many arguments", err)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("a b: usage is not shown:\n%s", stderr.String())
	}

	if _, err := parseArgs([]string{"-p", "x"}, io.Discard); !errors.As(err, &ue) {
		t.Errorf("-p x: err = %v, want a usageError", err)
	}

	stderr.Reset()
	if _, err := parseArgs([]string{"--translations", "{name}"}, &stderr); !errors.As(err, &ue) {
		t.Errorf("--translations {name}: err = %v, want a usageError", err)
	}
	if !strings.Contains(stderr.String(), "needs {name} and {lang} once each") {
		t.Errorf("--translations {name}: stderr does not tell what is wrong:\n%s", stderr.String())
	}
}

func TestParseArgsTranslations(t *testing.T) {
	t.Setenv("GH_MINI_TRANSLATIONS", "dir")
	if c, err := parseArgs(nil, io.Discard); err != nil || c.translations != "dir" {
		t.Errorf("from the environment: got %q, %v", c.translations, err)
	}
	if c, err := parseArgs([]string{"--translations", "{name}_{lang}"}, io.Discard); err != nil || c.translations != "{name}_{lang}" {
		t.Errorf("--translations over the environment: got %q, %v", c.translations, err)
	}
	t.Setenv("GH_MINI_TRANSLATIONS", "off,dir")
	var ue usageError
	if _, err := parseArgs(nil, io.Discard); err == nil || errors.As(err, &ue) || !strings.Contains(err.Error(), "$GH_MINI_TRANSLATIONS") {
		t.Errorf("a wrong $GH_MINI_TRANSLATIONS: err = %v", err)
	}
	if _, err := parseArgs([]string{"--translations", "suffix"}, io.Discard); err != nil {
		t.Errorf("--translations over a wrong $GH_MINI_TRANSLATIONS: %v", err)
	}
}

func TestRunVersion(t *testing.T) {
	var stdout strings.Builder
	if err := run([]string{"--version"}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "gh-mini "+version.String()+"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRunHelp(t *testing.T) {
	if err := run([]string{"-h"}, io.Discard, io.Discard); err != nil {
		t.Errorf("-h: %v", err)
	}
}

func TestRunMissingTarget(t *testing.T) {
	err := run([]string{filepath.Join(t.TempDir(), "nope")}, io.Discard, io.Discard)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want one for a missing file", err)
	}
}

func TestBrowserAddr(t *testing.T) {
	for _, tt := range []struct{ host, addr, want string }{
		{"localhost", "127.0.0.1:6419", "localhost:6419"},
		{"localhost", "[::1]:6419", "localhost:6419"},
		{"mini.test", "127.0.0.1:6419", "mini.test:6419"},
		{"127.0.0.1", "127.0.0.1:6419", "127.0.0.1:6419"},
		{"::1", "[::1]:6419", "[::1]:6419"},
		{"0.0.0.0", "0.0.0.0:6419", "localhost:6419"},
		{"::", "[::]:6419", "localhost:6419"},
		{"", "[::]:6419", "localhost:6419"},
	} {
		a, err := net.ResolveTCPAddr("tcp", tt.addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := browserAddr(tt.host, a); got != tt.want {
			t.Errorf("browserAddr(%q, %s) = %s, want %s", tt.host, tt.addr, got, tt.want)
		}
	}
}

func TestLanAddrs(t *testing.T) {
	var ifaddrs []net.Addr
	for _, cidr := range []string{"127.0.0.1/8", "192.168.1.5/24", "169.254.3.4/16", "::1/128", "fe80::1/64", "2001:db8::5/64", "10.0.0.7/8"} {
		ip, n, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip
		ifaddrs = append(ifaddrs, n)
	}
	for addr, want := range map[string]string{
		"0.0.0.0:6419":   "192.168.1.5:6419 10.0.0.7:6419",
		"[::]:6419":      "192.168.1.5:6419 10.0.0.7:6419",
		"127.0.0.1:6419": "",
		"[::1]:6419":     "",
		"10.0.0.7:6419":  "",
	} {
		a, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(lanAddrs(a, ifaddrs), " "); got != want {
			t.Errorf("lanAddrs(%s) = %q, want %q", addr, got, want)
		}
	}
}

func TestHostNames(t *testing.T) {
	name, err := os.Hostname()
	if err != nil {
		t.Skip(err)
	}
	machine := name
	if !strings.Contains(name, ".") {
		machine += " " + name + ".local"
	}
	for _, tt := range []struct{ host, addr, want string }{
		{"localhost", "127.0.0.1:6419", "localhost"},
		{"127.0.0.1", "127.0.0.1:6419", ""},
		{"::1", "[::1]:6419", ""},
		{"box.local", "192.168.1.5:6419", "box.local"},
		{"0.0.0.0", "0.0.0.0:6419", machine},
		{"", "[::]:6419", machine},
	} {
		a, err := net.ResolveTCPAddr("tcp", tt.addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(hostNames(tt.host, a), " "); got != tt.want {
			t.Errorf("hostNames(%q, %s) = %q, want %q", tt.host, tt.addr, got, tt.want)
		}
	}
}

// From the home directory, a file is served with its own directory, not
// the whole home.
func TestResolveFromHome(t *testing.T) {
	home := mkdirs(t)
	t.Setenv("HOME", home)
	t.Chdir(home)
	root, open, err := resolve("a/b/c.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "a", "b"); root != want || open != "/c.md" {
		t.Errorf("got %q, %q, want %q, /c.md", root, open, want)
	}
}

// On an interrupt the server stops at once, an open event stream and all.
func TestServeStops(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	streaming := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(streaming)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, h) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-streaming
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve waits on the stream")
	}
}
