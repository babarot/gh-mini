package markdown

import (
	"strings"
	"testing"
)

// HTML in a file must not run anything in the page, nor take the place of
// its elements.
func TestRawHTMLDropped(t *testing.T) {
	r := New()
	for name, src := range map[string]string{
		"script block":      "<script>\nalert(1)\n</script>\n",
		"inline script":     "a <script>alert(1)</script> b\n",
		"event handler":     `<img src="x.png" onerror="alert(1)">` + "\n",
		"svg onload":        `<svg onload="alert(1)"><circle r="1"/></svg>` + "\n",
		"javascript href":   `<a href="javascript:alert(1)">x</a>` + "\n",
		"javascript link":   "[x](javascript:alert(1))\n",
		"javascript image":  "![x](javascript:alert(1))\n",
		"style element":     "<style>body{display:none}</style>\n",
		"style attribute":   `<p style="position:fixed">x</p>` + "\n",
		"iframe":            `<iframe src="https://example.com"></iframe>` + "\n",
		"base":              `<base href="https://example.com/">` + "\n",
		"meta refresh":      `<meta http-equiv="refresh" content="0;url=https://example.com">` + "\n",
		"form":              `<form action="https://example.com"><input name="q"></form>` + "\n",
		"id":                `<div id="toc">x</div>` + "\n",
		"class":             `<div class="sidebar">x</div>` + "\n",
		"object":            `<object data="x.swf"></object>` + "\n",
		"srcset javascript": `<picture><source srcset="javascript:alert(1)"><img src="a.png"></picture>` + "\n",
	} {
		out, _, err := r.Render([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		got := strings.ToLower(string(out))
		for _, bad := range []string{"<script", "onerror", "onload", "javascript:", "<style", "style=", "<iframe", "<base", "<meta", "<form", "<input", `id="toc"`, "class=", "<object", "<svg"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: output has %q: %s", name, bad, out)
			}
		}
	}
}

// What READMEs use stays.
func TestRawHTMLKept(t *testing.T) {
	r := New()
	for name, tt := range map[string]struct{ src, want string }{
		"centered logo":   {`<p align="center"><img src="logo.png" width="100" alt="Build: ok"></p>` + "\n", `<p align="center"><img src="logo.png" width="100" alt="Build: ok"></p>`},
		"details":         {"<details open>\n<summary>More</summary>\n\nText\n\n</details>\n", "<details open=\"\">\n<summary>More</summary>"},
		"kbd and sup":     {"Press <kbd>Ctrl</kbd> x<sup>2</sup>\n", "<kbd>Ctrl</kbd> x<sup>2</sup>"},
		"picture":         {`<picture><source media="(prefers-color-scheme: dark)" srcset="dark.png"><img src="light.png"></picture>` + "\n", `<source media="(prefers-color-scheme: dark)" srcset="dark.png">`},
		"relative link":   {`<a href="docs/guide.md">Guide</a>` + "\n", `<a href="docs/guide.md" rel="nofollow">Guide</a>`},
		"https link":      {`<a href="https://example.com" title="A: b">x</a>` + "\n", `<a href="https://example.com" title="A: b" rel="nofollow">x</a>`},
		"table alignment": {`<table><tr><td align="right" colspan="2">x</td></tr></table>` + "\n", `<td align="right" colspan="2">x</td>`},
		"line break":      {"a<br>b\n", "a<br>b"},
		"inline markup":   {"<b>bold</b> <i>it</i> <strong>s</strong> <em>e</em> <span>sp</span> <code>c</code>\n", "<b>bold</b> <i>it</i> <strong>s</strong> <em>e</em> <span>sp</span> <code>c</code>"},
		"pre":             {"<pre>\n  indented\n</pre>\n", "<pre>\n  indented\n</pre>"},
		"code in a cell":  {"<table><tr><td><code>x</code></td></tr></table>\n", "<td><code>x</code></td>"},
		"textarea":        {"a <textarea>x</textarea> b\n", "a &lt;textarea&gt;x&lt;/textarea&gt; b"},
		"title block":     {"<title>T</title>\n", "&lt;title&gt;T&lt;/title&gt;"},
		// Apart from the page's own ids, as GitHub keeps them
		"anchor name":    {`<a name="top"></a>` + "\n", `<a name="user-content-top"></a>`},
		"id":             {`<h2 id="toc">x</h2>` + "\n", `<h2 id="user-content-toc">x</h2>`},
		"text direction": {`<p dir="rtl" lang="ar">x</p>` + "\n", `<p dir="rtl" lang="ar">x</p>`},
	} {
		out, _, err := r.Render([]byte(tt.src))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), tt.want) {
			t.Errorf("%s: missing %q in %s", name, tt.want, out)
		}
	}
}

