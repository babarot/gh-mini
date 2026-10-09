package server

import (
	"net/http"
	"net/http/httptest"
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
	host := `"host":"/_mini/plugin-host/` + srv.boot + `/partial"`
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

// A plugin's document is sandboxed, loads its two scripts and nothing
// else, and is there for this run only.
func TestPluginHost(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{"partial": partialPlugin})
	h := srv.Handler()
	r := get(t, h, "/_mini/plugin-host/"+srv.boot+"/partial")
	r.expect(t, http.StatusOK, `data-plugin="/_mini/plugins/`+srv.boot+`/partial/main.js"`)
	want := "sandbox allow-scripts; default-src 'none'; script-src http://localhost" + srv.static.prefix() + "/assets/js/plugin-host.js http://localhost/_mini/plugins/" + srv.boot + "/partial/"
	if got := r.header.Get("Content-Security-Policy"); got != want {
		t.Errorf("CSP %q, want %q", got, want)
	}
	if got := r.header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy %q", got)
	}
	for _, target := range []string{
		"/_mini/plugin-host/nope/partial",
		"/_mini/plugin-host/" + srv.boot + "/none",
		"/_mini/plugin-host/" + srv.boot + "/partial/x",
	} {
		if r := get(t, h, target); r.code != http.StatusNotFound {
			t.Errorf("%s: %d", target, r.code)
		}
	}
}

// A plugin's code is read by its sandbox, whose origin is "null", and by
// no other origin.
func TestPluginCode(t *testing.T) {
	srv := newPluginServer(t, map[string]map[string]string{"partial": partialPlugin})
	h := srv.Handler()
	code := "/_mini/plugins/" + srv.boot + "/partial/main.js"
	r := get(t, h, code, withHeader("Origin", "null"))
	r.expect(t, http.StatusOK, "export default")
	if got := r.header.Get("Access-Control-Allow-Origin"); got != "null" {
		t.Errorf("Access-Control-Allow-Origin %q", got)
	}
	if got := r.header.Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type %q", got)
	}
	if r := get(t, h, code, withHeader("Origin", "https://example.com")); r.header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("another origin may read it")
	}
	for _, target := range []string{
		"/_mini/plugins/nope/partial/main.js",
		"/_mini/plugins/" + srv.boot + "/partial/plugin.json",
		"/_mini/plugins/" + srv.boot + "/none/main.js",
	} {
		if r := get(t, h, target); r.code != http.StatusNotFound {
			t.Errorf("%s: %d", target, r.code)
		}
	}
	// A path with .. in it is cleaned, by a redirect, before it gets here
	if r := get(t, h, "/_mini/plugins/"+srv.boot+"/partial/../partial/main.js"); r.code != http.StatusTemporaryRedirect {
		t.Errorf("with ..: %d", r.code)
	}
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
