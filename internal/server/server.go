// Package server serves a directory of files as a small, local GitHub: a
// file tree, directory pages that list their files and show their README,
// and files rendered as GitHub renders them.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/babarot/gh-mini/internal/workspace"
	"github.com/chrishrb/go-grip/defaults"
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
	opts Options
	ws   *workspace.Workspace
	md   goldmark.Markdown
	tmpl *template.Template
	hub  *hub
}

const builtinTheme = "github"

// maxRender is the largest file rendered; larger ones only get a raw link.
const maxRender = 2 << 20

// New opens the root and, when reloading is on, starts watching it.
func New(opts Options) (*Server, error) {
	ws, err := workspace.Open(workspace.Options{
		Root:    opts.Root,
		Name:    opts.Name,
		Skip:    opts.Skip,
		Watched: opts.Reload,
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
		opts: opts,
		ws:   ws,
		md:   newMarkdown(ws.Repo()),
		tmpl: tmpl,
		hub:  &hub{subs: map[chan change]struct{}{}},
	}
	if opts.Reload {
		if err := s.watch(); err != nil {
			ws.Close()
			return nil, err
		}
	}
	return s, nil
}

// Handler routes the server's own files under /_mini/ and everything else
// to the files under the root.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "assets")
	mux.Handle("/_mini/assets/", http.StripPrefix("/_mini/assets/", http.FileServer(http.FS(static))))
	grip, _ := fs.Sub(defaults.StaticFiles, "static")
	mux.Handle("/_mini/grip/", http.StripPrefix("/_mini/grip/", http.FileServer(http.FS(grip))))
	chroma := chromaCSS()
	mux.HandleFunc("/_mini/chroma.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		io.WriteString(w, chroma)
	})
	mux.HandleFunc("/_mini/theme/", s.serveTheme)
	mux.HandleFunc("/_mini/api/tree", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(s.ws.Snapshot().Tree)
	})
	mux.HandleFunc("/_mini/events", s.serveEvents)
	mux.HandleFunc("/", s.serveFile)
	return mux
}

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

type entry struct {
	Name    string
	Href    string
	Dir     bool
	Ignored bool
	Link    bool
	Ago     string
}

type langTab struct {
	Label   string
	Href    string
	Current bool
}

type page struct {
	Title    string
	Name     string
	Branch   string
	Path     string
	Crumbs   []crumb
	Kind     string // dir, markdown, code, image, binary, notfound
	Content  template.HTML
	Entries  []entry
	Readme   string
	Langs    []langTab
	Ignored  bool
	Plain    bool
	Size     string
	Lines    int
	Themes   []string
	Theme    string
	Mode     string
	Reload   bool
	Markdown bool
}

