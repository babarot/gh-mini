package server

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/babarot/gh-mini/internal/markdown"
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
	// Kind is dir, markdown, code, image, binary, changes, notfound or
	// error.
	Kind string
	// Error tells why a page of kind error could not be shown.
	Error string
	// Dir is set for a directory, File for the other kinds but notfound.
	Dir  *dirView
	File *fileView
	// ChangeList is set on the Changes page
	ChangeList *changeList
	// Features loads the scripts the page's Markdown needs.
	Features markdown.Features
	// csp is the page's Content-Security-Policy
	csp string
	// status is what changed since the last commit as the viewer's
	// settings show it, nil when nothing is shown
	status *workspace.Status
}

// layout is what every page shows around its content.
type layout struct {
	Title string
	// Name is the directory's name, which keeps the viewer's state apart
	// from other directories'. Owner and Repo are what the page shows:
	// the GitHub repository of the origin remote, or no owner and the
	// directory's name.
	Name   string
	Owner  string
	Repo   string
	Branch string
	// Changes counts what changed since the last commit, in a git
	// repository only
	Changes *changesView
	Path    string
	Crumbs  []crumb
	Ignored bool
	// Settings holds the value of every setting, by key, and
	// SettingSections what the settings dialog shows, page by page.
	Settings        map[string]string
	SettingSections []settingSection
	// SettingAttrs are the settings set on <html> for CSS.
	SettingAttrs template.HTMLAttr
	Reload       bool
	// Seq is the latest change the page shows, for the page to ask for
	// the changes it missed, and Boot the server's boot ID, for it to
	// tell a restart
	Seq  uint64
	Boot string
	// Static is the URL of the server's own files.
	Static string
	// SidebarHidden closes the file tree from the first paint, as the
	// viewer left it.
	SidebarHidden bool
	About         aboutView
}

func (s *Server) newPage(r *http.Request, snap *workspace.Snapshot, rel, kind string) *page {
	if s.watcher != nil {
		s.watcher.WatchThemes()
	}
	p := &page{Kind: kind, csp: s.contentSecurityPolicy(r), layout: layout{
		Name:     s.opts.Name,
		Branch:   snap.Branch,
		Path:     rel,
		Settings: s.settings(r),
		Reload:   s.opts.Reload,
		Seq:      seqOf(r),
		Boot:     s.boot,
		Static:   s.static.prefix(),
		About:    s.about(),
	}}
	p.Owner, p.Repo, _ = strings.Cut(s.ws.Repo(), "/")
	if p.Repo == "" {
		p.Owner, p.Repo = "", s.opts.Name
	}
	full := p.Repo
	if p.Owner != "" {
		full = p.Owner + "/" + p.Repo
	}
	p.Title = full
	var etag string
	p.status, _, etag = s.statusFor(snap, p.Settings)
	p.Changes = newChangesView(p.status, etag)
	p.SettingAttrs = settingAttrs(p.Settings)
	p.SidebarHidden = cookie(r, sidebarCookie) == "hidden"
	p.SettingSections = s.settingSections(p.Settings)
	if rel != "." {
		p.Title = rel + " · " + full
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
	w.Header().Set("Content-Security-Policy", p.csp)
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

// sidebarCookie keeps the file tree closed from page to page. A cookie
// rather than localStorage, so that the page is rendered closed instead
// of closing after it shows.
const sidebarCookie = "gh-mini-sidebar"

// serveError shows why a path could not be read, in the page's layout:
// not allowed, not found, or anything else.
func (s *Server) serveError(w http.ResponseWriter, r *http.Request, snap *workspace.Snapshot, rel string, err error) {
	switch {
	case errors.Is(err, fs.ErrPermission):
		p := s.newPage(r, snap, rel, "error")
		p.Error = "You do not have permission to read this."
		w.WriteHeader(http.StatusForbidden)
		s.render(w, p)
	case errors.Is(err, fs.ErrNotExist):
		w.WriteHeader(http.StatusNotFound)
		s.render(w, s.newPage(r, snap, rel, "notfound"))
	default:
		p := s.newPage(r, snap, rel, "error")
		p.Error = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		s.render(w, p)
	}
}
