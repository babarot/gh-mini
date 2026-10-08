package server

import (
	"encoding/json"
	"slices"
	"testing"
)

// However many changes come before the stream writes, none is lost.
func TestSubscriberMergesChanges(t *testing.T) {
	h := newHub()
	sub := h.subscribe()
	for _, c := range []change{
		{Paths: []string{"a.md"}},
		{Paths: []string{"b.md", "a.md"}},
		{Structure: true},
		{Theme: true},
		{Paths: []string{"c.md"}},
		{Paths: []string{"d.md"}},
	} {
		h.publish(c)
	}
	<-sub.ready
	got, ok := sub.take()
	if !ok {
		t.Fatal("nothing pending")
	}
	if !slices.Equal(got.Paths, []string{"a.md", "b.md", "c.md", "d.md"}) || !got.Structure || !got.Theme {
		t.Errorf("merged change = %+v", got)
	}
	if _, ok := sub.take(); ok {
		t.Error("still pending after take")
	}
}

// A page catching up gets the changes after the latest it shows, merged,
// or a resync when they are no longer all kept.
func TestHubSince(t *testing.T) {
	h := newHub()
	if c := h.since(0); c.Resync || c.Seq != 0 || len(c.Paths) != 0 {
		t.Errorf("nothing changed: %+v", c)
	}
	h.publish(change{Paths: []string{"a.md"}})
	h.publish(change{Paths: []string{"b.md"}, Structure: true})
	h.publish(change{Paths: []string{"a.md"}})
	if c := h.since(1); c.Resync || c.Seq != 3 || !slices.Equal(c.Paths, []string{"b.md", "a.md"}) || !c.Structure {
		t.Errorf("since 1: %+v", c)
	}
	if c := h.since(3); c.Resync || c.Seq != 3 || len(c.Paths) != 0 || c.Structure {
		t.Errorf("since the latest: %+v", c)
	}
	if c := h.since(4); !c.Resync {
		t.Errorf("a change to come: %+v", c)
	}
	for range maxHistory {
		h.publish(change{Paths: []string{"c.md"}})
	}
	if c := h.since(2); !c.Resync {
		t.Errorf("changes no longer kept: %+v", c)
	}
	if c := h.since(3); c.Resync || !slices.Equal(c.Paths, []string{"c.md"}) {
		t.Errorf("since the oldest kept: %+v", c)
	}
}

// A page of another server, started before this one, is told to read
// everything again: its numbers are not this server's.
func TestServeChangesBoot(t *testing.T) {
	root, _ := newTestRepo(t)
	srv, err := New(Options{Root: root, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	srv.hub.publish(change{Paths: []string{"a.md"}})
	for boot, resync := range map[string]bool{srv.boot: false, "other": true, "": true} {
		r := get(t, srv.Handler(), "/_mini/api/changes?since=0&boot="+boot)
		var c change
		if err := json.Unmarshal([]byte(r.body), &c); err != nil {
			t.Fatalf("boot %q: %v: %s", boot, err, r.body)
		}
		if c.Resync != resync || c.Seq != 1 || (!resync && !slices.Equal(c.Paths, []string{"a.md"})) {
			t.Errorf("boot %q: %+v", boot, c)
		}
	}
}
