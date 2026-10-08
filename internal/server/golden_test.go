package server

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/babarot/gh-mini/internal/golden"
)

// fullDate is a date as dateLayout writes it.
var fullDate = regexp.MustCompile(`[A-Z][a-z]{2} \d{1,2}, \d{4}, \d{2}:\d{2} [A-Za-z0-9+-]+`)

// TestGoldenPages compares whole pages, one of each kind, with
// testdata/pages. The static files' version, a hash of them, is replaced
// so that a change to a script or a stylesheet does not change every page,
// and so are the commit's hash and the dates, of the commit and of the
// files listed, which change with the time they were made, and the root
// the About dialog shows, a new directory each run.
func TestGoldenPages(t *testing.T) {
	root, themes := newTestRepo(t)
	srv, err := New(Options{
		Root:      root,
		Name:      "repo",
		Skip:      []string{".git", "node_modules", ".DS_Store"},
		ThemesDir: themes,
		Version:   "1.2.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	h := srv.Handler()
	head := srv.lastCommit(".")

	for name, target := range map[string]string{
		"root":          "/",
		"dir":           "/docs/",
		"ignored-dir":   "/local-only/",
		"markdown":      "/docs/guide.md",
		"markdown-code": "/docs/guide.md?plain=1",
		"translation":   "/docs/guide.ja.md",
		"mermaid":       "/docs/diagram.md",
		"math":          "/docs/math.md",
		"code":          "/main.go",
		"image":         "/img.png",
		"binary":        "/bin.dat",
		"notfound":      "/nope",
	} {
		t.Run(name, func(t *testing.T) {
			r := get(t, h, target)
			if r.code != http.StatusOK && r.code != http.StatusNotFound {
				t.Fatalf("status %d", r.code)
			}
			body := strings.ReplaceAll(r.body, srv.static.prefix(), "/_mini/static/VERSION")
			body = strings.ReplaceAll(body, head.Short, "SHA")
			body = fullDate.ReplaceAllString(body, "DATE")
			body = strings.ReplaceAll(body, srv.serving, "ROOT")
			golden.Check(t, filepath.Join("testdata", "pages", name+".html"), []byte(body))
		})
	}
}
