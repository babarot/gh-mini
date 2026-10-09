package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newPluginServer serves the test repository with a directory of plugins,
// by name and file.
func newPluginServer(t *testing.T, plugins map[string]map[string]string) *Server {
	t.Helper()
	dir := t.TempDir()
	for name, files := range plugins {
		for file, body := range files {
			writeFile(t, filepath.Join(dir, name, file), []byte(body))
		}
	}
	return newServerFor(t, Options{PluginsDir: dir})
}

var partialPlugin = map[string]string{
	"plugin.json": `{"description": "Figures", "elements": ["Partial"], "read": ["figures/*.part.html", "/styles/*.css"]}`,
	"main.js":     "export default {};\n",
}

func TestReadable(t *testing.T) {
	for g, want := range map[string]bool{
		"figures/*.part.html": true,
		"/styles/*.css":       true,
		"*.md":                true,
		"":                    false,
		"/":                   false,
		"//x":                 false,
		"../x":                false,
		"a/../x":              false,
		"./x":                 false,
		`a\x`:                 false,
		"[":                   false,
	} {
		if got := readable(g); got != want {
			t.Errorf("readable(%q) = %v, want %v", g, got, want)
		}
	}
}

// The viewer's plugins are listed with those that come with gh-mini, by
// name; one of the same name as a built-in one takes its place, and one
// that cannot run tells why.
func TestPlugins(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{
		"partial":           partialPlugin,
		"front-matter-card": {"plugin.json": `{"frontMatter": true}`, "main.js": "export default {};\n"},
		"no-main":           {"plugin.json": `{"frontMatter": true}`},
		"bad-json":          {"plugin.json": `{`, "main.js": ""},
		"bad-element":       {"plugin.json": `{"elements": ["Image", "partial"]}`, "main.js": ""},
		"html-element":      {"plugin.json": `{"elements": ["BR"]}`, "main.js": ""},
		"bad-read":          {"plugin.json": `{"elements": ["X"], "read": ["../x"]}`, "main.js": ""},
		"shows-nothing":     {"plugin.json": `{}`, "main.js": ""},
		"csv":               {"plugin.json": `{"codeBlocks": ["csv", "c++"]}`, "main.js": ""},
		"code-case":         {"plugin.json": `{"codeBlocks": ["CSV"]}`, "main.js": ""},
		"code-mermaid":      {"plugin.json": `{"codeBlocks": ["mermaid"]}`, "main.js": ""},
		"Not_A_Name":        partialPlugin,
	})
	got := map[string]plugin{}
	var names []string
	for _, p := range srv.plugins() {
		got[p.Name] = p
		names = append(names, p.Name)
	}
	if want := "bad-element bad-json bad-read code-case code-mermaid csv front-matter-card html-element no-main partial shows-nothing"; strings.Join(names, " ") != want {
		t.Errorf("names: %s, want %s", strings.Join(names, " "), want)
	}
	if p := got["front-matter-card"]; p.Builtin {
		t.Error("the viewer's front-matter-card is taken as built in")
	}
	if p := got["partial"]; p.Err != "" || p.Description != "Figures" || len(p.Read) != 2 {
		t.Errorf("partial: %+v", p)
	}
	if p := got["csv"]; p.Err != "" || len(p.CodeBlocks) != 2 {
		t.Errorf("csv: %+v", p)
	}
	for name, want := range map[string]string{
		"no-main":       "No main.js",
		"bad-json":      "plugin.json: unexpected end of JSON input",
		"bad-element":   `element "partial"`,
		"html-element":  `element "BR"`,
		"bad-read":      `read "../x"`,
		"shows-nothing": "shows nothing",
		"code-case":     `code block "CSV"`,
		"code-mermaid":  "gh-mini draws it",
	} {
		if !strings.Contains(got[name].Err, want) {
			t.Errorf("%s: %q, want %q in it", name, got[name].Err, want)
		}
	}

	builtin := newServerFor(t, Options{}).plugins()
	if len(builtin) != 1 || builtin[0].Name != "front-matter-card" || !builtin[0].Builtin || builtin[0].Err != "" {
		t.Errorf("built-in plugins: %+v", builtin)
	}
}

// A page lists the plugins on for the viewer: their own until turned off,
// those that come with gh-mini once turned on. The settings list them all,
// one that cannot run turned off with why.
func TestPagePlugins(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{
		"partial": partialPlugin,
		"broken":  {"plugin.json": `{}`, "main.js": ""},
	})
	h := srv.Handler()
	host := `"host":"/_mini/plugins/` + srv.boot + `/partial/"`
	r := get(t, h, "/")
	r.expect(t, http.StatusOK,
		`<script type="application/json" id="mini.plugins">[{"name":"partial","frontMatter":false,"elements":["Partial"],"codeBlocks":[],"read":["figures/*.part.html","/styles/*.css"],`+host+`}]</script>`,
		`data-setting="plugin.partial" aria-labelledby="mini.setting-plugin.partial" checked>`,
		`data-setting="plugin.front-matter-card" aria-labelledby="mini.setting-plugin.front-matter-card">`,
		"shows nothing",
	)
	r = get(t, h, "/", withSettings(`{"plugin.partial":false,"plugin.front-matter-card":true}`))
	r.expect(t, http.StatusOK, `"name":"front-matter-card","frontMatter":true`)
	r.reject(t, `"name":"partial"`)
}

