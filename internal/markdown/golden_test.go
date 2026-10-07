package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-mini/internal/golden"
)

// TestGolden renders each testdata/*.md and compares it with the .html
// next to it.
func TestGolden(t *testing.T) {
	srcs, err := filepath.Glob(filepath.Join("testdata", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) == 0 {
		t.Fatal("no testdata")
	}
	for _, src := range srcs {
		name := strings.TrimSuffix(filepath.Base(src), ".md")
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := New("example/repo").Render(b)
			if err != nil {
				t.Fatal(err)
			}
			golden.Check(t, strings.TrimSuffix(src, ".md")+".html", got)
		})
	}
}
