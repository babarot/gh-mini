package markdown

import (
	"strings"
	"testing"
)

func render(t *testing.T, repo, src string) (string, Features) {
	t.Helper()
	b, f, err := New(repo).Render([]byte(src))
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
			name: "issue references",
			src:  "#1, owner/repo#2, (#3) and **#4**\n",
			want: []string{
				`<a href="https://github.com/example/repo/issues/1" class="issue-link">#1</a>`,
				`<a href="https://github.com/owner/repo/issues/2" class="issue-link">owner/repo#2</a>`,
				`(<a href="https://github.com/example/repo/issues/3" class="issue-link">#3</a>)`,
				`<strong><a href="https://github.com/example/repo/issues/4" class="issue-link">#4</a></strong>`,
			},
		},
		{
			name:   "no issue references in words, code and links",
			src:    "a#1 #2x `#3` [#4](x) <https://example.com/#5> &#35;6\n",
			reject: []string{"issue-link"},
		},
		{
			name: "issue references across lines keep the line break",
			src:  "see #1\nand #2\n",
			want: []string{"issues/1\" class=\"issue-link\">#1</a>\nand <a"},
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
				`<div class="highlight"><pre class="chroma"><code><span class="line"><span class="cl"><span class="kd">func</span>`,
				`<div class="highlight"><pre class="chroma"><code><span class="line"><span class="cl">indented`,
			},
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
				"<table>\n<thead><tr><th>title</th><th>tags</th><th>meta</th></tr></thead><tbody><tr>" +
					"<td>A &lt;b&gt;</td>" +
					"<td><table><tbody><tr><td>x</td><td>y</td></tr></tbody></table></td>" +
					"<td><table><thead><tr><th>k</th></tr></thead><tbody><tr><td>v</td></tr></tbody></table></td>" +
					"</tr></tbody></table>\n<h1",
			},
		},
		{
			name:   "a thematic break is not front matter",
			src:    "---\nnot: [yaml\n---\n",
			want:   []string{"<hr>"},
			reject: []string{"<table>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := render(t, "example/repo", tt.src)
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

func TestIssueReferencesWithoutRepo(t *testing.T) {
	got, _ := render(t, "", "#1 and owner/repo#2\n")
	if strings.Contains(got, "issues/1") {
		t.Errorf("#1 is linked without a repository: %s", got)
	}
	if !strings.Contains(got, `href="https://github.com/owner/repo/issues/2"`) {
		t.Errorf("owner/repo#2 is not linked: %s", got)
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
		if _, got := render(t, "", src); got.Mermaid != want.Mermaid || got.Math != want.Math {
			t.Errorf("%q: features = %+v, want %+v", src, got, want)
		}
	}
}

func TestRenderBOM(t *testing.T) {
	r := New("")
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
	_, f := render(t, "", "# Title :tada:\n\n## Use `go test` and [links](x.md)\n\n##### Too deep\n\n## Title :tada:\n\n### <b>raw</b> text\n")
	want := []Heading{
		{1, "title-tada", "Title 🎉"},
		{2, "use-go-test-and-linksxmd", "Use go test and links"},
		{2, "title-tada-1", "Title 🎉"},
		{3, "brawb-text", "raw text"}, // ids come from the source for now
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
