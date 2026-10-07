package markdown

import (
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// taskListTransformer gives task list items, and the lists that hold
// them, the classes GitHub gives them.
type taskListTransformer struct{}

func (taskListTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != ast.KindListItem {
			return ast.WalkContinue, nil
		}
		if block := n.FirstChild(); block == nil || block.FirstChild() == nil || block.FirstChild().Kind() != east.KindTaskCheckBox {
			return ast.WalkContinue, nil
		}
		n.SetAttributeString("class", "task-list-item")
		n.Parent().SetAttributeString("class", "contains-task-list")
		return ast.WalkContinue, nil
	})
}

type taskCheckBoxRenderer struct{}

func (taskCheckBoxRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(east.KindTaskCheckBox, func(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		_, _ = w.WriteString(`<input type="checkbox" class="task-list-item-checkbox" disabled=""`)
		if node.(*east.TaskCheckBox).IsChecked {
			_, _ = w.WriteString(` checked=""`)
		}
		_, _ = w.WriteString("> ")
		return ast.WalkContinue, nil
	})
}