// A component, a self-closing tag whose name starts with a capital as MDX
// writes one, is kept as an empty <mini-element> for a plugin to show.
func TestComponents(t *testing.T) {
	r := New()
	for name, tt := range map[string]struct{ src, want string }{
		"block": {
			"a\n\n<Partial name=\"elm\" />\n\nb\n",
			"<p>a</p>\n<mini-element data-tag=\"Partial\" data-attrs=\"{&#34;name&#34;:&#34;elm&#34;}\" data-block></mini-element>\n<p>b</p>",
		},
		// An HTML block goes on up to a blank line
		"block with a line after": {
			"<Partial name=\"elm\" />\nnext\n",
			"<mini-element data-tag=\"Partial\" data-attrs=\"{&#34;name&#34;:&#34;elm&#34;}\" data-block></mini-element>\nnext",
		},
		"inline": {
			"a <Badge text='new' on /> b\n",
			"<p>a <mini-element data-tag=\"Badge\" data-attrs=\"{&#34;on&#34;:true,&#34;text&#34;:&#34;new&#34;}\"></mini-element> b</p>",
		},
		"over lines": {
			"<Partial\n  name=\"elm\" />\n",
			"<mini-element data-tag=\"Partial\" data-attrs=\"{&#34;name&#34;:&#34;elm&#34;}\"></mini-element>",
		},
		// A name of HTML's, but not in capitals alone, is a component
		"name of HTML's": {
			"<Image src=\"a.png\" />\n",
			"<mini-element data-tag=\"Image\" data-attrs=\"{&#34;src&#34;:&#34;a.png&#34;}\" data-block></mini-element>",
		},
		"attribute case and entities": {
			"<Chart altText=\"a &amp; &quot;b&quot;\" />\n",
			"data-attrs=\"{&#34;altText&#34;:&#34;a \\u0026 \\&#34;b\\&#34;&#34;}\"",
		},
	} {
		out, _, err := r.Render([]byte(tt.src))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), tt.want) {
			t.Errorf("%s: missing %q in %s", name, tt.want, out)
		}
	}
	// HTML in capitals is HTML still
	for src, want := range map[string]string{
		"line one<BR/>line two\n":                 "line one<br/>line two",
		"<IMG SRC=\"logo.png\" WIDTH=\"100\"/>\n": `<img src="logo.png" width="100"/>`,
		"<P align=\"center\"/>\n":                 `<p align="center"/>`,
	} {
		out, _, err := r.Render([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "mini-element") || !strings.Contains(string(out), want) {
			t.Errorf("%q: %s", src, out)
		}
	}
	// A tag in lower case, or one with children, is dropped as before
	for _, src := range []string{"<partial name=\"x\" />\n", "<Callout>hi</Callout>\n"} {
		out, _, err := r.Render([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "mini-element") || strings.Contains(strings.ToLower(string(out)), "callout") {
			t.Errorf("%q: %s", src, out)
		}
	}
}

// A plugin's HTML keeps classes, and nothing else that a file's HTML would
// not keep.
func TestSanitizePlugin(t *testing.T) {
	got := string(SanitizePlugin([]byte(`<div class="box a"><script>alert(1)</script><span id="x" onclick="alert(1)" style="color:red">t</span><svg onload="alert(1)"></svg><style>p{}</style></div>`)))
	want := `<div class="box a"><span id="user-content-x">t</span></div>`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// A file's HTML still drops classes
	out, _, _ := New().Render([]byte(`<div class="box">x</div>` + "\n"))
	if strings.Contains(string(out), "class=") {
		t.Errorf("class kept in a file: %s", out)
	}
}
