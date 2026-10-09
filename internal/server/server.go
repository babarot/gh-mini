// Package server serves a directory of files as a small, local GitHub: a
// file tree, directory pages that list their files and show their README,
// and files rendered as GitHub renders them.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"path"
	"slices"
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
	// PluginsDir holds the viewer's own plugins, one directory each.
	PluginsDir string
	// Reload makes pages reload when the files they show change.
	Reload bool
	// NoChanges keeps gh-mini from reading what changed since the last
	// commit, and from showing it.
	NoChanges bool
	// PreviewPort is the port PreviewHandler is served on, on the same
	// host; zero turns HTML previews off.
	PreviewPort int
	// Translations are the layouts translations are named by, as
	// --translations takes them; empty for the default.
	Translations string
	// Version and Revision are shown in the About dialog; Revision is the
	// full hash of the commit built from, or empty.
	Version  string
	Revision string
	// Hosts are the names, other than localhost, the server answers to;
	// it answers to any IP address
	Hosts []string
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
	// translations are the layouts of Options.Translations, none when
	// they are off
	translations translationLayouts
	// serving is the root as the About dialog shows it
	serving string
	// boot tells this process from the one before it: a page from an
	// earlier run reloads when the event stream gives another one
	boot string
	// gh is the path of gh, which tells the branch's pull request, or ""
	// when it is not found
	gh  string
	prs prCache
}

// New opens the root and, when reloading is on, starts watching it.
func New(opts Options) (*Server, error) {
	opts.Translations = normalizeTranslations(opts.Translations)
	translations, err := parseTranslations(opts.Translations)
	if err != nil {
		return nil, fmt.Errorf("translations %w", err)
	}
	wsOpts := workspace.Options{Root: opts.Root, Name: opts.Name, Skip: opts.Skip, NoStatus: opts.NoChanges}
	if !opts.Reload {
		// Nothing tells when files change, so look again now and then
		wsOpts.MaxAge = 2 * time.Second
	}
	ws, err := workspace.Open(wsOpts)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("page.html").Funcs(template.FuncMap{
		"href": href,
		"join": path.Join,
		"inc":  func(i int) int { return i + 1 },
	}).ParseFS(assets, "assets/page.html")
	if err != nil {
		ws.Close()
		return nil, err
	}
	s := &Server{
		opts:    opts,
		ws:      ws,
		md:      markdown.New(),
		tmpl:    tmpl,
		hub:     newHub(),
		static:  newStaticFiles(),
		renders: newRenderCache(maxRenderCache),
		serving: shortenHome(opts.Root, homeDir()),

		translations: translations,
		boot:         newPreviewToken(),
	}
	if opts.PreviewPort != 0 {
		s.previewToken = newPreviewToken()
	}
	s.gh, _ = exec.LookPath("gh")
	// A default theme that does not exist would leave pages unstyled
	// with nothing to say why
	if opts.Theme != "" && !slices.Contains(s.themes(), opts.Theme) {
		log.Printf("gh-mini: no theme %q in %s; using %s", opts.Theme, opts.ThemesDir, builtinTheme)
		s.opts.Theme = ""
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
	mux.HandleFunc("/_mini/plugin-host/", s.servePluginHost)
	mux.HandleFunc("/_mini/plugins/", s.servePluginCode)
	mux.HandleFunc("/_mini/api/sanitize", s.serveSanitize)
	mux.HandleFunc("/_mini/api/tree", s.serveTree)
	mux.HandleFunc("/_mini/api/status", s.serveStatus)
	mux.HandleFunc("/_mini/api/branch", s.serveBranch)
	mux.HandleFunc("/_mini/api/pr", s.servePR)
	mux.HandleFunc("/_mini/changes", s.serveChangesPage)
	mux.HandleFunc("/_mini/events", s.serveEvents)
	mux.HandleFunc("/_mini/api/changes", s.serveChanges)
	mux.HandleFunc("/", s.servePath)
	return s.checkHost(mux)
}

// serveTree serves the tree as JSON. The browser asks again on every page,
// and gets 304 while the tree is the same.
func (s *Server) serveTree(w http.ResponseWriter, r *http.Request) {
	// ?path= gives the entries of one directory, for those the tree
	// leaves out
	if r.URL.Query().Has("path") {
		n, err := s.ws.Subtree(r.URL.Query().Get("path"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(n)
		return
	}
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

// serveBranch serves where the branch stands against its base as JSON,
// null on the base itself or without one. It is read again whenever HEAD
// or a branch of origin moves.
func (s *Server) serveBranch(w http.ResponseWriter, r *http.Request) {
	b := s.ws.Branch()
	if b != nil && b.OnBase {
		b = nil
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(b)
}

// serveStatus serves what changed since the last commit as JSON, with an
// ETag as the tree has.
func (s *Server) serveStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.ws.Snapshot()
	_, b, etag := s.statusFor(snap, s.settings(r))
	if b == nil {
		// Nothing to show: outside a repository, or with Changes off
		b, etag = []byte(`{"git":false,"files":{},"added":0,"deleted":0}`), `"none"`
	}
	// The settings, in a cookie, tell what is left out
	w.Header().Set("Vary", "Cookie")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
