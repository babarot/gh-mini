package markdown

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// codeRenderer highlights code blocks with Chroma's classes, in the
// <div class="highlight"> GitHub wraps them in. A Mermaid block is left
// as its source in <pre class="mermaid">, for Mermaid to draw.
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
	lang := language(node, src)
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
	_, _ = w.WriteString(`<div class="highlight">`)
	if err := chromahtml.New(chromahtml.WithClasses(true)).Format(w, styles.Fallback, it); err != nil {
		return ast.WalkStop, err
	}
	_, _ = w.WriteString("</div>\n")
	return ast.WalkSkipChildren, nil
}

// language is the language a code block is marked with, or "".
func language(node ast.Node, src []byte) string {
	if n, ok := node.(*ast.FencedCodeBlock); ok {
		return string(n.Language(src))
	}
	return ""
}
