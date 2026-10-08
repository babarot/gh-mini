package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/babarot/gh-mini/internal/workspace"
)

// change is what the browser is told after files change. Pages reload only
// for changes that affect them; Structure refreshes the sidebar.
type change struct {
	Paths []string `json:"paths"`
	// Dirs are directories where changes were too many to list
	Dirs      []string `json:"dirs,omitempty"`
	Structure bool     `json:"structure"`
	Theme     bool     `json:"theme"`
	// Resync tells that changes may have been missed: read everything
	Resync bool `json:"resync,omitempty"`
}

type hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

// subscriber is one browser's stream. Changes that come while it is still
// writing an earlier one are merged into one pending change rather than
// queued, so none is lost however slow the browser is.
type subscriber struct {
	mu      sync.Mutex
	pending *change
	seen    map[string]bool
	// ready has a value while pending is set
	ready chan struct{}
}

func (h *hub) subscribe() *subscriber {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub := &subscriber{ready: make(chan struct{}, 1)}
	h.subs[sub] = struct{}{}
	return sub
}

func (h *hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, sub)
}

func (h *hub) publish(c change) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		sub.add(c)
	}
}

func (sub *subscriber) add(c change) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.pending == nil {
		sub.pending = &change{}
		sub.seen = map[string]bool{}
	}
	for _, p := range c.Paths {
		if !sub.seen[p] {
			sub.seen[p] = true
			sub.pending.Paths = append(sub.pending.Paths, p)
		}
	}
	for _, d := range c.Dirs {
		if !sub.seen["dir:"+d] {
			sub.seen["dir:"+d] = true
			sub.pending.Dirs = append(sub.pending.Dirs, d)
		}
	}
	sub.pending.Structure = sub.pending.Structure || c.Structure
	sub.pending.Theme = sub.pending.Theme || c.Theme
	sub.pending.Resync = sub.pending.Resync || c.Resync
	select {
	case sub.ready <- struct{}{}:
	default:
	}
}

// take returns the pending change and clears it.
func (sub *subscriber) take() (change, bool) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.pending == nil {
		return change{}, false
	}
	c := *sub.pending
	sub.pending = nil
	return c, true
}

func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	sub := s.hub.subscribe()
	defer s.hub.unsubscribe(sub)
	fmt.Fprint(w, ": connected\n\n")
	// Sent on every connection, so that a browser that lost the stream
	// learns whether the server it reached is the one it had
	fmt.Fprintf(w, "event: boot\ndata: %s\n\n", s.boot)
	flusher.Flush()
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-sub.ready:
			c, ok := sub.take()
			if !ok {
				continue
			}
			b, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// notify tells the browsers what changed. HEAD moving alone changes
// nothing they show but the branch, which the next page load picks up.
func (s *Server) notify(e workspace.Event) {
	if len(e.Paths) == 0 && len(e.Dirs) == 0 && !e.Structure && !e.Theme && !e.Resync {
		return
	}
	s.hub.publish(change{Paths: e.Paths, Dirs: e.Dirs, Structure: e.Structure, Theme: e.Theme, Resync: e.Resync})
}
