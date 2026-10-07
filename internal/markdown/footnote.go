package markdown

import (
	"strconv"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// footnoteRenderer renders the footnotes goldmark parses in GitHub's
// markup. A note referred to more than once gets a link back to each
// reference, numbered from the second.
type footnoteRenderer struct{}

func (r footnoteRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(east.KindFootnoteLink, r.link)
	reg.Register(east.KindFootnoteBacklink, r.backlink)
	reg.Register(east.KindFootnote, r.note)
	reg.Register(east.KindFootnoteList, r.list)
}

// refID is the id of the reference-th reference to a note, from 0.
func refID(index, ref int) string {
	id := "fnref-" + strconv.Itoa(index)
	if ref > 0 {
		id += "-" + strconv.Itoa(ref+1)
	}
	return id
}

func (footnoteRenderer) link(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*east.FootnoteLink)
		i := strconv.Itoa(n.Index)
		_, _ = w.WriteString(`<sup><a href="#fn-` + i + `" id="` + refID(n.Index, n.RefIndex) + `" data-footnote-ref="">` + i + `</a></sup>`)
	}
	return ast.WalkContinue, nil
}

func (footnoteRenderer) backlink(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*east.FootnoteBacklink)
		label := "Back to reference " + strconv.Itoa(n.Index)
		if n.RefIndex > 0 {
			label += "-" + strconv.Itoa(n.RefIndex+1)
		}
		_, _ = w.WriteString(` <a href="#` + refID(n.Index, n.RefIndex) + `" data-footnote-backref="" aria-label="` + label + `" class="data-footnote-backref">↩`)
		if n.RefIndex > 0 {
			_, _ = w.WriteString("<sup>" + strconv.Itoa(n.RefIndex+1) + "</sup>")
		}
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (footnoteRenderer) note(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<li id="fn-` + strconv.Itoa(node.(*east.Footnote).Index) + "\">\n")
	} else {
		_, _ = w.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

func (footnoteRenderer) list(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<section class=\"footnotes\" data-footnotes=\"\">\n<ol>\n")
	} else {
		_, _ = w.WriteString("</ol>\n</section>\n")
	}
	return ast.WalkContinue, nil
}
