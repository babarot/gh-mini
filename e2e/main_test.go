// Package e2e drives gh-mini's pages in a headless Chrome: what the
// browser does with them, such as live reload, the file tree and the
// keyboard, is what no test of the server alone can see.
//
// The tests run with go test when Chrome is found, and are skipped
// otherwise, or with -short. With GH_MINI_E2E=1, as in CI, Chrome missing
// fails them instead.
package e2e

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/babarot/gh-mini/internal/server"
)

// browser is the Chrome the tests share, each in a tab of its own. The
// tests run one at a time, each on a port of its own, so none sees
// another's storage; the cookies, which ignore ports, are cleared for
// each. skip tells why there is no browser.
var (
	browser context.Context
	skip    string
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		skip = "-short"
		os.Exit(m.Run())
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.WindowSize(1280, 800))
	// Ubuntu keeps Chrome's sandbox from starting on CI's runners; the
	// pages opened are the tests' own
	if os.Getenv("CI") != "" {
		opts = append(opts, chromedp.NoSandbox)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(alloc)
	if err := chromedp.Run(ctx); err != nil {
		skip = "no Chrome: " + err.Error()
	} else {
		browser = ctx
	}
	code := m.Run()
	cancel()
	cancelAlloc()
	os.Exit(code)
}

// tab opens a tab of its own, closed when the test ends, with no cookies.
func tab(t *testing.T) context.Context {
	t.Helper()
	if browser == nil {
		if os.Getenv("GH_MINI_E2E") == "1" {
			t.Fatal(skip)
		}
		t.Skip(skip)
	}
	ctx, cancel := chromedp.NewContext(browser)
	ctx, cancelTimeout := context.WithTimeout(ctx, time.Minute)
	t.Cleanup(func() {
		cancelTimeout()
		cancel()
	})
	run(t, ctx, network.ClearBrowserCookies())
	return ctx
}

func run(t *testing.T, ctx context.Context, actions ...chromedp.Action) {
	t.Helper()
	if err := chromedp.Run(ctx, actions...); err != nil {
		t.Fatal(err)
	}
}

// eval returns the value of a JavaScript expression in the page.
func eval[T any](t *testing.T, ctx context.Context, expr string) T {
	t.Helper()
	var v T
	run(t, ctx, chromedp.Evaluate(expr, &v))
	return v
}

// waitFor waits until a JavaScript expression is true in the page. Changes
// to files reach a page a second or so later, gathered as they come, so
// tests wait for what they expect rather than for a time.
//
// It evaluates the expression in the page's own world, as eval does:
// chromedp.Poll runs in a world of its own, which shares the page's DOM
// but not its variables or its performance entries.
func waitFor(t *testing.T, ctx context.Context, expr string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var ok bool
		// A page reloading has no world to evaluate in for a moment
		err := chromedp.Run(ctx, chromedp.Evaluate("Boolean("+expr+")", &ok))
		if err == nil && ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: %v", expr, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// open opens a page of the app and waits until its tree is drawn, which is
// when the page's scripts have started.
func open(t *testing.T, ctx context.Context, url string) {
	t.Helper()
	run(t, ctx, chromedp.Navigate(url))
	waitFor(t, ctx, `document.querySelector(".tree .row") !== null`)
}

// subscribed waits until the page follows changes: it has asked for those
// it missed, which it does once the stream is passed to it, so any change
// after that reaches it.
func subscribed(t *testing.T, ctx context.Context) {
	t.Helper()
	waitFor(t, ctx, `performance.getEntriesByType("resource").some((e) => e.name.includes("/_mini/api/changes"))`)
}

// app is gh-mini serving a directory of its own.
type app struct {
	t    *testing.T
	root string
	addr string
	srv  *server.Server
	hs   *http.Server
}

// URL is the address of a page, such as "/a.md".
func (a *app) URL(path string) string {
	return "http://" + a.addr + path
}

// newApp serves a new directory holding files, by path, with live reload
// on.
func newApp(t *testing.T, files map[string]string) *app {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, root: root}
	for name, body := range files {
		a.write(name, body)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a.addr = ln.Addr().String()
	a.start(ln)
	t.Cleanup(a.stop)
	return a
}

func (a *app) start(ln net.Listener) {
	a.t.Helper()
	srv, err := server.New(server.Options{Root: a.root, Name: "repo", Reload: true, ThemesDir: filepath.Join(a.root, ".themes")})
	if err != nil {
		a.t.Fatal(err)
	}
	a.srv = srv
	a.hs = &http.Server{Handler: srv.Handler()}
	go func() {
		if err := a.hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.t.Error(err)
		}
	}()
}

// stop stops the server, if it runs.
func (a *app) stop() {
	if a.hs == nil {
		return
	}
	a.hs.Close()
	a.srv.Close()
	a.hs, a.srv = nil, nil
}

// restart stops the server, if it runs, and starts another on the same
// address, as running gh-mini again does.
func (a *app) restart() {
	a.t.Helper()
	a.stop()
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		a.t.Fatal(err)
	}
	a.start(ln)
}

// write writes a file under the root, making its directories.
func (a *app) write(name, body string) {
	a.t.Helper()
	p := filepath.Join(a.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		a.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		a.t.Fatal(err)
	}
}

// q quotes a string for JavaScript.
func q(s string) string {
	return fmt.Sprintf("%q", s)
}
