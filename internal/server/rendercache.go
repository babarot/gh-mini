package server

import (
	"container/list"
	"html/template"
	"io/fs"
	"sync"

	"github.com/babarot/gh-mini/internal/markdown"
)

// maxRenderCache bounds the rendered Markdown kept, in bytes of HTML.
const maxRenderCache = 32 << 20

// renderCache keeps rendered Markdown by file, its modification time and
// size, dropping the least recently used first. Rendering a large file
// takes long enough to be felt: about 80ms for 200KB.
type renderCache struct {
	mu    sync.Mutex
	max   int
	size  int
	ll    *list.List
	items map[renderKey]*list.Element
}

type renderKey struct {
	path string
	mod  int64
	size int64
}

type rendered struct {
	key  renderKey
	html template.HTML
	f    markdown.Features
}

func newRenderCache(max int) *renderCache {
	return &renderCache{max: max, ll: list.New(), items: map[renderKey]*list.Element{}}
}

func (c *renderCache) get(k renderKey) (*rendered, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.items[k]
	if e == nil {
		return nil, false
	}
	c.ll.MoveToFront(e)
	return e.Value.(*rendered), true
}

func (c *renderCache) put(r *rendered) {
	n := len(r.html)
	if n > c.max {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.items[r.key]; e != nil {
		return
	}
	for c.size+n > c.max {
		e := c.ll.Back()
		old := e.Value.(*rendered)
		c.size -= len(old.html)
		c.ll.Remove(e)
		delete(c.items, old.key)
	}
	c.items[r.key] = c.ll.PushFront(r)
	c.size += n
}

// renderMarkdownFile renders the Markdown file at rel, whose contents are
// src, reusing what was rendered while the file is unchanged.
func (s *Server) renderMarkdownFile(rel string, info fs.FileInfo, src []byte) (template.HTML, markdown.Features, error) {
	k := renderKey{path: rel, mod: info.ModTime().UnixNano(), size: info.Size()}
	if r, ok := s.renders.get(k); ok {
		return r.html, r.f, nil
	}
	html, f, err := s.renderMarkdown(src)
	if err != nil {
		return "", f, err
	}
	s.renders.put(&rendered{key: k, html: html, f: f})
	return html, f, nil
}
