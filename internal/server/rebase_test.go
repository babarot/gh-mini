package server

import (
	"html/template"
	"testing"
)

func TestRebase(t *testing.T) {
	in := template.HTML(`<p><a href="guide.md">g</a> <a href="../up.md">u</a> <a href="#top">t</a> <a href="/root.md">r</a> ` +
		`<a href="https://example.com/x">x</a> <a href="mailto:a@example.com">m</a> <a href="?lang=ja">q</a> <a href="a:b/c.md">c</a></p>` +
		`<p><a href="pic.png" target="_blank"><img src="pic.png" alt="a &amp; b"></a> <img src="data:image/png;base64,AA" alt=""></p>` +
		`<picture><source srcset="dark.png 1x, dark@2x.png 2x" media="(prefers-color-scheme: dark)"><img src="light.png"></picture>` +
		`<p>a &lt; b &amp; &#39;c&#39;</p>`)
	want := template.HTML(`<p><a href="ja/guide.md">g</a> <a href="ja/../up.md">u</a> <a href="#top">t</a> <a href="/root.md">r</a> ` +
		`<a href="https://example.com/x">x</a> <a href="mailto:a@example.com">m</a> <a href="?lang=ja">q</a> <a href="a:b/c.md">c</a></p>` +
		`<p><a href="ja/pic.png" target="_blank"><img src="ja/pic.png" alt="a &amp; b"></a> <img src="data:image/png;base64,AA" alt=""></p>` +
		`<picture><source srcset="ja/dark.png 1x, ja/dark@2x.png 2x" media="(prefers-color-scheme: dark)"><img src="ja/light.png"></picture>` +
		`<p>a &lt; b &amp; &#39;c&#39;</p>`)
	if got := rebase(in, "ja/"); got != want {
		t.Errorf("rebase:\n got %s\nwant %s", got, want)
	}
	if got := rebase(in, ""); got != in {
		t.Errorf("no prefix changed the HTML: %s", got)
	}
	for _, tt := range []struct{ dir, readme, want string }{
		{"docs", "docs/ja", "ja/"},
		{".", "ja", "ja/"},
		{"docs", "docs", ""},
		{"docs/guide", "docs/i18n/ja", "../i18n/ja/"},
	} {
		if got := readmePrefix(tt.dir, tt.readme); got != tt.want {
			t.Errorf("readmePrefix(%q, %q) = %q, want %q", tt.dir, tt.readme, got, tt.want)
		}
	}
}
