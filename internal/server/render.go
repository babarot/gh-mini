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
	"github.com/babarot/gh-mini/internal/markdown"
)

func (s *Server) renderMarkdown(src []byte) (template.HTML, markdown.Features, error) {
	b, f, err := s.md.Render(src)
	return template.HTML(b), f, err
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
