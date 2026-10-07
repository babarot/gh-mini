package markdown

import (
	"strings"
	"testing"
)

// HTML in a file must not run anything in the page, nor take the place of
// its elements.
func TestRawHTMLDropped(t *testing.T) {
	r := New("")
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
		for _, bad := range []string{"<script", "onerror", "onload", "javascript:", "<style", "style=", "<iframe", "<base", "<meta", "<form", "<input", "id=", "class=", "<object", "<svg"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: output has %q: %s", name, bad, out)
			}
		}
	}
}

// What READMEs use stays.
func TestRawHTMLKept(t *testing.T) {
	r := New("")
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
