package server

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestLangOf(t *testing.T) {
	for name, want := range map[string]string{
		"README.md":      "",
		"README.ja.md":   "ja",
		"guide.zh-TW.md": "zh-tw",
		"notes.markdown": "",
		"v1.2.md":        "",
		"my.notes.ja.md": "ja",
		"guide.ja.mdx":   "ja",
		"guide.mdx":      "",
		"notamd.ja.txt":  "",
		"guideXjaXmd":    "",
	} {
		if got := langOf(name); got != want {
			t.Errorf("langOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPickReadme(t *testing.T) {
	names := []string{"README.ja.md", "README.md"}
	tests := []struct{ query, saved, want string }{
		{"", "", "README.md"},
		{"ja", "", "README.ja.md"},
		{"", "ja", "README.ja.md"},
		{"default", "ja", "README.md"},
		{"fr", "", "README.md"},
	}
	for _, tt := range tests {
		if got := pickReadme(append([]string(nil), names...), tt.query, tt.saved); got != tt.want {
			t.Errorf("pickReadme(%q, %q) = %q, want %q", tt.query, tt.saved, got, tt.want)
		}
	}
	if !readmeRe.MatchString("README.mdx") || readmeRe.MatchString("READMEXmd") {
		t.Error("readmeRe: README.mdx must match, READMEXmd must not")
	}
	if got := pickReadme([]string{"readme.ja.md"}, "", ""); got != "readme.ja.md" {
		t.Errorf("only a translation: got %q", got)
	}
}

func TestHref(t *testing.T) {
	for rel, want := range map[string]string{
		".":           "/",
		"docs/a b.md": "/docs/a%20b.md",
		"docs/#1.md":  "/docs/%231.md",
		"日本語.md":      "/%E6%97%A5%E6%9C%AC%E8%AA%9E.md",
	} {
		if got := href(rel); got != want {
			t.Errorf("href(%q) = %q, want %q", rel, got, want)
		}
	}
	if got := dirHref("docs"); got != "/docs/" {
		t.Errorf("dirHref = %q", got)
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{
		0:               "0 Bytes",
		1023:            "1023 Bytes",
		1024:            "1.0 KB",
		1536:            "1.5 KB",
		1<<20 - 1:       "1024.0 KB",
		1 << 20:         "1.0 MB",
		5*1<<20 + 1<<19: "5.5 MB",
	} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestAgo(t *testing.T) {
	const day = 24 * time.Hour
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{0, "just now"},
		{59 * time.Second, "just now"},
		{time.Minute, "1 minute ago"},
		{59 * time.Minute, "59 minutes ago"},
		{time.Hour, "1 hour ago"},
		{23 * time.Hour, "23 hours ago"},
		{day, "1 day ago"},
		{29 * day, "29 days ago"},
		{30 * day, "1 month ago"},
		{364 * day, "12 months ago"},
		{365 * day, "1 year ago"},
		{3 * 365 * day, "3 years ago"},
	} {
		// Half a second more, so that time passing during the test does
		// not cross into the next unit
		if got := ago(time.Now().Add(-tt.d - time.Second/2)); got != tt.want {
			t.Errorf("ago(now - %v) = %q, want %q", tt.d, got, tt.want)
		}
	}
	// A file modified in the future, as with a skewed clock
	if got := ago(time.Now().Add(time.Hour)); got != "just now" {
		t.Errorf("ago(now + 1h) = %q, want %q", got, "just now")
	}
}

func TestIsText(t *testing.T) {
	for _, tt := range []struct {
		name string
		b    []byte
		want bool
	}{
		{"empty", nil, true},
		{"ascii", []byte("hello\n"), true},
		{"utf-8", []byte("日本語\n"), true},
		{"nul byte", []byte("a\x00b"), false},
		{"invalid utf-8", []byte{0xff, 0xfe, 'a', 'b', 'c', 'd'}, false},
		// The 8000-byte head may cut a multibyte character in two
		{"rune cut at the head's end", append(bytes.Repeat([]byte("a"), 7999), "日"...), true},
		{"nul after the head", append(bytes.Repeat([]byte("a"), 8000), 0), true},
	} {
		if got := isText(tt.b); got != tt.want {
			t.Errorf("isText(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRenderCodeLexer(t *testing.T) {
	for _, tt := range []struct {
		name, src, want string
	}{
		// By the file name
		{"main.go", "package main\n", `<span class="kn">package</span>`},
		// By the content, when the name tells nothing
		{"run", "#!/bin/sh\necho hi\n", `<span class="nb">echo</span>`},
		// Neither: plain text, still with line numbers
		{"notes", "just words\n", "just words"},
	} {
		got, err := renderCode(tt.name, []byte(tt.src))
		if err != nil {
			t.Fatalf("renderCode(%q): %v", tt.name, err)
		}
		if !strings.Contains(string(got), tt.want) || !strings.Contains(string(got), `href="#L1"`) {
			t.Errorf("renderCode(%q) does not contain %q and #L1:\n%s", tt.name, tt.want, got)
		}
	}
}

func TestChromaCSSSyntaxVars(t *testing.T) {
	css := chromaCSS()
	for _, want := range []string{
		// Each mode keeps GitHub's color as the fallback
		".chroma .k { color: var(--syntax-keyword, #cf222e) }",
		".chroma .k { color: var(--syntax-keyword, #ff7b72) }",
		".chroma .c1 { color: var(--syntax-comment, #57606a) }",
		// Inserted and deleted lines read a variable for the background too
		".chroma .gi { color: var(--syntax-inserted, #116329); background-color: var(--syntax-inserted-bg, #dafbe1) }",
		".chroma .gd { color: var(--syntax-deleted, #ffa198); background-color: var(--syntax-deleted-bg, #490202) }",
		// Plain text takes the page's text color
		".chroma .p { color: var(--fgColor-default) }",
		// The block takes the page's colors from markdown.css
		".chroma { -webkit-text-size-adjust: none; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("chromaCSS() does not contain %q", want)
		}
	}
	if strings.Contains(css, "#0d1117") {
		t.Error("chromaCSS() has github-dark's background")
	}
}

// Every color in markdown.css has a name a theme can set.
func TestMarkdownCSSNamedColors(t *testing.T) {
	b, err := assets.ReadFile("assets/markdown.css")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "--md-") {
		t.Error("markdown.css has unnamed --md-* colors")
	}
}

func TestCountLines(t *testing.T) {
	for in, want := range map[string]int{"": 0, "a": 1, "a\n": 1, "a\nb": 2, "a\nb\n": 2, "\n": 1} {
		if got := countLines([]byte(in)); got != want {
			t.Errorf("countLines(%q) = %d, want %d", in, got, want)
		}
	}
}
