package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-mini/internal/workspace"
)

// A 1x1 PNG
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// isolateGit keeps the machine's git configuration, such as commit signing
// or a global excludes file, out of the tests. The server's own git
// commands inherit the environment too.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// newTestRepo makes a committed git repository with ignored translations,
// an ignored directory, a skipped directory and a symlink out of the root,
// and returns its path and a themes directory with a sepia theme.
func newTestRepo(t *testing.T) (root, themes string) {
	t.Helper()
	isolateGit(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "repo")
	themes = filepath.Join(base, "themes")
	outside := filepath.Join(base, "outside")

	files := map[string]string{
		".gitignore":         "*.ja.md\nlocal-only/\n",
		"README.md":          "# Repo\n\nSee #1.\n",
		"README.ja.md":       "# リポジトリ\n",
		"docs/guide.md":      "# Guide\n\n## Install\n",
		"docs/guide.ja.md":   "# ガイド\n",
		"docs/a b.md":        "# Spaces\n",
		"docs/diagram.md":    "# Diagram\n\n```mermaid\ngraph TD; A-->B\n```\n",
		"docs/math.md":       "# Math\n\nEuler: $e^{i\\pi} = -1$\n",
		"docs/README.md":     "# Docs\n\n```mermaid\ngraph TD; A-->B\n```\n",
		"local-only/note.md": "# Note\n",
		"main.go":            "package main\n\nfunc main() {}\n",
		"bin.dat":            "a\x00b",
		"node_modules/x.js":  "x\n",
	}
	for name, body := range files {
		writeFile(t, filepath.Join(root, name), []byte(body))
	}
	writeFile(t, filepath.Join(root, "img.png"), pngBytes)
	writeFile(t, filepath.Join(outside, "secret.txt"), []byte("secret\n"))
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(themes, "sepia.css"), []byte(":root { --sepia: 1; }\n"))

	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "remote", "add", "origin", "https://github.com/example/repo.git")
	git(t, root, "add", "-A")
	git(t, root, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init")
	return root, themes
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
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
	return srv.Handler()
}

type response struct {
	code   int
	header http.Header
	body   string
}

func get(t *testing.T, h http.Handler, target string, edit ...func(*http.Request)) response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, f := range edit {
		f(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Result().Body)
	return response{code: rec.Code, header: rec.Header(), body: string(b)}
}

func withCookie(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}

func withHeader(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(name, value) }
}

func (r response) expect(t *testing.T, code int, contains ...string) {
	t.Helper()
	if r.code != code {
		t.Errorf("status = %d, want %d", r.code, code)
	}
	for _, s := range contains {
		if !strings.Contains(r.body, s) {
			t.Errorf("body does not contain %q", s)
		}
	}
}

func (r response) reject(t *testing.T, absent ...string) {
	t.Helper()
	for _, s := range absent {
		if strings.Contains(r.body, s) {
			t.Errorf("body contains %q", s)
		}
	}
}

// row returns the directory listing's row for a name.
func (r response) row(t *testing.T, name string) string {
	t.Helper()
	i := strings.Index(r.body, ">"+name+"</a>")
	if i < 0 {
		t.Fatalf("no row for %q", name)
	}
	start := strings.LastIndex(r.body[:i], "<tr")
	end := strings.Index(r.body[i:], "</tr>")
	return r.body[start : i+end]
}

func TestHandlerRootDir(t *testing.T) {
	r := get(t, newTestServer(t), "/")
	r.expect(t, http.StatusOK,
		`data-kind="dir"`,
		`<h1 id="repo">Repo</h1>`,
		`https://github.com/example/repo/issues/1`,
		`>README.md</a>`,
		"main\n",
	)
	r.reject(t, "node_modules")
	if row := r.row(t, "local-only"); !strings.Contains(row, `class="ignored"`) {
		t.Errorf("local-only is not marked ignored: %s", row)
	}
	if row := r.row(t, "docs"); strings.Contains(row, `class="ignored"`) {
		t.Errorf("docs is marked ignored: %s", row)
	}
}

