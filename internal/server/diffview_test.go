package server

import (
	"testing"

	"github.com/babarot/gh-mini/internal/diff"
)

// Marks cut across the highlighted segments, and both stay whole.
func TestSegmentsHTML(t *testing.T) {
	segs := []segment{{"k", "func"}, {"", " "}, {"nf", "main"}, {"", "(a<b)"}}
	for _, c := range []struct {
		marks []diff.Span
		want  string
	}{
		{nil, `<span class="k">func</span> <span class="nf">main</span>(a&lt;b)`},
		{[]diff.Span{{Start: 5, End: 9}}, `<span class="k">func</span> <mark><span class="nf">main</span></mark>(a&lt;b)`},
		{[]diff.Span{{Start: 2, End: 7}}, `<span class="k">fu</span><mark><span class="k">nc</span></mark><mark> </mark><mark><span class="nf">ma</span></mark><span class="nf">in</span>(a&lt;b)`},
		{[]diff.Span{{Start: 0, End: 1}, {Start: 11, End: 12}}, `<mark><span class="k">f</span></mark><span class="k">unc</span> <span class="nf">main</span>(a<mark>&lt;</mark>b)`},
	} {
		if got := segmentsHTML(segs, c.marks); got != c.want {
			t.Errorf("%v:\n got %s\nwant %s", c.marks, got, c.want)
		}
	}
}
