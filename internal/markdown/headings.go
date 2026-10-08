package markdown

import (
	"html"
	"strings"

	emojiast "github.com/yuin/goldmark-emoji/ast"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Heading is a heading of a rendered file, for its table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// headingOf returns the heading n is, if it goes in the table of contents.
func headingOf(n ast.Node, src []byte) (Heading, bool) {
	h, ok := n.(*ast.Heading)
	if !ok {
		return Heading{}, false
	}
	id, ok := h.AttributeString("id")
	if !ok {
		return Heading{}, false
	}
	idb, ok := id.([]byte)
	if !ok {
		return Heading{}, false
	}
	return Heading{Level: h.Level, ID: string(idb), Text: plainText(h, src)}, true
}

// plainText is the text a node shows: its words, code and emoji, without
// markup, images or the HTML written in it.
func plainText(n ast.Node, src []byte) string {
	return strings.TrimSpace(nodeText(n, src, false))
}

// nodeText is the text of a node, with entities as the characters they
// stand for. With shortcodes, an emoji is its name, as in :tada:.
func nodeText(n ast.Node, src []byte, shortcodes bool) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.WriteString(html.UnescapeString(string(c.Segment.Value(src))))
			if c.SoftLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.WriteString(html.UnescapeString(string(c.Value)))
		case *ast.AutoLink:
			// Its text is its label, not a child
			b.Write(c.Label(src))
			return ast.WalkSkipChildren, nil
		case *emojiast.Emoji:
			if shortcodes {
				b.WriteString(":" + string(c.ShortName) + ":")
			} else if c.Value != nil && len(c.Value.Unicode) > 0 {
				b.WriteString(string(c.Value.Unicode))
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML, *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// headingIDTransformer gives headings ids made from the text they show, as
// GitHub does: "## [1.2.0](url) - date" is #120---date, not one with the
// URL in it, and "A &amp; B" is #a--b. Goldmark makes them from the source.
type headingIDTransformer struct{}

func (headingIDTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			h.SetAttributeString("id", pc.IDs().Generate([]byte(nodeText(h, src, true)), ast.KindHeading))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
}