// A plugin's document, at its directory, is sandboxed, loads its scripts,
// compiles WebAssembly and fetches its files, and nothing else, and is
// there for this run only.
func TestPluginHost(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{"partial": partialPlugin})
	h := srv.Handler()
	dir := "/_mini/plugins/" + srv.boot + "/partial/"
	r := get(t, h, dir)
	r.expect(t, http.StatusOK, `data-plugin="`+dir+`main.js"`)
	want := "sandbox allow-scripts; default-src 'none'; script-src http://localhost" + srv.static.prefix() + "/assets/js/plugin-host.js http://localhost" + dir + " 'wasm-unsafe-eval'; connect-src http://localhost" + dir
	if got := r.header.Get("Content-Security-Policy"); got != want {
		t.Errorf("CSP %q, want %q", got, want)
	}
	if got := r.header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy %q", got)
	}
	for _, target := range []string{
		"/_mini/plugins/nope/partial/",
		"/_mini/plugins/" + srv.boot + "/none/",
		"/_mini/plugins/" + srv.boot + "/partial",
	} {
		if r := get(t, h, target); r.code != http.StatusNotFound {
			t.Errorf("%s: %d", target, r.code)
		}
	}
}

// A plugin's files are read by its sandbox, whose origin is "null", and by
// no other origin: its modules, WebAssembly and data, but not its hidden
// files, nor what a symlink points to out of its directory.
func TestPluginFiles(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{"partial": withPluginFiles(partialPlugin, map[string]string{
		"lib/draw.js":    "export const x = 1;\n",
		"lib/draw.wasm":  "\x00asm",
		"data.json":      "{}",
		"page.html":      "<script>x()</script>",
		"logo.png":       "\x89PNG",
		".env":           "SECRET=1",
		".git/config":    "x",
		"lib/.hidden.js": "x",
	})})
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, []byte("secret"))
	if err := os.Symlink(outside, filepath.Join(srv.opts.PluginsDir, "partial", "link.txt")); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	dir := "/_mini/plugins/" + srv.boot + "/partial/"
	for file, typ := range map[string]string{
		"main.js":       "text/javascript; charset=utf-8",
		"lib/draw.js":   "text/javascript; charset=utf-8",
		"lib/draw.wasm": "application/wasm",
		"data.json":     "application/json",
		"plugin.json":   "application/json",
	} {
		r := get(t, h, dir+file, withHeader("Origin", "null"))
		if r.code != http.StatusOK || r.header.Get("Content-Type") != typ || r.header.Get("Access-Control-Allow-Origin") != "null" {
			t.Errorf("%s: %d %q %q", file, r.code, r.header.Get("Content-Type"), r.header.Get("Access-Control-Allow-Origin"))
		}
	}
	if r := get(t, h, dir+"main.js", withHeader("Origin", "https://example.com")); r.header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("another origin may read it")
	}
	// Opened as a page, what would run runs in a sandbox
	for _, file := range []string{"main.js", "page.html"} {
		if r := get(t, h, dir+file); r.header.Get("Content-Security-Policy") != "sandbox" {
			t.Errorf("%s: CSP %q", file, r.header.Get("Content-Security-Policy"))
		}
	}
	if r := get(t, h, dir+"logo.png"); r.code != http.StatusOK || r.header.Get("Content-Security-Policy") != "" {
		t.Errorf("an image: %d %q", r.code, r.header.Get("Content-Security-Policy"))
	}
	for _, target := range []string{
		"/_mini/plugins/nope/partial/main.js",
		"/_mini/plugins/" + srv.boot + "/none/main.js",
		dir + ".env",
		dir + ".git/config",
		dir + "lib/.hidden.js",
		dir + "lib",
		dir + "lib/",
		dir + "link.txt",
	} {
		if r := get(t, h, target); r.code != http.StatusNotFound {
			t.Errorf("%s: %d", target, r.code)
		}
	}
	// A path with .. in it is cleaned, by a redirect, before it gets here
	if r := get(t, h, dir+"../partial/main.js"); r.code != http.StatusTemporaryRedirect {
		t.Errorf("with ..: %d", r.code)
	}
	// One that comes with gh-mini
	if r := get(t, newServerFor(t, Options{}).Handler(), "/_mini/plugins/"); r.code != http.StatusNotFound {
		t.Errorf("no boot: %d", r.code)
	}
	builtin := newServerFor(t, Options{})
	if r := get(t, builtin.Handler(), "/_mini/plugins/"+builtin.boot+"/front-matter-card/plugin.json"); r.code != http.StatusOK {
		t.Errorf("built-in plugin.json: %d", r.code)
	}
}

