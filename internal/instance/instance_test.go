package instance

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got, err := Dir(); err != nil || got != filepath.Join("/state", "gh-mini", "servers") {
		t.Errorf("with XDG_STATE_HOME: got %q, %v", got, err)
	}
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	if got, err := Dir(); err != nil || got != filepath.Join(home, ".local", "state", "gh-mini", "servers") {
		t.Errorf("without XDG_STATE_HOME: got %q, %v", got, err)
	}
}

// serve registers a server in dir answering at a URL of its own, and
// returns it with a channel closed when it is asked to shut down. The
// server stops then, as gh-mini does.
func serve(t *testing.T, dir, served string) (*Instance, chan struct{}) {
	t.Helper()
	stopped := make(chan struct{})
	var in *Instance
	ts := httptest.NewUnstartedServer(nil)
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in.Handler(func() {
			close(stopped)
			go ts.Close()
		}).ServeHTTP(w, r)
	})
	ts.Start()
	t.Cleanup(ts.Close)
	addr, _ := ts.Listener.Addr().(*net.TCPAddr)
	in, err := Register(dir, addr.Port, served, "owner/name", ts.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	return in, stopped
}

func TestRegister(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "servers")
	in, _ := serve(t, dir, "/srv")
	info, err := os.Stat(in.file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}
	if filepath.Base(in.file) != strconv.Itoa(os.Getpid())+".json" {
		t.Errorf("file = %s, want one named after the process", in.file)
	}
	in.Unregister()
	if _, err := os.Stat(in.file); !os.IsNotExist(err) {
		t.Errorf("file still there after Unregister: %v", err)
	}
}

func TestListAndShutdown(t *testing.T) {
	dir := t.TempDir()
	in, stopped := serve(t, dir, "/srv")

	got, err := List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Port != in.Port || got[0].Dir != "/srv" || got[0].Repo != "owner/name" || got[0].URL+"/" != in.URL+"/" {
		t.Fatalf("List = %+v, want the server registered", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := got[0].Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Error("the server was not asked to shut down")
	}
}

func TestListRemovesGone(t *testing.T) {
	dir := t.TempDir()
	// Nothing listens on a port just let go
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr, _ := ln.Addr().(*net.TCPAddr)
	ln.Close()
	gone, err := Register(dir, addr.Port, "/gone", "", "http://"+addr.String())
	if err != nil {
		t.Fatal(err)
	}
	// Something else, not knowing the token, on another
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	reused := filepath.Join(dir, "1.json")
	data := `{"pid":1,"port":1,"dir":"/other","url":"` + other.URL + `","token":"x"}`
	if err := os.WriteFile(reused, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	// And a file that is not one
	if err := os.WriteFile(filepath.Join(dir, "2.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("List = %+v, want none", got)
	}
	for _, f := range []string{gone.file, reused} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s not removed: %v", f, err)
		}
	}
}

func TestListWithoutDir(t *testing.T) {
	got, err := List(context.Background(), filepath.Join(t.TempDir(), "none"))
	if err != nil || got != nil {
		t.Errorf("List = %v, %v; want nothing", got, err)
	}
}

func TestHandlerNeedsToken(t *testing.T) {
	in := &Instance{Token: "secret"}
	called := false
	h := in.Handler(func() { called = true })
	for _, tc := range []struct {
		method, path, token string
		want                int
	}{
		{"POST", "shutdown", "", http.StatusForbidden},
		{"POST", "shutdown", "wrong", http.StatusForbidden},
		{"GET", "shutdown", "secret", http.StatusMethodNotAllowed},
		{"POST", "ping", "secret", http.StatusMethodNotAllowed},
		{"GET", "other", "secret", http.StatusNotFound},
		{"GET", "ping", "secret", http.StatusNoContent},
	} {
		r := httptest.NewRequest(tc.method, Path+tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s with %q: %d, want %d", tc.method, tc.path, tc.token, w.Code, tc.want)
		}
	}
	if called {
		t.Error("shut down without the token")
	}
	r := httptest.NewRequest("POST", Path+"shutdown", nil)
	r.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !called {
		t.Error("not shut down with the token")
	}
}
