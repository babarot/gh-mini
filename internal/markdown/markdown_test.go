package markdown

import (
	"strings"
	"testing"
)

func render(t *testing.T, src string) (string, Features) {
	t.Helper()
	b, f, err := New().Render([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return string(b), f
}

func TestRender(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		want   []string
		reject []string
	}{
		{
			name: "heading ids as GitHub's",
			src:  "# Hello World_x\n\n## 日本語の見出し\n\n# Hello World_x\n",
			want: []string{`<h1 id="hello-world_x">`, `<h2 id="日本語の見出し">`, `<h1 id="hello-world_x-1">`},
		},
		{
			name: "alert",
			src:  "> [!note]\n> Body with `code`.\n",
			want: []string{
				`<div class="markdown-alert markdown-alert-note">`,
				`<p class="markdown-alert-title"><svg class="octicon octicon-info mr-2"`,
				"</svg>Note</p>\n<p>Body with <code>code</code>.</p>\n</div>",
			},
			reject: []string{"[!note]", "<blockquote>"},
		},
		{
			name:   "alert marker followed by text is a blockquote",
			src:    "> [!WARNING] Text\n",
			want:   []string{"<blockquote>\n<p>[!WARNING] Text</p>"},
			reject: []string{"markdown-alert"},
		},
		{
			name:   "alert marker alone is a blockquote",
			src:    "> [!TIP]\n",
			want:   []string{"<blockquote>"},
			reject: []string{"markdown-alert"},
		},
		{
			name: "task list",
			src:  "- [ ] todo\n- [x] done\n  - nested\n- plain\n",
			want: []string{
				`<ul class="contains-task-list">`,
				`<li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled=""> todo</li>`,
				`<input type="checkbox" class="task-list-item-checkbox" disabled="" checked=""> done`,
				"<ul>\n<li>nested</li>",
				"<li>plain</li>",
			},
		},
		{
			name: "ordered list keeps its start",
			src:  "3. three\n4. four\n",
			want: []string{`<ol start="3">`},
		},
		{
			name: "footnotes",
			src:  "A[^n] and B[^n].\n\n[^n]: Note.\n",
			want: []string{
				`<sup><a href="#fn-1" id="fnref-1" data-footnote-ref="">1</a></sup>`,
				`<sup><a href="#fn-1" id="fnref-1-2" data-footnote-ref="">1</a></sup>`,
				"<section class=\"footnotes\" data-footnotes=\"\">\n<ol>\n<li id=\"fn-1\">",
				`<a href="#fnref-1" data-footnote-backref="" aria-label="Back to reference 1" class="data-footnote-backref">↩</a>`,
				`<a href="#fnref-1-2" data-footnote-backref="" aria-label="Back to reference 1-2" class="data-footnote-backref">↩<sup>2</sup></a>`,
			},
		},
		{
			name:   "references to issues and people are text, as in a file on GitHub",
			src:    "#1, owner/repo#2, GH-3 and @octocat\n",
			want:   []string{"<p>#1, owner/repo#2, GH-3 and @octocat</p>"},
			reject: []string{"<a"},
		},
		{
			name: "a link to an issue shows its URL",
			src:  "https://github.com/acme/widget/issues/9\n",
			want: []string{`<a href="https://github.com/acme/widget/issues/9">https://github.com/acme/widget/issues/9</a>`},
		},
		{
			name:   "hashtags are text",
			src:    "#tag\n",
			want:   []string{"<p>#tag</p>"},
			reject: []string{"hashtag"},
		},
		{
			name: "inline math",
			src:  "Math $a<b$ and $`x^2`$.\n",
			want: []string{`<span class="math-inline">\(a&lt;b\)</span>`, `<span class="math-inline">\(x^2\)</span>`},
		},
		{
			name:   "prices are not math",
			src:    "It costs $5 and $6, or $ 7 $.\n",
			want:   []string{"It costs $5 and $6, or $ 7 $."},
			reject: []string{"math-inline"},
		},
		{
			name: "math blocks",
			src:  "$$\na < b\n$$\n\n$$c$$\n\n```math\nE = mc^2\n```\n",
			want: []string{
				"<div class=\"math-display\">\\[a &lt; b\n\\]</div>",
				`<div class="math-display">\[c\]</div>`,
				"<div class=\"math-display\">\\[E = mc^2\n\\]</div>",
			},
			reject: []string{"<pre"},
		},
		{
			name: "code blocks",
			src:  "```go\nfunc main() {}\n```\n\n    indented\n",
			want: []string{
				`<div class="highlight" data-lang="go"><pre class="chroma"><code><span class="line"><span class="cl"><span class="kd">func</span>`,
				`<div class="highlight"><pre class="chroma"><code><span class="line"><span class="cl">indented`,
			},
		},
		{
			name: "info string",
			src:  "```csv\tsep=\";\"  \n1;2\n```\n\n```a\"><script>x\n1\n```\n",
			want: []string{
				`<div class="highlight" data-lang="csv" data-meta="sep=&#34;;&#34;">`,
				`<div class="highlight" data-lang="a&#34;&gt;&lt;script&gt;x">`,
			},
			reject: []string{"<script"},
		},
		{
			name:   "math after a tab",
			src:    "```math\tx\nE = mc^2\n```\n",
			want:   []string{`<div class="math-display">`},
			reject: []string{"highlight"},
		},
		{
			name:   "mermaid blocks are left for Mermaid",
			src:    "```mermaid\ngraph TD; A-->B<br>\n```\n",
			want:   []string{"<pre class=\"mermaid\">graph TD; A--&gt;B&lt;br&gt;\n</pre>\n"},
			reject: []string{"highlight"},
		},
		{
			name:   "raw HTML is kept as GitHub keeps it",
			src:    "<details>\n<summary>More</summary>\n\nHidden\n\n</details>\n\n<!-- a\nb -->\n",
			want:   []string{"<details>\n<summary>More</summary>\n<p>Hidden</p>\n</details>\n"},
			reject: []string{"<!--", "--&gt;", "-->"},
		},
		{
			name: "front matter",
			src:  "---\ntitle: A <b>\ntags: [x, y]\nmeta: {k: v}\n---\n# Body\n",
			want: []string{
				// The front matter as JSON, for a plugin to show
				`<div class="mini-frontmatter" data-front-matter="{&#34;meta&#34;:{&#34;k&#34;:&#34;v&#34;},&#34;tags&#34;:[&#34;x&#34;,&#34;y&#34;],&#34;title&#34;:&#34;A \u003cb\u003e&#34;}">`,
				"<table>\n<thead><tr><th>title</th><th>tags</th><th>meta</th></tr></thead><tbody><tr>" +
					"<td>A &lt;b&gt;</td>" +
					"<td><table><tbody><tr><td>x</td><td>y</td></tr></tbody></table></td>" +
					"<td><table><thead><tr><th>k</th></tr></thead><tbody><tr><td>v</td></tr></tbody></table></td>" +
					"</tr></tbody></table>\n</div>\n<h1",
			},
		},
		{
			// As on GitHub, YAML that does not parse is front matter with an
			// error, not a rule and a heading
			name:   "broken front matter shows its error",
			src:    "---\nnot: [yaml\n---\n",
			want:   []string{"Error in user YAML", "<pre><code>not: [yaml\n</code></pre>"},
			reject: []string{"<hr>", "<table>", "data-front-matter"},
		},
		{
			name:   "a rule around a line that is no mapping is not front matter",
			src:    "---\nJust text\n---\n",
			want:   []string{"<hr>"},
			reject: []string{"Error in user YAML", "<table>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := render(t, tt.src)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in\n%s", w, got)
				}
			}
			for _, r := range tt.reject {
				if strings.Contains(got, r) {
					t.Errorf("unexpected %q in\n%s", r, got)
				}
			}
		})
	}
}

