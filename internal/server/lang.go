package server

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type langTab struct {
	Label   string
	Href    string
	Current bool
}

// Translations are Markdown files named after another and a language,
// such as guide.ja.md for guide.md. How they are named is a layout: a
// template of the path of a translation from the directory of its
// original, without the extension, with {name} for the original's name
// and {lang} for the language. --translations gives the layouts as
// presets and templates separated by commas, tried in order, or "off".
const (
	translationsOff     = "off"
	defaultTranslations = "suffix"
)

// translationPresets are the layouts the settings dialog offers by name.
var translationPresets = []struct{ name, template string }{
	{"suffix", "{name}.{lang}"},
	{"dir", "{lang}/{name}"},
}

// langPattern is what {lang} matches, such as ja or zh-TW; the code is
// checked against langCodes, so that the .go of api.go.md is no language.
const langPattern = `[A-Za-z]{2}(?:-[A-Za-z]{2})?`

// langCodes are the ISO 639-1 language codes.
var langCodes = map[string]bool{
	"aa": true, "ab": true, "ae": true, "af": true, "ak": true, "am": true, "an": true, "ar": true, "as": true, "av": true, "ay": true, "az": true,
	"ba": true, "be": true, "bg": true, "bh": true, "bi": true, "bm": true, "bn": true, "bo": true, "br": true, "bs": true, "ca": true, "ce": true,
	"ch": true, "co": true, "cr": true, "cs": true, "cu": true, "cv": true, "cy": true, "da": true, "de": true, "dv": true, "dz": true, "ee": true,
	"el": true, "en": true, "eo": true, "es": true, "et": true, "eu": true, "fa": true, "ff": true, "fi": true, "fj": true, "fo": true, "fr": true,
	"fy": true, "ga": true, "gd": true, "gl": true, "gn": true, "gu": true, "gv": true, "ha": true, "he": true, "hi": true, "ho": true, "hr": true,
	"ht": true, "hu": true, "hy": true, "hz": true, "ia": true, "id": true, "ie": true, "ig": true, "ii": true, "ik": true, "io": true, "is": true,
	"it": true, "iu": true, "ja": true, "jv": true, "ka": true, "kg": true, "ki": true, "kj": true, "kk": true, "kl": true, "km": true, "kn": true,
	"ko": true, "kr": true, "ks": true, "ku": true, "kv": true, "kw": true, "ky": true, "la": true, "lb": true, "lg": true, "li": true, "ln": true,
	"lo": true, "lt": true, "lu": true, "lv": true, "mg": true, "mh": true, "mi": true, "mk": true, "ml": true, "mn": true, "mr": true, "ms": true,
	"mt": true, "my": true, "na": true, "nb": true, "nd": true, "ne": true, "ng": true, "nl": true, "nn": true, "no": true, "nr": true, "nv": true,
	"ny": true, "oc": true, "oj": true, "om": true, "or": true, "os": true, "pa": true, "pi": true, "pl": true, "ps": true, "pt": true, "qu": true,
	"rm": true, "rn": true, "ro": true, "ru": true, "rw": true, "sa": true, "sc": true, "sd": true, "se": true, "sg": true, "si": true, "sk": true,
	"sl": true, "sm": true, "sn": true, "so": true, "sq": true, "sr": true, "ss": true, "st": true, "su": true, "sv": true, "sw": true, "ta": true,
	"te": true, "tg": true, "th": true, "ti": true, "tk": true, "tl": true, "tn": true, "to": true, "tr": true, "ts": true, "tt": true, "tw": true,
	"ty": true, "ug": true, "uk": true, "ur": true, "uz": true, "ve": true, "vi": true, "vo": true, "wa": true, "wo": true, "xh": true, "yi": true,
	"yo": true, "za": true, "zh": true, "zu": true,
}

// translationSettings parses the choices of the translations setting: off,
// the presets and, when it is none of them, the --translations value.
func translationSettings(value string) (map[string]translationLayouts, []choice, error) {
	choices := []choice{{translationsOff, "Off"}}
	for _, p := range translationPresets {
		choices = append(choices, choice{p.name, translationsLabel(p.name)})
	}
	if !slices.ContainsFunc(choices, func(c choice) bool { return c.Value == value }) {
		choices = append(choices, choice{value, translationsLabel(value)})
	}
	layouts := make(map[string]translationLayouts, len(choices))
	for _, c := range choices {
		ls, err := parseTranslations(c.Value)
		if err != nil {
			return nil, nil, fmt.Errorf("translations %w", err)
		}
		layouts[c.Value] = ls
	}
	return layouts, choices, nil
}

