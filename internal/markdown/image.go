package markdown

import (
	"bytes"

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
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link, *ast.AutoLink:
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			// data: images are allowed as images but not as links
			if !bytes.HasPrefix(bytes.ToLower(n.Destination), []byte("data:")) {
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
