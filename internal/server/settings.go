package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// The viewer's settings live in one cookie as JSON, so that the server
// renders the first paint with them; localStorage would only be read after
// the page shows.
//
// The cookie holds only the keys the viewer picked. A key left out follows
// the server's default, such as --theme, and is never written back. Every
// gh-mini on localhost shares the cookie, whatever its port, so the page
// changes only the keys it sets and keeps the rest, even ones this server
// does not know (see settings.js).
//
// To add a setting, add it to settingDefs, in the Section the dialog lists
// it under. With Attr, its value is set as
// <html data-<key>>, for CSS to match; for anything else the page must do
// at once, add an applier for the key in settings.js.
const settingsCookie = "gh-mini-settings"

// setting is one entry in the settings dialog.
type setting struct {
	Key         string
	Label       string
	Description string
	// Section is the page of the dialog it is on; defaultSection when
	// empty. Sections are listed in the order their first setting is.
	Section string
	// Control is how the dialog shows it: "select" for choices that are
	// many or grow at run time, "segmented" for a few fixed choices,
	// "toggle" for on and off, whose values are "true" and "false".
	Control string
	// Choices lists the values a select or segmented setting takes.
	Choices func(s *Server) []choice
	// Default is the value when the viewer has not picked one.
	Default func(s *Server) string
	// Attr sets the value as <html data-<key>> on every page.
	Attr bool
}

type choice struct {
	Value string
	Label string
}

var settingDefs = []setting{
	{
		Key:         "theme",
		Section:     "Appearance",
		Label:       "Theme",
		Description: "Built-in themes and CSS files in the themes directory",
		Control:     "select",
		Choices:     themeChoices,
		Default:     defaultTheme,
	},
	{
		Key:         "mode",
		Section:     "Appearance",
		Label:       "Mode",
		Description: "Auto follows the system",
		Control:     "segmented",
		Choices: func(*Server) []choice {
			return []choice{{"", "Auto"}, {"light", "Light"}, {"dark", "Dark"}}
		},
		Default: func(*Server) string { return "" },
	},
	{
		Key:         "htmlPreview",
		Section:     "Files",
		Label:       "HTML preview",
		Description: "Open HTML files rendered, with their scripts, rather than as code",
		Control:     "toggle",
		Default:     func(*Server) string { return "false" },
	},
}

// legacyCookies are where settings were kept before settingsCookie.
// Remove them, and migrate in settings.js, once they are gone.
var legacyCookies = map[string]string{
	"theme": "gh-mini-theme",
	"mode":  "gh-mini-mode",
}

func (d setting) valid(s *Server, v string) bool {
	if d.Control == "toggle" {
		return v == "true" || v == "false"
	}
	for _, c := range d.Choices(s) {
		if c.Value == v {
			return true
		}
	}
	return false
}

// settings returns the value of every setting for a request: the one in
// the cookie when it is valid, else the default.
func (s *Server) settings(r *http.Request) map[string]string {
	stored := storedSettings(r)
	out := make(map[string]string, len(settingDefs))
	for _, d := range settingDefs {
		v, ok := stored[d.Key]
		if !ok {
			if name, legacy := legacyCookies[d.Key]; legacy {
				v, ok = cookieValue(r, name)
			}
		}
		if ok && d.valid(s, v) {
			out[d.Key] = v
		} else {
			out[d.Key] = d.Default(s)
		}
	}
	return out
}

// storedSettings reads the settings cookie, turning booleans into "true"
// and "false" and dropping values of other types. A broken cookie counts
// as none.
func storedSettings(r *http.Request) map[string]string {
	raw, ok := cookieValue(r, settingsCookie)
	if !ok {
		return nil
	}
	if unescaped, err := url.PathUnescape(raw); err == nil {
		raw = unescaped
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		switch v := v.(type) {
		case string:
			out[k] = v
		case bool:
			if v {
				out[k] = "true"
			} else {
				out[k] = "false"
			}
		}
	}
	return out
}

func cookieValue(r *http.Request, name string) (string, bool) {
	c, err := r.Cookie(name)
	if err != nil {
		return "", false
	}
	return c.Value, true
}

func themeChoices(s *Server) []choice {
	var out []choice
	for _, t := range s.themes() {
		out = append(out, choice{Value: t, Label: t})
	}
	return out
}

func defaultTheme(s *Server) string {
	if s.opts.Theme != "" {
		return s.opts.Theme
	}
	return builtinTheme
}

// settingAttrs renders the settings marked Attr as attributes for <html>.
// Keys are ours, so only the values need escaping.
func settingAttrs(values map[string]string) template.HTMLAttr {
	var b strings.Builder
	for _, d := range settingDefs {
		if d.Attr {
			fmt.Fprintf(&b, ` data-%s="%s"`, d.Key, template.HTMLEscapeString(values[d.Key]))
		}
	}
	return template.HTMLAttr(b.String())
}

// settingView is a setting as the dialog shows it, with its current value.
type settingView struct {
	setting
	Value   string
	Options []choice
}

// settingSection is a page of the settings dialog.
type settingSection struct {
	Name string
	// ID names the section in element ids and in what the page keeps of
	// the one last open
	ID       string
	Settings []settingView
}

// defaultSection is the section of a setting that names none.
const defaultSection = "General"

func (s *Server) settingSections(values map[string]string) []settingSection {
	var out []settingSection
	index := map[string]int{}
	for _, d := range settingDefs {
		v := settingView{setting: d, Value: values[d.Key]}
		if d.Choices != nil {
			v.Options = d.Choices(s)
		}
		name := cmp.Or(d.Section, defaultSection)
		i, ok := index[name]
		if !ok {
			i = len(out)
			index[name] = i
			out = append(out, settingSection{Name: name, ID: sectionID(name)})
		}
		out[i].Settings = append(out[i].Settings, v)
	}
	return out
}

// sectionID makes a section's name fit for an element id.
func sectionID(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case 'a' <= r && r <= 'z', '0' <= r && r <= '9':
			return r
		case 'A' <= r && r <= 'Z':
			return r + 'a' - 'A'
		}
		return '-'
	}, name)
}
