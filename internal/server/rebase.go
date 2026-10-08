package server

import (
	"bytes"
	"html/template"
	"strings"

	"golang.org/x/net/html"
)

// rebase makes the relative URLs in rendered HTML resolve from the page
// of another directory: a README shown on the page of a directory that
// does not hold it, as a translation in ja/ is, links and shows images
// next to it, while the page resolves them next to itself. prefix is the
// way from the page's directory to the README's, such as "ja/".
//
// Only the tags with a relative URL are written again; the rest of the
// HTML is kept byte for byte.
func rebase(content template.HTML, prefix string) template.HTML {
	if prefix == "" {
		return content
	}
	var out bytes.Buffer
	z := html.NewTokenizer(strings.NewReader(string(content)))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return template.HTML(out.String())
		}
		raw := z.Raw()
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			out.Write(raw)
			continue
		}
		// Raw is reused by the next call, and Token reads the same bytes
		raw = append([]byte(nil), raw...)
		t := z.Token()
		changed := false
		for i, a := range t.Attr {
			var v string
			switch a.Key {
			case "href", "src", "poster":
				v = rebaseURL(a.Val, prefix)
			case "srcset":
				v = rebaseSrcset(a.Val, prefix)
			default:
				continue
			}
			if v != a.Val {
				t.Attr[i].Val = v
				changed = true
			}
		}
		if changed {
			out.WriteString(t.String())
		} else {
			out.Write(raw)
		}
	}
}

// rebaseURL puts prefix before a URL relative to the page's directory,
// and leaves alone one with a scheme, from the root, or within the page.
func rebaseURL(u, prefix string) string {
	switch {
	case u == "", strings.HasPrefix(u, "/"), strings.HasPrefix(u, "#"), strings.HasPrefix(u, "?"):
		return u
	}
	if i := strings.IndexAny(u, ":/?#"); i > 0 && u[i] == ':' {
		return u
	}
	return prefix + u
}

// rebaseSrcset rebases each URL of a srcset, such as "a.png 1x, b.png 2x".
func rebaseSrcset(set, prefix string) string {
	parts := strings.Split(set, ",")
	for i, p := range parts {
		f := strings.Fields(p)
		if len(f) == 0 {
			continue
		}
		f[0] = rebaseURL(f[0], prefix)
		parts[i] = strings.Join(f, " ")
	}
	return strings.Join(parts, ", ")
}
