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
	req.Host = "localhost"
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
		`<p>See #1.</p>`,
		`>README.md</a>`,
		"main\n",
	)
	r.reject(t, "node_modules")
	if row := r.row(t, "local-only"); !strings.Contains(row, `class="ignored hideable"`) {
		t.Errorf("local-only is not marked an ignored directory to hide: %s", row)
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

// The viewer can turn the language switch off; the README is then the
// original's, whatever language was picked last.
func TestHandlerLanguageSwitchOff(t *testing.T) {
	h := newTestServer(t)
	off := withSettings(`{"languageSwitch":false}`)
	r := get(t, h, "/", off, withCookie("gh-mini-lang", "ja"))
	r.expect(t, http.StatusOK, `<h1 id="repo">Repo</h1>`)
	r.reject(t, `class="segmented langs"`)
	get(t, h, "/docs/guide.md", off).reject(t, `class="segmented langs"`)
	get(t, h, "/", withSettings(`{"languageSwitch":true}`)).expect(t, http.StatusOK, `class="segmented langs"`)
}

// With --translations off there are no translations, whatever the viewer
// picked, and the setting says why it does nothing.
func TestHandlerTranslationsOff(t *testing.T) {
	srv := newServerFor(t, Options{Translations: "off"})
	h := srv.Handler()
	r := get(t, h, "/", withSettings(`{"languageSwitch":true}`), withCookie("gh-mini-lang", "ja"))
	r.expect(t, http.StatusOK, `<h1 id="repo">Repo</h1>`,
		`<div class="setting unavailable">`,
		`--translations off</div>`,
		`<input type="checkbox" role="switch" data-setting="languageSwitch" aria-labelledby="mini.setting-languageSwitch" disabled>`)
	r.reject(t, `class="segmented langs"`)
	get(t, h, "/docs/guide.md").reject(t, `class="segmented langs"`)
}

func TestHandlerTranslationsLayout(t *testing.T) {
	root, _ := newTestRepo(t)
	writeFile(t, filepath.Join(root, "ja", "README.md"), []byte("# 日本語\n"))
	writeFile(t, filepath.Join(root, "docs", "guide_fr.md"), []byte("# Guide en français\n"))
	srv, err := New(Options{Root: root, Name: "repo", Translations: "dir, {name}_{lang}.md"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()

	get(t, h, "/?lang=ja").expect(t, http.StatusOK, `>日本語</h1>`, `<a href="/ja/README.md">ja/README.md</a>`, `>JA</a>`)
	get(t, h, "/docs/guide.md").expect(t, http.StatusOK, `<a href="/docs/guide_fr.md" data-lang="FR">FR</a>`)
	if r := get(t, h, "/ja/"); strings.Contains(r.body, `class="segmented langs"`) || !strings.Contains(r.body, `>日本語</h1>`) {
		t.Error("ja/: want its README without a language switch")
	}
	// The layouts are the command line's; the viewer does not pick them
	get(t, h, "/?lang=ja", withSettings(`{"translations":"suffix"}`)).expect(t, http.StatusOK, `>日本語</h1>`)

	if _, err := New(Options{Root: root, Name: "repo", Translations: "{name}"}); err == nil {
		t.Error("New with a layout without {lang} is no error")
	}
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
	get(t, newTestServer(t), "/main.go").expect(t, http.StatusOK, `data-kind="code"`, "3 lines",
		// Each line with its number, so that a wrapped one keeps it
		`<span class="line"><span class="ln" id="L1"><a class="lnlinks" href="#L1">1</a></span><span class="cl">`)
}

func TestHandlerLastCommit(t *testing.T) {
	h := newTestServer(t)
	r := get(t, h, "/main.go")
	r.expect(t, http.StatusOK, `<span class="commit-subject">init</span>`,
		`src="https://avatars.githubusercontent.com/u/e?s=40&amp;email=t%40example.com"`)
	// Not pushed, so not on GitHub to link to
	r.reject(t, "github.com/example/repo/commit/")
	get(t, h, "/docs/guide.ja.md").reject(t, `class="box commit"`)
	// Avatars turned off: the commit without asking GitHub
	r = get(t, h, "/main.go", withSettings(`{"avatars":false}`))
	r.expect(t, http.StatusOK, `<span class="commit-subject">init</span>`)
	r.reject(t, "avatars.githubusercontent.com")
}

func TestHandlerLastCommitPushed(t *testing.T) {
	root, _ := newTestRepo(t)
	git(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	sha := srv.ws.LastCommit("main.go").SHA
	get(t, srv.Handler(), "/main.go").expect(t, http.StatusOK,
		`<a class="commit-subject" href="https://github.com/example/repo/commit/`+sha+`">init</a>`)

	// Pushed elsewhere than GitHub: nothing to link to or to ask for a
	// picture
	git(t, root, "remote", "set-url", "origin", "https://gitlab.com/example/repo.git")
	other, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	get(t, other.Handler(), "/main.go").reject(t, "avatars.githubusercontent.com", "/commit/")
}

// The page shows the repository of the origin remote, whatever the
// directory is called, and the directory's name without one. The viewer's
// state stays under the directory's name, so that two worktrees of a
// repository keep theirs apart.
func TestHandlerRepoName(t *testing.T) {
	root, _ := newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "worktree-a"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	get(t, srv.Handler(), "/docs/").expect(t, http.StatusOK,
		`<title>docs · example/repo</title>`, `data-name="worktree-a"`,
		`<a class="owner" href="https://github.com/example">example</a>`, `<a href="/">repo</a>`)

	git(t, root, "remote", "remove", "origin")
	other, err := New(Options{Root: root, Name: "worktree-a"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	r := get(t, other.Handler(), "/docs/")
	r.expect(t, http.StatusOK, `<title>docs · worktree-a</title>`, `<a href="/">worktree-a</a>`)
	r.reject(t, `class="owner"`)
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
	get(t, h, "/", withCookie("gh-mini-mode", "dark")).expect(t, http.StatusOK, `<html lang="en" data-mode="dark" data-wide="false" data-wrap="false" data-hideIgnoredDirs="false">`)
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

// The stream names the server's boot ID first, the one its pages carry,
// so that a page from an earlier run can tell the server was restarted.
func TestHandlerEventsBoot(t *testing.T) {
	_, _, url := newReloadServer(t)
	res, err := http.Get(url + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	m := regexp.MustCompile(`data-boot="([0-9a-f]+)"`).FindSubmatch(b)
	if m == nil {
		t.Fatal("the page has no boot ID")
	}
	res, err = http.Get(url + "/_mini/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	var lines []string
	for len(lines) < 4 && sc.Scan() {
		lines = append(lines, sc.Text())
	}
	want := []string{": connected", "", "event: boot", "data: " + string(m[1])}
	if !slices.Equal(lines, want) {
		t.Errorf("stream starts %q, want %q", lines, want)
	}
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

// A byte order mark is left out of the page, not out of the raw file.
func TestHandlerBOM(t *testing.T) {
	root, _ := newTestRepo(t)
	writeFile(t, filepath.Join(root, "bom.md"), []byte("\xef\xbb\xbf# BOM\n"))
	writeFile(t, filepath.Join(root, "bom.txt"), []byte("\xef\xbb\xbfline\n"))
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	get(t, h, "/bom.md").expect(t, http.StatusOK, `<h1 id="bom">`)
	if r := get(t, h, "/bom.txt"); strings.Contains(r.body, "\xef\xbb\xbf") {
		t.Error("the code view shows the byte order mark")
	}
	if r := get(t, h, "/bom.txt?raw"); !strings.HasPrefix(r.body, "\xef\xbb\xbf") {
		t.Error("raw lost the byte order mark")
	}
}

func TestTableOfContents(t *testing.T) {
	h := newTestServer(t)
	get(t, h, "/docs/guide.md").expect(t, http.StatusOK,
		`<aside class="toc" id="mini.toc">`, `<a class="l1" href="#guide">Guide</a>`, `<a class="l2" href="#install">Install</a>`)
	// One heading is no table of contents
	if r := get(t, h, "/docs/a%20b.md"); strings.Contains(r.body, `id="mini.toc"`) {
		t.Error("a table of contents for one heading")
	}
}

func TestSidebarCookie(t *testing.T) {
	h := newTestServer(t)
	if r := get(t, h, "/"); strings.Contains(r.body, `class="sidebar-hidden"`) {
		t.Error("hidden without the cookie")
	}
	get(t, h, "/", withCookie("gh-mini-sidebar", "hidden")).expect(t, http.StatusOK, `class="sidebar-hidden"`)
	if r := get(t, h, "/", withCookie("gh-mini-sidebar", "shown")); strings.Contains(r.body, `class="sidebar-hidden"`) {
		t.Error("hidden with the cookie saying shown")
	}
}

func TestTreeOfIgnoredDir(t *testing.T) {
	h := newTestServer(t)
	r := get(t, h, "/_mini/api/tree")
	if strings.Contains(r.body, "local-only/note.md") || !strings.Contains(r.body, `"path":"local-only","dir":true,"ignored":true,"lazy":true`) {
		t.Errorf("tree: %s", r.body)
	}
	get(t, h, "/_mini/api/tree?path=local-only").expect(t, http.StatusOK, `"path":"local-only/note.md"`)
	for _, bad := range []string{"..", "nope", "../outside"} {
		get(t, h, "/_mini/api/tree?path="+bad).expect(t, http.StatusNotFound)
	}
}

func TestHandlerFileWithSlash(t *testing.T) {
	h := newTestServer(t)
	for target, want := range map[string]string{
		"/docs/guide.md/":         "/docs/guide.md",
		"/docs/guide.md/?plain=1": "/docs/guide.md?plain=1",
		"/docs?lang=ja":           "/docs/?lang=ja",
	} {
		r := get(t, h, target)
		if r.code != http.StatusMovedPermanently || r.header.Get("Location") != want {
			t.Errorf("%s: %d to %q, want 301 to %q", target, r.code, r.header.Get("Location"), want)
		}
	}
}

// A file named after a language-like extension is no translation.
func TestNoLanguageForExtensions(t *testing.T) {
	root, _ := newTestRepo(t)
	writeFile(t, filepath.Join(root, "api.md"), []byte("# API\n"))
	writeFile(t, filepath.Join(root, "api.go.md"), []byte("# API in Go\n"))
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if r := get(t, srv.Handler(), "/api.md"); strings.Contains(r.body, `class="segmented langs"`) {
		t.Error("api.md and api.go.md shown as translations")
	}
}

func TestHandlerPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	root, _ := newTestRepo(t)
	writeFile(t, filepath.Join(root, "secret.txt"), []byte("x\n"))
	writeFile(t, filepath.Join(root, "locked", "f.md"), []byte("# f\n"))
	for _, p := range []string{"secret.txt", "locked"} {
		if err := os.Chmod(filepath.Join(root, p), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(filepath.Join(root, p), 0o755) })
	}
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	for _, target := range []string{"/secret.txt", "/locked/", "/locked/f.md"} {
		get(t, h, target).expect(t, http.StatusForbidden, `data-kind="error"`, "permission")
	}
	get(t, h, "/nope.md").expect(t, http.StatusNotFound, `data-kind="notfound"`)
}

func TestPlainReadme(t *testing.T) {
	root, _ := newTestRepo(t)
	writeFile(t, filepath.Join(root, "plain", "README"), []byte("Plain <text>\n"))
	writeFile(t, filepath.Join(root, "both", "README.txt"), []byte("not this\n"))
	writeFile(t, filepath.Join(root, "both", "README.md"), []byte("# This\n"))
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	get(t, h, "/plain/").expect(t, http.StatusOK, `>README</a>`, "<pre>Plain &lt;text&gt;\n</pre>")
	r := get(t, h, "/both/")
	r.expect(t, http.StatusOK, `<h1 id="this">This</h1>`)
	r.reject(t, "not this")
}

func TestHandlerEncodings(t *testing.T) {
	root, _ := newTestRepo(t)
	// "héllo\n" in UTF-16LE with its byte order mark
	writeFile(t, filepath.Join(root, "utf16.txt"), []byte{0xff, 0xfe, 'h', 0, 0xe9, 0, 'l', 0, 'l', 0, 'o', 0, '\n', 0})
	// "日本" in Shift_JIS
	writeFile(t, filepath.Join(root, "sjis.txt"), []byte{0x93, 0xfa, 0x96, 0x7b, '\n'})
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := srv.Handler()
	get(t, h, "/utf16.txt").expect(t, http.StatusOK, `data-kind="code"`, "héllo")
	get(t, h, "/sjis.txt").expect(t, http.StatusOK, `data-kind="code"`, `class="notice"`, "not UTF-8")
	get(t, h, "/bin.dat").expect(t, http.StatusOK, `data-kind="binary"`)
	if r := get(t, h, "/README.md"); strings.Contains(r.body, `class="notice"`) {
		t.Error("a notice for UTF-8")
	}
}
