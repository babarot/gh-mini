package markdown

import (
	"bytes"
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A component with children is an open tag and a close tag of the same
// name, as MDX writes one, around Markdown:
//
//	<Callout type="warn">
//
//	Some **Markdown**.
//
//	</Callout>
//
// CommonMark reads the two tags as HTML blocks of their own, and what is
// between as Markdown, so they are siblings in the tree; inline, as in
// <Kbd>Ctrl</Kbd>, they are siblings among a paragraph's inlines. The
// children stay the page's own, rendered from Markdown, in a <mini-element>
// a plugin wraps; with no plugin, they show as if the tags were not there,
// as on GitHub, which drops the tags.

var (
	openTag  = regexp.MustCompile(`^<([A-Z][A-Za-z0-9-]*)((?:\s+[^\s"'<>/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'=<>` + "`" + `]+))?)*)\s*>$`)
	closeTag = regexp.MustCompile(`^</([A-Z][A-Za-z0-9-]*)\s*>$`)
)

// tagKind is what a line or a raw inline of HTML is to a component.
type tagKind int

const (
	notTag tagKind = iota
	openKind
	closeKind
	selfClosingKind
)

// componentTagOf tells a component's tag alone in b: its kind, name and,
// for an open or self-closing one, attributes.
func componentTagOf(b []byte) (kind tagKind, name string, attrs map[string]any) {
	b = bytes.TrimSpace(b)
	if m := closeTag.FindSubmatch(b); m != nil && IsComponent(string(m[1])) {
		return closeKind, string(m[1]), nil
	}
	if m := openTag.FindSubmatch(b); m != nil && IsComponent(string(m[1])) {
		return openKind, string(m[1]), parseAttrs(m[2])
	}
	if m := componentTag.FindSubmatch(b); m != nil && IsComponent(string(m[1])) {
		return selfClosingKind, string(m[1]), parseAttrs(m[2])
	}
	return notTag, "", nil
}

// parseAttrs are a tag's attributes by name, a value-less one true.
func parseAttrs(b []byte) map[string]any {
	attrs := map[string]any{}
	for _, a := range componentAttr.FindAllSubmatch(b, -1) {
		switch {
		case a[2] != nil:
			attrs[string(a[1])] = html.UnescapeString(string(a[2]))
		case a[3] != nil:
			attrs[string(a[1])] = html.UnescapeString(string(a[3]))
		case a[4] != nil:
			attrs[string(a[1])] = html.UnescapeString(string(a[4]))
		default:
			attrs[string(a[1])] = true
		}
	}
	return attrs
}

var kindComponent = ast.NewNodeKind("Component")

// component is a component with children, on lines of its own (Block) or
// among inlines.
type component struct {
	Tag   string
	Attrs map[string]any
	Block bool
}

type componentBlock struct {
	ast.BaseBlock
	component
}

func (n *componentBlock) Kind() ast.NodeKind { return kindComponent }

func (n *componentBlock) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Tag": n.Tag}, nil)
}

type componentInline struct {
	ast.BaseInline
	component
}

func (n *componentInline) Kind() ast.NodeKind { return kindComponent }

func (n *componentInline) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Tag": n.Tag}, nil)
}

// componentTransformer pairs the open and close tags of components, and
// puts what is between them in a component.
type componentTransformer struct{}

func (componentTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var blocks []*ast.HTMLBlock
	var paragraphs []*ast.Paragraph
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.HTMLBlock:
			blocks = append(blocks, n)
		case *ast.Paragraph:
			paragraphs = append(paragraphs, n)
		}
		return ast.WalkContinue, nil
	})
	for _, b := range blocks {
		splitHTMLBlock(b, src)
	}
	for _, p := range paragraphs {
		liftClose(p, src)
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			pair(n, src)
		}
		return ast.WalkContinue, nil
	})
}

