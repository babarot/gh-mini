// Package lexers replaces some of chroma's lexers with ones that tell more
// kinds of token apart. Importing it registers them.
package lexers

import (
	"embed"
	"io/fs"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

//go:embed *.xml
var files embed.FS

func init() {
	paths, _ := fs.Glob(files, "*.xml")
	for _, p := range paths {
		// A lexer of the same name replaces chroma's
		lexers.Register(chroma.MustNewXMLLexer(files, p))
	}
}
