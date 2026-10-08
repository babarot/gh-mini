package server

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	_ "github.com/babarot/gh-mini/internal/lexers"
	"github.com/babarot/gh-mini/internal/markdown"
)

func (s *Server) renderMarkdown(src []byte) (template.HTML, markdown.Features, error) {
	b, f, err := s.md.Render(src)
	return template.HTML(b), f, err
}

// renderCode highlights a source file with line numbers that link to #L<n>,
// as GitHub's blob view does. Without highlight, the file keeps its line
// numbers but is shown as plain text.
func renderCode(name string, src []byte, highlight bool) (template.HTML, error) {
	var lexer chroma.Lexer
	if highlight {
		lexer = lexers.Match(name)
		if lexer == nil {
			lexer = lexers.Analyse(string(src))
		}
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
		// Each line with its number, rather than the numbers in a column
		// of their own, so that a line wrapped keeps its number beside it
		chromahtml.WithLineNumbers(true),
		chromahtml.WithLinkableLineNumbers(true, "L"),
	)
	var buf bytes.Buffer
	if err := f.Format(&buf, styles.Fallback, it); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// highlightLines highlights a file line by line, for a diff to show the
// lines it has: the file is read whole, so that a line in a comment or a
// string that began above it is colored as one. It is nil when nothing
// knows the language.
func highlightLines(name string, src []byte) []template.HTML {
	lexer := lexers.Match(name)
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		return nil
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, string(src))
	if err != nil {
		return nil
	}
	var lines []template.HTML
	var b strings.Builder
	for t := it(); t != chroma.EOF; t = it() {
		cls := tokenClass(t.Type)
		for i, part := range strings.Split(t.Value, "\n") {
			if i > 0 {
				lines = append(lines, template.HTML(b.String()))
				b.Reset()
			}
			if part == "" {
				continue
			}
			if cls != "" {
				b.WriteString(`<span class="` + cls + `">`)
			}
			b.WriteString(template.HTMLEscapeString(part))
			if cls != "" {
				b.WriteString("</span>")
			}
		}
	}
	if b.Len() > 0 {
		lines = append(lines, template.HTML(b.String()))
	}
	return lines
}

// tokenClass is the class chroma's HTML gives a token, which chroma.css
// colors.
func tokenClass(t chroma.TokenType) string {
	for ; t != 0; t = t.Parent() {
		if cls, ok := chroma.StandardTypes[t]; ok {
			return cls
		}
	}
	return chroma.StandardTypes[t]
}

// chromaCSS is the highlighting stylesheet for both modes, each nested under
// the mode it applies to. Its colors follow the theme: the block and plain
// text take the page's colors, and the kinds of token a theme recolors read
// a --syntax-* variable first, which nothing defines unless a theme does.
func chromaCSS() string {
	var b strings.Builder
	write := func(selector, style string) {
		var css bytes.Buffer
		_ = chromahtml.New(chromahtml.WithClasses(true)).WriteCSS(&css, styles.Get(style))
		fmt.Fprintf(&b, "%s {\n%s}\n", selector, themeColors(css.String()))
	}
	write(`:root:not([data-mode="dark"])`, "github")
	write(`:root[data-mode="dark"]`, "github-dark")
	return b.String()
}

var (
	// cssRule is one line of chroma's stylesheet: /* TokenType */ selector { ... }
	cssRule = regexp.MustCompile(`(?m)^/\* (\w+) \*/ (.*)$`)
	// cssColor is a color declaration, not a background-color one.
	cssColor = regexp.MustCompile(`([{;]\s*)color: (#[0-9a-fA-F]+)`)
	// cssColors is a color or background-color declaration.
	cssColors = regexp.MustCompile(`\s*(?:background-)?color: #[0-9a-fA-F]+;?`)
	// cssBackground is a background-color declaration.
	cssBackground = regexp.MustCompile(`background-color: (#[0-9a-fA-F]+)`)
)

// themeColors makes the colors in chroma's stylesheet follow the theme.
func themeColors(css string) string {
	return cssRule.ReplaceAllStringFunc(css, func(line string) string {
		t, err := chroma.TokenTypeString(cssRule.FindStringSubmatch(line)[1])
		if err != nil {
			return line
		}
		switch {
		case t == chroma.Background, t == chroma.PreWrapper:
			// The block is drawn by markdown.css and app.css
			return cssColors.ReplaceAllString(line, "")
		case isPlainText(t):
			return cssColor.ReplaceAllString(line, "${1}color: var(--fgColor-default)")
		case t == chroma.GenericPrompt, t == chroma.GenericOutput:
			// A shell session's prompt and output, apart from its commands
			return cssColor.ReplaceAllString(line, "${1}color: var(--fgColor-muted)")
		}
		if name := syntaxVar(t); name != "" {
			line = cssColor.ReplaceAllString(line, "${1}color: var(--syntax-"+name+", $2)")
			// Inserted and deleted lines are marked by their background too
			return cssBackground.ReplaceAllString(line, "background-color: var(--syntax-"+name+"-bg, $1)")
		}
		return line
	})
}

// isPlainText reports whether a kind of token is drawn in the text color.
func isPlainText(t chroma.TokenType) bool {
	switch t {
	case chroma.Name, chroma.NameOther, chroma.Punctuation, chroma.Text,
		chroma.Generic, chroma.GenericEmph, chroma.GenericStrong:
		return true
	}
	return false
}

// syntaxVar names the --syntax-* variable for a kind of token, or "" for
// kinds a theme does not recolor.
func syntaxVar(t chroma.TokenType) string {
	switch {
	case t == chroma.KeywordType, t == chroma.NameClass, t == chroma.NameNamespace:
		return "type"
	case t == chroma.KeywordConstant, t.InSubCategory(chroma.NameBuiltin),
		t == chroma.NameConstant, t == chroma.NameDecorator:
		return "constant"
	case t.InCategory(chroma.Keyword):
		return "keyword"
	case t.InSubCategory(chroma.NameFunction):
		return "function"
	case t == chroma.NameTag, t == chroma.NameAttribute:
		return "tag"
	case t.InSubCategory(chroma.NameVariable):
		return "variable"
	case t.InSubCategory(chroma.LiteralString):
		return "string"
	case t.InSubCategory(chroma.LiteralNumber):
		return "number"
	case t.InCategory(chroma.Operator):
		return "operator"
	case t.InCategory(chroma.Comment):
		return "comment"
	case t == chroma.GenericInserted:
		return "inserted"
	case t == chroma.GenericDeleted:
		return "deleted"
	}
	return ""
}
