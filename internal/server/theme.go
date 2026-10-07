package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const builtinTheme = "github"

// themes lists the built-in theme and the viewer's own.
func (s *Server) themes() []string {
	list := []string{builtinTheme}
	if s.opts.ThemesDir == "" {
		return list
	}
	matches, _ := filepath.Glob(filepath.Join(s.opts.ThemesDir, "*.css"))
	sort.Strings(matches)
	for _, m := range matches {
		name := strings.TrimSuffix(filepath.Base(m), ".css")
		if name != builtinTheme {
			list = append(list, name)
		}
	}
	return list
}

// serveTheme serves a theme's CSS, read on every request so that editing a
// theme shows on the next reload. The built-in theme adds nothing.
func (s *Server) serveTheme(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	name := strings.TrimSuffix(path.Base(r.URL.Path), ".css")
	if name == builtinTheme || s.opts.ThemesDir == "" || strings.ContainsAny(name, `/\`) {
		return
	}
	b, err := os.ReadFile(filepath.Join(s.opts.ThemesDir, name+".css"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Write(b)
}

func (s *Server) currentTheme(r *http.Request) string {
	if c, err := r.Cookie("gh-mini-theme"); err == nil {
		for _, t := range s.themes() {
			if t == c.Value {
				return t
			}
		}
	}
	if s.opts.Theme != "" {
		return s.opts.Theme
	}
	return builtinTheme
}
