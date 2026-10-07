package server

import (
	"html/template"
	"testing"
)

func TestRenderCacheEvicts(t *testing.T) {
	c := newRenderCache(10)
	put := func(path, html string) {
		c.put(&rendered{key: renderKey{path: path}, html: template.HTML(html)})
	}
	has := func(path string) bool {
		_, ok := c.get(renderKey{path: path})
		return ok
	}
	put("a", "1234")
	put("b", "1234")
	has("a") // a is now the most recent
	put("c", "1234")
	if !has("a") || has("b") || !has("c") {
		t.Errorf("a %v, b %v, c %v; want b dropped", has("a"), has("b"), has("c"))
	}
	put("big", "12345678901")
	if has("big") {
		t.Error("kept an entry larger than the cache")
	}
}
