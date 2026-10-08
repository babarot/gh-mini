package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// selectedLines lists the lines picked in a source file.
const selectedLines = `[...document.querySelectorAll(".code .line.selected")].map((l) => l.querySelector(".ln").id).join(",")`

// clickLine clicks a line's number, with Shift if shift.
func clickLine(t *testing.T, ctx context.Context, n string, shift bool) {
	t.Helper()
	pos := eval[[]float64](t, ctx, `(() => { const r = document.querySelector("#L`+n+` a").getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()`)
	var opts []chromedp.MouseOption
	if shift {
		opts = append(opts, chromedp.ButtonModifiers(input.ModifierShift))
	}
	run(t, ctx, chromedp.MouseClickXY(pos[0], pos[1], opts...))
}

// #L3-L5 picks a range of lines, a click on a number picks its line
// without scrolling, and a click with Shift the lines from it.
func TestLineRange(t *testing.T) {
	var src strings.Builder
	for i := range 100 {
		src.WriteString("x := " + string(rune('a'+i%26)) + "\n")
	}
	a := newApp(t, map[string]string{"code.go": src.String()})
	ctx := tab(t)
	open(t, ctx, a.URL("/code.go#L40-L42"))
	waitFor(t, ctx, selectedLines+` === "L40,L41,L42"`)
	// Under the top bar, not behind it
	if top := eval[float64](t, ctx, `document.getElementById("L40").getBoundingClientRect().top`); top < 56 || top > 120 {
		t.Errorf("L40 at %v", top)
	}

	y := eval[float64](t, ctx, `window.scrollY`)
	clickLine(t, ctx, "45", false)
	waitFor(t, ctx, selectedLines+` === "L45" && location.hash === "#L45"`)
	if got := eval[float64](t, ctx, `window.scrollY`); got != y {
		t.Errorf("a click on a number scrolled from %v to %v", y, got)
	}
	clickLine(t, ctx, "48", true)
	waitFor(t, ctx, selectedLines+` === "L45,L46,L47,L48" && location.hash === "#L45-L48"`)
	clickLine(t, ctx, "43", true)
	waitFor(t, ctx, selectedLines+` === "L43,L44,L45" && location.hash === "#L43-L45"`)
	if got := eval[string](t, ctx, `getSelection().toString()`); got != "" {
		t.Errorf("Shift selected text: %q", got)
	}
}

// On a narrow screen the tree opens over the page, dimmed under it, and
// Escape or a click on the page puts it away.
func TestNarrowSidebar(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	run(t, ctx, chromedp.EmulateViewport(375, 700))
	open(t, ctx, a.URL("/a.md"))
	const shown = `document.body.classList.contains("sidebar-shown")`
	const expanded = `document.getElementById("mini.sidebar-toggle").getAttribute("aria-expanded")`
	toggle := `[id="mini.sidebar-toggle"]`

	run(t, ctx, chromedp.Click(toggle, chromedp.ByQuery))
	waitFor(t, ctx, shown+` && `+expanded+` === "true"`)
	if bg := eval[string](t, ctx, `getComputedStyle(document.body, "::after").backgroundColor`); bg == "rgba(0, 0, 0, 0)" {
		t.Error("the page is not dimmed under the tree")
	}
	run(t, ctx, chromedp.KeyEvent(kb.Escape))
	waitFor(t, ctx, `!`+shown+` && `+expanded+` === "false"`)

	run(t, ctx, chromedp.Click(toggle, chromedp.ByQuery))
	waitFor(t, ctx, shown)
	run(t, ctx, chromedp.MouseClickXY(360, 600))
	waitFor(t, ctx, `!`+shown)

	run(t, ctx, chromedp.KeyEvent("t"))
	waitFor(t, ctx, shown+` && document.activeElement.id === "mini.tree-filter"`)
	run(t, ctx, chromedp.KeyEvent(kb.Escape))
	waitFor(t, ctx, `!`+shown)
}

