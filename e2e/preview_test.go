package e2e

import "testing"

// The preview of an HTML file takes the height of the window, and the box
// of its last commit above it stays a line.
func TestHTMLPreviewLayout(t *testing.T) {
	a := newRepoApp(t, map[string]string{"page.html": "<!doctype html><h1>Page</h1>\n"}, func(a *app) {})
	ctx := tab(t)
	open(t, ctx, a.URL("/page.html?preview=1"))
	waitFor(t, ctx, `document.body.dataset.kind === "html" && document.querySelector(".commit") !== null`)
	height := func(sel string) float64 {
		t.Helper()
		return eval[float64](t, ctx, `document.querySelector("`+sel+`").getBoundingClientRect().height`)
	}
	if h := height(".commit"); h > 60 {
		t.Errorf("commit box is %.0fpx high", h)
	}
	if h := height(".html-preview"); h < 400 {
		t.Errorf("preview is %.0fpx high", h)
	}
}
