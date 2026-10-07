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
	Paths     []string `json:"paths"`
	Structure bool     `json:"structure"`
	Theme     bool     `json:"theme"`
}

type hub struct {
	mu   sync.Mutex
	subs map[chan change]struct{}
}

func (h *hub) subscribe() chan change {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan change, 4)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(ch chan change) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
}

func (h *hub) publish(c change) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- c:
		default:
		}
	}
}

func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n")
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
		case c := <-ch:
			b, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// notify tells the browsers what changed. HEAD moving alone changes
// nothing they show but the branch, which the next page load picks up.
func (s *Server) notify(e workspace.Event) {
	if len(e.Paths) == 0 && !e.Structure && !e.Theme {
		return
	}
	s.hub.publish(change{Paths: e.Paths, Structure: e.Structure, Theme: e.Theme})
}
