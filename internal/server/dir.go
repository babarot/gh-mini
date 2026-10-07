package server

import (
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
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
	var readmes []string
	for _, d := range dirents {
		if s.ws.Skipped(d.Name()) {
			continue
		}
		child := path.Join(rel, d.Name())
		e := entry{Name: d.Name(), Dir: d.IsDir(), Ignored: snap.Ignored(child)}
		if info, err := d.Info(); err == nil {
			e.Ago = ago(info.ModTime())
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
		if !e.Dir && isReadme(d.Name()) {
			readmes = append(readmes, d.Name())
		}
	}
	sort.SliceStable(v.Entries, func(i, j int) bool {
		a, b := v.Entries[i], v.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	if readme := pickReadme(readmes, r.URL.Query().Get("lang"), cookie(r, "gh-mini-lang")); readme != "" {
		name := path.Join(rel, readme)
		// Stat before reading, so that a file changing in between is
		// cached under its old key and rendered again next time
		info, err := s.ws.FS().Stat(name)
		var b []byte
		if err == nil {
			b, err = s.ws.FS().ReadFile(name)
		}
		if err == nil {
			v.Readme = readme
			v.Content, p.Features, _ = s.renderMarkdownFile(name, info, b)
			for _, name := range readmes {
				lang := langOf(name)
				v.Langs = append(v.Langs, langTab{
					Label:   langLabel(lang),
					Href:    "?lang=" + url.QueryEscape(orDefault(lang)),
					Current: name == readme,
				})
			}
			if len(v.Langs) < 2 {
				v.Langs = nil
			}
		}
	}
	s.render(w, p)
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
