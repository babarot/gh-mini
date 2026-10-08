package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	// Status tells that what changed since the last commit changed
	Status bool `json:"status,omitempty"`
	// Head tells that HEAD or a branch of origin moved, which changes
	// where the branch stands against its base
	Head bool `json:"head,omitempty"`
	// Resync tells that changes may have been missed: read everything
	Resync bool `json:"resync,omitempty"`
	// Seq numbers the changes, the latest one merged into this
	Seq uint64 `json:"seq"`
}

// maxHistory bounds the changes kept for pages catching up.
const maxHistory = 256

// A page is rendered before it subscribes, is left in the back/forward
// cache unsubscribed, and loses the stream while the server restarts. So
// that it misses no change in between, the hub numbers its changes and
// keeps the latest; a page is rendered with the number of the latest
// change it shows, and asks for those after it on each of these.
type hub struct {
	mu      sync.Mutex
	subs    map[*subscriber]struct{}
	seq     uint64
	history []change
}

func newHub() *hub {
	return &hub{subs: map[*subscriber]struct{}{}}
}

// current is the number of the latest change.
func (h *hub) current() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seq
}

// since merges the changes after seq; a resync when they are not all
// kept.
func (h *hub) since(seq uint64) change {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case seq > h.seq, len(h.history) > 0 && seq < h.history[0].Seq-1:
		return change{Resync: true, Seq: h.seq}
	}
	merged := &subscriber{ready: make(chan struct{}, 1)}
	for _, c := range h.history {
		if c.Seq > seq {
			merged.add(c)
		}
	}
	c, _ := merged.take()
	c.Seq = h.seq
	return c
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
	h.seq++
	c.Seq = h.seq
	h.history = append(h.history, c)
	if len(h.history) > maxHistory {
		h.history = append(h.history[:0:0], h.history[len(h.history)-maxHistory:]...)
	}
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
	sub.pending.Status = sub.pending.Status || c.Status
	sub.pending.Head = sub.pending.Head || c.Head
	sub.pending.Resync = sub.pending.Resync || c.Resync
	sub.pending.Seq = max(sub.pending.Seq, c.Seq)
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

// serveChanges tells a page the changes after ?since=, which it missed.
func (s *Server) serveChanges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	seq, err := strconv.ParseUint(q.Get("since"), 10, 64)
	if err != nil {
		http.Error(w, "bad since", http.StatusBadRequest)
		return
	}
	// Not no-store, which would keep the page out of the back/forward
	// cache
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	c := change{Resync: true, Seq: s.hub.current()}
	// The numbers of another server, started before this one, are not
	// this one's
	if q.Get("boot") == s.boot {
		c = s.hub.since(seq)
	}
	_ = json.NewEncoder(w).Encode(c)
}

type seqKey struct{}

// withSeq notes on a request the number of the latest change, before
// anything the page shows is read: a change told after it may then be
// shown already, and reload the page once more, but none is missed.
func (s *Server) withSeq(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), seqKey{}, s.hub.current()))
}

func seqOf(r *http.Request) uint64 {
	seq, _ := r.Context().Value(seqKey{}).(uint64)
	return seq
}

// notify tells the browsers what changed. HEAD or a branch of origin
// moving changes where the branch stands, which Head tells; when it
// changes what is uncommitted, Status tells that too.
func (s *Server) notify(e workspace.Event) {
	if len(e.Paths) == 0 && len(e.Dirs) == 0 && !e.Structure && !e.Theme && !e.Resync && !e.Status && !e.GitHead {
		return
	}
	s.hub.publish(change{Paths: e.Paths, Dirs: e.Dirs, Structure: e.Structure, Theme: e.Theme, Resync: e.Resync, Status: e.Status, Head: e.GitHead})
}
