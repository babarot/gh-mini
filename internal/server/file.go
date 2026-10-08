package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/babarot/gh-mini/internal/workspace"
)

// maxRender is the largest file rendered; larger ones only get a raw link.
const maxRender = 2 << 20

// fileView is a file's page.
type fileView struct {
	Content template.HTML
	Langs   []langTab
	// Views switches between a rendered view and the code, for Markdown
	// and HTML files
	Views []viewTab
	// PreviewURL is where the iframe of an HTML preview loads the file
	PreviewURL string
	Size       string
	Lines      int
	// Notice tells something about how the file is shown, such as bytes
	// that are not UTF-8
	Notice string
}

type viewTab struct {
	Label   string
	Href    string
	Current bool
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
	case err != nil && !errors.Is(err, fs.ErrPermission):
		// Missing, or out of the root through a symlink
		w.WriteHeader(http.StatusNotFound)
		s.render(w, s.newPage(r, snap, rel, "notfound"))
	case err != nil:
		s.serveError(w, r, snap, rel, err)
	case info.IsDir() && !strings.HasSuffix(r.URL.Path, "/"):
		http.Redirect(w, r, withQuery(dirHref(rel), r), http.StatusMovedPermanently)
	case !info.IsDir() && strings.HasSuffix(r.URL.Path, "/"):
		// A file's page at a directory's URL would resolve its relative
		// links one level too deep
		http.Redirect(w, r, withQuery(href(rel), r), http.StatusMovedPermanently)
	case info.IsDir():
		s.follow(snap, rel, rel)
		s.serveDir(w, r, snap, rel)
	case wantsRaw(r):
		s.serveRaw(w, r, rel, info)
	default:
		s.follow(snap, rel, path.Dir(rel))
		s.serveFile(w, r, snap, rel, info)
	}
}

// follow makes sure the page of an ignored path reloads when it changes:
// directories git ignores are only watched while someone looks at them.
// The directory is watched rather than the file, which editors replace on
// saving.
func (s *Server) follow(snap *workspace.Snapshot, rel, dir string) {
	if s.watcher != nil && snap.Ignored(rel) {
		s.watcher.WatchDir(dir)
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
	if runsScripts(rel) {
		// Opened as a page, from here or from a link on any site, a file's
		// scripts would run as gh-mini and could read every file through
		// it. A sandbox gives the document an origin of its own and no
		// scripts; it does nothing to a file loaded as an image or a style
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// runsScripts tells the files a browser runs scripts in when it opens them
// as a page. Others, such as PDF, are left alone: a sandbox also stops the
// browser's PDF viewer.
func runsScripts(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm", ".xhtml", ".svg", ".xml":
		return true
	}
	return false
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, snap *workspace.Snapshot, rel string, info fs.FileInfo) {
	p := s.newPage(r, snap, rel, "")
	v := &fileView{Size: humanSize(info.Size()), Langs: s.langTabs(rel)}
	p.File = v
	plain := r.URL.Query().Get("plain") == "1"
	if isHTML(rel) && s.opts.PreviewPort != 0 {
		preview := s.htmlPreview(r, p.Settings)
		v.Views = []viewTab{{"Preview", "?preview=1", preview}, {"Code", "?plain=1", !preview}}
		if preview {
			p.Kind = "html"
			v.PreviewURL = s.previewURL(r, rel)
			http.SetCookie(w, &http.Cookie{
				Name:     s.previewCookie(),
				Value:    s.previewToken,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			s.render(w, p)
			return
		}
	}
	if isMarkdown(rel) {
		v.Views = []viewTab{{"Preview", "?", !plain}, {"Code", "?plain=1", plain}}
	}
	switch {
	case isImage(rel):
		p.Kind = "image"
	case info.Size() > maxRender:
		p.Kind = "binary"
	default:
		b, err := s.ws.FS().ReadFile(rel)
		if err != nil {
			s.serveError(w, r, snap, rel, err)
			return
		}
		text, notice, ok := decodeText(b)
		if !ok {
			p.Kind = "binary"
			break
		}
		b, v.Notice = text, notice
		v.Lines = countLines(b)
		if isMarkdown(rel) && !plain {
			p.Kind = "markdown"
			v.Content, p.Features, err = s.renderMarkdownFile(rel, info, b)
		} else {
			p.Kind = "code"
			v.Content, err = renderCode(path.Base(rel), b)
		}
		if err != nil {
			s.serveError(w, r, snap, rel, err)
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

func isHTML(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".html" || ext == ".htm"
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

// decodeText returns a file's text as UTF-8 to show, without a byte order
// mark, raw keeping the file as it is. UTF-16 with its byte order mark is
// decoded. Other text that is not UTF-8, such as Shift_JIS, is shown with
// what could not be read as U+FFFD, and a notice saying so; a NUL in the
// first 8000 bytes makes it binary.
func decodeText(b []byte) (text []byte, notice string, ok bool) {
	switch {
	case bytes.HasPrefix(b, []byte{0xff, 0xfe}):
		return decodeUTF16(b[2:], binary.LittleEndian), "", true
	case bytes.HasPrefix(b, []byte{0xfe, 0xff}):
		return decodeUTF16(b[2:], binary.BigEndian), "", true
	}
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, "", false
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return b, "", true
	}
	return bytes.ToValidUTF8(b, []byte("\uFFFD")), "This file is not UTF-8: what could not be read shows as \uFFFD.", true
}

func decodeUTF16(b []byte, order binary.ByteOrder) []byte {
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = order.Uint16(b[2*i:])
	}
	return []byte(string(utf16.Decode(units)))
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

// countLines counts lines as GitHub does: a last line without a newline
// counts too.
func countLines(b []byte) int {
	n := bytes.Count(b, []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// withQuery is a URL with the request's query, so that a redirect keeps
// ?plain=1 and the like.
func withQuery(u string, r *http.Request) string {
	if r.URL.RawQuery == "" {
		return u
	}
	return u + "?" + r.URL.RawQuery
}