// splitHTMLBlock splits a block at its lines that are a component's tag
// alone, so that each is a block of its own: an HTML block goes on up to a
// blank line, and takes in the lines after a tag, other tags among them.
// Blocks that are text up to their end, <pre> or a comment, are not split,
// nor are such in a block: a line in them is text.
func splitHTMLBlock(n *ast.HTMLBlock, src []byte) {
	if n.HTMLBlockType != ast.HTMLBlockType6 && n.HTMLBlockType != ast.HTMLBlockType7 {
		return
	}
	lines := n.Lines()
	if lines.Len() < 2 {
		return
	}
	var parts []*text.Segments
	var run *text.Segments
	split := false
	// end closes the comment or element of text the lines are in, if any
	var end *regexp.Regexp
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		value := line.Value(src)
		switch k, _, _ := componentTagOf(value); {
		case end != nil:
			// What closes it may open another
			if e := end.FindIndex(value); e != nil {
				end = textEnd(value[e[1]:])
			}
		case k != notTag:
			split = true
			one := text.NewSegments()
			one.Append(line)
			parts = append(parts, run, one)
			run = nil
			continue
		default:
			end = textEnd(value)
		}
		if run == nil {
			run = text.NewSegments()
		}
		run.Append(line)
	}
	if !split {
		return
	}
	parts = append(parts, run)
	parent := n.Parent()
	for _, lines := range parts {
		if lines == nil {
			continue
		}
		b := ast.NewHTMLBlock(n.HTMLBlockType)
		b.SetLines(lines)
		parent.InsertBefore(parent, n, b)
	}
	parent.RemoveChild(parent, n)
}

var (
	textStart = regexp.MustCompile(`(?i)<(!--|pre\b|script\b|style\b|textarea\b)`)
	textEnds  = map[string]*regexp.Regexp{
		"!--":      regexp.MustCompile(`-->`),
		"pre":      regexp.MustCompile(`(?i)</pre\s*>`),
		"script":   regexp.MustCompile(`(?i)</script\s*>`),
		"style":    regexp.MustCompile(`(?i)</style\s*>`),
		"textarea": regexp.MustCompile(`(?i)</textarea\s*>`),
	}
)

// textEnd is what ends a comment or an element of text, <pre> and the
// like, that a line opens and does not close, nil when it opens none.
func textEnd(line []byte) *regexp.Regexp {
	for {
		loc := textStart.FindSubmatchIndex(line)
		if loc == nil {
			return nil
		}
		end := textEnds[strings.ToLower(string(line[loc[2]:loc[3]]))]
		line = line[loc[1]:]
		e := end.FindIndex(line)
		if e == nil {
			return end
		}
		line = line[e[1]:]
	}
}

// liftClose takes a component's close tag at the end of a paragraph, on a
// line of its own, out of it, as a block after it. A close tag cannot end
// a paragraph as an open one can begin a block, so MDX's
//
//	<Callout>
//
//	Text
//	</Callout>
//
// has it as the paragraph's last inline. One closing a tag opened in the
// paragraph is left there.
func liftClose(p *ast.Paragraph, src []byte) {
	last, ok := p.LastChild().(*ast.RawHTML)
	if !ok {
		return
	}
	k, name, _ := componentTagOf(rawValue(last, src))
	if k != closeKind {
		return
	}
	prev := last.PreviousSibling()
	if prev != nil {
		t, ok := prev.(*ast.Text)
		if !ok || !t.SoftLineBreak() {
			return
		}
	}
	depth := 0
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		if r, ok := c.(*ast.RawHTML); ok {
			switch k, n, _ := componentTagOf(rawValue(r, src)); {
			case k == openKind && n == name:
				depth++
			case k == closeKind && n == name:
				depth--
			}
		}
	}
	if depth >= 0 {
		return
	}
	b := ast.NewHTMLBlock(ast.HTMLBlockType7)
	b.SetLines(last.Segments)
	p.RemoveChild(p, last)
	if t, ok := prev.(*ast.Text); ok {
		t.SetSoftLineBreak(false)
	}
	parent := p.Parent()
	parent.InsertAfter(parent, p, b)
	if !p.HasChildren() {
		parent.RemoveChild(parent, p)
	}
}

func rawValue(n *ast.RawHTML, src []byte) []byte {
	var b bytes.Buffer
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		b.Write(segment.Value(src))
	}
	return b.Bytes()
}

