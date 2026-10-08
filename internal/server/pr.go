package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// prTimeout bounds a run of gh, which asks GitHub.
	prTimeout = 5 * time.Second
	// prMaxAge is how long a pull request's state is kept: it is merged
	// or closed on GitHub, with nothing changed here.
	prMaxAge = time.Minute
)

// pullRequest is the pull request of the branch, as the top bar shows it.
type pullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	// State is open, draft, merged or closed
	State string `json:"state"`
	URL   string `json:"url"`
}

// prCache keeps the pull request of a branch at a commit for a while, and
// runs gh once for the requests that come while it runs.
type prCache struct {
	mu   sync.Mutex
	key  string
	at   time.Time
	pr   *pullRequest
	wait chan struct{}
}

func (c *prCache) get(key string, find func() *pullRequest) *pullRequest {
	c.mu.Lock()
	for c.wait != nil {
		wait := c.wait
		c.mu.Unlock()
		<-wait
		c.mu.Lock()
	}
	if c.key == key && time.Since(c.at) < prMaxAge {
		defer c.mu.Unlock()
		return c.pr
	}
	wait := make(chan struct{})
	c.wait = wait
	c.mu.Unlock()

	pr := find()
	c.mu.Lock()
	c.key, c.at, c.pr, c.wait = key, time.Now(), pr, nil
	c.mu.Unlock()
	close(wait)
	return pr
}

// ghPullRequest asks gh for the pull request of a branch on GitHub, or
// nil: none, gh not signed in, or GitHub not reached.
func ghPullRequest(gh, dir, branch string) *pullRequest {
	ctx, cancel := context.WithTimeout(context.Background(), prTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gh, "pr", "view", branch, "--json", "number,title,state,isDraft,url")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var v struct {
		Number  int
		Title   string
		State   string
		IsDraft bool
		URL     string
	}
	if json.Unmarshal(out, &v) != nil || v.Number == 0 || !strings.HasPrefix(v.URL, "https://") {
		return nil
	}
	pr := &pullRequest{Number: v.Number, Title: v.Title, State: strings.ToLower(v.State), URL: v.URL}
	if pr.State == "open" && v.IsDraft {
		pr.State = "draft"
	}
	return pr
}

// servePR serves the branch's pull request as JSON, or nothing: on the
// base, for a branch not pushed, without gh, or outside a repository on
// GitHub. The page asks for it once shown, as gh may take a while.
func (s *Server) servePR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	b := s.ws.Branch()
	if s.gh == "" || s.ws.Repo() == "" || b == nil || b.OnBase || b.Detached || b.Upstream == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// The branch as named on the remote, which may name it otherwise
	_, branch, _ := strings.Cut(b.Upstream, "/")
	pr := s.prs.get(branch+"\x00"+b.Head(), func() *pullRequest {
		return ghPullRequest(s.gh, s.ws.Root(), branch)
	})
	if pr == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pr)
}
