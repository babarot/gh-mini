package server

import (
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
// To add a setting, add it to settingDefs. With Attr, its value is set as
// <html data-<key>>, for CSS to match; for anything else the page must do
// at once, add an applier for the key in settings.js.
const settingsCookie = "gh-mini-settings"

// setting is one entry in the settings dialog.
type setting struct {
	Key         string
	Label       string
	Description string
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
		Label:       "Theme",
		Description: "CSS files in the themes directory",
		Control:     "select",
		Choices:     themeChoices,
		Default:     defaultTheme,
	},
	{
		Key:         "mode",
		Label:       "Mode",
		Description: "Auto follows the system",
		Control:     "segmented",
		Choices: func(*Server) []choice {
			return []choice{{"", "Auto"}, {"light", "Light"}, {"dark", "Dark"}}
		},
		Default: func(*Server) string { return "" },
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

func (s *Server) settingViews(values map[string]string) []settingView {
	out := make([]settingView, 0, len(settingDefs))
	for _, d := range settingDefs {
		v := settingView{setting: d, Value: values[d.Key]}
		if d.Choices != nil {
			v.Options = d.Choices(s)
		}
		out = append(out, v)
	}
	return out
}
