package server

import (
	"net/http"
	"net/url"
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
	themeLink := func(name string) string { return `id="theme" href="/_mini/theme/` + name + `.css"` }
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
	get(t, srv.Handler(), "/").expect(t, http.StatusOK, `id="theme" href="/_mini/theme/sepia.css"`)
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
		`id="settings-open"`,
		`<dialog class="settings" id="settings"`,
		`<select data-setting="theme"`,
		`<option value="sepia" selected>sepia</option>`,
		`data-setting="mode" data-value="dark" aria-checked="true" class="selected">Dark</button>`,
		`data-setting="mode" data-value="" aria-checked="false">Auto</button>`,
		`<script type="application/json" id="settings-data">{"mode":"dark","theme":"sepia"}</script>`,
	)
	r.reject(t, `id="theme-select"`, `id="mode-select"`)
}
