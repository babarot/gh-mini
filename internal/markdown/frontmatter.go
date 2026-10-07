package markdown

import (
	"bytes"
	"html"

	"gopkg.in/yaml.v3"
)

// splitFrontMatter splits off YAML front matter: a mapping between a first
// line of --- and the next line of ---.
func splitFrontMatter(src []byte) (fm *yaml.Node, body []byte, ok bool) {
	rest, found := bytes.CutPrefix(src, []byte("---\n"))
	if !found {
		if rest, found = bytes.CutPrefix(src, []byte("---\r\n")); !found {
			return nil, src, false
		}
	}
	for i := 0; i < len(rest); {
		end := len(rest)
		if nl := bytes.IndexByte(rest[i:], '\n'); nl >= 0 {
			end = i + nl + 1
		}
		if string(bytes.TrimRight(rest[i:end], "\r\n")) == "---" {
			var doc yaml.Node
			if yaml.Unmarshal(rest[:i], &doc) != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
				return nil, src, false
			}
			return doc.Content[0], rest[end:], true
		}
		i = end
	}
	return nil, src, false
}

// renderFrontMatter writes front matter as GitHub shows it: a table whose
// head is the keys, with a table for a value that holds more than one.
func renderFrontMatter(buf *bytes.Buffer, n *yaml.Node) {
	buf.WriteString("<table>\n")
	writeYAML(buf, n)
	buf.WriteString("</table>\n")
}

// writeYAML writes the rows of a mapping or a sequence.
func writeYAML(buf *bytes.Buffer, n *yaml.Node) {
	switch n.Kind {
	case yaml.MappingNode:
		buf.WriteString("<thead><tr>")
		for i := 0; i+1 < len(n.Content); i += 2 {
			buf.WriteString("<th>")
			buf.WriteString(html.EscapeString(n.Content[i].Value))
			buf.WriteString("</th>")
		}
		buf.WriteString("</tr></thead><tbody><tr>")
		for i := 0; i+1 < len(n.Content); i += 2 {
			writeCell(buf, n.Content[i+1])
		}
		buf.WriteString("</tr></tbody>")
	case yaml.SequenceNode:
		buf.WriteString("<tbody><tr>")
		for _, c := range n.Content {
			writeCell(buf, c)
		}
		buf.WriteString("</tr></tbody>")
	}
}

func writeCell(buf *bytes.Buffer, n *yaml.Node) {
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	buf.WriteString("<td>")
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		buf.WriteString("<table>")
		writeYAML(buf, n)
		buf.WriteString("</table>")
	} else {
		buf.WriteString(html.EscapeString(n.Value))
	}
	buf.WriteString("</td>")
}