func TestHandlerReadmeLanguage(t *testing.T) {
	h := newTestServer(t)
	get(t, h, "/?lang=ja").expect(t, http.StatusOK, `>リポジトリ</h1>`, `class="segmented langs"`, `>JA</a>`)
	get(t, h, "/", withCookie("gh-mini-lang", "ja")).expect(t, http.StatusOK, `>リポジトリ</h1>`)
	get(t, h, "/?lang=default", withCookie("gh-mini-lang", "ja")).expect(t, http.StatusOK, `<h1 id="repo">Repo</h1>`)
}

func TestHandlerDirRedirect(t *testing.T) {
	r := get(t, newTestServer(t), "/docs")
	if r.code != http.StatusMovedPermanently || r.header.Get("Location") != "/docs/" {
		t.Errorf("got %d to %q, want 301 to /docs/", r.code, r.header.Get("Location"))
	}
}

func TestHandlerMarkdown(t *testing.T) {
	h := newTestServer(t)
	get(t, h, "/docs/guide.md").expect(t, http.StatusOK,
		`data-kind="markdown"`,
		`<h2 id="install">Install</h2>`,
		`>Preview</a>`, `>Code</a>`,
		`href="/docs/guide.ja.md"`, `>JA</a>`,
	)
	get(t, h, "/docs/guide.md?plain=1").expect(t, http.StatusOK,
		`data-kind="code"`,
		`>Preview</a>`, `>Code</a>`,
	)
	get(t, h, "/docs/guide.ja.md").expect(t, http.StatusOK, `data-kind="markdown"`, `>ガイド</h1>`)
	get(t, h, "/docs/a%20b.md").expect(t, http.StatusOK, `data-kind="markdown"`, `>Spaces</h1>`)
}

func TestHandlerCode(t *testing.T) {
	get(t, newTestServer(t), "/main.go").expect(t, http.StatusOK, `data-kind="code"`, `href="#L1"`, "3 lines")
}

func TestHandlerImageAndRaw(t *testing.T) {
	h := newTestServer(t)
	get(t, h, "/img.png").expect(t, http.StatusOK, `data-kind="image"`, `<img src="?raw"`)
	for _, r := range []response{
		get(t, h, "/img.png", withHeader("Sec-Fetch-Dest", "image")),
		get(t, h, "/img.png?raw"),
	} {
		if r.code != http.StatusOK || !bytes.Equal([]byte(r.body), pngBytes) {
			t.Errorf("raw image: status %d, %d bytes", r.code, len(r.body))
		}
	}
	get(t, h, "/bin.dat").expect(t, http.StatusOK, `data-kind="binary"`, "This file is binary or too large to show.", "3 Bytes")
	if r := get(t, h, "/bin.dat?raw"); r.code != http.StatusOK || r.body != "a\x00b" {
		t.Errorf("raw binary: status %d, body %q", r.code, r.body)
	}
}

// A text file too large to render gets only a raw link, as a binary file
// does.
func TestHandlerTooLarge(t *testing.T) {
	root, _ := newTestRepo(t)
	big := bytes.Repeat([]byte("# x\n"), maxRender/4+1)
	writeFile(t, filepath.Join(root, "big.md"), big)
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	h := srv.Handler()
	r := get(t, h, "/big.md")
	r.expect(t, http.StatusOK, `data-kind="binary"`, "This file is binary or too large to show.", "2.0 MB")
	r.reject(t, `<h1 id="x">`)
	if r := get(t, h, "/big.md?raw"); r.code != http.StatusOK || len(r.body) != len(big) {
		t.Errorf("raw: status %d, %d bytes, want %d", r.code, len(r.body), len(big))
	}
}

func TestHandlerNotFound(t *testing.T) {
	get(t, newTestServer(t), "/nope").expect(t, http.StatusNotFound, `data-kind="notfound"`)
}

func TestHandlerSymlinkOutOfRoot(t *testing.T) {
	h := newTestServer(t)
	for _, target := range []string{"/link/", "/link/secret.txt", "/link/secret.txt?raw"} {
		r := get(t, h, target)
		if r.code == http.StatusOK || strings.Contains(r.body, "secret\n") {
			t.Errorf("%s shows what is outside the root (status %d)", target, r.code)
		}
	}
}