func TestFeatures(t *testing.T) {
	for src, want := range map[string]Features{
		"# Plain\n":                        {},
		"```mermaid\ngraph TD; A-->B\n```": {Mermaid: true},
		"Euler $e^{i\\pi}$\n":              {Math: true},
		"```math\nx\n```\n":                {Math: true},
		"$$\nx\n$$\n":                      {Math: true},
	} {
		if _, got := render(t, src); got.Mermaid != want.Mermaid || got.Math != want.Math {
			t.Errorf("%q: features = %+v, want %+v", src, got, want)
		}
	}
}

func TestRenderBOM(t *testing.T) {
	r := New()
	for name, tt := range map[string]struct{ src, want string }{
		"heading":      {"\xef\xbb\xbf# Title\n", `<h1 id="title">Title</h1>`},
		"front matter": {"\xef\xbb\xbf---\ntitle: x\n---\n# Body\n", "<th>title</th>"},
	} {
		out, _, err := r.Render([]byte(tt.src))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), tt.want) || strings.Contains(string(out), "\xef\xbb\xbf") {
			t.Errorf("%s: %s", name, out)
		}
	}
}

func TestHeadings(t *testing.T) {
	_, f := render(t, "# Title :tada:\n\n## Use `go test` and [links](x.md)\n\n##### Deep\n\n## Title :tada:\n\n### <b>raw</b> text\n")
	want := []Heading{
		{1, "title-tada", "Title 🎉"},
		{2, "use-go-test-and-links", "Use go test and links"},
		{5, "deep", "Deep"},
		{2, "title-tada-1", "Title 🎉"},
		{3, "raw-text", "raw text"},
	}
	if len(f.Headings) != len(want) {
		t.Fatalf("headings = %+v", f.Headings)
	}
	for i, h := range f.Headings {
		if h != want[i] {
			t.Errorf("heading %d = %+v, want %+v", i, h, want[i])
		}
	}
}

