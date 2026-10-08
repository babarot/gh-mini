package server

import (
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/babarot/gh-mini/internal/workspace"
)

// dirView is a directory's page: its files, and its README.
type dirView struct {
	Entries []entry
	Readme  string
	Content template.HTML
	Langs   []langTab
}

type entry struct {
	Name    string
	Href    string
	Dir     bool
	Ignored bool
	Link    bool
	Ago     string
	// Date is when it was modified, which Ago tells roughly
	Date string
	// Status is how it changed since the last commit, and Gone is set on
	// a file deleted since, which git still has
	Status *entryStatus
	Gone   bool
	// Class is the row's: ignored, hideable when the setting to hide
	// ignored directories may, and changed with its letter, s-M and so on
	Class string
}

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request, snap *workspace.Snapshot, rel string) {
	p := s.newPage(r, snap, rel, "dir")
	v := &dirView{}
	p.Dir = v
	dirents, err := fs.ReadDir(s.ws.FS().FS(), rel)
	if err != nil {
		s.serveError(w, r, snap, rel, err)
		return
	}
	tr := s.translationsFor(p.Settings)
	// The README of this directory and its translations
	var readme *translationGroup
	// A README of a document elsewhere, such as ja/README.md in the
	// directory ja, shown without its translations when there is no other
	var otherReadme string
	// A README that is not Markdown, shown as text when there is no other
	var plainReadme string
	for _, d := range dirents {
		if s.ws.Skipped(d.Name()) {
			continue
		}
		child := path.Join(rel, d.Name())
		e := entry{Name: d.Name(), Dir: d.IsDir(), Ignored: snap.Ignored(child)}
		if info, err := d.Info(); err == nil {
			e.Ago = ago(info.ModTime())
			e.Date = info.ModTime().Format(dateLayout)
			if info.Mode()&fs.ModeSymlink != 0 {
				e.Link = true
				if st, err := s.ws.FS().Stat(child); err == nil {
					e.Dir = st.IsDir()
				}
			}
		}
		e.Href = href(child)
		if e.Dir {
			e.Href = dirHref(child)
		}
		v.Entries = append(v.Entries, e)
		if !e.Dir && readme == nil && isMarkdown(d.Name()) {
			if g, _ := tr.split(child); g.dir == rel && g.name == "readme" {
				readme = &g
			} else if strings.EqualFold(strings.TrimSuffix(d.Name(), path.Ext(d.Name())), "readme") && otherReadme == "" {
				otherReadme = child
			}
		}
		if !e.Dir && plainReadme == "" && isPlainReadme(d.Name()) {
			plainReadme = d.Name()
		}
	}
	changed := childStatus(p.status, rel)
	listed := map[string]bool{}
	for i := range v.Entries {
		e := &v.Entries[i]
		listed[e.Name] = true
		if !e.Ignored {
			e.Status = changed[e.Name]
		}
	}
	// Files deleted are listed until the deletion is committed, as is a
	// directory with nothing left in it
	for name, st := range changed {
		if listed[name] {
			continue
		}
		child := path.Join(rel, name)
		e := entry{Name: name, Status: st, Gone: true, Dir: st.Letter == "", Ago: "deleted"}
		e.Href = href(child)
		if e.Dir {
			e.Href = dirHref(child)
		}
		v.Entries = append(v.Entries, e)
	}
	for i := range v.Entries {
		e := &v.Entries[i]
		var cls []string
		if e.Ignored {
			cls = append(cls, "ignored")
			// Inside an ignored directory, everything is: nothing to hide
			if e.Dir && !p.Ignored {
				cls = append(cls, "hideable")
			}
		}
		if e.Status != nil && e.Status.Letter != "" {
			cls = append(cls, "changed", "s-"+e.Status.Letter)
		}
		if e.Gone {
			cls = append(cls, "gone")
		}
		e.Class = strings.Join(cls, " ")
	}
	sort.SliceStable(v.Entries, func(i, j int) bool {
		a, b := v.Entries[i], v.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	var readmes []member
	if readme != nil {
		readmes = tr.members(s.ws.FS().FS(), s.ws.Skipped, *readme)
	} else if otherReadme != "" {
		readmes = []member{{Rel: otherReadme}}
	}
	if len(readmes) > 0 {
		picked := pickReadme(readmes, r.URL.Query().Get("lang"), cookie(r, "gh-mini-lang"))
		name := picked.Rel
		// Stat before reading, so that a file changing in between is
		// cached under its old key and rendered again next time
		info, err := s.ws.FS().Stat(name)
		var b []byte
		if err == nil {
			b, err = readRegular(s.ws.FS(), name, info)
		}
		if err == nil {
			v.Readme = strings.TrimPrefix(name, rel+"/")
			v.Content, p.Features, _ = s.renderMarkdownFile(name, info, b)
			// A README elsewhere, as a translation in ja/ is, links next
			// to itself
			if d := path.Dir(name); d != rel {
				v.Content = rebase(v.Content, readmePrefix(rel, d))
			}
			for _, m := range readmes {
				v.Langs = append(v.Langs, langTab{
					Label:   langLabel(m.Lang),
					Href:    "?lang=" + url.QueryEscape(orDefault(m.Lang)),
					Current: m == picked,
				})
			}
			if len(v.Langs) < 2 {
				v.Langs = nil
			}
		}
	} else if plainReadme != "" {
		if b, err := readRegular(s.ws.FS(), path.Join(rel, plainReadme), nil); err == nil {
			if text, _, ok := decodeText(b); ok {
				v.Readme = plainReadme
				v.Content = template.HTML("<pre>" + template.HTMLEscapeString(string(text)) + "</pre>")
			}
		}
	}
	s.render(w, p)
}

// readmePrefix is the way from a directory to the one of its README, as
// "ja/" from docs to docs/ja. Both are relative to the root.
func readmePrefix(dir, readmeDir string) string {
	r, err := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(readmeDir))
	if err != nil || r == "." {
		return ""
	}
	return filepath.ToSlash(r) + "/"
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	case d < 365*24*time.Hour:
		return plural(int(d.Hours()/24/30), "month")
	default:
		return plural(int(d.Hours()/24/365), "year")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return strconv.Itoa(n) + " " + unit + "s ago"
}
