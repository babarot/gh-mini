package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const builtinTheme = "github"

// builtinThemes lists the themes that come with gh-mini: the default, which
// adds nothing, and the ones under assets/themes.
func builtinThemes() []string {
	list := []string{builtinTheme}
	matches, _ := fs.Glob(assets, "assets/themes/*.css")
	for _, m := range matches {
		list = append(list, strings.TrimSuffix(path.Base(m), ".css"))
	}
	return list
}

// themes lists the built-in themes and the viewer's own.
func (s *Server) themes() []string {
	list := builtinThemes()
	if s.opts.ThemesDir == "" {
		return list
	}
	matches, _ := filepath.Glob(filepath.Join(s.opts.ThemesDir, "*.css"))
	sort.Strings(matches)
	for _, m := range matches {
		name := strings.TrimSuffix(filepath.Base(m), ".css")
		if !slices.Contains(list, name) {
			list = append(list, name)
		}
	}
	return list
}

// serveTheme serves a theme's CSS, read on every request so that editing a
// theme shows on the next reload. The viewer's own theme wins over a
// built-in one of the same name, and the default theme adds nothing.
func (s *Server) serveTheme(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	name := strings.TrimSuffix(path.Base(r.URL.Path), ".css")
	if name == builtinTheme || strings.ContainsAny(name, `/\`) {
		return
	}
	if s.opts.ThemesDir != "" {
		if b, err := os.ReadFile(filepath.Join(s.opts.ThemesDir, name+".css")); err == nil {
			w.Write(b)
			return
		}
	}
	b, err := fs.ReadFile(assets, "assets/themes/"+name+".css")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Write(b)
}
