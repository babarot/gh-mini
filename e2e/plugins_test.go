package e2e

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// partial is a plugin as tellme.tokyo's <Partial name="x" /> needs: it
// shows figures/x.part.html next to the file, with its styles.
var partial = map[string]string{
	".plugins/partial/plugin.json": `{"elements": ["Partial"], "read": ["figures/*.part.html"]}`,
	".plugins/partial/main.js": `export default {
  elements: {
    async Partial({ name }, ctx) {
      const src = await ctx.read("figures/" + name + ".part.html");
      const css = [];
      const html = src.replace(/<style>([\s\S]*?)<\/style>/g, (_, c) => (css.push(c), ""));
      return { html, css: css.join("\n") };
    },
  },
};
`,
}

func withFiles(files ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		for k, v := range f {
			out[k] = v
		}
	}
	return out
}

const shown = `document.querySelector('mini-element[data-plugin="partial"] > div')?.shadowRoot`

// A plugin shows a component with a file it reads, whose styles stay in
// what it shows, and the page's out of it; what would run is dropped, and
// what is fixed to the window stays in its box. Saving the file it read
// shows it again.
func TestPluginPartial(t *testing.T) {
	a := newApp(t, withFiles(partial, map[string]string{
		"post/index.md": "# Post\n\nText\n\n<Partial name=\"fig\" />\n\nAfter\n",
		"post/figures/fig.part.html": `<style>.box { color: rgb(1, 2, 3); } p { color: rgb(9, 9, 9); } .cover { position: fixed; inset: 0; }</style>
<div class="box">Figure<script>window.parent.pwned = 1</script><img src="x" onerror="window.parent.pwned = 1"></div>
<div class="cover"></div>
`,
	}))
	ctx := tab(t)
	open(t, ctx, a.URL("/post/index.md"))
	subscribed(t, ctx)
	waitFor(t, ctx, shown+`?.querySelector(".box")?.textContent === "Figure"`)
	if got := eval[string](t, ctx, `getComputedStyle(`+shown+`.querySelector(".box")).color`); got != "rgb(1, 2, 3)" {
		t.Errorf("figure's color: %s", got)
	}
	if got := eval[string](t, ctx, `getComputedStyle(document.querySelector(".markdown-body p")).color`); got == "rgb(9, 9, 9)" {
		t.Error("the figure's styles reach the page")
	}
	if eval[bool](t, ctx, shown+`.querySelector("script, [onerror]") !== null || window.pwned === 1`) {
		t.Error("what would run is kept")
	}
	// The cover fills the component's box, not the window
	if !eval[bool](t, ctx, `(() => {
		const box = document.querySelector('mini-element[data-plugin="partial"]').getBoundingClientRect();
		const c = `+shown+`.querySelector(".cover").getBoundingClientRect();
		return c.top >= box.top - 1 && c.bottom <= box.bottom + 1 && c.height < innerHeight / 2;
	})()`) {
		t.Error("a fixed element covers the page")
	}

	a.write("post/figures/fig.part.html", `<div class="box">Again</div>`)
	waitFor(t, ctx, shown+`?.querySelector(".box")?.textContent === "Again"`)
}

