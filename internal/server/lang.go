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

// langSuffix is the language in name.<lang>.md, such as README.ja.md or
// guide.zh-TW.md; the code is checked against langCodes, so that the .go
// of api.go.md is no language.
var langSuffix = regexp.MustCompile(`(?i)\.([a-z]{2})(?:-[a-z]{2})?$`)

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

// splitLang splits a Markdown file's name into the name of the document and
// its language, "" for the original. ok is false for other files.
func splitLang(name string) (base, lang string, ok bool) {
	ext := path.Ext(name)
	if !isMarkdown(name) {
		return "", "", false
	}
	stem := strings.TrimSuffix(name, ext)
	if m := langSuffix.FindStringSubmatchIndex(stem); m != nil && langCodes[strings.ToLower(stem[m[2]:m[3]])] {
		return strings.ToLower(stem[:m[0]]), strings.ToLower(stem[m[0]+1:]), true
	}
	return strings.ToLower(stem), "", true
}

// isReadme tells README.md and its translations.
func isReadme(name string) bool {
	base, _, ok := splitLang(name)
	return ok && base == "readme"
}

// langOf returns the language of a translated Markdown file, or "" for the
// original.
func langOf(name string) string {
	_, lang, _ := splitLang(name)
	return lang
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
	base, _, ok := splitLang(path.Base(rel))
	if !ok {
		return nil
	}
	dir := path.Dir(rel)
	dirents, err := fs.ReadDir(s.ws.FS().FS(), dir)
	if err != nil {
		return nil
	}
	var tabs []langTab
	for _, d := range dirents {
		dbase, dlang, ok := splitLang(d.Name())
		if d.IsDir() || !ok || dbase != base {
			continue
		}
		tabs = append(tabs, langTab{
			Label:   langLabel(dlang),
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
