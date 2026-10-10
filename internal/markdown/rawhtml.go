package markdown

import (
	"bytes"
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// HTML written in Markdown is kept as far as GitHub keeps it, and the rest
// is dropped: README files rely on <details>, <p align="center"> or
// <img width>, but a page must not run what a file says, as it is served
// with gh-mini's own pages and API.
//
// Only the HTML a file writes is sanitized, node by node, not the whole
// page, so what this package renders itself (alerts, code, math, footnotes)
// needs no allowance here. An element opened in one node and closed in
// another passes, as the policy keeps or drops each tag on its own.

// rawHTMLPolicy is GitHub's allowance, roughly. It is built from scratch
// rather than from bluemonday.UGCPolicy, which allows id on every element,
// and an allowance cannot be taken back: an id would let a file take the
// place of the page's own elements.
var rawHTMLPolicy = newRawHTMLPolicy()

func newRawHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowStandardURLs()
	p.RequireNoFollowOnLinks(true)
	p.AllowImages()
	p.AllowTables()
	p.AllowLists()

	p.AllowElements(
		"p", "div", "span", "br", "hr", "blockquote", "pre", "code",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"b", "i", "strong", "em", "u", "s", "strike", "del", "ins", "mark", "small",
		"sub", "sup", "kbd", "samp", "var", "q", "cite", "abbr", "dfn", "tt",
		"details", "summary", "picture", "figure", "figcaption",
		"dl", "dt", "dd", "ruby", "rt", "rp", "time", "wbr", "bdo", "caption",
	)
	p.AllowAttrs("href").OnElements("a")
	// Kept for links within the page, as <a name="top"> is, but named
	// apart from the page's own elements by userContent
	p.AllowAttrs("name").Matching(anchorName).OnElements("a")
	p.AllowAttrs("id").Matching(anchorName).Globally()
	p.AllowAttrs("dir").Matching(textDir).Globally()
	p.AllowAttrs("lang").Matching(langTag).Globally()
	p.AllowAttrs("open").OnElements("details")
	p.AllowAttrs("align").OnElements("p", "div", "img", "h1", "h2", "h3", "h4", "h5", "h6", "table", "tr", "td", "th")
	p.AllowAttrs("width", "height").Matching(bluemonday.NumberOrPercent).OnElements("img", "td", "th", "table")
	p.AllowAttrs("colspan", "rowspan").Matching(bluemonday.Integer).OnElements("td", "th")
	p.AllowAttrs("start").Matching(bluemonday.Integer).OnElements("ol")
	// Any text: bluemonday's own checks reject values with ":" or "&",
	// common in alt and title, and the values are escaped anyway
	p.AllowAttrs("alt").Matching(anyText).OnElements("img")
	p.AllowAttrs("title").Matching(anyText).Globally()
	p.AllowAttrs("srcset").Matching(srcset).OnElements("source", "img")
	p.AllowAttrs("media", "type").Matching(anyText).OnElements("source")
	p.AllowElements("source")
	return p
}

var (
	anyText = regexp.MustCompile(`^[^\x00]*$`)
	// anchorName is an id or name without spaces, as a fragment names it
	anchorName = regexp.MustCompile(`^[^\s\x00]+$`)
	textDir    = regexp.MustCompile(`^(?i:ltr|rtl|auto)$`)
	langTag    = regexp.MustCompile(`^[A-Za-z]{1,8}(?:-[A-Za-z0-9]{1,8})*$`)
	// idAttr is an id or a name as bluemonday writes them, the only way:
	// a quote in text or in another attribute's value is escaped
	idAttr = regexp.MustCompile(`(\s(?:id|name)=")`)
	// srcset is candidates of a URL and a descriptor, separated by commas;
	// each URL is http(s) or relative
	srcset = regexp.MustCompile(`^\s*(?:(?:https?://|[^:\s,]+(?:\s|,|$))[^\s,]*(?:\s+[0-9.]+[wx])?\s*(?:,\s*|$))+$`)
)

// rawHTMLRenderer writes the HTML in a file through rawHTMLPolicy. It
// replaces goldmark's renderers, which drop such HTML altogether unless
// told to write it as it is.
type rawHTMLRenderer struct{}

func (rawHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, renderRawHTML)
	reg.Register(ast.KindHTMLBlock, renderHTMLBlock)
}

func renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	var b bytes.Buffer
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		b.Write(segment.Value(source))
	}
	if p, ok := placeholder(b.Bytes(), false); ok {
		_, _ = w.Write(p)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.Write(sanitize(b.Bytes()))
	return ast.WalkSkipChildren, nil
}

// renderHTMLBlock sanitizes a block at once, its closing line included: a
// comment over several lines ends on that line, and apart from the rest
// its "-->" would be text.
func renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	var b bytes.Buffer
	first := 0
	// A block of a tag alone on its line goes on up to a blank line, so
	// the lines after a component are the rest of the block
	if n.Lines().Len() > 0 {
		line := n.Lines().At(0)
		if p, ok := placeholder(line.Value(source), true); ok {
			_, _ = w.Write(p)
			_, _ = w.Write([]byte("\n"))
			first = 1
		}
	}
	for i := first; i < n.Lines().Len(); i++ {
		line := n.Lines().At(i)
		b.Write(line.Value(source))
	}
	if n.HasClosure() {
		b.Write(n.ClosureLine.Value(source))
	}
	if b.Len() == 0 {
		return ast.WalkContinue, nil
	}
	_, _ = w.Write(sanitize(b.Bytes()))
	return ast.WalkContinue, nil
}

// sanitize drops what rawHTMLPolicy does not allow, and puts user-content-
// before ids and names, as GitHub does: a file's id="settings" must not
// take the place of the page's own element, or a heading's. A link to
// #top finds user-content-top in the page.
func sanitize(b []byte) []byte {
	b = tagFilter.ReplaceAll(b, []byte("&lt;$1"))
	return idAttr.ReplaceAll(rawHTMLPolicy.SanitizeBytes(b), []byte("${1}"+userContent))
}

// tagFilter is the tags GFM's tagfilter shows as text, as GitHub does,
// that the policy would drop leaving their content as if it were the
// page's: <textarea>x</textarea> shows as written. Scripts, styles and
// frames are dropped with their content instead.
var tagFilter = regexp.MustCompile(`(?i)<(/?(?:textarea|title|xmp|noembed|noframes|plaintext)\b)`)

// userContent is put before the ids and names a file's HTML gives.
const userContent = "user-content-"

// A component is a self-closing tag whose name starts with a capital, as
// MDX writes one, such as <Partial name="figure" /> or <Image src="a.png" />.
// GitHub drops it, as it does any tag it does not know, and so does a page
// here, but it is kept as an empty <mini-element> naming it, with its
// attributes as JSON, for a plugin to show; with none, it shows nothing.
var (
	componentTag  = regexp.MustCompile(`^<([A-Z][A-Za-z0-9-]*)((?:\s+[^\s"'<>/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'=<>` + "`" + `]+))?)*)\s*/>$`)
	componentAttr = regexp.MustCompile(`([^\s"'<>/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+)))?`)
)

// placeholder is the <mini-element> of a component, when b is one alone.
// One on a line of its own, a block, is shown as one.
func placeholder(b []byte, block bool) ([]byte, bool) {
	m := componentTag.FindSubmatch(bytes.TrimSpace(b))
	if m == nil || !IsComponent(string(m[1])) {
		return nil, false
	}
	attrs := parseAttrs(m[2])
	j, err := json.Marshal(attrs)
	if err != nil {
		return nil, false
	}
	var out bytes.Buffer
	out.WriteString(`<mini-element data-tag="` + html.EscapeString(string(m[1])) + `" data-attrs="` + html.EscapeString(string(j)) + `"`)
	if block {
		out.WriteString(` data-block`)
	}
	out.WriteString(`></mini-element>`)
	return out.Bytes(), true
}

// pluginHTMLPolicy is what a plugin's HTML keeps: what a file's HTML
// keeps, and <slot>, classes, styles, roles, and SVG to draw with. It is
// shown apart from the page, in a shadow root, where the page's classes
// and ids do not reach, so a class or an id takes the place of nothing of
// the page's. What a plugin gives back may be a file's text it was given,
// so what would run, or load from elsewhere, still goes.
var pluginHTMLPolicy = func() *bluemonday.Policy {
	p := newRawHTMLPolicy()
	p.AllowAttrs("class").Matching(anyText).Globally()
	p.AllowAttrs("style").Matching(safeValue).Globally()
	p.AllowAttrs("role", "aria-label", "aria-hidden", "aria-labelledby", "aria-describedby").Matching(anyText).Globally()
	// Where a component's children go
	p.AllowNoAttrs().OnElements("slot")
	p.AllowNoAttrs().OnElements(svgElements...)
	p.AllowAttrs(svgAttrs...).Matching(safeValue).OnElements(svgElements...)
	p.AllowAttrs("type").Matching(safeValue).OnElements("feColorMatrix")
	return p
}()