// tagOf tells the component's tag that a node is alone, a block or an
// inline.
func tagOf(n ast.Node, src []byte) (tagKind, string, map[string]any) {
	switch n := n.(type) {
	case *ast.HTMLBlock:
		if n.HTMLBlockType != ast.HTMLBlockType6 && n.HTMLBlockType != ast.HTMLBlockType7 || n.Lines().Len() != 1 {
			return notTag, "", nil
		}
		line := n.Lines().At(0)
		return componentTagOf(line.Value(src))
	case *ast.RawHTML:
		return componentTagOf(rawValue(n, src))
	}
	return notTag, "", nil
}

// pair puts the children of n between an open tag and the close tag of
// its name in a component. A close tag pairs with the nearest open one of
// its name, leaving those opened after it unpaired, as an HTML parser
// does; one with none, and an open one never closed, are left, and
// dropped as any other tag.
func pair(n ast.Node, src []byte) {
	type open struct {
		node  ast.Node
		name  string
		attrs map[string]any
	}
	var stack []open
	for c := n.FirstChild(); c != nil; {
		next := c.NextSibling()
		k, name, attrs := tagOf(c, src)
		switch k {
		case openKind:
			stack = append(stack, open{c, name, attrs})
		case closeKind:
			i := len(stack) - 1
			for i >= 0 && stack[i].name != name {
				i--
			}
			if i < 0 {
				break
			}
			o := stack[i]
			stack = stack[:i]
			info := component{Tag: o.name, Attrs: o.attrs, Block: c.Type() == ast.TypeBlock}
			var comp ast.Node
			if info.Block {
				comp = &componentBlock{component: info}
			} else {
				comp = &componentInline{component: info}
			}
			for m := o.node.NextSibling(); m != c; {
				after := m.NextSibling()
				comp.AppendChild(comp, m)
				m = after
			}
			n.ReplaceChild(n, o.node, comp)
			n.RemoveChild(n, c)
		}
		c = next
	}
}

// componentRenderer writes a component as a <mini-element> holding its
// children, which tells its tag and attributes, and its own child
// components, those it is the nearest component around, for a plugin
// that needs them, as one of tabs does its tabs' labels.
type componentRenderer struct{}

func (componentRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindComponent, func(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		var c component
		switch n := node.(type) {
		case *componentBlock:
			c = n.component
		case *componentInline:
			c = n.component
		}
		if !entering {
			_, _ = w.WriteString("</mini-element>")
			if c.Block {
				_, _ = w.WriteString("\n")
			}
			return ast.WalkContinue, nil
		}
		attrs, err := json.Marshal(c.Attrs)
		if err != nil {
			return ast.WalkStop, err
		}
		children, err := json.Marshal(childComponents(node, src))
		if err != nil {
			return ast.WalkStop, err
		}
		_, _ = w.WriteString(`<mini-element data-tag="` + html.EscapeString(c.Tag) + `" data-attrs="` + html.EscapeString(string(attrs)) + `" data-children="` + html.EscapeString(string(children)) + `"`)
		if c.Block {
			_, _ = w.WriteString(" data-block>\n")
		} else {
			_, _ = w.WriteString(">")
		}
		return ast.WalkContinue, nil
	})
}

// childTag is a child component as a plugin is told it.
type childTag struct {
	Tag   string         `json:"tag"`
	Attrs map[string]any `json:"attrs"`
}

// childComponents are the components in n, in order, but not those in
// them.
func childComponents(n ast.Node, src []byte) []childTag {
	out := []childTag{}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		_ = ast.Walk(c, func(m ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch m := m.(type) {
			case *componentBlock:
				out = append(out, childTag{m.Tag, m.Attrs})
				return ast.WalkSkipChildren, nil
			case *componentInline:
				out = append(out, childTag{m.Tag, m.Attrs})
				return ast.WalkSkipChildren, nil
			case *ast.HTMLBlock:
				if m.Lines().Len() > 0 {
					line := m.Lines().At(0)
					if k, name, attrs := componentTagOf(line.Value(src)); k == selfClosingKind {
						out = append(out, childTag{name, attrs})
					}
				}
			case *ast.RawHTML:
				if k, name, attrs := componentTagOf(rawValue(m, src)); k == selfClosingKind {
					out = append(out, childTag{name, attrs})
				}
			}
			return ast.WalkContinue, nil
		})
	}
	return out
}
