// Command gh-mini serves a directory as a small, local GitHub.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/babarot/gh-mini/internal/server"
	"github.com/babarot/gh-mini/internal/version"
)

const usage = `gh-mini serves a directory as a small, local GitHub: a file tree, directory
pages with their README, and Markdown and code rendered as GitHub does.
Files git ignores are shown too, marked "local".

Usage:
  gh-mini [flags] [DIR | FILE]

With a FILE, the current directory is served when it holds the file,
unless it is your home directory or /, and the file is opened.

Themes are CSS files in %s.

Flags:
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var ue usageError
		if errors.As(err, &ue) {
			// The flag package has told what is wrong already
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "gh-mini:", err)
		os.Exit(1)
	}
}

// config is what the command line asks for.
type config struct {
	port        int
	previewPort int
	host        string
	noOpen      bool
	noReload    bool
	theme       string
	skip        []string
	themesDir   string
	version     bool
	// target is the directory or file to serve, "" for the current
	// directory.
	target string
}

// usageError is a flag the flag package could not parse.
type usageError struct{ error }

// parseArgs reads the command line, args without the program name. Usage
// and what is wrong with the flags are written to stderr. With -h, it
// returns flag.ErrHelp.
func parseArgs(args []string, stderr io.Writer) (config, error) {
	c := config{themesDir: defaultThemesDir()}
	var skip string
	fs := flag.NewFlagSet("gh-mini", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&c.port, "p", 6419, "")
	fs.IntVar(&c.port, "port", 6419, "port to listen on; the next free one is used when it is taken")
	fs.StringVar(&c.host, "host", "localhost", "address to listen on")
	fs.IntVar(&c.previewPort, "preview-port", 0, "port for HTML previews; 0 picks a free one")
	fs.BoolVar(&c.noOpen, "no-open", false, "do not open the browser")
	fs.BoolVar(&c.noReload, "no-reload", false, "do not reload pages when files change")
	fs.StringVar(&c.theme, "theme", os.Getenv("GH_MINI_THEME"), "theme to use until one is picked in the page ($GH_MINI_THEME)")
	fs.StringVar(&skip, "skip", ".git,node_modules,.DS_Store", "comma-separated names left out of the tree")
	fs.StringVar(&c.themesDir, "themes", c.themesDir, "directory of themes")
	fs.BoolVar(&c.version, "version", false, "print the version")
	fs.Usage = func() {
		fmt.Fprintf(stderr, usage, c.themesDir)
		fs.VisitAll(func(f *flag.Flag) {
			if f.Usage == "" {
				return
			}
			name := "--" + f.Name
			if f.Name == "port" {
				name = "-p, --port"
			}
			fmt.Fprintf(stderr, "  %-14s %s", name, f.Usage)
			if f.DefValue != "" && f.DefValue != "false" {
				fmt.Fprintf(stderr, " (default %q)", f.DefValue)
			}
			fmt.Fprintln(stderr)
		})
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, err
		}
		return c, usageError{err}
	}
	c.skip = splitList(skip)
	if fs.NArg() > 1 {
		fs.Usage()
		return c, errors.New("too many arguments")
	}
	c.target = fs.Arg(0)
	return c, nil
}

func run(args []string, stdout, stderr io.Writer) error {
	c, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.version {
		fmt.Fprintln(stdout, "gh-mini", version.String())
		return nil
	}

	root, open, err := resolve(c.target)
	if err != nil {
		return err
	}

	ln, err := listen(c.host, c.port)
	if err != nil {
		return err
	}
	// HTML previews come from a second port, another origin; without it
	// they are shown as code
	pln, err := net.Listen("tcp", net.JoinHostPort(c.host, strconv.Itoa(c.previewPort)))
	if err != nil {
		fmt.Fprintln(stderr, "gh-mini: HTML previews are off:", err)
		pln = nil
	}

	opts := server.Options{
		Root:      root,
		Name:      filepath.Base(root),
		Skip:      c.skip,
		Theme:     c.theme,
		ThemesDir: c.themesDir,
		Reload:    !c.noReload,
		Version:   version.String(),
		Revision:  version.Revision(),
	}
	if pln != nil {
		opts.PreviewPort = pln.Addr().(*net.TCPAddr).Port
	}
	srv, err := server.New(opts)
	if err != nil {
		return err
	}
	if pln != nil {
		go func() {
			if err := http.Serve(pln, srv.PreviewHandler()); err != nil {
				fmt.Fprintln(stderr, "gh-mini: HTML previews stopped:", err)
			}
		}()
	}

	url := fmt.Sprintf("http://%s%s", browserAddr(ln.Addr()), open)
	fmt.Fprintf(stdout, "gh-mini: serving %s at %s\n", root, url)
	if ifaddrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range lanAddrs(ln.Addr(), ifaddrs) {
			fmt.Fprintf(stdout, "gh-mini: also at http://%s%s\n", addr, open)
		}
	}
	if pln != nil {
		fmt.Fprintf(stdout, "gh-mini: HTML previews on port %d\n", opts.PreviewPort)
	}
	if !c.noOpen {
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
		if cwd, err = filepath.EvalSymlinks(cwd); err == nil && !tooWide(cwd) {
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

// browserAddr is the address a browser opens for a listener. One on every
// address, as with --host 0.0.0.0, is reached as localhost: browsers
// refuse to open 0.0.0.0 or [::].
func browserAddr(a net.Addr) string {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsUnspecified() {
		return a.String()
	}
	return net.JoinHostPort("localhost", strconv.Itoa(tcp.Port))
}

// lanAddrs gives the addresses other machines can reach the server at, if
// it listens on all of them: the IPv4 addresses of the machine's
// interfaces, but not loopback or link-local ones.
func lanAddrs(a net.Addr, ifaddrs []net.Addr) []string {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsUnspecified() {
		return nil
	}
	var out []string
	for _, ia := range ifaddrs {
		n, ok := ia.(*net.IPNet)
		if !ok || n.IP.To4() == nil || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() {
			continue
		}
		out = append(out, net.JoinHostPort(n.IP.String(), strconv.Itoa(tcp.Port)))
	}
	return out
}

// tooWide tells a current directory too wide to serve for a file in it:
// the home directory or /, which a file there does not mean to walk.
func tooWide(dir string) bool {
	if dir == string(filepath.Separator) {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	return dir == home
}
