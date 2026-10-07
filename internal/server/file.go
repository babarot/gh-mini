package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/babarot/gh-mini/internal/workspace"
)

// maxRender is the largest file rendered; larger ones only get a raw link.
const maxRender = 2 << 20

// fileView is a file's page.
type fileView struct {
	Content template.HTML
	Langs   []langTab
	// Plain shows a Markdown file as code
	Plain bool
	Size  string
	Lines int
}

// servePath serves the page of a file or directory under the root, or the
// file itself when it is asked for raw.
func (s *Server) servePath(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if rel == "" {
		rel = "."
	}
	snap := s.ws.Snapshot()
	info, err := s.ws.FS().Stat(rel)
	switch {
	case err != nil:
		w.WriteHeader(http.StatusNotFound)
		s.render(w, s.newPage(r, snap, rel, "notfound"))
	case info.IsDir() && !strings.HasSuffix(r.URL.Path, "/"):
		http.Redirect(w, r, dirHref(rel), http.StatusMovedPermanently)
	case info.IsDir():
		s.serveDir(w, r, snap, rel)
	case wantsRaw(r):
		s.serveRaw(w, r, rel, info)
	default:
		s.serveFile(w, r, snap, rel, info)
	}
}

func (s *Server) serveRaw(w http.ResponseWriter, r *http.Request, rel string, info fs.FileInfo) {
	f, err := s.ws.FS().Open(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, snap *workspace.Snapshot, rel string, info fs.FileInfo) {
	p := s.newPage(r, snap, rel, "")
	v := &fileView{Size: humanSize(info.Size()), Langs: s.langTabs(rel)}
	p.File = v
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
		v.Lines = strings.Count(string(b), "\n")
		v.Plain = r.URL.Query().Get("plain") == "1"
		if isMarkdown(rel) && !v.Plain {
			p.Kind = "markdown"
			v.Content, p.Features, err = s.renderMarkdown(b)
		} else {
			p.Kind = "code"
			v.Content, err = renderCode(path.Base(rel), b)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.render(w, p)
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
