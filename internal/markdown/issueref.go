package markdown

import (
	"regexp"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// issueRef finds #123, owner/name#123 and GH-123, GitHub's other way to
// write #123.
var issueRef = regexp.MustCompile(`(?:([A-Za-z0-9][-A-Za-z0-9]*)/([-A-Za-z0-9_.]+))?#([0-9]+)\b|\bGH-([0-9]+)\b`)

// issueRefTransformer links #123 to an issue of the repository, and
// owner/name#123 to one of another, as GitHub does. It leaves alone what
// is in code and links, and references glued to a word, such as a#1.
type issueRefTransformer struct {
	repo string
}

func (t issueRefTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var runs [][]*ast.Text
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindCodeSpan, ast.KindLink, ast.KindAutoLink, ast.KindImage:
			return ast.WalkSkipChildren, nil
		}
		runs = append(runs, textRuns(n, src)...)
		return ast.WalkContinue, nil
	})
	for _, run := range runs {
		t.link(src, run)
	}
}

// rawOpenTag and rawCloseTag find the HTML written around text that must
// not be linked: code, preformatted text and links.
var (
	rawOpenTag  = regexp.MustCompile(`(?i)^<(a|code|pre)[\s>]`)
	rawCloseTag = regexp.MustCompile(`(?i)^</(a|code|pre)\s*>`)
)

// textRuns returns the runs of a node's children that are one stretch of
// the source, split into texts only by the inline parsers. Text between
// <code>, <pre> or <a> and its end tag, written as HTML, is left out.
func textRuns(n ast.Node, src []byte) [][]*ast.Text {
	var runs [][]*ast.Text
	var run []*ast.Text
	flush := func() {
		if len(run) > 0 {
			runs = append(runs, run)
		}
		run = nil
	}
	inside := 0
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if raw, ok := c.(*ast.RawHTML); ok && raw.Segments.Len() > 0 {
			seg := raw.Segments.At(0)
			tag := seg.Value(src)
			switch {
			case rawOpenTag.Match(tag):
				inside++
			case rawCloseTag.Match(tag) && inside > 0:
				inside--
			}
			flush()
			continue
		}
		t, ok := c.(*ast.Text)
		if !ok || t.IsRaw() || inside > 0 {
			flush()
			continue
		}
		if len(run) > 0 {
			last := run[len(run)-1]
			if last.SoftLineBreak() || last.HardLineBreak() || last.Segment.Stop != t.Segment.Start {
				flush()
			}
		}
		run = append(run, t)
	}
	flush()
	return runs
}

// link replaces a run with texts and links to the issues it refers to.
func (t issueRefTransformer) link(src []byte, run []*ast.Text) {
	first, last := run[0], run[len(run)-1]
	start, stop := first.Segment.Start, last.Segment.Stop
	var nodes []ast.Node
	at := start
	for _, m := range issueRef.FindAllSubmatchIndex(src[start:stop], -1) {
		for i := range m {
			if m[i] >= 0 {
				m[i] += start
			}
		}
		if m[0] > 0 && isRefGlue(src[m[0]-1]) {
			continue
		}
		var url string
		switch {
		case m[2] >= 0:
			url = "https://github.com/" + string(src[m[2]:m[5]]) + "/issues/" + string(src[m[6]:m[7]])
		case m[6] >= 0 && t.repo != "":
			url = "https://github.com/" + t.repo + "/issues/" + string(src[m[6]:m[7]])
		case m[8] >= 0 && t.repo != "":
			url = "https://github.com/" + t.repo + "/issues/" + string(src[m[8]:m[9]])
		default:
			continue
		}
		if m[0] > at {
			nodes = append(nodes, ast.NewTextSegment(text.NewSegment(at, m[0])))
		}
		l := ast.NewLink()
		l.Destination = []byte(url)
		l.SetAttributeString("class", "issue-link")
		l.AppendChild(l, ast.NewTextSegment(text.NewSegment(m[0], m[1])))
		nodes = append(nodes, l)
		at = m[1]
	}
	if nodes == nil {
		return
	}
	tail := ast.NewTextSegment(text.NewSegment(at, stop))
	tail.SetSoftLineBreak(last.SoftLineBreak())
	tail.SetHardLineBreak(last.HardLineBreak())
	nodes = append(nodes, tail)

	parent := first.Parent()
	for _, n := range nodes {
		parent.InsertBefore(parent, first, n)
	}
	for _, n := range run {
		parent.RemoveChild(parent, n)
	}
}

// isRefGlue tells whether a reference after c is part of something else.
func isRefGlue(c byte) bool {
	return c == '_' || c == '/' || c == '\\' || c == '&' || c == '#' || c == '.' || c == '-' ||
		'0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// issueURL finds a link to an issue, a pull request or a discussion on
// GitHub, which GitHub shows as owner/name#123, or #123 in its own
// repository.
var issueURL = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/(?:issues|pull|discussions)/([0-9]+)/?$`)

// issueURLTransformer shortens a bare link to an issue as GitHub does.
type issueURLTransformer struct {
	repo string
}

func (t issueURLTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var links []*ast.AutoLink
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.AutoLink); ok && entering && l.AutoLinkType == ast.AutoLinkURL {
			links = append(links, l)
		}
		return ast.WalkContinue, nil
	})
	for _, l := range links {
		url := l.URL(src)
		m := issueURL.FindSubmatch(url)
		if m == nil {
			continue
		}
		label := string(m[1]) + "#" + string(m[2])
		if string(m[1]) == t.repo {
			label = "#" + string(m[2])
		}
		link := ast.NewLink()
		link.Destination = url
		link.SetAttributeString("class", "issue-link")
		link.AppendChild(link, ast.NewString([]byte(label)))
		l.Parent().ReplaceChild(l.Parent(), l, link)
	}
}
