package server

import (
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const builtinTheme = "github"

// builtinThemes lists the themes under assets/themes, the default first.
func builtinThemes() []string {
	list := []string{builtinTheme}
	matches, _ := fs.Glob(assets, "assets/themes/*.css")
	for _, m := range matches {
		if name := strings.TrimSuffix(path.Base(m), ".css"); name != builtinTheme {
			list = append(list, name)
		}
	}
	return list
}

// themes lists the built-in themes, the viewer's own, and those of the
// plugins on in values, or of every plugin that can run when values is
// nil, as for the theme given at start.
func (s *Server) themes(values map[string]string) []string {
	list := builtinThemes()
	if s.opts.ThemesDir != "" {
		matches, _ := filepath.Glob(filepath.Join(s.opts.ThemesDir, "*.css"))
		sort.Strings(matches)
		for _, m := range matches {
			name := strings.TrimSuffix(filepath.Base(m), ".css")
			if !slices.Contains(list, name) {
				list = append(list, name)
			}
		}
	}
	for _, p := range s.themePlugins(values) {
		for _, name := range slices.Sorted(maps.Keys(p.Themes)) {
			if !slices.Contains(list, name) {
				list = append(list, name)
			}
		}
	}
	return list
}

// themePlugins are the plugins with themes that are on in values, by name;
// all that can run when values is nil.
func (s *Server) themePlugins(values map[string]string) []plugin {
	var out []plugin
	for _, p := range s.plugins() {
		if p.Err == "" && len(p.Themes) > 0 && (values == nil || values[pluginSetting(p.Name)] == "true") {
			out = append(out, p)
		}
	}
	return out
}

// themeCSS is a theme's CSS: the viewer's own file wins, then a plugin's
// theme, the first by the plugin's name, then the built-in one.
func (s *Server) themeCSS(name string, values map[string]string) ([]byte, bool) {
	if s.opts.ThemesDir != "" {
		if b, err := os.ReadFile(filepath.Join(s.opts.ThemesDir, name+".css")); err == nil {
			return b, true
		}
	}
	for _, p := range s.themePlugins(values) {
		if file, ok := p.Themes[name]; ok {
			b, err := p.readFile(file)
			return b, err == nil
		}
	}
	b, err := fs.ReadFile(assets, "assets/themes/"+name+".css")
	return b, err == nil
}

// serveTheme serves a theme's CSS, read on every request so that editing a
// theme shows on the next reload. A theme the viewer has not, as one of a
// plugin turned off, is the built-in default, which the page shows then.
func (s *Server) serveTheme(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Cookie")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	name := strings.TrimSuffix(path.Base(r.URL.Path), ".css")
	if strings.ContainsAny(name, `/\`) {
		return
	}
	values := s.settings(r)
	if !slices.Contains(s.themes(values), name) {
		name = builtinTheme
	}
	b, ok := s.themeCSS(name, values)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write(b)
}
