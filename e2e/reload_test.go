package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const heading = `document.querySelector(".markdown-body h1")?.textContent`

// A page reloads when its file changes, and keeps where it was scrolled.
func TestLiveReload(t *testing.T) {
	long := "# One\n\n"
	for range 200 {
		long += "Line\n\n"
	}
	a := newApp(t, map[string]string{"a.md": long, "b.md": "# B\n"})
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	subscribed(t, ctx)
	eval[any](t, ctx, `window.scrollTo(0, 1500)`)
	a.write("a.md", "# Two\n\n"+long[len("# One\n\n"):])
	waitFor(t, ctx, heading+` === "Two"`)
	waitFor(t, ctx, `Math.abs(window.scrollY - 1500) < 2`)

	// Another file changing leaves the page as it is. A file added after
	// it shows in the tree once the change before it has come
	eval[any](t, ctx, `window.marker = 1`)
	a.write("b.md", "# B2\n")
	a.write("c.md", "# C\n")
	waitFor(t, ctx, `document.querySelector('.tree .row[data-path="c.md"]') !== null`)
	if !eval[bool](t, ctx, `window.marker === 1`) {
		t.Error("the page reloaded for another file")
	}
}

// A file added shows in the tree, and a page not found becomes the file
// once it is made.
func TestLiveTree(t *testing.T) {
	a := newApp(t, map[string]string{"a.md": "# A\n"})
	ctx := tab(t)
	open(t, ctx, a.URL("/new.md"))
	subscribed(t, ctx)
	a.write("new.md", "# New\n")
	waitFor(t, ctx, heading+` === "New"`)
	waitFor(t, ctx, `document.querySelector('.tree .row[data-path="new.md"]') !== null`)
}

// A page open while the server restarts reads the new one, with what
// changed while it was down.
func TestRestart(t *testing.T) {
	a := newApp(t, map[string]string{"a.md": "# Before\n"})
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	subscribed(t, ctx)
	boot := eval[string](t, ctx, `document.body.dataset.boot`)
	a.stop()
	a.write("a.md", "# After\n")
	a.restart()
	waitFor(t, ctx, heading+` === "After"`)
	if eval[string](t, ctx, `document.body.dataset.boot`) == boot {
		t.Error("the page is still the old server's")
	}
}

// A change between the page being rendered and its subscribing reaches it
// still: it asks for what it missed. The page's scripts are held back
// until the server has told the change, to anyone who listened.
func TestCatchUp(t *testing.T) {
	a := newApp(t, map[string]string{"a.md": "# Before\n"})
	ctx := tab(t)
	paused := make(chan fetch.RequestID, 1)
	chromedp.ListenTarget(ctx, func(ev any) {
		// This runs on the tab's event loop, which must not wait
		if e, ok := ev.(*fetch.EventRequestPaused); ok {
			select {
			case paused <- e.RequestID:
			default:
			}
		}
	})
	run(t, ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*/assets/js/main.js"}}))
	run(t, ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := page.Navigate(a.URL("/a.md")).Do(ctx)
		return err
	}))
	var id fetch.RequestID
	select {
	case id = <-paused:
	case <-time.After(15 * time.Second):
		t.Fatal("main.js was never asked for")
	}
	// main.js is asked for from the head, maybe before the body is read
	waitFor(t, ctx, `document.body?.dataset.seq`)
	seq := eval[string](t, ctx, `document.body.dataset.seq`)
	a.write("a.md", "# After\n")
	a.waitChange(seq)
	run(t, ctx, fetch.ContinueRequest(id), fetch.Disable())
	waitFor(t, ctx, heading+` === "After"`)
}

// waitChange waits until the server has told a change after seq.
func (a *app) waitChange(seq string) {
	a.t.Helper()
	n, err := strconv.ParseUint(seq, 10, 64)
	if err != nil {
		a.t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(a.URL("/_mini/api/changes?since=0&boot="))
		if err == nil {
			var c struct{ Seq uint64 }
			_ = json.NewDecoder(resp.Body).Decode(&c)
			resp.Body.Close()
			if c.Seq > n {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	a.t.Fatal("the server told no change")
}
