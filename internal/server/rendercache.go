package server

import (
	"container/list"
	"html/template"
	"io/fs"
	"path"
	"sync"

	"github.com/babarot/gh-mini/internal/markdown"
)

// maxRenderCache bounds the rendered files kept, in bytes of HTML.
const maxRenderCache = 32 << 20

// renderCache keeps rendered Markdown and highlighted code by file, its
// modification time and size, dropping the least recently used first.
// Rendering a large file takes long enough to be felt: about 80ms for
// 200KB of Markdown, and half a second for 512KB of code.
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
	// code is the file shown as code, rather than Markdown rendered
	code bool
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

// renderCodeFile highlights the source file at rel, whose contents are src,
// reusing what was rendered while the file is unchanged.
func (s *Server) renderCodeFile(rel string, info fs.FileInfo, src []byte, highlight bool) (template.HTML, error) {
	k := renderKey{path: rel, mod: info.ModTime().UnixNano(), size: info.Size(), code: true}
	if r, ok := s.renders.get(k); ok {
		return r.html, nil
	}
	html, err := renderCode(path.Base(rel), src, highlight)
	if err != nil {
		return "", err
	}
	s.renders.put(&rendered{key: k, html: html})
	return html, nil
}
