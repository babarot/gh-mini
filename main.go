// Command gh-mini serves a directory as a small, local GitHub.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/babarot/gh-mini/internal/server"
)

var version = "dev"

const usage = `gh-mini serves a directory as a small, local GitHub: a file tree, directory
pages with their README, and Markdown and code rendered as GitHub does.
Files git ignores are shown too, marked "local".

Usage:
  gh-mini [flags] [DIR | FILE]

With a FILE, the current directory is served when it holds the file, and
the file is opened.

Themes are CSS files in %s.

Flags:
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-mini:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		port      int
		host      string
		noOpen    bool
		noReload  bool
		theme     string
		skip      string
		showVer   bool
		themesDir = defaultThemesDir()
	)
	flag.IntVar(&port, "p", 6419, "")
	flag.IntVar(&port, "port", 6419, "port to listen on; the next free one is used when it is taken")
	flag.StringVar(&host, "host", "localhost", "address to listen on")
	flag.BoolVar(&noOpen, "no-open", false, "do not open the browser")
	flag.BoolVar(&noReload, "no-reload", false, "do not reload pages when files change")
	flag.StringVar(&theme, "theme", os.Getenv("GH_MINI_THEME"), "theme to use until one is picked in the page ($GH_MINI_THEME)")
	flag.StringVar(&skip, "skip", ".git,node_modules,.DS_Store", "comma-separated names left out of the tree")
	flag.StringVar(&themesDir, "themes", themesDir, "directory of themes")
	flag.BoolVar(&showVer, "version", false, "print the version")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), usage, themesDir)
		flag.VisitAll(func(f *flag.Flag) {
			if f.Usage == "" {
				return
			}
			name := "--" + f.Name
			if f.Name == "port" {
				name = "-p, --port"
			}
			fmt.Fprintf(flag.CommandLine.Output(), "  %-14s %s", name, f.Usage)
			if f.DefValue != "" && f.DefValue != "false" {
				fmt.Fprintf(flag.CommandLine.Output(), " (default %q)", f.DefValue)
			}
			fmt.Fprintln(flag.CommandLine.Output())
		})
	}
	flag.Parse()
	if showVer {
		fmt.Println("gh-mini", version)
		return nil
	}
	if flag.NArg() > 1 {
		flag.Usage()
		return errors.New("too many arguments")
	}

	root, open, err := resolve(flag.Arg(0))
	if err != nil {
		return err
	}

	srv, err := server.New(server.Options{
		Root:      root,
		Name:      filepath.Base(root),
		Skip:      splitList(skip),
		Theme:     theme,
		ThemesDir: themesDir,
		Reload:    !noReload,
	})
	if err != nil {
		return err
	}

	ln, err := listen(host, port)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s%s", ln.Addr().String(), open)
	fmt.Printf("gh-mini: serving %s at %s\n", root, url)
	if !noOpen {
		openBrowser(url)
	}
	return http.Serve(ln, srv.Handler())
}

// resolve returns the directory to serve and the URL path to open.
func resolve(arg string) (string, string, error) {
	if arg == "" {
		arg = "."
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", "", err
	}
	// Watch events and paths under the root must agree, so resolve /tmp
	// and other symlinks once here
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return abs, "/", nil
	}
	root := filepath.Dir(abs)
	if cwd, err := os.Getwd(); err == nil {
		if cwd, err = filepath.EvalSymlinks(cwd); err == nil {
			if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
				root = cwd
			}
		}
	}
	rel, _ := filepath.Rel(root, abs)
	return root, "/" + filepath.ToSlash(rel), nil
}

func listen(host string, port int) (net.Listener, error) {
	var last error
	for p := port; p < port+20; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(p)))
		if err == nil {
			return ln, nil
		}
		last = err
	}
	return nil, last
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, url).Start()
}

func defaultThemesDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "gh-mini", "themes")
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