func TestHandlerTree(t *testing.T) {
	r := get(t, newTestServer(t), "/_mini/api/tree")
	var root workspace.Node
	if err := json.Unmarshal([]byte(r.body), &root); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range root.Children {
		names = append(names, c.Name)
		if c.Name == "node_modules" {
			t.Error("node_modules is in the tree")
		}
	}
	// Directories first, then files, ignoring case. A symlink is listed as
	// a file, since the walk does not follow it
	want := "docs,local-only,.gitignore,bin.dat,img.png,link,main.go,README.ja.md,README.md"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("root children = %s, want %s", got, want)
	}
	find := func(p string) *workspace.Node {
		var walk func(n *workspace.Node) *workspace.Node
		walk = func(n *workspace.Node) *workspace.Node {
			if n.Path == p {
				return n
			}
			for _, c := range n.Children {
				if f := walk(c); f != nil {
					return f
				}
			}
			return nil
		}
		return walk(&root)
	}
	for p, ignored := range map[string]bool{
		"local-only":       true,
		"README.ja.md":     true,
		"docs/guide.ja.md": true,
		"README.md":        false,
		"docs/guide.md":    false,
	} {
		n := find(p)
		if n == nil {
			t.Errorf("%s is not in the tree", p)
		} else if n.Ignored != ignored {
			t.Errorf("%s ignored = %v, want %v", p, n.Ignored, ignored)
		}
	}
}

// The ignored mark on a file's page must not depend on the tree having
// been built by an earlier request.
func TestHandlerIgnoredFileFirst(t *testing.T) {
	get(t, newTestServer(t), "/docs/guide.ja.md").expect(t, http.StatusOK, `<span class="badge" title="Ignored by git`)
}

func TestHandlerThemes(t *testing.T) {
	h := newTestServer(t)
	get(t, h, "/_mini/theme/github.css").expect(t, http.StatusOK, "--borderColor-success-emphasis: #238636")
	get(t, h, "/_mini/theme/sepia.css").expect(t, http.StatusOK, "--sepia: 1")
	get(t, h, "/_mini/theme/nope.css").expect(t, http.StatusNotFound)
	get(t, h, "/_mini/theme/nord.css").expect(t, http.StatusOK, "--bgColor-default: #2e3440")
	get(t, h, "/_mini/theme/tokyo-night.css").expect(t, http.StatusOK, "--bgColor-default: #1a1b26")
	get(t, h, "/", withSettings(`{"theme":"nord"}`)).expect(t, http.StatusOK,
		`href="/_mini/theme/nord.css"`, `<option value="nord" selected>`)
	get(t, h, "/", withCookie("gh-mini-theme", "sepia")).expect(t, http.StatusOK,
		`href="/_mini/theme/sepia.css"`, `<option value="sepia" selected>`)
	get(t, h, "/", withCookie("gh-mini-theme", "unknown")).expect(t, http.StatusOK,
		`href="/_mini/theme/github.css"`)
	get(t, h, "/", withCookie("gh-mini-mode", "dark")).expect(t, http.StatusOK, `<html lang="en" data-mode="dark">`)
}

// A theme of the viewer's own replaces the built-in one of the same name.
func TestHandlerThemeOverridesBuiltin(t *testing.T) {
	root, themes := newTestRepo(t)
	writeFile(t, filepath.Join(themes, "nord.css"), []byte(":root { --mine: 1; }\n"))
	srv, err := New(Options{Root: root, Name: "repo", Skip: []string{".git"}, ThemesDir: themes})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	r := get(t, h, "/_mini/theme/nord.css")
	r.expect(t, http.StatusOK, "--mine: 1")
	if strings.Contains(r.body, "#2e3440") {
		t.Errorf("built-in nord served instead of the viewer's: %q", r.body)
	}
	if n := strings.Count(get(t, h, "/").body, `<option value="nord"`); n != 1 {
		t.Errorf("nord listed %d times", n)
	}
}

// events reads the server's event stream and sends each change on.
func events(t *testing.T, url string) <-chan change {
	t.Helper()
	res, err := http.Get(url + "/_mini/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	sc := bufio.NewScanner(res.Body)
	if !sc.Scan() || sc.Text() != ": connected" {
		t.Fatalf("first line = %q", sc.Text())
	}
	out := make(chan change, 16)
	go func() {
		defer close(out)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var c change
				if json.Unmarshal([]byte(data), &c) == nil {
					out <- c
				}
			}
		}
	}()
	return out
}

