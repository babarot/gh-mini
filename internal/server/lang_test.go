package server

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
)

func mustTranslations(t *testing.T, value string) translationLayouts {
	t.Helper()
	ls, err := parseTranslations(value)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func TestParseTranslations(t *testing.T) {
	for _, value := range []string{"", "off", "suffix", "dir", "suffix, dir", "{name}_{lang}", "{name}_{lang}.md", "i18n/{lang}/{name}"} {
		if err := CheckTranslations(value); err != nil {
			t.Errorf("CheckTranslations(%q): %v", value, err)
		}
	}
	for _, value := range []string{
		"off,suffix",
		"{name}",
		"{lang}",
		"{name}.{lang}.{lang}",
		"{name}{lang}",
		"{lang}{name}",
		"{name}.{region}.{lang}",
		"/{lang}/{name}",
		"../{lang}/{name}",
		"./{lang}/{name}",
		"{lang}//{name}",
		"{name}/{lang}",
	} {
		if err := CheckTranslations(value); err == nil {
			t.Errorf("CheckTranslations(%q) is no error", value)
		}
	}
	if ls := mustTranslations(t, "off"); ls != nil {
		t.Errorf("off: got %d layouts", len(ls))
	}
}

func TestTranslationsSplit(t *testing.T) {
	type split struct{ dir, name, lang string }
	for value, cases := range map[string]map[string]split{
		"suffix": {
			"README.md":           {".", "readme", ""},
			"README.ja.md":        {".", "readme", "ja"},
			"docs/guide.zh-TW.md": {"docs", "guide", "zh-tw"},
			"notes.markdown":      {".", "notes", ""},
			"v1.2.md":             {".", "v1.2", ""},
			"my.notes.ja.md":      {".", "my.notes", "ja"},
			"guide.ja.mdx":        {".", "guide", "ja"},
			"api.go.md":           {".", "api.go", ""},
			"build.sh.md":         {".", "build.sh", ""},
			"notes.github.md":     {".", "notes.github", ""},
			"guide.pt-BR.md":      {".", "guide", "pt-br"},
			"docs/ja/guide.md":    {"docs/ja", "guide", ""},
		},
		"dir": {
			"README.md":        {".", "readme", ""},
			"ja/README.md":     {".", "readme", "ja"},
			"docs/zh-TW/a.md":  {"docs", "a", "zh-tw"},
			"docs/go/a.md":     {"docs/go", "a", ""},
			"docs/guide.ja.md": {"docs", "guide.ja", ""},
		},
		"{name}_{lang},suffix": {
			"README_ja.md": {".", "readme", "ja"},
			"README.ja.md": {".", "readme", "ja"},
			"my_api_go.md": {".", "my_api_go", ""},
		},
		"i18n/{lang}/{name}": {
			"docs/i18n/ja/guide.md": {"docs", "guide", "ja"},
			"docs/ja/guide.md":      {"docs/ja", "guide", ""},
		},
		"off": {
			"README.ja.md": {".", "readme.ja", ""},
		},
	} {
		ls := mustTranslations(t, value)
		for rel, want := range cases {
			g, lang := ls.split(rel)
			if got := (split{g.dir, g.name, lang}); got != want {
				t.Errorf("%s: split(%q) = %v, want %v", value, rel, got, want)
			}
		}
	}
}

func TestTranslationsMembers(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md":                {},
		"README.ja.md":             {},
		"README_fr.md":             {},
		"ja/README.md":             {},
		"skip/README.md":           {},
		"id/README.md":             {},
		"docs/guide.md":            {},
		"docs/i18n/ja/guide.md":    {},
		"docs/i18n/zh-TW/guide.md": {},
		"docs/i18n/go/guide.md":    {},
		"docs/other.md":            {},
		"main.go":                  {},
	}
	skipped := func(name string) bool { return name == "skip" }
	members := func(value, rel string) []member {
		ls := mustTranslations(t, value)
		g, _ := ls.split(rel)
		return ls.members(fs.FS(fsys), skipped, g)
	}
	for _, tt := range []struct {
		value, rel string
		want       []member
	}{
		{"suffix", "README.ja.md", []member{{"README.md", ""}, {"README.ja.md", "ja"}}},
		{"dir", "README.md", []member{{"README.md", ""}, {"id/README.md", "id"}, {"ja/README.md", "ja"}}},
		{"suffix,dir,{name}_{lang}", "ja/README.md", []member{{"README.md", ""}, {"README.ja.md", "ja"}, {"README_fr.md", "fr"}, {"id/README.md", "id"}, {"ja/README.md", "ja"}}},
		{"i18n/{lang}/{name}", "docs/guide.md", []member{{"docs/guide.md", ""}, {"docs/i18n/ja/guide.md", "ja"}, {"docs/i18n/zh-TW/guide.md", "zh-tw"}}},
		{"off", "README.md", []member{{"README.md", ""}}},
		{"suffix", "docs/other.md", []member{{"docs/other.md", ""}}},
	} {
		if got := members(tt.value, tt.rel); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: members of %q = %v, want %v", tt.value, tt.rel, got, tt.want)
		}
	}
}

func TestPickReadme(t *testing.T) {
	members := []member{{"README.md", ""}, {"README.ja.md", "ja"}}
	tests := []struct{ query, saved, want string }{
		{"", "", "README.md"},
		{"ja", "", "README.ja.md"},
		{"JA", "", "README.ja.md"},
		{"", "ja", "README.ja.md"},
		{"default", "ja", "README.md"},
		{"fr", "", "README.md"},
	}
	for _, tt := range tests {
		if got := pickReadme(members, tt.query, tt.saved); got.Rel != tt.want {
			t.Errorf("pickReadme(%q, %q) = %q, want %q", tt.query, tt.saved, got.Rel, tt.want)
		}
	}
	if got := pickReadme([]member{{"readme.ja.md", "ja"}}, "", ""); got.Rel != "readme.ja.md" {
		t.Errorf("only a translation: got %q", got.Rel)
	}
}

func TestTranslationsLabel(t *testing.T) {
	for value, want := range map[string]string{
		"suffix":                "{name}.{lang}.md",
		"dir":                   "{lang}/{name}.md",
		"{name}_{lang}":         "{name}_{lang}.md",
		"dir,{name}_{lang}.mdx": "{lang}/{name}.md, {name}_{lang}.mdx",
	} {
		if got := translationsLabel(value); got != want {
			t.Errorf("translationsLabel(%q) = %q, want %q", value, got, want)
		}
	}
}