// translationsLabel shows layouts as the templates they are, with the
// presets' spelled out and .md added, such as "{lang}/{name}.md", so that
// the dialog tells patterns and not file names.
func translationsLabel(value string) string {
	var labels []string
	for _, p := range strings.Split(value, ",") {
		for _, preset := range translationPresets {
			if p == preset.name {
				p = preset.template
			}
		}
		if !isMarkdown(p) {
			p += ".md"
		}
		labels = append(labels, p)
	}
	return strings.Join(labels, ", ")
}

// translations are the layouts a page uses, as the viewer picked them.
func (s *Server) translationsFor(settings map[string]string) translationLayouts {
	return s.translations[settings["translations"]]
}

// translationLayout is one layout, parsed.
type translationLayout struct {
	// path matches the path of a translation without its extension: the
	// directory of the original, then the template
	path *regexp.Regexp
	// segments are the template's parts between slashes, and dirs the
	// patterns of the ones before the last that hold {lang}, nil for the
	// others
	segments []string
	dirs     []*regexp.Regexp
}

// translationLayouts are the layouts in use, none when translations are
// off.
type translationLayouts []translationLayout

// normalizeTranslations trims the layouts of a --translations value; an
// empty one is the default.
func normalizeTranslations(value string) string {
	var parts []string
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return defaultTranslations
	}
	return strings.Join(parts, ",")
}

// CheckTranslations tells what is wrong with a --translations value.
func CheckTranslations(value string) error {
	_, err := parseTranslations(value)
	return err
}

func parseTranslations(value string) (translationLayouts, error) {
	parts := strings.Split(normalizeTranslations(value), ",")
	if len(parts) == 1 && parts[0] == translationsOff {
		return nil, nil
	}
	var out translationLayouts
	for _, p := range parts {
		if p == translationsOff {
			return nil, errors.New(`"off" goes alone`)
		}
		tmpl := p
		for _, preset := range translationPresets {
			if p == preset.name {
				tmpl = preset.template
			}
		}
		l, err := parseTranslationLayout(tmpl)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, l)
	}
	return out, nil
}

func parseTranslationLayout(tmpl string) (translationLayout, error) {
	if isMarkdown(tmpl) {
		tmpl = strings.TrimSuffix(tmpl, path.Ext(tmpl))
	}
	if strings.Count(tmpl, "{name}") != 1 || strings.Count(tmpl, "{lang}") != 1 {
		return translationLayout{}, errors.New("needs {name} and {lang} once each")
	}
	if strings.Contains(tmpl, "{name}{lang}") || strings.Contains(tmpl, "{lang}{name}") {
		return translationLayout{}, errors.New("needs something between {name} and {lang}")
	}
	if strings.ContainsAny(strings.NewReplacer("{name}", "", "{lang}", "").Replace(tmpl), "{}") {
		return translationLayout{}, errors.New("knows only {name} and {lang}")
	}
	l := translationLayout{segments: strings.Split(tmpl, "/")}
	for i, seg := range l.segments {
		if seg == "" || seg == "." || seg == ".." {
			return translationLayout{}, errors.New("must be a path down from the original's directory")
		}
		last := i == len(l.segments)-1
		if strings.Contains(seg, "{name}") && !last {
			return translationLayout{}, errors.New("needs {name} in the file name")
		}
		if !last {
			var re *regexp.Regexp
			if strings.Contains(seg, "{lang}") {
				re = regexp.MustCompile("^" + templatePattern(seg) + "$")
			}
			l.dirs = append(l.dirs, re)
		}
	}
	l.path = regexp.MustCompile(`^(?:(?P<dir>.*)/)?` + templatePattern(tmpl) + "$")
	return l, nil
}

// templatePattern turns a template into a regular expression, with groups
// for {name} and {lang}.
func templatePattern(tmpl string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(tmpl, '{')
		if i < 0 {
			b.WriteString(regexp.QuoteMeta(tmpl))
			return b.String()
		}
		b.WriteString(regexp.QuoteMeta(tmpl[:i]))
		if strings.HasPrefix(tmpl[i:], "{name}") {
			b.WriteString(`(?P<name>[^/]+)`)
		} else {
			b.WriteString(`(?P<lang>` + langPattern + `)`)
		}
		tmpl = tmpl[i+len("{name}"):]
	}
}