func (s *Server) newPage(r *http.Request, snap *workspace.Snapshot, rel string) *page {
	p := &page{
		Name:   s.opts.Name,
		Branch: snap.Branch,
		Path:   rel,
		Themes: s.themes(),
		Theme:  s.currentTheme(r),
		Mode:   cookie(r, "gh-mini-mode"),
		Reload: s.opts.Reload,
	}
	if p.Mode != "light" && p.Mode != "dark" {
		p.Mode = ""
	}
	p.Title = s.opts.Name
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

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if rel == "" {
		rel = "."
	}
	snap := s.ws.Snapshot()
	info, err := s.ws.FS().Stat(rel)
	if err != nil {
		p := s.newPage(r, snap, rel)
		p.Kind = "notfound"
		w.WriteHeader(http.StatusNotFound)
		s.render(w, p)
		return
	}
	if info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, dirHref(rel), http.StatusMovedPermanently)
			return
		}
		s.serveDir(w, r, snap, rel)
		return
	}
	if wantsRaw(r) {
		f, err := s.ws.FS().Open(rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		return
	}

	p := s.newPage(r, snap, rel)
	p.Size = humanSize(info.Size())
	p.Langs = s.langTabs(rel)
	switch {
	case isImage(rel):
		p.Kind = "image"
	case info.Size() > maxRender:
		p.Kind = "binary"
	default:
		b, err := s.ws.FS().ReadFile(rel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !isText(b) {
			p.Kind = "binary"
			break
		}
		p.Lines = strings.Count(string(b), "\n")
		p.Plain = r.URL.Query().Get("plain") == "1"
		if isMarkdown(rel) && !p.Plain {
			p.Kind = "markdown"
			p.Markdown = true
			p.Content, err = s.renderMarkdown(b)
		} else {
			p.Kind = "code"
			p.Content, err = renderCode(path.Base(rel), b)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.render(w, p)
}

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request, snap *workspace.Snapshot, rel string) {
	p := s.newPage(r, snap, rel)
	p.Kind = "dir"
	dirents, err := fs.ReadDir(s.ws.FS().FS(), rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var readmes []string
	for _, d := range dirents {
		if s.ws.Skipped(d.Name()) {
			continue
		}
		child := path.Join(rel, d.Name())
		e := entry{Name: d.Name(), Dir: d.IsDir(), Ignored: snap.Ignored(child)}
		if info, err := d.Info(); err == nil {
			e.Ago = ago(info.ModTime())
			if info.Mode()&fs.ModeSymlink != 0 {
				e.Link = true
				if st, err := s.ws.FS().Stat(child); err == nil {
					e.Dir = st.IsDir()
				}
			}
		}
		e.Href = href(child)
		if e.Dir {
			e.Href = dirHref(child)
		}
		p.Entries = append(p.Entries, e)
		if !e.Dir && readmeRe.MatchString(d.Name()) {
			readmes = append(readmes, d.Name())
		}
	}
	sort.SliceStable(p.Entries, func(i, j int) bool {
		a, b := p.Entries[i], p.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	if readme := pickReadme(readmes, r.URL.Query().Get("lang"), cookie(r, "gh-mini-lang")); readme != "" {
		b, err := s.ws.FS().ReadFile(path.Join(rel, readme))
		if err == nil {
			p.Readme = readme
			p.Markdown = true
			p.Content, _ = s.renderMarkdown(b)
			for _, name := range readmes {
				lang := langOf(name)
				p.Langs = append(p.Langs, langTab{
					Label:   langLabel(lang),
					Href:    "?lang=" + url.QueryEscape(orDefault(lang)),
					Current: name == readme,
				})
			}
			if len(p.Langs) < 2 {
				p.Langs = nil
			}
		}
	}
	s.render(w, p)
}

func (s *Server) render(w http.ResponseWriter, p *page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.Execute(w, p); err != nil {
		fmt.Fprintf(w, "<pre>%s</pre>", template.HTMLEscapeString(err.Error()))
	}
}

// wantsRaw tells a request for the file itself, such as an image in a page
// or ?raw, from a visit to the file's page.
func wantsRaw(r *http.Request) bool {
	if r.URL.Query().Has("raw") {
		return true
	}
	dest := r.Header.Get("Sec-Fetch-Dest")
	return dest != "" && dest != "document" && dest != "iframe"
}

var (
	readmeRe = regexp.MustCompile(`(?i)^readme(\.[a-z]{2}(-[a-z]{2})?)?\.(md|markdown)$`)
	// name.<lang>.md, such as README.ja.md or guide.zh-TW.md
	langRe = regexp.MustCompile(`(?i)^(.+?)(?:\.([a-z]{2}(?:-[a-z]{2})?))?\.(md|markdown)$`)
)

// langOf returns the language of a translated Markdown file, or "" for the
// original.
func langOf(name string) string {
	m := langRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[2])
}

func langLabel(lang string) string {
	if lang == "" {
		return "Default"
	}
	return strings.ToUpper(lang)
}

func orDefault(lang string) string {
	if lang == "" {
		return "default"
	}
	return lang
}

// pickReadme chooses README.md, or its translation for the language asked
// for in the URL or picked before.
func pickReadme(names []string, query, saved string) string {
	if len(names) == 0 {
		return ""
	}
	want := query
	if want == "" {
		want = saved
	}
	if want == "default" {
		want = ""
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) < len(names[j]) })
	for _, n := range names {
		if langOf(n) == strings.ToLower(want) {
			return n
		}
	}
	for _, n := range names {
		if langOf(n) == "" {
			return n
		}
	}
	return names[0]
}

// langTabs links a Markdown file to its translations next to it:
// guide.md, guide.ja.md and so on.
func (s *Server) langTabs(rel string) []langTab {
	m := langRe.FindStringSubmatch(path.Base(rel))
	if m == nil {
		return nil
	}
	base := strings.ToLower(m[1])
	dir := path.Dir(rel)
	dirents, err := fs.ReadDir(s.ws.FS().FS(), dir)
	if err != nil {
		return nil
	}
	var tabs []langTab
	for _, d := range dirents {
		dm := langRe.FindStringSubmatch(d.Name())
		if d.IsDir() || dm == nil || strings.ToLower(dm[1]) != base {
			continue
		}
		tabs = append(tabs, langTab{
			Label:   langLabel(strings.ToLower(dm[2])),
			Href:    href(path.Join(dir, d.Name())),
			Current: d.Name() == path.Base(rel),
		})
	}
	if len(tabs) < 2 {
		return nil
	}
	sort.SliceStable(tabs, func(i, j int) bool {
		return tabs[i].Label == "Default" || (tabs[j].Label != "Default" && tabs[i].Label < tabs[j].Label)
	})
	return tabs
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

func isMarkdown(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".md" || ext == ".markdown" || ext == ".mdx"
}

func isImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".avif", ".ico", ".bmp":
		return true
	}
	return false
}

func isText(b []byte) bool {
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	for _, c := range head {
		if c == 0 {
			return false
		}
	}
	return utf8.Valid(head) || utf8.Valid(head[:max(0, len(head)-3)])
}

func humanSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d Bytes", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}
