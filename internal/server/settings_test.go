package server

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// withSettings sets the settings cookie as the page writes it, with
// encodeURIComponent.
func withSettings(json string) func(*http.Request) {
	return withCookie(settingsCookie, strings.ReplaceAll(url.QueryEscape(json), "+", "%20"))
}

func TestSettingsFromCookie(t *testing.T) {
	h := newTestServer(t)
	themeLink := func(name string) string { return `id="mini.theme" href="/_mini/theme/` + name + `.css"` }
	for _, tt := range []struct {
		name  string
		edit  []func(*http.Request)
		theme string
		mode  string // "" for Auto, which has no data-mode
	}{
		{"none", nil, "github", ""},
		{"new", []func(*http.Request){withSettings(`{"theme":"sepia","mode":"dark"}`)}, "sepia", "dark"},
		{"legacy", []func(*http.Request){withCookie("gh-mini-theme", "sepia"), withCookie("gh-mini-mode", "light")}, "sepia", "light"},
		{"new wins, legacy fills the rest", []func(*http.Request){
			withSettings(`{"theme":"github"}`), withCookie("gh-mini-theme", "sepia"), withCookie("gh-mini-mode", "dark"),
		}, "github", "dark"},
		{"invalid values", []func(*http.Request){withSettings(`{"theme":"elsewhere","mode":"purple","x":1}`)}, "github", ""},
		{"broken JSON", []func(*http.Request){withSettings(`{"theme":`)}, "github", ""},
	} {
		r := get(t, h, "/", tt.edit...)
		if !strings.Contains(r.body, themeLink(tt.theme)) {
			t.Errorf("%s: theme is not %s", tt.name, tt.theme)
		}
		hasMode := strings.Contains(r.body, `data-mode="`+tt.mode+`"`)
		if tt.mode == "" {
			hasMode = !strings.Contains(r.body, `<html lang="en" data-mode=`)
		}
		if !hasMode {
			t.Errorf("%s: mode is not %q", tt.name, tt.mode)
		}
	}
}

func TestSettingsDefaultThemeFromOptions(t *testing.T) {
	root, themes := newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "repo", ThemesDir: themes, Theme: "sepia"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	get(t, srv.Handler(), "/").expect(t, http.StatusOK, `id="mini.theme" href="/_mini/theme/sepia.css"`)
}

// A setting marked Attr shows on <html> for CSS; a toggle takes JSON
// booleans.
func TestSettingsAttr(t *testing.T) {
	saved := settingDefs
	defer func() { settingDefs = saved }()
	settingDefs = append(append([]setting(nil), saved...), setting{
		Key: "wide", Label: "Wide", Control: "toggle", Attr: true,
		Default: func(*Server) string { return "false" },
	})
	h := newTestServer(t)
	get(t, h, "/").expect(t, http.StatusOK, ` data-wide="false">`)
	get(t, h, "/", withSettings(`{"wide":true}`)).expect(t, http.StatusOK, ` data-wide="true">`)
	get(t, h, "/", withSettings(`{"wide":"yes"}`)).expect(t, http.StatusOK, ` data-wide="false">`)
}

func TestSettingsDialog(t *testing.T) {
	h := newTestServer(t)
	r := get(t, h, "/", withSettings(`{"theme":"sepia","mode":"dark"}`))
	r.expect(t, http.StatusOK,
		`popovertarget="mini.menu"`,
		`data-open="mini.settings"`,
		`<dialog class="settings" id="mini.settings"`,
		`<select data-setting="theme"`,
		`<option value="sepia" selected>sepia</option>`,
		`data-setting="mode" data-value="dark" aria-checked="true" tabindex="0" class="selected">Dark</button>`,
		`data-setting="mode" data-value="" aria-checked="false" tabindex="-1">Auto</button>`,
		`<script type="application/json" id="mini.settings-data">{"avatars":"true","changedWords":"true","changes":"true","hideIgnoredDirs":"false","htmlPreview":"false","ignoreWhitespace":"false","languageSwitch":"true","mode":"dark","openChanged":"file","plugin.front-matter-card":"false","theme":"sepia","treeMarks":"letter","untracked":"true","wide":"false","wrap":"false"}</script>`,
	)
	r.reject(t, `id="theme-select"`, `id="mode-select"`)
}

// A default theme that does not exist falls back to the built-in one.
func TestSettingsMissingDefaultTheme(t *testing.T) {
	root, themes := newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "repo", ThemesDir: themes, Theme: "nope"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	get(t, srv.Handler(), "/").expect(t, http.StatusOK, `id="mini.theme" href="/_mini/theme/github.css"`)
}