// On a wide screen with the tree closed, t shows it for the moment, and
// Escape closes it again as it was kept; nothing opens it behind a dialog.
func TestPeekSidebar(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	const hidden = `document.body.classList.contains("sidebar-hidden")`
	run(t, ctx, chromedp.Click(`[id="mini.sidebar-toggle"]`, chromedp.ByQuery))
	waitFor(t, ctx, hidden+` && document.cookie.includes("gh-mini-sidebar=hidden")`)

	run(t, ctx, chromedp.KeyEvent("t"))
	waitFor(t, ctx, `!`+hidden+` && document.getElementById("mini.sidebar-toggle").getAttribute("aria-expanded") === "true"`)
	run(t, ctx, chromedp.KeyEvent(kb.Escape))
	waitFor(t, ctx, hidden+` && document.cookie.includes("gh-mini-sidebar=hidden")`)

	run(t, ctx, chromedp.KeyEvent(","))
	waitFor(t, ctx, `document.getElementById("mini.settings").open`)
	run(t, ctx, chromedp.KeyEvent("t"))
	if !eval[bool](t, ctx, hidden) {
		t.Error("t opened the tree behind the settings")
	}
}

// The menu takes the focus as it opens, and the arrow keys move it.
func TestMenuFocus(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	const focused = `document.activeElement.querySelector("span")?.textContent`
	run(t, ctx, chromedp.Click(`[id="mini.menu-open"]`, chromedp.ByQuery))
	waitFor(t, ctx, focused+` === "Settings"`)
	for _, step := range []struct{ key, want string }{
		{kb.ArrowDown, "Keyboard shortcuts"},
		{kb.ArrowDown, "About gh-mini"},
		{kb.ArrowDown, "Settings"},
		{kb.ArrowUp, "About gh-mini"},
	} {
		run(t, ctx, chromedp.KeyEvent(step.key))
		waitFor(t, ctx, focused+` === `+q(step.want))
	}
	run(t, ctx, chromedp.KeyEvent(kb.Escape))
	waitFor(t, ctx, `document.activeElement.id === "mini.menu-open"`)
}

// Diagrams take Mermaid's colors under the github theme, and the theme's
// under another, drawn again when the theme is picked.
func TestMermaidTheme(t *testing.T) {
	a := newApp(t, map[string]string{"m.md": "# M\n\n```mermaid\ngraph LR\n  A[Start] --> B[Done]\n```\n"})
	ctx := tab(t)
	open(t, ctx, a.URL("/m.md"))
	const fill = `getComputedStyle(document.querySelector(".mermaid .node rect")).fill`
	waitFor(t, ctx, `document.querySelector(".mermaid .node rect") !== null`)
	github := eval[string](t, ctx, fill)
	eval[any](t, ctx, `(() => { const s = document.querySelector("[data-setting=theme]"); s.value = "nord"; s.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	waitFor(t, ctx, `document.getElementById("mini.theme").href.includes("nord")`)
	waitFor(t, ctx, `document.querySelector(".mermaid .node rect") !== null && `+fill+` !== `+q(github))
	if got, bg := eval[string](t, ctx, fill), eval[string](t, ctx, `getComputedStyle(document.body).backgroundColor`); got != bg {
		t.Errorf("under nord a node is %s, want the page's %s", got, bg)
	}
}

// A link to #here finds the <a name="here"> a file's HTML gives, which
// the page names user-content-here. Not #top: a browser scrolls to the
// top for it, with no element of that name.
func TestUserContentAnchor(t *testing.T) {
	body := "# Anchors\n\n" + strings.Repeat("Text\n\n", 100) + "<a name=\"here\"></a>\n\nHere\n\n" +
		strings.Repeat("Text\n\n", 100) + "[Back up](#here)\n"
	a := newApp(t, map[string]string{"anchors.md": body})
	ctx := tab(t)
	at := `document.getElementsByName("user-content-here")[0].getBoundingClientRect().top`
	open(t, ctx, a.URL("/anchors.md#here"))
	waitFor(t, ctx, at+` > 0 && `+at+` < 150`)
	eval[any](t, ctx, `window.scrollTo(0, document.body.scrollHeight)`)
	run(t, ctx, chromedp.Click(`.markdown-body a[href="#here"]`, chromedp.ByQuery))
	waitFor(t, ctx, at+` > 0 && `+at+` < 150`)
}

// A page prints the file alone.
func TestPrint(t *testing.T) {
	a := newApp(t, treeFiles)
	ctx := tab(t)
	open(t, ctx, a.URL("/a.md"))
	run(t, ctx, emulation.SetEmulatedMedia().WithMedia("print"))
	for _, sel := range []string{".topbar", ".sidebar", ".crumbs", ".box-header"} {
		if d := eval[string](t, ctx, `getComputedStyle(document.querySelector(`+q(sel)+`)).display`); d != "none" {
			t.Errorf("%s prints, display %s", sel, d)
		}
	}
}
