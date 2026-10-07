package markdown

import (
	"strings"

	emojiast "github.com/yuin/goldmark-emoji/ast"
	"github.com/yuin/goldmark/ast"
)

// Heading is a heading of a rendered file, for its table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// maxTocLevel is the deepest heading in the table of contents.
const maxTocLevel = 4

// headingOf returns the heading n is, if it goes in the table of contents.
func headingOf(n ast.Node, src []byte) (Heading, bool) {
	h, ok := n.(*ast.Heading)
	if !ok || h.Level > maxTocLevel {
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
// markup or the HTML written in it.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(src))
			if c.SoftLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		case *emojiast.Emoji:
			if c.Value != nil && len(c.Value.Unicode) > 0 {
				b.WriteString(string(c.Value.Unicode))
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}
