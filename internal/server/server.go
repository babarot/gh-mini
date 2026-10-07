// Package server serves a directory of files as a small, local GitHub: a
// file tree, directory pages that list their files and show their README,
// and files rendered as GitHub renders them.
package server

import (
	"embed"
	"html/template"
	"net/http"
	"path"
	"time"

	"github.com/babarot/gh-mini/internal/markdown"
	"github.com/babarot/gh-mini/internal/workspace"
)

//go:embed assets
var assets embed.FS

// Options configures a Server.
type Options struct {
	// Root is the absolute path of the directory to serve.
	Root string
	// Name is shown as the repository name.
	Name string
	// Skip lists file and directory names left out of the tree.
	Skip []string
	// Theme is the theme used until the viewer picks another.
	Theme string
	// ThemesDir holds the viewer's own themes, one CSS file each.
	ThemesDir string
	// Reload makes pages reload when the files they show change.
	Reload bool
	// PreviewPort is the port PreviewHandler is served on, on the same
	// host; zero turns HTML previews off.
	PreviewPort int
}

// Server serves one directory.
type Server struct {
	opts    Options
	ws      *workspace.Workspace
	watcher *workspace.Watcher
	md      *markdown.Renderer
	tmpl    *template.Template
	hub     *hub
	static  *staticFiles
	renders *renderCache
	// previewToken is the value of the preview cookie
	previewToken string
}

// New opens the root and, when reloading is on, starts watching it.
func New(opts Options) (*Server, error) {
	wsOpts := workspace.Options{Root: opts.Root, Name: opts.Name, Skip: opts.Skip}
	if !opts.Reload {
		// Nothing tells when files change, so look again now and then
		wsOpts.MaxAge = 2 * time.Second
	}
	ws, err := workspace.Open(wsOpts)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("page.html").Funcs(template.FuncMap{
		"href":       href,
		"join":       path.Join,
		"inc":        func(i int) int { return i + 1 },
		"isMarkdown": isMarkdown,
	}).ParseFS(assets, "assets/page.html")
	if err != nil {
		ws.Close()
		return nil, err
	}
	s := &Server{
		opts:    opts,
		ws:      ws,
		md:      markdown.New(ws.Repo()),
		tmpl:    tmpl,
		hub:     &hub{subs: map[*subscriber]struct{}{}},
		static:  newStaticFiles(),
		renders: newRenderCache(maxRenderCache),
	}
	if opts.PreviewPort != 0 {
		s.previewToken = newPreviewToken()
	}
	if opts.Reload {
		s.watcher, err = workspace.Watch(ws, opts.ThemesDir, s.notify)
		if err != nil {
			ws.Close()
			return nil, err
		}
	}
	return s, nil
}

// Close stops watching and closes the root.
func (s *Server) Close() error {
	if s.watcher != nil {
		s.watcher.Close()
	}
	return s.ws.Close()
}

// Handler routes the server's own files under /_mini/ and everything else
// to the files under the root.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/_mini/static/", s.static)
	mux.HandleFunc("/_mini/theme/", s.serveTheme)
	mux.HandleFunc("/_mini/api/tree", s.serveTree)
	mux.HandleFunc("/_mini/events", s.serveEvents)
	mux.HandleFunc("/", s.servePath)
	return mux
}

// serveTree serves the tree as JSON. The browser asks again on every page,
// and gets 304 while the tree is the same.
func (s *Server) serveTree(w http.ResponseWriter, r *http.Request) {
	snap := s.ws.Snapshot()
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", snap.TreeETag)
	if r.Header.Get("If-None-Match") == snap.TreeETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(snap.TreeJSON)
}