// Settings are grouped by section, in the order of their first setting;
// one that names none goes under defaultSection.
func TestSettingSections(t *testing.T) {
	saved := settingDefs
	defer func() { settingDefs = saved }()
	none := func(*Server) string { return "" }
	settingDefs = []setting{
		{Key: "a", Section: "Look and Feel", Control: "toggle", Default: none},
		{Key: "b", Control: "toggle", Default: none},
		{Key: "c", Section: "Look and Feel", Control: "toggle", Default: none},
	}
	var got []string
	for _, sec := range (&Server{}).settingSections(map[string]string{}) {
		keys := ""
		for _, s := range sec.Settings {
			keys += s.Key
		}
		got = append(got, sec.Name+"|"+sec.ID+"|"+keys)
	}
	// The plugins, here the one that comes with gh-mini, come last
	want := []string{"Look and Feel|look-and-feel|ac", "General|general|b", "Plugins|plugins|plugin.front-matter-card"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSettingsDialogSections(t *testing.T) {
	get(t, newTestServer(t), "/").expect(t, http.StatusOK,
		`<button type="button" role="tab" id="mini.settings-tab-appearance" data-section="appearance" aria-controls="mini.settings-appearance" aria-selected="true" tabindex="0">Appearance</button>`,
		`<section class="settings-pane" role="tabpanel" id="mini.settings-files" aria-labelledby="mini.settings-tab-files" hidden>`,
	)
}

// Full width and wrapped code are set on <html>, for CSS.
func TestSettingsLayoutAttrs(t *testing.T) {
	h := newTestServer(t)
	get(t, h, "/").expect(t, http.StatusOK, ` data-wide="false"`, ` data-wrap="false"`)
	get(t, h, "/", withSettings(`{"wide":true,"wrap":true}`)).expect(t, http.StatusOK, ` data-wide="true"`, ` data-wrap="true"`)
}

// Only the outermost directory git ignores is marked to hide, and only
// outside one: a listing of an ignored directory shows what is in it.
// Files git ignores are never marked.
func TestHideIgnoredDirs(t *testing.T) {
	root, themes := newTestRepo(t)
	writeFile(t, filepath.Join(root, "local-only", "sub", "x.md"), []byte("# X\n"))
	srv, err := New(Options{Root: root, Name: "repo", ThemesDir: themes})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	h := srv.Handler()
	r := get(t, h, "/", withSettings(`{"hideIgnoredDirs":true}`))
	r.expect(t, http.StatusOK, ` data-hideIgnoredDirs="true"`)
	if row := r.row(t, "README.ja.md"); !strings.Contains(row, `class="ignored"`) {
		t.Errorf("an ignored file is marked to hide: %s", row)
	}
	if row := get(t, h, "/local-only/").row(t, "sub"); strings.Contains(row, "hideable") {
		t.Errorf("a directory in an ignored one is marked to hide: %s", row)
	}
}

// changedRepo serves a repository with a file modified, one untracked,
// one deleted and one changed in its whitespace alone.
func changedRepo(t *testing.T, opts Options) http.Handler {
	t.Helper()
	root, themes := newTestRepo(t)
	writeFile(t, filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println() }\n"))
	writeFile(t, filepath.Join(root, "docs", "new.md"), []byte("# New\n"))
	writeFile(t, filepath.Join(root, "docs", "guide.md"), []byte("#  Guide\n\n## Install\n"))
	if err := os.Remove(filepath.Join(root, "docs", "math.md")); err != nil {
		t.Fatal(err)
	}
	opts.Root, opts.Name, opts.ThemesDir = root, "repo", themes
	opts.Skip = []string{".git", "node_modules"}
	srv, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv.Handler()
}

// With Changes off, nothing tells what changed.
func TestSettingChangesOff(t *testing.T) {
	h := changedRepo(t, Options{})
	off := withSettings(`{"changes":false}`)
	r := get(t, h, "/", off)
	r.reject(t, `id="mini.changes"`, "mini.changed-only", "status-badge")
	// The settings under it are shown doing nothing
	r.expect(t, http.StatusOK, `class="setting child disabled" data-parent="changes"`)
	get(t, h, "/main.go", off).reject(t, "uncommitted", ">Diff")
	for _, target := range []string{"/_mini/changes", "/docs/math.md"} {
		if r := get(t, h, target, off); r.code != http.StatusNotFound {
			t.Errorf("%s: %d", target, r.code)
		}
	}
	get(t, h, "/_mini/api/status", off).expect(t, http.StatusOK, `"files":{}`)
}

// --no-changes reads nothing from git, and the setting says why it is off.
func TestNoChangesFlag(t *testing.T) {
	h := changedRepo(t, Options{NoChanges: true})
	r := get(t, h, "/")
	r.reject(t, `id="mini.changes"`)
	r.expect(t, http.StatusOK, "gh-mini was started with --no-changes", `class="setting child disabled" data-parent="changes"`)
	get(t, h, "/_mini/api/status").expect(t, http.StatusOK, `"git":false`)
}

func TestSettingUntracked(t *testing.T) {
	h := changedRepo(t, Options{})
	off := withSettings(`{"untracked":false}`)
	r := get(t, h, "/_mini/api/status", off)
	r.reject(t, "docs/new.md")
	r.expect(t, http.StatusOK, "main.go")
	if on := get(t, h, "/_mini/api/status"); on.header.Get("ETag") == r.header.Get("ETag") {
		t.Error("the same ETag with untracked files and without")
	}
	get(t, h, "/", off).expect(t, http.StatusOK, "3 changes")
	get(t, h, "/_mini/changes", off).reject(t, "docs/new.md")
}

func TestSettingIgnoreWhitespace(t *testing.T) {
	h := changedRepo(t, Options{})
	get(t, h, "/docs/guide.md?diff=1").expect(t, http.StatusOK, `class="text add"`)
	get(t, h, "/docs/guide.md?diff=1", withSettings(`{"ignoreWhitespace":true}`)).expect(t, http.StatusOK, "Only whitespace changed.")
}

func TestSettingOpenChanged(t *testing.T) {
	h := changedRepo(t, Options{})
	diff := withSettings(`{"openChanged":"diff"}`)
	r := get(t, h, "/main.go", diff)
	r.expect(t, http.StatusOK, `data-kind="diff"`, `href="?diff=0">Code`)
	get(t, h, "/main.go?diff=0", diff).expect(t, http.StatusOK, `data-kind="code"`)
	// A file not changed opens as it is
	get(t, h, "/README.md", diff).expect(t, http.StatusOK, `data-kind="markdown"`)
	get(t, h, "/main.go").expect(t, http.StatusOK, `data-kind="code"`)
}
