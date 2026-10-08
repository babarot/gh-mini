package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-mini/internal/golden"
)

// TestGoldenPages compares whole pages, one of each kind, with
// testdata/pages. The static files' version, a hash of them, is replaced
// so that a change to a script or a stylesheet does not change every page,
// and so are the commit's hash and date, which change with the time it was
// made.
func TestGoldenPages(t *testing.T) {
	root, themes := newTestRepo(t)
	srv, err := New(Options{
		Root:      root,
		Name:      "repo",
		Skip:      []string{".git", "node_modules", ".DS_Store"},
		ThemesDir: themes,
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
			body = strings.ReplaceAll(body, head.Date, "DATE")
			golden.Check(t, filepath.Join("testdata", "pages", name+".html"), []byte(body))
		})
	}
}
