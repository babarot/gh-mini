package server

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/chrishrb/go-grip/pkg/alert"
	"github.com/chrishrb/go-grip/pkg/details"
	"github.com/chrishrb/go-grip/pkg/footnote"
	"github.com/chrishrb/go-grip/pkg/frontmatter"
	"github.com/chrishrb/go-grip/pkg/ghissue"
	"github.com/chrishrb/go-grip/pkg/highlighting"
	"github.com/chrishrb/go-grip/pkg/mathjax"
	"github.com/chrishrb/go-grip/pkg/slug"
	"github.com/chrishrb/go-grip/pkg/tasklist"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/hashtag"
	"go.abhg.dev/goldmark/mermaid"
)

// newMarkdown builds the same goldmark pipeline as go-grip, whose extensions
// do the work of rendering like GitHub.
func newMarkdown(repo string) goldmark.Markdown {
	var issueOpts []ghissue.Option
	if repo != "" {
		issueOpts = append(issueOpts, ghissue.WithRepository(repo))
	}
	return goldmark.New(
		goldmark.WithExtensions(
			extension.Linkify,
			extension.Table,
			extension.Strikethrough,
			footnote.Footnote,
			tasklist.TaskList,
			emoji.Emoji,
			&hashtag.Extender{},
			alert.New(),
			highlighting.Highlighting,
			&mermaid.Extender{RenderMode: mermaid.RenderModeClient, NoScript: true},
			mathjax.MathJax,
			ghissue.New(issueOpts...),
			details.New(),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}

// features tells which scripts a rendered Markdown file needs.
type features struct {
	Mermaid bool
	Math    bool
}

func (s *Server) renderMarkdown(src []byte) (template.HTML, features, error) {
	var prefix []byte
	if fm, body, ok := frontmatter.Extract(src); ok {
		if table, err := frontmatter.RenderTable(fm); err == nil {
			prefix = table
			src = body
		}
	}
	// Heading ids like GitHub's, so that anchors written for GitHub work
	ctx := parser.NewContext(parser.WithIDs(slug.NewIDs()))
	doc := s.md.Parser().Parse(text.NewReader(src), parser.WithContext(ctx))
	var f features
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n.Kind() {
			case mermaid.Kind:
				f.Mermaid = true
			case mathjax.KindMathBlock, mathjax.KindInlineMath:
				f.Math = true
			}
		}
		return ast.WalkContinue, nil
	})
	var buf bytes.Buffer
	buf.Write(prefix)
	if err := s.md.Renderer().Render(&buf, src, doc); err != nil {
		return "", f, err
	}
	return template.HTML(buf.String()), f, nil
}

// renderCode highlights a source file with line numbers that link to #L<n>,
// as GitHub's blob view does.
func renderCode(name string, src []byte) (template.HTML, error) {
	lexer := lexers.Match(name)
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, string(src))
	if err != nil {
		return "", err
	}
	f := chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.WithLineNumbers(true),
		chromahtml.LineNumbersInTable(true),
		chromahtml.WithLinkableLineNumbers(true, "L"),
	)
	var buf bytes.Buffer
	if err := f.Format(&buf, styles.Fallback, it); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// chromaCSS is the highlighting stylesheet for both modes, each nested under
// the mode it applies to.
func chromaCSS() string {
	var b strings.Builder
	write := func(selector, style string) {
		var css bytes.Buffer
		_ = chromahtml.New(chromahtml.WithClasses(true)).WriteCSS(&css, styles.Get(style))
		fmt.Fprintf(&b, "%s {\n%s}\n", selector, css.String())
	}
	write(`:root:not([data-mode="dark"])`, "github")
	write(`:root[data-mode="dark"]`, "github-dark")
	return b.String()
}
