package server

import (
	"net/http"
	"net/url"
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
		`<script type="application/json" id="mini.settings-data">{"htmlPreview":"false","mode":"dark","theme":"sepia","translations":"suffix"}</script>`,
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
	want := []string{"Look and Feel|look-and-feel|ac", "General|general|b"}
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
