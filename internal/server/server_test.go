package server

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

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

func TestDecodeText(t *testing.T) {
	for _, tt := range []struct {
		name   string
		b      []byte
		ok     bool
		notice bool
	}{
		{"empty", nil, true, false},
		{"ascii", []byte("hello\n"), true, false},
		{"utf-8", []byte("日本語\n"), true, false},
		{"utf-8 with a byte order mark", []byte("\xef\xbb\xbfhi\n"), true, false},
		{"utf-16le", []byte{0xff, 0xfe, 'h', 0, 'i', 0}, true, false},
		{"utf-16be", []byte{0xfe, 0xff, 0, 'h', 0, 'i'}, true, false},
		{"nul byte", []byte("a\x00b"), false, false},
		{"not utf-8", []byte{'a', 0xc3, 0x28, 'b'}, true, true},
		{"nul after the head", append(bytes.Repeat([]byte("a"), 8000), 0), true, false},
	} {
		text, notice, ok := decodeText(tt.b)
		if ok != tt.ok || (notice != "") != tt.notice {
			t.Errorf("decodeText(%s) = ok %v, notice %q; want ok %v, notice %v", tt.name, ok, notice, tt.ok, tt.notice)
		}
		if ok && !utf8.Valid(text) {
			t.Errorf("decodeText(%s) gave text that is not UTF-8", tt.name)
		}
	}
	if text, _, _ := decodeText([]byte{0xff, 0xfe, 'h', 0, 0xe9, 0}); string(text) != "hé" {
		t.Errorf("utf-16le decoded to %q", text)
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
