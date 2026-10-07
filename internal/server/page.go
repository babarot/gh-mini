package server

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/babarot/gh-mini/internal/workspace"
)

func cookie(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

type crumb struct {
	Name string
	Href string
}

// page is what page.html renders: the parts every page has, and the view
// of its kind.
type page struct {
	layout
	// Kind is dir, markdown, code, image, binary or notfound.
	Kind string
	// Dir is set for a directory, File for the other kinds but notfound.
	Dir  *dirView
	File *fileView
	// Features loads the scripts the page's Markdown needs.
	Features features
}

// layout is what every page shows around its content.
type layout struct {
	Title   string
	Name    string
	Branch  string
	Path    string
	Crumbs  []crumb
	Ignored bool
	Themes  []string
	// Settings holds the value of every setting, by key.
	Settings map[string]string
	// SettingAttrs are the settings set on <html> for CSS.
	SettingAttrs template.HTMLAttr
	Reload       bool
	// Static is the URL of the server's own files.
	Static string
}

func (s *Server) newPage(r *http.Request, snap *workspace.Snapshot, rel, kind string) *page {
	if s.watcher != nil {
		s.watcher.WatchThemes()
	}
	p := &page{Kind: kind, layout: layout{
		Name:     s.opts.Name,
		Branch:   snap.Branch,
		Path:     rel,
		Themes:   s.themes(),
		Settings: s.settings(r),
		Reload:   s.opts.Reload,
		Static:   s.static.prefix(),
	}}
	p.Title = s.opts.Name
	p.SettingAttrs = settingAttrs(p.Settings)
	if rel != "." {
		p.Title = rel + " · " + s.opts.Name
		parts := strings.Split(rel, "/")
		for i, name := range parts {
			p.Crumbs = append(p.Crumbs, crumb{Name: name, Href: dirHref(strings.Join(parts[:i+1], "/"))})
		}
		p.Ignored = snap.Ignored(rel)
	}
	return p
}

func (s *Server) render(w http.ResponseWriter, p *page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.Execute(w, p); err != nil {
		fmt.Fprintf(w, "<pre>%s</pre>", template.HTMLEscapeString(err.Error()))
	}
}

// dirHref is the URL of a directory under the root.
func dirHref(rel string) string {
	if rel == "." || rel == "" {
		return "/"
	}
	return href(rel) + "/"
}

// href is the URL of a path under the root when it is a file.
func href(rel string) string {
	if rel == "." || rel == "" {
		return "/"
	}
	return (&url.URL{Path: "/" + rel}).EscapedPath()
}
