package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// HTML files are previewed in an iframe from a second server on its own
// port: another origin, so that a file's scripts run in full, modules and
// fetch included, yet cannot reach gh-mini's own pages and API.
//
// That server answers only requests carrying previewCookie, which the
// main server sets, HttpOnly and SameSite=Strict, on a page showing a
// preview. Cookies and SameSite ignore ports, so the iframe on this host
// sends it, and the file's own loads too, while a page on another site
// framing or linking the preview port does not. Processes on this machine
// are no concern: the main port already serves them every file.

func newPreviewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// previewCookie is named after the port, so that instances on one host
// keep their own.
func (s *Server) previewCookie() string {
	return "gh-mini-preview-" + strconv.Itoa(s.opts.PreviewPort)
}

// PreviewHandler serves the files under the root as they are, to browsers
// holding the preview cookie, for the server on Options.PreviewPort.
func (s *Server) PreviewHandler() http.Handler {
	return s.checkHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, err := r.Cookie(s.previewCookie())
		if err != nil || s.previewToken == "" ||
			subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.previewToken)) != 1 {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" {
			rel = "."
		}
		info, err := s.ws.FS().Stat(rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if info.IsDir() {
			// Relative URLs in its index.html resolve against the slash
			if !strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, dirHref(rel), http.StatusMovedPermanently)
				return
			}
			rel = path.Join(rel, "index.html")
			if info, err = s.ws.FS().Stat(rel); err != nil || info.IsDir() {
				http.NotFound(w, r)
				return
			}
		}
		if !info.Mode().IsRegular() {
			// A named pipe or a device would keep the request waiting
			http.NotFound(w, r)
			return
		}
		f, err := s.ws.FS().Open(rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	}))
}

// htmlPreview tells whether an HTML file's page shows its preview: when the
// URL asks for it, else when the setting says so. Only for a visit from a
// gh-mini page or typed in: a page on another site linking here must not
// get this page to set the preview cookie and run the file.
func (s *Server) htmlPreview(r *http.Request, settings map[string]string) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
	default:
		return false
	}
	q := r.URL.Query()
	switch {
	case q.Get("plain") == "1":
		return false
	case q.Get("preview") == "1":
		return true
	}
	return settings["htmlPreview"] == "true"
}

// previewURL is the file on the preview server, on the host the browser
// reached this server by, so that the preview cookie set here goes there.
func (s *Server) previewURL(r *http.Request, rel string) string {
	return s.previewOrigin(r) + href(rel)
}

func (s *Server) previewOrigin(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.opts.PreviewPort))
}