// translationGroup names a document, whatever its language: the directory
// of the original and its name without the extension, in lower case.
type translationGroup struct {
	dir, name string
}

// split tells the document a Markdown file is of and its language, "" for
// the original.
func (ls translationLayouts) split(rel string) (translationGroup, string) {
	stem := strings.TrimSuffix(rel, path.Ext(rel))
	for _, l := range ls {
		m := l.path.FindStringSubmatch(stem)
		if m == nil {
			continue
		}
		lang := strings.ToLower(m[l.path.SubexpIndex("lang")])
		if !langCodes[lang[:2]] {
			continue
		}
		dir := m[l.path.SubexpIndex("dir")]
		if dir == "" {
			dir = "."
		}
		return translationGroup{dir: dir, name: strings.ToLower(m[l.path.SubexpIndex("name")])}, lang
	}
	return translationGroup{dir: path.Dir(rel), name: strings.ToLower(path.Base(stem))}, ""
}

// member is a Markdown file of a document, in one language.
type member struct {
	Rel  string
	Lang string
}

// members lists the files of a document, the shortest path first.
func (ls translationLayouts) members(fsys fs.FS, skipped func(string) bool, g translationGroup) []member {
	var out []member
	seen := map[string]bool{}
	collect := func(dir string) {
		if seen[dir] {
			return
		}
		seen[dir] = true
		dirents, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return
		}
		for _, d := range dirents {
			if d.IsDir() || skipped(d.Name()) || !isMarkdown(d.Name()) {
				continue
			}
			rel := path.Join(dir, d.Name())
			if dg, lang := ls.split(rel); dg == g {
				out = append(out, member{Rel: rel, Lang: lang})
			}
		}
	}
	collect(g.dir)
	for _, l := range ls {
		dirs := []string{g.dir}
		for i, re := range l.dirs {
			var next []string
			for _, dir := range dirs {
				if re == nil {
					next = append(next, path.Join(dir, l.segments[i]))
					continue
				}
				dirents, err := fs.ReadDir(fsys, dir)
				if err != nil {
					continue
				}
				for _, d := range dirents {
					if !skipped(d.Name()) && re.MatchString(d.Name()) {
						next = append(next, path.Join(dir, d.Name()))
					}
				}
			}
			dirs = next
		}
		for _, dir := range dirs {
			collect(dir)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Rel) != len(out[j].Rel) {
			return len(out[i].Rel) < len(out[j].Rel)
		}
		return out[i].Rel < out[j].Rel
	})
	return out
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

// pickReadme chooses the README in the language asked for in the URL or
// picked before, else the original, of a README's members.
func pickReadme(members []member, query, saved string) member {
	want := strings.ToLower(query)
	if want == "" {
		want = strings.ToLower(saved)
	}
	if want == "default" {
		want = ""
	}
	for _, m := range members {
		if m.Lang == want {
			return m
		}
	}
	for _, m := range members {
		if m.Lang == "" {
			return m
		}
	}
	return members[0]
}

// langTabs links a Markdown file to its translations: guide.md,
// guide.ja.md and so on.
func (s *Server) langTabs(tr translationLayouts, rel string) []langTab {
	if !isMarkdown(rel) {
		return nil
	}
	g, _ := tr.split(rel)
	members := tr.members(s.ws.FS().FS(), s.ws.Skipped, g)
	if len(members) < 2 {
		return nil
	}
	var tabs []langTab
	for _, m := range members {
		tabs = append(tabs, langTab{
			Label:   langLabel(m.Lang),
			Href:    href(m.Rel),
			Current: m.Rel == rel,
		})
	}
	sort.SliceStable(tabs, func(i, j int) bool {
		return tabs[i].Label == "Default" || (tabs[j].Label != "Default" && tabs[i].Label < tabs[j].Label)
	})
	return tabs
}

// isPlainReadme tells a README that is not Markdown, such as README or
// README.txt, which GitHub shows too.
func isPlainReadme(name string) bool {
	switch strings.ToLower(name) {
	case "readme", "readme.txt", "readme.rst", "readme.adoc", "readme.org":
		return true
	}
	return false
}