// svgElements are the SVG elements a plugin's HTML keeps, those that draw.
// Out are those that run or load: script, foreignObject, which holds HTML
// apart from what the policy reads, the animations, which set attributes,
// use and image, which load other documents, and feImage.
var svgElements = []string{
	"svg", "g", "defs", "symbol", "title", "desc",
	"path", "rect", "circle", "ellipse", "line", "polyline", "polygon",
	"text", "tspan", "textPath", "marker",
	"linearGradient", "radialGradient", "stop", "pattern", "clipPath", "mask",
	"filter", "feGaussianBlur", "feOffset", "feBlend", "feFlood", "feComposite",
	"feMerge", "feMergeNode", "feColorMatrix", "feDropShadow",
}

// svgAttrs are the attributes those keep, of geometry and painting.
var svgAttrs = strings.Fields(`
	viewBox preserveAspectRatio x y x1 y1 x2 y2 cx cy r rx ry width height d points
	transform pathLength fill fill-opacity fill-rule stroke stroke-width stroke-opacity
	stroke-dasharray stroke-dashoffset stroke-linecap stroke-linejoin stroke-miterlimit
	opacity color visibility display font-family font-size font-weight font-style
	text-anchor dominant-baseline alignment-baseline letter-spacing dx dy rotate
	textLength lengthAdjust offset stop-color stop-opacity gradientUnits gradientTransform
	spreadMethod fx fy patternUnits patternContentUnits patternTransform clipPathUnits
	clip-path clip-rule mask maskUnits maskContentUnits filter filterUnits primitiveUnits
	in in2 result stdDeviation mode operator k1 k2 k3 k4 values flood-color flood-opacity
	markerWidth markerHeight markerUnits refX refY orient marker-start marker-mid marker-end
	vector-effect xmlns version`)

// safeValue is a value, of an SVG attribute or a style, that loads
// nothing: a paren only after a function that computes, without others in
// it, or url() of a fragment, an element of the same SVG; no backslash,
// which CSS would read as an escape.
var safeValue = regexp.MustCompile(`^(?:[^()\\]|(?i:rgba?|hsla?|var|calc|translate|rotate|scale|matrix|skew[xy])\([^()\\]*\)|url\(\s*['"]?#[\w.:-]+['"]?\s*\))*$`)

// SanitizePlugin drops from a plugin's HTML what the page must not run, as
// sanitize does for a file's. Its ids stay as written: in the plugin's
// shadow root they are its own, and its SVG names them in url(#id).
func SanitizePlugin(b []byte) []byte {
	return pluginHTMLPolicy.SanitizeBytes(b)
}

// componentName is a component's name: letters, digits and hyphens, as
// CommonMark takes a tag's name, starting with a capital.
var componentName = regexp.MustCompile(`^[A-Z][A-Za-z0-9-]*$`)

// IsComponent tells whether a tag of a name is a component. A name of an
// element of HTML's in capitals alone is HTML, as old READMEs write it,
// such as <BR/> or <IMG SRC="a.png"/>; <Image /> is a component.
func IsComponent(name string) bool {
	if !componentName.MatchString(name) {
		return false
	}
	return name != strings.ToUpper(name) || !htmlElements[strings.ToLower(name)]
}

// htmlElements are the names of HTML's elements, and of those it had.
var htmlElements = func() map[string]bool {
	m := map[string]bool{}
	for _, name := range strings.Fields(`
		a abbr acronym address applet area article aside audio b base basefont bdi bdo bgsound big blink
		blockquote body br button canvas caption center cite code col colgroup data datalist dd del details
		dfn dialog dir div dl dt em embed fieldset figcaption figure font footer form frame frameset h1 h2
		h3 h4 h5 h6 head header hgroup hr html i iframe image img input ins isindex kbd keygen label legend
		li link listing main map mark marquee math menu menuitem meta meter nav nobr noembed noframes
		noscript object ol optgroup option output p param picture plaintext pre progress q rb rp rt rtc ruby
		s samp script search section select slot small source spacer span strike strong style sub summary
		sup svg table tbody td template textarea tfoot th thead time title tr track tt u ul var video wbr xmp`) {
		m[name] = true
	}
	return m
}()
