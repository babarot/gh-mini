package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/babarot/gh-mini/internal/markdown"
)

// A plugin shows a part of a Markdown file its own way: the front matter,
// a component such as <Partial name="figure" />, or code blocks of a
// language; or brings themes, CSS for the whole page, as a theme file is. It is a directory of
// plugin.json, which tells what it shows and which files it reads, and
// main.js, an ES module. Plugins come with gh-mini, under assets/plugins,
// or are the viewer's own, in Options.PluginsDir; never from the directory
// served, whose files are only ever data to a plugin.
//
// A plugin runs apart from the page, in an iframe sandboxed to an origin
// of its own, with nothing to load or fetch: it reaches neither the page
// nor gh-mini's API. It is handed what it shows, and reads files through
// the page, which gives it only those its plugin.json names. What it gives
// back is sanitized, and shown in a shadow root, where the page's styles
// and its own keep apart (see plugins.js).
//
// The URLs of a plugin's iframe and code hold the boot ID: a page on
// another site, which can send the Origin "null" the iframe sends too, must
// not load them.

// plugin is a plugin, as plugin.json tells it.
type plugin struct {
	Name        string   `json:"-"`
	Description string   `json:"description"`
	FrontMatter bool     `json:"frontMatter"`
	Elements    []string `json:"elements"`
	CodeBlocks  []string `json:"codeBlocks"`
	// Themes are the plugin's themes, by name, each a CSS file of its
	Themes map[string]string `json:"themes"`
	Read   []string          `json:"read"`
	// Builtin is set for a plugin that comes with gh-mini
	Builtin bool `json:"-"`
	// Err tells why the plugin cannot run, "" when it can
	Err string `json:"-"`
	// fsys holds its files, and dir is where, for a viewer's plugin
	fsys fs.FS
	dir  string
}

var (
	pluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// themeName is the name of a plugin's theme
	themeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	// codeLanguage is a language a plugin shows code blocks of
	codeLanguage = regexp.MustCompile(`^[a-z0-9][a-z0-9_+.#-]*$`)
)

