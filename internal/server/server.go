// Package server serves a directory of files as a small, local GitHub: a
// file tree, directory pages that list their files and show their README,
// and files rendered as GitHub renders them.
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"path"

	"github.com/babarot/gh-mini/internal/workspace"
	"github.com/yuin/goldmark"
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
}

// Server serves one directory.
type Server struct {
	opts    Options
	ws      *workspace.Workspace
	watcher *workspace.Watcher
	md      goldmark.Markdown
	tmpl    *template.Template
	hub     *hub
	static  *staticFiles
}

// New opens the root and, when reloading is on, starts watching it.
func New(opts Options) (*Server, error) {
	ws, err := workspace.Open(workspace.Options{
		Root: opts.Root,
		Name: opts.Name,
		Skip: opts.Skip,
	})
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
		opts:   opts,
		ws:     ws,
		md:     newMarkdown(ws.Repo()),
		tmpl:   tmpl,
		hub:    &hub{subs: map[chan change]struct{}{}},
		static: newStaticFiles(),
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
	mux.HandleFunc("/_mini/api/tree", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.ws.Snapshot().Tree)
	})
	mux.HandleFunc("/_mini/events", s.serveEvents)
	mux.HandleFunc("/", s.servePath)
	return mux
}
