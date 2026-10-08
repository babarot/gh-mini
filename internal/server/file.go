package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/babarot/gh-mini/internal/workspace"
)

// maxRender is the largest file rendered; larger ones only get a raw link.
const maxRender = 2 << 20

// maxHighlight is the largest source file highlighted; larger ones are
// shown as plain text with their line numbers. Highlighting takes about a
// second for each megabyte and puts around 28 elements on each line,
// which the browser takes longer still to lay out.
const maxHighlight = 512 << 10

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
	Commit *commitView
}

// commitView is the last commit that changed a file.
type commitView struct {
	Author string
	// Avatar is the author's picture on GitHub, found by email
	Avatar  string
	Subject string
	Short   string
	Ago     string
	// Date is the time in full, shown on hover
	Date string
	// URL is the commit on GitHub, when it was pushed there
	URL string
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
	case !info.Mode().IsRegular() && wantsRaw(r):
		// A named pipe or a device would keep the request waiting on it
		http.NotFound(w, r)
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
	ctype := rawType(info.Name(), f)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", fileETag(info))
	if !passive(ctype) {
		// Opened as a page, from here or from a link on any site, a file's
		// scripts would run as gh-mini and could read every file through
		// it. A sandbox gives the document an origin of its own and no
		// scripts; it does nothing to a file loaded as an image or a style
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// fileETag tells one version of a file from another by its modification
// time to the nanosecond and its size. Last-Modified alone, to the second,
// would keep a file written twice in a second, as a build or a formatter
// does, at its first version.
func fileETag(info fs.FileInfo) string {
	return fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size())
}

// rawType is the Content-Type a file is served raw with: the one of its
// extension, else text, as GitHub serves files raw. A file with no type
// of its own is not sniffed into HTML, which would run its scripts.
func rawType(name string, f io.ReadSeeker) string {
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	var b [512]byte
	n, _ := io.ReadFull(f, b[:])
	_, _ = f.Seek(0, io.SeekStart)
	t := http.DetectContentType(b[:n])
	if strings.HasPrefix(t, "text/") || strings.Contains(t, "xml") {
		return "text/plain; charset=utf-8"
	}
	return t
}

// passive tells the types a browser runs no scripts in when it opens them
// as a page, and which are left out of the sandbox: it would also stop the
// browser's own viewers, such as that of PDF. Every other type, text
// included, is sandboxed, as types the browser runs scripts in are many:
// HTML, SVG, XHTML, XML with a stylesheet and others by the system's MIME
// types.
func passive(ctype string) bool {
	t, _, _ := strings.Cut(ctype, ";")
	t = strings.TrimSpace(strings.ToLower(t))
	switch {
	case t == "application/pdf":
		return true
	case strings.HasPrefix(t, "image/"):
		return t != "image/svg+xml"
	case strings.HasPrefix(t, "video/"), strings.HasPrefix(t, "audio/"), strings.HasPrefix(t, "font/"):
		return true
	}
	return false
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, snap *workspace.Snapshot, rel string, info fs.FileInfo) {
	p := s.newPage(r, snap, rel, "")
	v := &fileView{Size: humanSize(info.Size()), Langs: s.langTabs(s.translationsFor(p.Settings), rel)}
	p.File = v
	if !p.Ignored {
		v.Commit = s.lastCommit(rel)
		if v.Commit != nil && p.Settings["avatars"] == "false" {
			v.Commit.Avatar = ""
		}
	}
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
	case !info.Mode().IsRegular():
		p.Kind = "binary"
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
			highlight := len(b) <= maxHighlight
			if !highlight && v.Notice == "" {
				v.Notice = "This file is too large to highlight, so it is shown as plain text."
			}
			v.Content, err = s.renderCodeFile(rel, info, b, highlight)
		}
		if err != nil {
			s.serveError(w, r, snap, rel, err)
			return
		}
	}
	s.render(w, p)
}

func (s *Server) lastCommit(rel string) *commitView {
	c := s.ws.LastCommit(rel)
	if c == nil {
		return nil
	}
	v := &commitView{
		Author:  c.Author,
		Subject: c.Subject,
		Short:   c.SHA[:min(7, len(c.SHA))],
		Ago:     ago(c.Time),
		Date:    c.Time.Format("Jan 2, 2006, 15:04 MST"),
	}
	if repo := s.ws.Repo(); repo != "" {
		// GitHub answers an email it does not know with a generated
		// picture, never an error
		v.Avatar = "https://avatars.githubusercontent.com/u/e?s=40&email=" + url.QueryEscape(c.Email)
		if c.Pushed {
			v.URL = "https://github.com/" + repo + "/commit/" + c.SHA
		}
	}
	return v
}

// errNotRegular is a file that is not a regular one, such as a named pipe,
// which reading would wait on until something writes to it.
var errNotRegular = errors.New("not a regular file")

// readRegular reads a regular file, of which info is the Stat, or nil to
// Stat it here.
func readRegular(root *os.Root, name string, info fs.FileInfo) ([]byte, error) {
	if info == nil {
		var err error
		if info, err = root.Stat(name); err != nil {
			return nil, err
		}
	}
	if !info.Mode().IsRegular() {
		return nil, errNotRegular
	}
	return root.ReadFile(name)
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
