package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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

// watch follows every directory under the root except skipped ones, and the
// themes directory, and tells the browsers what changed.
func (s *Server) watch() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	addTree := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if p != dir && s.skipped(d.Name()) {
				return fs.SkipDir
			}
			if err := w.Add(p); err != nil {
				log.Printf("watch %s: %v", p, err)
			}
			return nil
		})
	}
	addTree(s.opts.Root)
	themes := s.opts.ThemesDir
	if themes != "" {
		if _, err := os.Stat(themes); err == nil {
			_ = w.Add(themes)
		}
	}

	go func() {
		defer w.Close()
		var (
			pending change
			seen    = map[string]bool{}
			timer   = time.NewTimer(time.Hour)
		)
		timer.Stop()
		for {
			select {
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Printf("watch: %v", err)
			case e, ok := <-w.Events:
				if !ok {
					return
				}
				if e.Has(fsnotify.Chmod) && !e.Has(fsnotify.Write) {
					continue
				}
				if themes != "" && filepath.Dir(e.Name) == themes {
					pending.Theme = true
				} else {
					rel, err := filepath.Rel(s.opts.Root, e.Name)
					if err != nil {
						continue
					}
					rel = filepath.ToSlash(rel)
					if s.skipped(filepath.Base(rel)) {
						continue
					}
					if !seen[rel] {
						seen[rel] = true
						pending.Paths = append(pending.Paths, rel)
					}
					if e.Has(fsnotify.Create) || e.Has(fsnotify.Remove) || e.Has(fsnotify.Rename) ||
						filepath.Base(rel) == ".gitignore" {
						pending.Structure = true
					}
					if e.Has(fsnotify.Create) {
						if fi, err := os.Stat(e.Name); err == nil && fi.IsDir() {
							addTree(e.Name)
						}
					}
				}
				timer.Reset(150 * time.Millisecond)
			case <-timer.C:
				if pending.Structure {
					s.invalidateTree()
				}
				s.hub.publish(pending)
				pending = change{}
				seen = map[string]bool{}
			}
		}
	}()
	return nil
}
