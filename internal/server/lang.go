package server

import (
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

type langTab struct {
	Label   string
	Href    string
	Current bool
}

var (
	readmeRe = regexp.MustCompile(`(?i)^readme(\.[a-z]{2}(-[a-z]{2})?)?\.(md|markdown)$`)
	// name.<lang>.md, such as README.ja.md or guide.zh-TW.md
	langRe = regexp.MustCompile(`(?i)^(.+?)(?:\.([a-z]{2}(?:-[a-z]{2})?))?\.(md|markdown)$`)
)

// langOf returns the language of a translated Markdown file, or "" for the
// original.
func langOf(name string) string {
	m := langRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[2])
}

func langLabel(lang string) string {
	if lang == "" {
		return "Default"
	}
	return strings.ToUpper(lang)
}

func orDefault(lang string) string {
	if lang == "" {
		return "default"
	}
	return lang
}

// pickReadme chooses README.md, or its translation for the language asked
// for in the URL or picked before.
func pickReadme(names []string, query, saved string) string {
	if len(names) == 0 {
		return ""
	}
	want := query
	if want == "" {
		want = saved
	}
	if want == "default" {
		want = ""
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) < len(names[j]) })
	for _, n := range names {
		if langOf(n) == strings.ToLower(want) {
			return n
		}
	}
	for _, n := range names {
		if langOf(n) == "" {
			return n
		}
	}
	return names[0]
}

// langTabs links a Markdown file to its translations next to it:
// guide.md, guide.ja.md and so on.
func (s *Server) langTabs(rel string) []langTab {
	m := langRe.FindStringSubmatch(path.Base(rel))
	if m == nil {
		return nil
	}
	base := strings.ToLower(m[1])
	dir := path.Dir(rel)
	dirents, err := fs.ReadDir(s.ws.FS().FS(), dir)
	if err != nil {
		return nil
	}
	var tabs []langTab
	for _, d := range dirents {
		dm := langRe.FindStringSubmatch(d.Name())
		if d.IsDir() || dm == nil || strings.ToLower(dm[1]) != base {
			continue
		}
		tabs = append(tabs, langTab{
			Label:   langLabel(strings.ToLower(dm[2])),
			Href:    href(path.Join(dir, d.Name())),
			Current: d.Name() == path.Base(rel),
		})
	}
	if len(tabs) < 2 {
		return nil
	}
	sort.SliceStable(tabs, func(i, j int) bool {
		return tabs[i].Label == "Default" || (tabs[j].Label != "Default" && tabs[i].Label < tabs[j].Label)
	})
	return tabs
}