func newReloadServer(t *testing.T) (root, themes, url string) {
	t.Helper()
	root, themes = newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "repo", Skip: []string{".git"}, ThemesDir: themes, Reload: true})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.CloseClientConnections()
		ts.Close()
		srv.Close()
	})
	return root, themes, ts.URL
}

// waitFor collects changes until want returns true for what was collected.
func waitFor(t *testing.T, ch <-chan change, want func(change) bool) {
	t.Helper()
	var got change
	timeout := time.After(5 * time.Second)
	for !want(got) {
		select {
		case c, ok := <-ch:
			if !ok {
				t.Fatal("stream closed")
			}
			got.Theme = got.Theme || c.Theme
			got.Structure = got.Structure || c.Structure
			got.Paths = append(got.Paths, c.Paths...)
		case <-timeout:
			t.Fatalf("got %+v", got)
		}
	}
}

func TestHandlerEvents(t *testing.T) {
	root, themes, url := newReloadServer(t)
	ch := events(t, url)
	writeFile(t, filepath.Join(root, "docs", "guide.md"), []byte("# Changed\n"))
	writeFile(t, filepath.Join(themes, "sepia.css"), []byte(":root {}\n"))
	waitFor(t, ch, func(c change) bool { return c.Theme && slices.Contains(c.Paths, "docs/guide.md") })
}

// A directory git ignores is watched once its page is looked at.
func TestHandlerEventsInIgnoredDir(t *testing.T) {
	root, _, url := newReloadServer(t)
	ch := events(t, url)
	res, err := http.Get(url + "/local-only/note.md")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	writeFile(t, filepath.Join(root, "local-only", "note.md"), []byte("# Changed\n"))
	waitFor(t, ch, func(c change) bool { return slices.Contains(c.Paths, "local-only/note.md") })
}

func TestHandlerStaticFiles(t *testing.T) {
	h := newTestServer(t)
	page := get(t, h, "/")
	m := regexp.MustCompile(`href="(/_mini/static/[0-9a-f]{12})/assets/app.css"`).FindStringSubmatch(page.body)
	if m == nil {
		t.Fatal("the page does not link a versioned app.css")
	}
	prefix := m[1]
	for _, p := range []string{
		"/assets/app.css", "/assets/js/main.js", "/assets/js/util.js", "/assets/js/mermaid.js", "/assets/vendor/mermaid.min.js", "/chroma.css",
		// What MathJax loads by itself, relative to its script and fontPath
		"/assets/vendor/mathjax/input/tex/extensions/boldsymbol.js",
		"/assets/vendor/mathjax/sre/speech-worker.js",
		"/assets/vendor/mathjax-newcm-font/chtml/woff2/mjx-ncm-ab.woff2",
		"/assets/vendor/mathjax-newcm-font/chtml/dynamic/double-struck.js",
	} {
		r := get(t, h, prefix+p)
		if r.code != http.StatusOK || r.body == "" {
			t.Errorf("%s: status %d, %d bytes", p, r.code, len(r.body))
		}
		if cc := r.header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Errorf("%s: Cache-Control = %q", p, cc)
		}
	}
	old := get(t, h, "/_mini/static/000000000000/assets/app.css")
	if old.code != http.StatusOK || old.header.Get("Cache-Control") != "no-cache" {
		t.Errorf("another version: status %d, Cache-Control %q", old.code, old.header.Get("Cache-Control"))
	}
	get(t, h, prefix+"/nope/x").expect(t, http.StatusNotFound)
	if cc := get(t, h, "/_mini/theme/sepia.css").header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("theme Cache-Control = %q", cc)
	}
}

