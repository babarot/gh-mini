// Package markdown renders Markdown as GitHub does: GitHub Flavored
// Markdown with GitHub's additions, such as alerts, math, footnotes and
// links to issues, in the markup github-markdown-css styles.
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Features tells which scripts a rendered file needs.
type Features struct {
	Mermaid bool
	Math    bool
	// Headings are the headings for the table of contents, in order.
	Headings []Heading
}

// Renderer renders Markdown. It is safe for concurrent use.
type Renderer struct {
	md goldmark.Markdown
}

// New returns a Renderer that links #123 to the issues of repo, given as
// "owner/name". With no repo, only owner/name#123 is linked.
func New(repo string) *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Linkify,
			extension.Table,
			extension.Strikethrough,
			emoji.Emoji,
		),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(
				util.Prioritized(mathBlockParser{}, 701),
				util.Prioritized(extension.NewFootnoteBlockParser(), 999),
			),
			parser.WithInlineParsers(
				// Before the link parser, which also starts at [
				util.Prioritized(extension.NewTaskCheckBoxParser(), 0),
				util.Prioritized(extension.NewFootnoteParser(), 101),
				util.Prioritized(inlineMathParser{}, 501),
			),
			parser.WithASTTransformers(
				util.Prioritized(headingIDTransformer{}, 100),
				util.Prioritized(imageLinkTransformer{}, 100),
				util.Prioritized(alertTransformer{}, 100),
				util.Prioritized(taskListTransformer{}, 100),
				util.Prioritized(mathCodeTransformer{}, 100),
				util.Prioritized(issueRefTransformer{repo: repo}, 100),
				util.Prioritized(extension.NewFootnoteASTTransformer(), 999),
			),
		),
		goldmark.WithRendererOptions(
			// Not html.WithUnsafe: HTML in a file goes through
			// rawHTMLRenderer, and links to javascript: and the like are
			// dropped
			renderer.WithNodeRenderers(
				util.Prioritized(rawHTMLRenderer{}, 100),
				util.Prioritized(codeRenderer{}, 100),
				util.Prioritized(alertRenderer{}, 100),
				util.Prioritized(taskCheckBoxRenderer{}, 100),
				util.Prioritized(mathRenderer{}, 100),
				util.Prioritized(footnoteRenderer{}, 100),
			),
		),
	)
	return &Renderer{md: md}
}

// Render renders src, showing its YAML front matter as a table.
func (r *Renderer) Render(src []byte) ([]byte, Features, error) {
	// A byte order mark would keep the first line from being a heading or
	// the start of front matter
	src = bytes.TrimPrefix(src, utf8BOM)
	var buf bytes.Buffer
	if fm, body, ok := splitFrontMatter(src); ok {
		renderFrontMatter(&buf, fm)
		src = body
	}
	ctx := parser.NewContext(parser.WithIDs(newIDs()))
	doc := r.md.Parser().Parse(text.NewReader(src), parser.WithContext(ctx))
	var f Features
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n.Kind() {
			case ast.KindFencedCodeBlock:
				f.Mermaid = f.Mermaid || language(n, src) == "mermaid"
			case kindMathBlock, kindInlineMath:
				f.Math = true
			case ast.KindHeading:
				if h, ok := headingOf(n, src); ok {
					f.Headings = append(f.Headings, h)
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if err := r.md.Renderer().Render(&buf, src, doc); err != nil {
		return nil, f, err
	}
	return buf.Bytes(), f, nil
}

var utf8BOM = []byte("\xef\xbb\xbf")
