package server

import (
	"slices"
	"testing"
)

// However many changes come before the stream writes, none is lost.
func TestSubscriberMergesChanges(t *testing.T) {
	h := &hub{subs: map[*subscriber]struct{}{}}
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