func TestHandlerScriptsForFeatures(t *testing.T) {
	h := newTestServer(t)
	const mermaid, mathjax = "mermaid.min.js", "tex-mml-chtml.js"
	for target, want := range map[string][2]bool{
		"/":                {false, false},
		"/docs/guide.md":   {false, false},
		"/docs/diagram.md": {true, false},
		"/docs/math.md":    {false, true},
		"/docs/":           {true, false}, // its README has a diagram
		"/main.go":         {false, false},
	} {
		r := get(t, h, target)
		if got := strings.Contains(r.body, mermaid); got != want[0] {
			t.Errorf("%s: Mermaid loaded = %v, want %v", target, got, want[0])
		}
		if got := strings.Contains(r.body, mathjax); got != want[1] {
			t.Errorf("%s: MathJax loaded = %v, want %v", target, got, want[1])
		}
	}
}

func TestHandlerTreeETag(t *testing.T) {
	root, themes := newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "repo", Skip: []string{".git"}, ThemesDir: themes, Reload: true})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()

	first := get(t, h, "/_mini/api/tree")
	etag := first.header.Get("ETag")
	if first.code != http.StatusOK || etag == "" {
		t.Fatalf("status %d, ETag %q", first.code, etag)
	}
	if r := get(t, h, "/_mini/api/tree", withHeader("If-None-Match", etag)); r.code != http.StatusNotModified || r.body != "" {
		t.Errorf("same tree: status %d, %d bytes", r.code, len(r.body))
	}

	writeFile(t, filepath.Join(root, "added.md"), []byte("# Added\n"))
	deadline := time.Now().Add(5 * time.Second)
	for {
		r := get(t, h, "/_mini/api/tree", withHeader("If-None-Match", etag))
		if r.code == http.StatusOK {
			if r.header.Get("ETag") == etag || !strings.Contains(r.body, `"added.md"`) {
				t.Errorf("changed tree: ETag %q, body has added.md: %v", r.header.Get("ETag"), strings.Contains(r.body, `"added.md"`))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the tree never changed")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A changed file is rendered again, though its page was cached.
func TestHandlerRenderAfterChange(t *testing.T) {
	root, _ := newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	for _, target := range []string{"/docs/guide.md", "/docs/"} {
		get(t, h, target)
	}
	later := time.Now().Add(time.Minute)
	for name, body := range map[string]string{"docs/guide.md": "# Guide v2\n", "docs/README.md": "# Docs v2\n"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		writeFile(t, p, []byte(body))
		if err := os.Chtimes(p, later, later); err != nil {
			t.Fatal(err)
		}
	}
	get(t, h, "/docs/guide.md").expect(t, http.StatusOK, ">Guide v2</h1>")
	get(t, h, "/docs/").expect(t, http.StatusOK, ">Docs v2</h1>")
}

func TestHandlerRawSandbox(t *testing.T) {
	root, _ := newTestRepo(t)
	for name, body := range map[string]string{
		"page.html": "<script>1</script>", "pic.svg": "<svg/>", "doc.pdf": "%PDF-1.4", "notes.md": "# x",
	} {
		writeFile(t, filepath.Join(root, name), []byte(body))
	}
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	for name, want := range map[string]bool{"page.html": true, "pic.svg": true, "doc.pdf": false, "notes.md": false, "img.png": false} {
		r := get(t, h, "/"+name+"?raw")
		if got := r.header.Get("Content-Security-Policy") == "sandbox"; got != want || r.code != http.StatusOK {
			t.Errorf("%s: status %d, sandboxed %v, want %v", name, r.code, got, want)
		}
	}
	// Loaded as an image, an SVG is the same bytes
	if r := get(t, h, "/pic.svg", withHeader("Sec-Fetch-Dest", "image")); r.body != "<svg/>" {
		t.Errorf("svg as image: %q", r.body)
	}
}

// Headings named like the page's own parts must not share their ids.
func TestHeadingIDsApartFromPage(t *testing.T) {
	root, _ := newTestRepo(t)
	writeFile(t, filepath.Join(root, "ids.md"), []byte("# Settings\n\n## TOC\n\n## Tree\n\n## Theme\n\n[go](#settings)\n"))
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	r := get(t, srv.Handler(), "/ids.md")
	r.expect(t, http.StatusOK, `<h1 id="settings">`, `<h2 id="toc">`, `id="mini.settings"`, `id="mini.toc"`)
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(` id="([^"]+)"`).FindAllStringSubmatch(r.body, -1) {
		if seen[m[1]] {
			t.Errorf("id %q appears twice", m[1])
		}
		seen[m[1]] = true
	}
}
