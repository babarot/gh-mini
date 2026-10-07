package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

const testPreviewPort = 7000

func newPreviewServer(t *testing.T) (*Server, string) {
	t.Helper()
	root, themes := newTestRepo(t)
	for name, body := range map[string]string{
		"site/index.html":   `<link rel="stylesheet" href="style.css">`,
		"site/style.css":    "body { color: red }",
		"site/app.js":       "console.log(1)",
		"site/nested/a.txt": "a",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)), []byte(body))
	}
	srv, err := New(Options{Root: root, Name: "repo", ThemesDir: themes, PreviewPort: testPreviewPort})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, root
}

func previewGet(t *testing.T, srv *Server, method, target string, edit ...func(*http.Request)) response {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for _, f := range edit {
		f(req)
	}
	rec := httptest.NewRecorder()
	srv.PreviewHandler().ServeHTTP(rec, req)
	return response{code: rec.Code, header: rec.Header(), body: rec.Body.String()}
}

func TestPreviewServer(t *testing.T) {
	srv, _ := newPreviewServer(t)
	token := withCookie("gh-mini-preview-7000", srv.previewToken)

	for target, ctype := range map[string]string{
		"/site/index.html": "text/html; charset=utf-8",
		"/site/app.js":     "text/javascript; charset=utf-8",
		"/site/style.css":  "text/css; charset=utf-8",
		"/site/":           "text/html; charset=utf-8",
	} {
		r := previewGet(t, srv, http.MethodGet, target, token)
		if r.code != http.StatusOK || r.header.Get("Content-Type") != ctype {
			t.Errorf("%s: status %d, Content-Type %q", target, r.code, r.header.Get("Content-Type"))
		}
	}
	if r := previewGet(t, srv, http.MethodGet, "/site", token); r.code != http.StatusMovedPermanently || r.header.Get("Location") != "/site/" {
		t.Errorf("/site: status %d to %q", r.code, r.header.Get("Location"))
	}
	for _, target := range []string{"/site/nested/", "/nope.html", "/link/secret.txt", "/_mini/api/tree"} {
		if r := previewGet(t, srv, http.MethodGet, target, token); r.code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", target, r.code)
		}
	}
	if r := previewGet(t, srv, http.MethodPost, "/site/index.html", token); r.code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d", r.code)
	}
	for name, edit := range map[string][]func(*http.Request){
		"no cookie":    nil,
		"wrong value":  {withCookie("gh-mini-preview-7000", "x")},
		"another port": {withCookie("gh-mini-preview-7001", srv.previewToken)},
	} {
		if r := previewGet(t, srv, http.MethodGet, "/site/index.html", edit...); r.code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, r.code)
		}
	}
}

func TestPreviewTokenOnlyWithPort(t *testing.T) {
	srv := newServerFor(t, Options{})
	if srv.previewToken != "" {
		t.Error("a token without a preview port")
	}
	if r := previewGet(t, srv, http.MethodGet, "/README.md", withCookie("gh-mini-preview-0", "")); r.code != http.StatusNotFound {
		t.Errorf("status %d, want 404", r.code)
	}
}

func newServerFor(t *testing.T, opts Options) *Server {
	t.Helper()
	root, _ := newTestRepo(t)
	opts.Root, opts.Name = root, "repo"
	srv, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}
