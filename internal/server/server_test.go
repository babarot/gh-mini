package server

import "testing"

func TestLangOf(t *testing.T) {
	for name, want := range map[string]string{
		"README.md":      "",
		"README.ja.md":   "ja",
		"guide.zh-TW.md": "zh-tw",
		"notes.markdown": "",
		"v1.2.md":        "",
		"my.notes.ja.md": "ja",
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
