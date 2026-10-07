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
// <div class="highlight"> GitHub wraps them in.
type codeRenderer struct{}

func (r codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
	reg.Register(ast.KindCodeBlock, r.render)
}

func (codeRenderer) render(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var lexer chroma.Lexer
	if n, ok := node.(*ast.FencedCodeBlock); ok {
		if lang := n.Language(src); lang != nil {
			lexer = lexers.Get(string(lang))
		}
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	var code strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(src))
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