// A plugin reads only the files its plugin.json names, and reaches neither
// the page nor gh-mini's API.
func TestPluginSandbox(t *testing.T) {
	a := newApp(t, map[string]string{
		".plugins/probe/plugin.json": `{"elements": ["Read", "Reach"], "read": ["figures/*.part.html", "/styles/*.css"]}`,
		".plugins/probe/main.js": `export default {
  elements: {
    async Read({ path }, ctx) {
      try { await ctx.read(path); return "<p>read</p>"; } catch (e) { return "<p>denied</p>"; }
    },
    async Reach() {
      const out = [];
      try { window.parent.document.title; out.push("parent"); } catch (e) { out.push("no parent"); }
      try { await fetch("/_mini/api/tree"); out.push("fetched"); } catch (e) { out.push("no fetch"); }
      try { document.cookie; out.push("cookie"); } catch (e) { out.push("no cookie"); }
      return "<p>" + out.join(", ") + "</p>";
    },
  },
};
`,
		"secret.md":                "secret\n",
		"styles/a.css":             "p {}\n",
		"post/a.md":                "a\n",
		"post/figures/x.part.html": "x\n",
		"post/index.md": "<Read path=\"figures/x.part.html\" />\n\n" +
			"<Read path=\"figures/../figures/x.part.html\" />\n\n" +
			"<Read path=\"a.md\" />\n\n" +
			"<Read path=\"../secret.md\" />\n\n" +
			"<Read path=\"figures/../../secret.md\" />\n\n" +
			// With a / before it, a path is from the directory served
			"<Read path=\"/styles/a.css\" />\n\n" +
			"<Read path=\"/secret.md\" />\n\n" +
			"<Read path=\"styles/a.css\" />\n\n" +
			"<Reach />\n",
	})
	ctx := tab(t)
	open(t, ctx, a.URL("/post/index.md"))
	results := `Array.from(document.querySelectorAll("mini-element")).map((e) => e.firstElementChild?.shadowRoot?.textContent ?? e.textContent)`
	waitFor(t, ctx, results+`.every((s) => s !== "")`)
	got := eval[[]string](t, ctx, results)
	want := []string{"read", "read", "denied", "denied", "denied", "read", "denied", "denied", "no parent, no fetch, no cookie"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// The front matter is a table until a plugin shows it, as the card that
// comes with gh-mini does once turned on; a plugin that fails leaves the
// table, and a component it fails on tells why.
func TestPluginFrontMatter(t *testing.T) {
	a := newApp(t, map[string]string{
		".plugins/broken/plugin.json": `{"elements": ["Broken"]}`,
		".plugins/broken/main.js":     `export default { elements: { Broken() { throw new Error("oops"); } } };`,
		"post.md":                     "---\ntitle: Hello\ntags: [a, b]\n---\n\n<Broken />\n",
	})
	ctx := tab(t)
	open(t, ctx, a.URL("/post.md"))
	waitFor(t, ctx, `document.querySelector(".mini-plugin-error")?.textContent === "plugin broken: oops"`)
	if !eval[bool](t, ctx, `document.querySelector(".mini-frontmatter table") !== null`) {
		t.Error("no table")
	}

	// Turned on in the settings, the page shows it at once
	toggle := `document.querySelector('[data-setting="plugin.front-matter-card"]')`
	run(t, ctx, chromedp.Evaluate(toggle+`.click()`, nil))
	card := `document.querySelector('.mini-frontmatter[data-plugin="front-matter-card"] > div')?.shadowRoot`
	waitFor(t, ctx, card+`?.querySelector(".title")?.textContent === "Hello"`)
	if got := eval[int](t, ctx, card+`.querySelectorAll(".tag").length`); got != 2 {
		t.Errorf("tags: %d", got)
	}
	// And off, the table is back
	waitFor(t, ctx, `document.querySelector(".tree .row") !== null`)
	run(t, ctx, chromedp.Evaluate(toggle+`.click()`, nil))
	waitFor(t, ctx, `document.querySelector(".mini-frontmatter table") !== null && document.querySelector(".tree .row") !== null`)
}

// A directory's page shows its README, whose components read files next
// to the README, and reloads when they change.
func TestPluginReadme(t *testing.T) {
	a := newApp(t, withFiles(partial, map[string]string{
		"post/README.md":             "# Post\n\n<Partial name=\"fig\" />\n",
		"post/figures/fig.part.html": `<div class="box">Figure</div>`,
	}))
	ctx := tab(t)
	open(t, ctx, a.URL("/post/"))
	subscribed(t, ctx)
	waitFor(t, ctx, shown+`?.querySelector(".box")?.textContent === "Figure"`)
	a.write("post/figures/fig.part.html", `<div class="box">Again</div>`)
	waitFor(t, ctx, shown+`?.querySelector(".box")?.textContent === "Again"`)
}

// A component with children shows them in the plugin's box, where they
// are still the page's: its styles, ids and copy buttons theirs, and the
// plugin's styles theirs only through ::slotted, which gives way to the
// page's but with !important. A plugin
// is told its child components; without one, or failing, the children
// show plainly.
func TestPluginChildren(t *testing.T) {
	fence := "```"
	a := newApp(t, map[string]string{
		".plugins/boxes/plugin.json": `{"elements": ["Callout", "Tabs", "Broken"]}`,
		".plugins/boxes/main.js": `export default {
  elements: {
    Callout({ type }, ctx) {
      return {
        html: '<div class="box ' + type + '">' + (ctx.block ? "block" : "inline") + '<slot></slot></div>',
        css: ".box { color: rgb(1, 2, 3); } p { color: rgb(9, 9, 9); } ::slotted(p) { color: rgb(7, 7, 7); margin-bottom: 3px; } ::slotted(h2) { margin-top: 0 !important; }",
      };
    },
    Tabs(attrs, ctx) {
      return '<div class="labels">' + ctx.children.map((c) => c.attrs.label).join("|") + "</div><slot></slot>";
    },
    Broken() { throw new Error("oops"); },
  },
};
`,
		"post.md": "<Callout type=\"warn\">\n\n## Inside\n\nText\n\n" + fence + "sh\necho hi\n" + fence + "\n\nLast\n</Callout>\n\n" +
			"<Tabs>\n\n<Tab label=\"a\">\n\nA\n\n</Tab>\n\n<Tab label=\"b\" />\n\n</Tabs>\n\n" +
			"<Broken>\n\nkept\n\n</Broken>\n\n" +
			"<Plain>\n\nplain\n\n</Plain>\n",
	})
	ctx := tab(t)
	open(t, ctx, a.URL("/post.md"))
	callout := `document.querySelector('mini-element[data-tag="Callout"]')`
	waitFor(t, ctx, callout+`?.dataset.plugin === "boxes"`)
	if got := eval[string](t, ctx, callout+`.firstElementChild.shadowRoot.querySelector(".box.warn").textContent`); got != "block" {
		t.Errorf("box: %q", got)
	}
	// The children are in the page, in the plugin's slot
	if !eval[bool](t, ctx, `(() => {
		const h = document.getElementById("inside");
		return h !== null && h.assignedSlot !== null && document.querySelector('mini-element[data-tag="Callout"] .copy-btn') !== null;
	})()`) {
		t.Error("the children are not the page's, in the slot")
	}
	styles := eval[[]string](t, ctx, `(() => {
		const p = document.querySelector('mini-element[data-tag="Callout"] p');
		return [p.textContent, getComputedStyle(p).color, getComputedStyle(p).marginBottom, getComputedStyle(document.getElementById("inside")).marginTop];
	})()`)
	// ::slotted sets what the page leaves, and what it sets with !important
	if styles[0] != "Text" || styles[1] != "rgb(7, 7, 7)" || styles[2] == "3px" || styles[3] != "0px" {
		t.Errorf("children's text, color, margin and heading margin: %q", styles)
	}
	if got := eval[string](t, ctx, `Array.from(document.querySelectorAll('mini-element[data-tag="Callout"] p')).at(-1).textContent`); got != "Last" {
		t.Errorf("last paragraph: %q", got)
	}

	tabs := `document.querySelector('mini-element[data-tag="Tabs"]')`
	waitFor(t, ctx, tabs+`?.firstElementChild?.shadowRoot?.querySelector(".labels")?.textContent === "a|b"`)

	broken := `document.querySelector('mini-element[data-tag="Broken"]')`
	waitFor(t, ctx, broken+`.querySelector(".mini-plugin-error")?.textContent === "plugin boxes: oops"`)
	if got := eval[string](t, ctx, broken+`.querySelector("p")?.textContent`); got != "kept" {
		t.Errorf("a failing plugin's children: %q", got)
	}
	plain := `document.querySelector('mini-element[data-tag="Plain"]')`
	if got := eval[[]string](t, ctx, `[getComputedStyle(`+plain+`).display, `+plain+`.querySelector("p").textContent]`); got[0] != "contents" || got[1] != "plain" {
		t.Errorf("without a plugin: %q", got)
	}
}

// A plugin shows code blocks of a language, written in any case, as a
// table here, told the info string's rest and how the page shows; the
// code moves into its box, for a slot to show. A plugin that fails leaves
// the code, and a language no plugin shows stays code.
func TestPluginCodeBlocks(t *testing.T) {
	fence := "```"
	a := newApp(t, map[string]string{
		".plugins/csv/plugin.json": `{"codeBlocks": ["csv", "broken"]}`,
		".plugins/csv/main.js": `export default {
  codeBlocks: {
    csv(code, ctx) {
      const rows = code.trim().split("\n").map((l) => "<tr>" + l.split(",").map((c) => "<td>" + c + "</td>").join("") + "</tr>");
      return "<table><caption>" + [ctx.lang, ctx.meta, ctx.mode, ctx.theme].join("|") + "</caption>" + rows.join("") + "</table><details><summary>Source</summary><slot></slot></details>";
    },
    broken() { throw new Error("oops"); },
  },
};
`,
		"data.md": fence + "CSV\ttitle=\"x\"\na,b\nc,d\n" + fence + "\n\n" +
			fence + "broken\nx\n" + fence + "\n\n" +
			fence + "go\npackage main\n" + fence + "\n",
	})
	ctx := tab(t)
	open(t, ctx, a.URL("/data.md"))
	box := `document.querySelector('mini-element[data-code="csv"]')`
	waitFor(t, ctx, box+`?.dataset.plugin === "csv"`)
	got := eval[[]string](t, ctx, `(() => {
		const root = `+box+`.firstElementChild.shadowRoot;
		return [root.querySelector("caption").textContent, Array.from(root.querySelectorAll("td")).map((td) => td.textContent).join(""), `+box+`.querySelector(".highlight").assignedSlot ? "slotted" : ""];
	})()`)
	// The mode is the browser's, Auto resolved
	mode := "light"
	if eval[bool](t, ctx, `matchMedia("(prefers-color-scheme: dark)").matches`) {
		mode = "dark"
	}
	if got[0] != `CSV|title="x"|`+mode+`|github` || got[1] != "abcd" || got[2] != "slotted" {
		t.Errorf("table, info, code: %q", got)
	}

	broken := `document.querySelector('mini-element[data-code="broken"]')`
	waitFor(t, ctx, broken+`?.querySelector(".mini-plugin-error")?.textContent === "plugin csv: oops"`)
	if !eval[bool](t, ctx, broken+`.querySelector(".highlight pre")?.textContent === "x\n"`) {
		t.Error("a failing plugin's code is gone")
	}
	if eval[bool](t, ctx, `document.querySelector('.highlight[data-lang="go"]').closest("mini-element") !== null`) {
		t.Error("a language no plugin shows is boxed")
	}
}

// A plugin imports its own modules, fetches its own files, relative to
// itself, and compiles WebAssembly; editing one shows again on the page.
func TestPluginFiles(t *testing.T) {
	a := newApp(t, map[string]string{
		".plugins/files/plugin.json":  `{"elements": ["Files"]}`,
		".plugins/files/lib/label.js": `export const label = "one";` + "\n",
		".plugins/files/data.json":    `{"n": 2}`,
		// The smallest WebAssembly module
		".plugins/files/m.wasm": "\x00asm\x01\x00\x00\x00",
		".plugins/files/main.js": `import { label } from "./lib/label.js";
export default {
  elements: {
    async Files() {
      const data = await (await fetch("./data.json")).json();
      const { instance } = await WebAssembly.instantiateStreaming(fetch("./m.wasm"));
      return "<p>" + label + " " + data.n + (instance ? " wasm" : "") + "</p>";
    },
  },
};
`,
		"post.md": "<Files />\n",
	})
	ctx := tab(t)
	open(t, ctx, a.URL("/post.md"))
	subscribed(t, ctx)
	text := `document.querySelector('mini-element[data-plugin="files"] > div')?.shadowRoot?.textContent`
	waitFor(t, ctx, text+` === "one 2 wasm"`)
	a.write(".plugins/files/lib/label.js", `export const label = "two";`+"\n")
	waitFor(t, ctx, text+` === "two 2 wasm"`)
}
