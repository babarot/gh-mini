package markdown

import (
	"html"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	_ "github.com/babarot/gh-mini/internal/lexers"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// codeRenderer highlights code blocks with Chroma's classes, in the
// <div class="highlight"> GitHub wraps them in, which tells the language
// and the rest of the info string, for a plugin that shows the language.
// A Mermaid block is left as its source in <pre class="mermaid">, for
// Mermaid to draw.
type codeRenderer struct{}

func (r codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
	reg.Register(ast.KindCodeBlock, r.render)
}

func (codeRenderer) render(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var code strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(src))
	}
	lang, meta := info(node, src)
	if lang == "mermaid" {
		_, _ = w.WriteString(`<pre class="mermaid">`)
		_, _ = w.Write(util.EscapeHTML([]byte(code.String())))
		_, _ = w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code.String())
	if err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString(`<div class="highlight"` + infoAttrs(lang, meta) + `>`)
	if err := chromahtml.New(chromahtml.WithClasses(true)).Format(w, styles.Fallback, it); err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString("</div>\n")
	return ast.WalkSkipChildren, nil
}

// language is the language a code block is marked with, or "".
func language(node ast.Node, src []byte) string {
	lang, _ := info(node, src)
	return lang
}

// info is the language a code block is marked with, and the rest of its
// info string, split at any space: goldmark's Language splits at a space
// alone, taking "csv\tsep=;" as the language.
func info(node ast.Node, src []byte) (lang, meta string) {
	n, ok := node.(*ast.FencedCodeBlock)
	if !ok || n.Info == nil {
		return "", ""
	}
	s := strings.TrimSpace(string(n.Info.Segment.Value(src)))
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

// infoAttrs are the attributes that tell a code block's language and
// meta, escaped, as both are the file's text.
func infoAttrs(lang, meta string) string {
	var b strings.Builder
	if lang != "" {
		b.WriteString(` data-lang="` + html.EscapeString(lang) + `"`)
	}
	if meta != "" {
		b.WriteString(` data-meta="` + html.EscapeString(meta) + `"`)
	}
	return b.String()
}
