package markdown

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

// ids makes heading ids as GitHub does, so that anchors written for GitHub
// work. Goldmark's own turns underscores into dashes and drops what is not
// ASCII.
type ids map[string]bool

func newIDs() parser.IDs {
	return ids{}
}

func (s ids) Generate(value []byte, kind ast.NodeKind) []byte {
	id := slug(string(value))
	if id == "" {
		id = "heading"
	}
	if s[id] {
		for i := 1; ; i++ {
			if c := id + "-" + strconv.Itoa(i); !s[c] {
				id = c
				break
			}
		}
	}
	s[id] = true
	return []byte(id)
}

func (s ids) Put(value []byte) {
	s[string(value)] = true
}

// slug keeps letters, digits, marks, dashes and underscores, lowercased,
// turns spaces into dashes and drops the rest.
func slug(s string) string {
	var b strings.Builder
	// Not trimmed: the parser trims a heading, and a space left by an image
	// or HTML at its start is a dash on GitHub, as in #-image-head
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
