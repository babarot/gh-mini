// Package instance keeps track of the gh-mini servers running on this
// machine, so that one can list them and shut them down.
//
// A server registers itself with a file in Dir, named after its process,
// telling the directory it serves, the URL it is reached at and a token
// of its own. The token is all that lets a request shut it down: it is
// in a file only the user can read, and a page in a browser cannot learn
// it. A server killed before it removes its file leaves the file behind;
// List finds the server gone and removes the file then.
package instance

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Path is where the control requests are answered, under the server's URL.
const Path = "/_mini/control/"

// Instance is a gh-mini server, as its file tells it.
type Instance struct {
	PID  int    `json:"pid"`
	Port int    `json:"port"`
	Dir  string `json:"dir"`
	URL  string `json:"url"`
	// Repo is the GitHub repository of the origin remote, "owner/name",
	// or "" when there is none.
	Repo string `json:"repo,omitempty"`
	// Token authorizes the control requests.
	Token string `json:"token"`

	file string
}

// Dir is the directory the servers' files are kept in:
// $XDG_STATE_HOME/gh-mini/servers, ~/.local/state/gh-mini/servers without
// it.
func Dir() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "gh-mini", "servers"), nil
}

// Register writes the file for the server of this process, in dir. The
// Instance returned holds the token its Handler checks. Unregister removes
// the file.
func Register(dir string, port int, served, repo, url string) (*Instance, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	in := &Instance{
		PID:   os.Getpid(),
		Port:  port,
		Dir:   served,
		Repo:  repo,
		URL:   strings.TrimSuffix(url, "/"),
		Token: hex.EncodeToString(b),
		file:  filepath.Join(dir, strconv.Itoa(os.Getpid())+".json"),
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	// Written whole under another name first, so that List never reads
	// half of it
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), in.file); err != nil {
		return nil, err
	}
	return in, nil
}

// Unregister removes the server's file. A file left behind is removed by
// List, so there is nothing to do when it cannot be.
func (in *Instance) Unregister() {
	_ = os.Remove(in.file)
}

// Handler answers the control requests for the server, at Path: GET ping
// tells it is running, POST shutdown calls shutdown after answering.
// Requests without the token are refused.
func (in *Instance) Handler(shutdown func()) http.Handler {
	var once sync.Once
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(in.Token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, Path) {
		case "ping":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "shutdown":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			once.Do(shutdown)
		default:
			http.NotFound(w, r)
		}
	})
}

// List returns the servers running, by port. The files of servers gone
// are removed.
func List(ctx context.Context, dir string) ([]*Instance, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []*Instance
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		file := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		in := &Instance{file: file}
		if err := json.Unmarshal(data, in); err != nil {
			continue
		}
		all = append(all, in)
	}

	alive := make([]bool, len(all))
	var wg sync.WaitGroup
	for i, in := range all {
		wg.Go(func() {
			ok, gone := in.ping(ctx)
			alive[i] = ok
			if gone {
				in.Unregister()
			}
		})
	}
	wg.Wait()

	var out []*Instance
	for i, in := range all {
		if alive[i] {
			out = append(out, in)
		}
	}
	slices.SortFunc(out, func(a, b *Instance) int {
		if a.Port != b.Port {
			return a.Port - b.Port
		}
		return strings.Compare(a.Dir, b.Dir)
	})
	return out, nil
}

// client is for the control requests. They go to this machine, so a
// server that does not answer soon is taken as not answering.
var client = &http.Client{Timeout: 2 * time.Second}

// ping tells whether the server answers as itself, and whether it is
// surely gone: nothing listens at its URL, or something else does. A
// server too busy to answer is neither.
func (in *Instance) ping(ctx context.Context) (ok, gone bool) {
	res, err := in.do(ctx, http.MethodGet, "ping")
	if err != nil {
		var opErr interface{ Timeout() bool }
		if errors.As(err, &opErr) && opErr.Timeout() || ctx.Err() != nil {
			return false, false
		}
		return false, true
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return false, true
	}
	return true, false
}

// Shutdown asks the server to stop, and waits until it has, or until ctx
// is done.
func (in *Instance) Shutdown(ctx context.Context) error {
	res, err := in.do(ctx, http.MethodPost, "shutdown")
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%s: %s", in.URL, res.Status)
	}
	// It stops after answering; it is gone when it no longer answers
	for {
		if ok, _ := in.ping(ctx); !ok {
			if ctx.Err() != nil {
				return fmt.Errorf("%s did not stop: %w", in.URL, ctx.Err())
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s did not stop: %w", in.URL, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (in *Instance) do(ctx context.Context, method, action string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, in.URL+Path+action, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token)
	return client.Do(req)
}
