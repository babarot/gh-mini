package server

import (
	"net/http"
	"strings"
)

// contentSecurityPolicy is the CSP of gh-mini's pages, a second guard
// behind the sanitizing of HTML in Markdown: scripts run only from
// gh-mini's own files, so nothing a file manages to put in a page runs.
//
//   - style-src allows inline styles: Mermaid and MathJax add <style>
//     while drawing, and styles run no code
//   - img-src allows any image, as Markdown shows badges and the like
//   - worker-src allows blob:, as MathJax makes its speech worker so; only
//     gh-mini's scripts can make one
//   - frame-src allows the preview server, for HTML previews, and
//     gh-mini itself, for the sandboxes plugins run in
func (s *Server) contentSecurityPolicy(r *http.Request) string {
	frame := "'self'"
	if s.opts.PreviewPort != 0 {
		frame += " " + s.previewOrigin(r)
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src * data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"worker-src 'self' blob:",
		"frame-src " + frame,
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'none'",
	}, "; ")
}
