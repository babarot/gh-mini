package markdown

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Math is written as GitHub takes it: $...$ or $`...`$ inline, and $$ or
// a math code block on lines of its own. It is rendered as text between
// MathJax's delimiters, for MathJax to typeset in the browser.

var (
	kindInlineMath = ast.NewNodeKind("InlineMath")
	kindMathBlock  = ast.NewNodeKind("MathBlock")
)

type inlineMath struct {
	ast.BaseInline
	Value text.Segment
}

func (n *inlineMath) Kind() ast.NodeKind { return kindInlineMath }

func (n *inlineMath) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Value": string(n.Value.Value(src))}, nil)
}

type mathBlock struct {
	ast.BaseBlock
	// oneLine is set for $$...$$, which ends where it starts.
	oneLine bool
}

func (n *mathBlock) Kind() ast.NodeKind { return kindMathBlock }
func (n *mathBlock) IsRaw() bool        { return true }

func (n *mathBlock) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

// inlineMathParser parses $...$ and $`...`$ within a line. As in Pandoc,
// $ opens math only before a non-space and closes it only after one and
// not before a digit; math ends at the next $, so that prices such as
// $5 and $6 stay text.
type inlineMathParser struct{}

func (inlineMathParser) Trigger() []byte { return []byte{'$'} }

func (inlineMathParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, seg := block.PeekLine()
	// $$x$$ in a line is math too; without its closing $$ it is text,
	// and the second $ must not start math either
	if block.PrecendingCharacter() == '$' {
		return nil
	}
	if len(line) > 2 && line[1] == '$' {
		end := bytes.Index(line[2:], []byte("$$"))
		if end <= 0 {
			return nil
		}
		block.Advance(2 + end + 2)
		return &inlineMath{Value: text.NewSegment(seg.Start+2, seg.Start+2+end)}
	}
	if len(line) > 1 && line[1] == '`' {
		end := bytes.Index(line[2:], []byte("`$"))
		if end < 0 {
			return nil
		}
		block.Advance(2 + end + 2)
		return &inlineMath{Value: text.NewSegment(seg.Start+2, seg.Start+2+end)}
	}
	if len(line) < 3 || isSpace(line[1]) || line[1] == '$' {
		return nil
	}
	for i := 2; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '$':
			if isSpace(line[i-1]) || (i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9') {
				return nil
			}
			block.Advance(i + 1)
			return &inlineMath{Value: text.NewSegment(seg.Start+1, seg.Start+i)}
		}
	}
	return nil
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// mathBlockParser parses math between lines of $$, or $$...$$ on one line.
type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || !bytes.HasPrefix(line[pos:], []byte("$$")) {
		return nil, parser.NoChildren
	}
	rest := bytes.TrimRight(line[pos+2:], " \t\r\n")
	n := &mathBlock{}
	if len(rest) == 0 {
		return n, parser.NoChildren
	}
	if len(rest) > 2 && bytes.HasSuffix(rest, []byte("$$")) {
		start := seg.Start + pos + 2
		n.Lines().Append(text.NewSegment(start, start+len(rest)-2))
		n.oneLine = true
		advanceLine(reader, line)
		return n, parser.NoChildren
	}
	return nil, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	if node.(*mathBlock).oneLine {
		return parser.Close
	}
	line, seg := reader.PeekLine()
	if string(bytes.TrimSpace(line)) == "$$" {
		advanceLine(reader, line)
		return parser.Close
	}
	seg.ForceNewline = true
	node.Lines().Append(seg)
	reader.Advance(len(line) - 1)
	return parser.Continue | parser.NoChildren
}

// advanceLine moves past a line but its newline, as goldmark's block
// parsers do when they close.
func advanceLine(reader text.Reader, line []byte) {
	n := len(line)
	if n > 0 && line[n-1] == '\n' {
		n--
	}
	reader.Advance(n)
}

func (mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}
func (mathBlockParser) CanInterruptParagraph() bool                                { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool                                { return false }

// mathCodeTransformer turns code blocks in the math language into math.
type mathCodeTransformer struct{}

func (mathCodeTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	var blocks []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if c, ok := n.(*ast.FencedCodeBlock); ok && entering && string(c.Language(reader.Source())) == "math" {
			blocks = append(blocks, c)
		}
		return ast.WalkContinue, nil
	})
	for _, c := range blocks {
		m := &mathBlock{}
		m.SetLines(c.Lines())
		c.Parent().ReplaceChild(c.Parent(), c, m)
	}
}

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindInlineMath, func(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<span class="math-inline">\(`)
			_, _ = w.Write(util.EscapeHTML(node.(*inlineMath).Value.Value(src)))
			_, _ = w.WriteString(`\)</span>`)
		}
		return ast.WalkContinue, nil
	})
	reg.Register(kindMathBlock, func(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<div class="math-display">\[`)
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				_, _ = w.Write(util.EscapeHTML(seg.Value(src)))
			}
			_, _ = w.WriteString("\\]</div>\n")
		}
		return ast.WalkContinue, nil
	})
}