func withPluginFiles(base, more map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// A plugin's HTML comes back without what would run, its classes kept.
func TestSanitizeAPI(t *testing.T) {
	h := newServerFor(t, Options{}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/_mini/api/sanitize", strings.NewReader(`<div class="box" onclick="x()">a<script>x()</script></div>`))
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got, want := rec.Body.String(), `<div class="box">a</div>`; rec.Code != http.StatusOK || got != want {
		t.Errorf("%d %q, want %q", rec.Code, got, want)
	}
	if r := get(t, h, "/_mini/api/sanitize"); r.code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", r.code)
	}
}

// A plugin may bring themes, alone with no code: they are listed and
// served while it is on, the viewer's theme file winning over them and
// they over a built-in one; off, the page shows the built-in default.
func TestPluginThemes(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{
		"looks": {
			"plugin.json":      `{"themes": {"paper": "themes/paper.css", "nord": "themes/nord.css", "sepia": "themes/sepia.css"}}`,
			"themes/paper.css": ":root { --paper: 1; }",
			"themes/nord.css":  ":root { --plugin-nord: 1; }",
			"themes/sepia.css": ":root { --plugin-sepia: 1; }",
		},
		"bad-theme": {"plugin.json": `{"themes": {"Paper": "paper.css"}}`},
		"bad-file":  {"plugin.json": `{"themes": {"paper": "../paper.css"}}`},
		"not-css":   {"plugin.json": `{"themes": {"paper": "paper.js"}}`},
	})
	// The themes directory, from the test repository, has sepia
	themes := filepath.Join(t.TempDir(), "themes")
	writeFile(t, filepath.Join(themes, "sepia.css"), []byte(":root { --sepia: 1; }"))
	srv.opts.ThemesDir = themes
	got := map[string]plugin{}
	for _, p := range srv.plugins() {
		got[p.Name] = p
	}
	if p := got["looks"]; p.Err != "" {
		t.Errorf("a plugin of themes alone: %q", p.Err)
	}
	for name, want := range map[string]string{"bad-theme": `theme "Paper"`, "bad-file": `"../paper.css"`, "not-css": `"paper.js"`} {
		if !strings.Contains(got[name].Err, want) {
			t.Errorf("%s: %q, want %q in it", name, got[name].Err, want)
		}
	}

	h := srv.Handler()
	r := get(t, h, "/", withSettings(`{"theme":"paper"}`))
	r.expect(t, http.StatusOK, `<option value="paper" selected>paper</option>`, `href="/_mini/theme/paper.css"`)
	// It runs nothing on the page
	r.reject(t, `"name":"looks"`)
	get(t, h, "/_mini/theme/paper.css").expect(t, http.StatusOK, "--paper: 1")
	get(t, h, "/_mini/theme/nord.css").expect(t, http.StatusOK, "--plugin-nord: 1")
	get(t, h, "/_mini/theme/sepia.css").expect(t, http.StatusOK, "--sepia: 1")
	if r := get(t, h, "/_mini/theme/paper.css"); r.header.Get("Vary") != "Cookie" {
		t.Errorf("Vary %q", r.header.Get("Vary"))
	}

	off := withSettings(`{"theme":"paper","plugin.looks":false}`)
	r = get(t, h, "/", off)
	r.expect(t, http.StatusOK, `href="/_mini/theme/github.css"`)
	r.reject(t, `<option value="paper"`)
	get(t, h, "/_mini/theme/paper.css", off).expect(t, http.StatusOK, "--borderColor-success-emphasis: #238636")
	get(t, h, "/_mini/theme/nord.css", off).expect(t, http.StatusOK, "--bgColor-default: #2e3440")
}

// A theme of a plugin may be the default, kept at start, and the viewer
// who turned the plugin off gets the built-in one.
func TestPluginDefaultTheme(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "looks", "plugin.json"), []byte(`{"themes": {"paper": "paper.css"}}`))
	writeFile(t, filepath.Join(dir, "looks", "paper.css"), []byte(":root { --paper: 1; }"))
	srv := newServerFor(t, Options{PluginsDir: dir, Theme: "paper"})
	h := srv.Handler()
	get(t, h, "/").expect(t, http.StatusOK, `href="/_mini/theme/paper.css"`)
	r := get(t, h, "/", withSettings(`{"plugin.looks":false}`))
	r.expect(t, http.StatusOK, `href="/_mini/theme/github.css"`, `<option value="github" selected>`)
}