func TestBrokenFrontMatter(t *testing.T) {
	out, _ := render(t, "---\ntitle: [unclosed\ndate: x\n---\n# Body\n")
	html := string(out)
	for _, want := range []string{"Error in user YAML", "<pre><code>title: [unclosed\ndate: x\n</code></pre>", `<h1 id="body">Body</h1>`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, "<hr>") {
		t.Errorf("front matter became a rule: %s", html)
	}
}

func TestInlineDoubleDollarMath(t *testing.T) {
	for src, want := range map[string]string{
		"$$x$$ inline start\n":    `<span class="math-inline">\(x\)</span> inline start`,
		"It costs $$10 or so\n":   "It costs $$10 or so",
		"$$10 or $$20\n":          "$$10 or $$20",
		"a $$y^2$$ b and $z$ c\n": `a <span class="math-inline">\(y^2\)</span> b and <span class="math-inline">\(z\)</span> c`,
	} {
		out, _ := render(t, src)
		if !strings.Contains(string(out), want) {
			t.Errorf("%q: got %s, want %q", src, out, want)
		}
	}
}

// Heading ids come from the text shown, as on GitHub.
func TestHeadingIDsFromText(t *testing.T) {
	for src, want := range map[string]string{
		"## Foo [link](x.md)\n":                `id="foo-link"`,
		"## A &amp; B &lt;tag&gt;\n":           `id="a--b-tag"`,
		"## <img src=\"x.png\"> Image head\n":  `id="-image-head"`,
		"## [1.2.0](https://x) - 2024-01-02\n": `id="120---2024-01-02"`,
		"## Release :tada:\n":                  `id="release-tada"`,
		"## Use `go test`\n":                   `id="use-go-test"`,
		"## 日本語の見出し\n":                         `id="日本語の見出し"`,
		"## See https://example.com now\n":     `id="see-httpsexamplecom-now"`,
		"## Mail <a@example.com>\n":            `id="mail-aexamplecom"`,
	} {
		out, _ := render(t, src)
		if !strings.Contains(string(out), want) {
			t.Errorf("%q: got %s, want %s", src, out, want)
		}
	}
}

func TestImageLinks(t *testing.T) {
	for src, want := range map[string]string{
		"![shot](shot.png)\n":             `<a href="shot.png" target="_blank" rel="noopener noreferrer"><img src="shot.png" alt="shot"></a>`,
		"[![badge](b.svg)](https://ci)\n": `<a href="https://ci"><img src="b.svg" alt="badge"></a>`,
		// In a link written in HTML, inline and around a block
		`<a href="https://ci">![badge](b.svg)</a>` + "\n":                  `<a href="https://ci" rel="nofollow"><img src="b.svg" alt="badge"></a>`,
		"<a href=\"https://ci\">\n\n![badge](b.svg)\n\n</a>\n":             `<p><img src="b.svg" alt="badge"></p>`,
		`<a href="https://ci">x</a> ![after](a.png) <abbr>y</abbr>` + "\n": `<a href="a.png" target="_blank" rel="noopener noreferrer"><img src="a.png" alt="after"></a>`,
	} {
		out, _ := render(t, src)
		if !strings.Contains(string(out), want) {
			t.Errorf("%q: got %s, want %s", src, out, want)
		}
	}
	if out, _ := render(t, "![x](data:image/png;base64,AAAA)\n"); strings.Contains(string(out), "<a") {
		t.Errorf("a data: image is linked: %s", out)
	}
}

// The JSON of front matter keeps keys as written, which JSON could not
// have as numbers or booleans, follows aliases, and keeps a scalar's type
// where JSON has one.
func TestFrontMatterJSON(t *testing.T) {
	got, _ := render(t, "---\n1: one\ntrue: yes\ndate: 2025-01-29\nn: .nan\nlist: &l [a, 1, 1.5, false, null]\nref: *l\n---\n")
	want := `data-front-matter="{&#34;1&#34;:&#34;one&#34;,&#34;date&#34;:&#34;2025-01-29&#34;,&#34;list&#34;:[&#34;a&#34;,1,1.5,false,null],&#34;n&#34;:&#34;.nan&#34;,&#34;ref&#34;:[&#34;a&#34;,1,1.5,false,null],&#34;true&#34;:&#34;yes&#34;}"`
	if !strings.Contains(got, want) {
		t.Errorf("missing %s in\n%s", want, got)
	}
}
