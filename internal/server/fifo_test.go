//go:build unix

package server

import (
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A named pipe is not read: reading would wait until something writes.
func TestHandlerNamedPipe(t *testing.T) {
	root, _ := newTestRepo(t)
	for _, name := range []string{"pipe.md", "sub/README.md", "site/index.html"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(p, 0o644); err != nil {
			t.Skip(err)
		}
	}
	srv, err := New(Options{Root: root, Name: "repo", PreviewPort: testPreviewPort})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	h := srv.Handler()
	token := withCookie("gh-mini-preview-7000", srv.previewToken)
	done := make(chan struct{})
	go func() {
		defer close(done)
		get(t, h, "/pipe.md").expect(t, http.StatusOK, `data-kind="binary"`)
		get(t, h, "/pipe.md?raw").expect(t, http.StatusNotFound)
		get(t, h, "/sub/").expect(t, http.StatusOK, `data-kind="dir"`)
		previewGet(t, srv, http.MethodGet, "/site/index.html", token).expect(t, http.StatusNotFound)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a request waits on the pipe")
	}
}
