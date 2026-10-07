package markdown

import (
	"bytes"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// HTML written in Markdown is kept as far as GitHub keeps it, and the rest
// is dropped: README files rely on <details>, <p align="center"> or
// <img width>, but a page must not run what a file says, as it is served
// with gh-mini's own pages and API.
//
// Only the HTML a file writes is sanitized, node by node, not the whole
// page, so what this package renders itself (alerts, code, math, footnotes)
// needs no allowance here. An element opened in one node and closed in
// another passes, as the policy keeps or drops each tag on its own.

// rawHTMLPolicy is GitHub's allowance, roughly. It is built from scratch
// rather than from bluemonday.UGCPolicy, which allows id on every element,
// and an allowance cannot be taken back: an id would let a file take the
// place of the page's own elements.
var rawHTMLPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowStandardURLs()
	p.RequireNoFollowOnLinks(true)
	p.AllowImages()
	p.AllowTables()
	p.AllowLists()

	p.AllowElements(
		"p", "div", "span", "br", "hr", "blockquote", "pre", "code",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"b", "i", "strong", "em", "u", "s", "strike", "del", "ins", "mark", "small",
		"sub", "sup", "kbd", "samp", "var", "q", "cite", "abbr", "dfn", "tt",
		"details", "summary", "picture", "figure", "figcaption",
		"dl", "dt", "dd", "ruby", "rt", "rp", "time", "wbr", "bdo", "caption",
	)
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("open").OnElements("details")
	p.AllowAttrs("align").OnElements("p", "div", "img", "h1", "h2", "h3", "h4", "h5", "h6", "table", "tr", "td", "th")
	p.AllowAttrs("width", "height").Matching(bluemonday.NumberOrPercent).OnElements("img", "td", "th", "table")
	p.AllowAttrs("colspan", "rowspan").Matching(bluemonday.Integer).OnElements("td", "th")
	p.AllowAttrs("start").Matching(bluemonday.Integer).OnElements("ol")
	// Any text: bluemonday's own checks reject values with ":" or "&",
	// common in alt and title, and the values are escaped anyway
	p.AllowAttrs("alt").Matching(anyText).OnElements("img")
	p.AllowAttrs("title").Matching(anyText).Globally()
	p.AllowAttrs("srcset").Matching(srcset).OnElements("source", "img")
	p.AllowAttrs("media", "type").Matching(anyText).OnElements("source")
	p.AllowElements("source")
	return p
}()

var (
	anyText = regexp.MustCompile(`^[^\x00]*$`)
	// srcset is candidates of a URL and a descriptor, separated by commas;
	// each URL is http(s) or relative
	srcset = regexp.MustCompile(`^\s*(?:(?:https?://|[^:\s,]+(?:\s|,|$))[^\s,]*(?:\s+[0-9.]+[wx])?\s*(?:,\s*|$))+$`)
)

// rawHTMLRenderer writes the HTML in a file through rawHTMLPolicy. It
// replaces goldmark's renderers, which drop such HTML altogether unless
// told to write it as it is.
type rawHTMLRenderer struct{}

func (rawHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, renderRawHTML)
	reg.Register(ast.KindHTMLBlock, renderHTMLBlock)
}

func renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	var b bytes.Buffer
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		b.Write(segment.Value(source))
	}
	_, _ = w.Write(rawHTMLPolicy.SanitizeBytes(b.Bytes()))
	return ast.WalkSkipChildren, nil
}

// renderHTMLBlock sanitizes a block at once, its closing line included: a
// comment over several lines ends on that line, and apart from the rest
// its "-->" would be text.
func renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	var b bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		line := n.Lines().At(i)
		b.Write(line.Value(source))
	}
	if n.HasClosure() {
		b.Write(n.ClosureLine.Value(source))
	}
	_, _ = w.Write(rawHTMLPolicy.SanitizeBytes(b.Bytes()))
	return ast.WalkContinue, nil
}