// plugins lists the plugins, the viewer's own and those that come with
// gh-mini, by name. A viewer's plugin takes the place of a built-in one of
// the same name, as a theme does. They are read on every page, as themes
// are, so that a plugin added shows on the next load.
func (s *Server) plugins() []plugin {
	var out []plugin
	if s.opts.PluginsDir != "" {
		entries, _ := os.ReadDir(s.opts.PluginsDir)
		for _, e := range entries {
			if pluginName.MatchString(e.Name()) && isDir(s.opts.PluginsDir, e) {
				dir := filepath.Join(s.opts.PluginsDir, e.Name())
				p := loadPlugin(e.Name(), os.DirFS(dir), false)
				p.dir = dir
				out = append(out, p)
			}
		}
	}
	builtin, _ := fs.Sub(assets, "assets/plugins")
	entries, _ := fs.ReadDir(builtin, ".")
	for _, e := range entries {
		if !e.IsDir() || slices.ContainsFunc(out, func(p plugin) bool { return p.Name == e.Name() }) {
			continue
		}
		sub, _ := fs.Sub(builtin, e.Name())
		out = append(out, loadPlugin(e.Name(), sub, true))
	}
	slices.SortFunc(out, func(a, b plugin) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// isDir tells a directory, or a symlink to one, as a plugin may be linked
// from where it is written.
func isDir(dir string, e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	info, err := os.Stat(filepath.Join(dir, e.Name()))
	return err == nil && info.IsDir()
}

func loadPlugin(name string, fsys fs.FS, builtin bool) plugin {
	p := plugin{Name: name, Builtin: builtin, fsys: fsys}
	b, err := fs.ReadFile(fsys, "plugin.json")
	if err != nil {
		p.Err = "No plugin.json"
		return p
	}
	if err := json.Unmarshal(b, &p); err != nil {
		p.Err = "plugin.json: " + err.Error()
		return p
	}
	if err := p.check(); err != nil {
		p.Err = "plugin.json: " + err.Error()
		return p
	}
	// A plugin of themes alone runs no code
	if _, err := fs.Stat(fsys, "main.js"); err != nil && p.hooks() {
		p.Err = "No main.js"
	}
	return p
}

// hooks tells whether the plugin shows anything with code of its own.
func (p *plugin) hooks() bool {
	return p.FrontMatter || len(p.Elements) > 0 || len(p.CodeBlocks) > 0
}

// pluginFile tells a path of a plugin's own file, as it is served: not a
// hidden one, nor one in a hidden directory.
func pluginFile(rel string) bool {
	return fs.ValidPath(rel) && rel != "." && !slices.ContainsFunc(strings.Split(rel, "/"), func(part string) bool { return strings.HasPrefix(part, ".") })
}

// readFile reads a file of the plugin's; for a viewer's plugin, through
// its directory, which a symlink in it does not lead out of.
func (p *plugin) readFile(rel string) ([]byte, error) {
	if !pluginFile(rel) {
		return nil, fs.ErrNotExist
	}
	fsys := p.fsys
	if p.dir != "" {
		root, err := os.OpenRoot(p.dir)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		fsys = root.FS()
	}
	if info, err := fs.Stat(fsys, rel); err != nil || info.IsDir() {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(fsys, rel)
}

// check tells what is wrong with what plugin.json says.
func (p *plugin) check() error {
	if !p.hooks() && len(p.Themes) == 0 {
		return errors.New("shows nothing: give frontMatter, elements, codeBlocks or themes")
	}
	for name, file := range p.Themes {
		if !themeName.MatchString(name) {
			return fmt.Errorf("theme %q: a name is in lower case, of letters, digits, _ and -", name)
		}
		if !pluginFile(file) || path.Ext(file) != ".css" {
			return fmt.Errorf("theme %q: %q is not a CSS file of the plugin's, relative to its directory, without .. or names starting with .", name, file)
		}
	}
	for _, e := range p.Elements {
		if !markdown.IsComponent(e) {
			return fmt.Errorf("element %q: a name is of letters, digits and hyphens, and starts with a capital, as <Partial /> does; one of HTML's in capitals alone, such as BR, is HTML", e)
		}
	}
	for _, l := range p.CodeBlocks {
		if !codeLanguage.MatchString(l) {
			return fmt.Errorf("code block %q: a language is in lower case, of letters, digits and _+.#-, matched as the fence writes it in any case", l)
		}
		if l == "mermaid" || l == "math" {
			return fmt.Errorf("code block %q: gh-mini draws it", l)
		}
	}
	for _, g := range p.Read {
		if !readable(g) {
			return fmt.Errorf("read %q: a pattern is relative to the file shown, or to the directory served when it starts with /, without ..", g)
		}
	}
	return nil
}

// readable tells a pattern of files a plugin may read: relative to the
// directory of the file shown, and under it, or with a / before it,
// relative to the directory served, as a site's shared styles are.
func readable(g string) bool {
	g = strings.TrimPrefix(g, "/")
	if g == "" || strings.HasPrefix(g, "/") || strings.Contains(g, `\`) || path.Clean(g) != g {
		return false
	}
	for _, part := range strings.Split(g, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	_, err := path.Match(g, "")
	return err == nil
}

// pluginView is a plugin as the page takes it.
type pluginView struct {
	Name        string   `json:"name"`
	FrontMatter bool     `json:"frontMatter"`
	Elements    []string `json:"elements"`
	CodeBlocks  []string `json:"codeBlocks"`
	Read        []string `json:"read"`
	Host        string   `json:"host"`
}

// pluginSetting is the key of the setting that turns a plugin on.
func pluginSetting(name string) string {
	return "plugin." + name
}

// enabledPlugins are the plugins that run on a page with these settings.
func (s *Server) enabledPlugins(plugins []plugin, settings map[string]string) []pluginView {
	out := []pluginView{}
	for _, p := range plugins {
		// A plugin of themes alone has nothing to run on a page
		if p.Err != "" || settings[pluginSetting(p.Name)] != "true" || !p.hooks() {
			continue
		}
		out = append(out, pluginView{
			Name:        p.Name,
			FrontMatter: p.FrontMatter,
			Elements:    cmpOrEmpty(p.Elements),
			CodeBlocks:  cmpOrEmpty(p.CodeBlocks),
			Read:        cmpOrEmpty(p.Read),
			Host:        "/_mini/plugins/" + s.boot + "/" + p.Name + "/",
		})
	}
	return out
}

func cmpOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// pluginSettings are the settings that turn each plugin on: the viewer's
// own are on until turned off, as putting one in the directory asks for
// it, and those that come with gh-mini off until turned on.
func pluginSettings(plugins []plugin) []setting {
	var out []setting
	for _, p := range plugins {
		def := "true"
		desc := p.Description
		if p.Builtin {
			def = "false"
			desc = strings.TrimSpace(desc + " (comes with gh-mini)")
		}
		reason := p.Err
		out = append(out, setting{
			Key:         pluginSetting(p.Name),
			Section:     "Plugins",
			Label:       p.Name,
			Description: desc,
			Control:     "toggle",
			Default:     func(*Server) string { return def },
			Unavailable: func(*Server) string { return reason },
		})
	}
	return out
}

// findPlugin is the plugin of a name that can run.
func (s *Server) findPlugin(name string) (plugin, bool) {
	for _, p := range s.plugins() {
		if p.Name == name && p.Err == "" {
			return p, true
		}
	}
	return plugin{}, false
}

// pluginPath splits /<prefix><boot>/<name>[/<rest>], for the boot ID of
// this run only. dir is set when the path goes on past the name.
func (s *Server) pluginPath(r *http.Request, prefix string) (name, rest string, dir, ok bool) {
	p := strings.TrimPrefix(r.URL.Path, prefix)
	boot, p, _ := strings.Cut(p, "/")
	if boot != s.boot {
		return "", "", false, false
	}
	name, rest, dir = strings.Cut(p, "/")
	return name, rest, dir, pluginName.MatchString(name)
}

// servePlugin serves a plugin: at its directory, the document it runs
// in, and under it, its files. The document is at the directory so that
// what the plugin names relative to it, as fetch("./data.json") does,
// resolves among its files, as its imports do.
func (s *Server) servePlugin(w http.ResponseWriter, r *http.Request) {
	name, rest, dir, ok := s.pluginPath(r, "/_mini/plugins/")
	if !ok || !dir {
		http.NotFound(w, r)
		return
	}
	p, ok := s.findPlugin(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if rest == "" {
		s.servePluginHost(w, r, name)
		return
	}
	s.servePluginFile(w, r, p, rest)
}

// servePluginHost serves the document a plugin runs in: plugin-host.js,
// which imports the plugin's main.js once the page has handed it a channel
// to talk on. Its policy sandboxes it, even opened on its own, to an origin
// of its own, where it loads those two scripts and the plugin's modules,
// compiles WebAssembly, and fetches the plugin's files, and nothing else.
func (s *Server) servePluginHost(w http.ResponseWriter, r *http.Request, name string) {
	// 'self' would be the sandbox's own origin, which matches nothing
	origin := "http://" + r.Host
	host := s.static.prefix() + "/assets/js/plugin-host.js"
	code := "/_mini/plugins/" + s.boot + "/" + name + "/"
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"sandbox allow-scripts",
		"default-src 'none'",
		"script-src " + origin + host + " " + origin + code + " 'wasm-unsafe-eval'",
		"connect-src " + origin + code,
	}, "; "))
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html>\n<meta charset=\"utf-8\">\n<script src=\"%s\" data-plugin=\"%s\"></script>\n",
		template.HTMLEscapeString(host), template.HTMLEscapeString(code+"main.js"))
}

// servePluginFile serves a file of a plugin's: its modules, WebAssembly
// and data, which its sandbox, whose origin is "null", imports and fetches;
// so it allows that origin, and none other, to read them. Hidden files are
// not served, nor, for a viewer's plugin, what a symlink in its directory
// points to out of it.
func (s *Server) servePluginFile(w http.ResponseWriter, r *http.Request, p plugin, rel string) {
	b, err := p.readFile(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Vary", "Origin")
	if r.Header.Get("Origin") == "null" {
		w.Header().Set("Access-Control-Allow-Origin", "null")
	}
	ctype := pluginFileType(rel)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !passive(ctype) {
		// Opened as a page, a plugin's HTML would run as gh-mini, as a
		// file served raw would; a sandbox does nothing to a module
		// imported or a file fetched
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

// pluginFileType is the type a plugin's file is served as.
func pluginFileType(name string) string {
	switch path.Ext(name) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".wasm":
		return "application/wasm"
	case ".json":
		return "application/json"
	case ".css":
		return "text/css; charset=utf-8"
	}
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// maxPluginHTML is the most HTML a plugin may give back at once.
const maxPluginHTML = 4 << 20

// serveSanitize sanitizes a plugin's HTML for the page to show. It only
// gives back what it is given, less what must not run, so a request from
// another site gets nothing it can read, and needs no check.
func (s *Server) serveSanitize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPluginHTML))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(markdown.SanitizePlugin(b))
}
