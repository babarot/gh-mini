package markdown

import (
	"bytes"
	"regexp"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// imageLinkTransformer wraps an image that is not in a link in a link to
// itself, opening in a new tab, as GitHub does: a screenshot shrunk to
// the page's width can then be seen in full.
type imageLinkTransformer struct{}

func (imageLinkTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	var images []*ast.Image
	src := reader.Source()
	// links counts the <a> a file's HTML opened and has not closed yet, as
	// around a badge: an image in one is a link already
	links := 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link, *ast.AutoLink:
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			links = countLinks(links, src, n.Segments.Sliced(0, n.Segments.Len())...)
		case *ast.HTMLBlock:
			links = countLinks(links, src, n.Lines().Sliced(0, n.Lines().Len())...)
			if n.HasClosure() {
				links = countLinks(links, src, n.ClosureLine)
			}
		case *ast.Image:
			// data: images are allowed as images but not as links
			if links == 0 && !bytes.HasPrefix(bytes.ToLower(n.Destination), []byte("data:")) {
				images = append(images, n)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	for _, img := range images {
		link := ast.NewLink()
		link.Destination = img.Destination
		link.SetAttributeString("target", []byte("_blank"))
		link.SetAttributeString("rel", []byte("noopener noreferrer"))
		parent := img.Parent()
		parent.ReplaceChild(parent, img, link)
		link.AppendChild(link, img)
	}
}

// linkTag is an <a> opened or closed in HTML.
var linkTag = regexp.MustCompile(`(?i)<(/?)a(?:\s[^>]*)?>`)

// countLinks adds to open the <a> that the HTML in segs opens, less those
// it closes.
func countLinks(open int, src []byte, segs ...text.Segment) int {
	for _, seg := range segs {
		for _, m := range linkTag.FindAllSubmatch(seg.Value(src), -1) {
			if len(m[1]) == 0 {
				open++
			} else if open > 0 {
				open--
			}
		}
	}
	return open
}
