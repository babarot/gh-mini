package server

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/babarot/gh-mini/internal/diff"
	"github.com/babarot/gh-mini/internal/workspace"
)

const (
	// maxDiffLines is the most lines of one file's diff a page shows; a
	// longer one is folded, and read on a page of its own.
	maxDiffLines = 1500
	// maxPageLines is the most lines of diffs a page shows; the files
	// after them are folded.
	maxPageLines = 20000
)

// changeList is the Changes page: the diffs of the files changed since
// the last commit.
type changeList struct {
	// Show is which changes: all, staged, unstaged or untracked
	Show  string
	Tabs  []showTab
	Files []*changedFile
	Added int
	// Deleted counts the lines deleted
	Deleted int
	// One is set on the page of one file folded on the list
	One bool
	// All is the list of all files, from the page of one
	All string
}

type showTab struct {
	Label   string
	Href    string
	Count   int
	Current bool
}

// changedFile is one file on the Changes page.
type changedFile struct {
	Path   string
	From   string
	Letter string
	Label  string
	// Staged and Unstaged tell which of the two a file has changes in,
	// when all changes are shown
	Staged   bool
	Unstaged bool
	Added    int
	Deleted  int
	Binary   bool
	// Blocks are the five squares of the bar GitHub draws: g added, r
	// deleted, or neither
	Blocks []string
	// Href is the file's page, none for a file deleted
	Href string
	// Anchor is the id of the file on the page
	Anchor string
	Diff   template.HTML
	// Notice is shown instead of a diff, as for a binary file
	Notice string
	// Load is where a diff too long to show at once is read
	Load string
}

var shows = []struct{ key, label string }{
	{"all", "All"},
	{"staged", "Staged"},
	{"unstaged", "Unstaged"},
	{"untracked", "Untracked"},
}

// inShow tells whether a file's change is among those shown.
func inShow(f *workspace.FileStatus, show string) bool {
	switch show {
	case "staged":
		return f.X != "." && f.X != "?"
	case "unstaged":
		return f.Y != "." && f.Y != "?"
	case "untracked":
		return f.X == "?"
	}
	return true
}

func (s *Server) serveChangesPage(w http.ResponseWriter, r *http.Request) {
	r = s.withSeq(r)
	snap := s.ws.Snapshot()
	st := snap.Status
	if st == nil || !st.Git {
		w.WriteHeader(http.StatusNotFound)
		s.render(w, s.newPage(r, snap, "_mini/changes", "notfound"))
		return
	}
	p := s.newPage(r, snap, ".", "changes")
	p.Title = "Changes · " + p.Title
	q := r.URL.Query()
	show := q.Get("show")
	if !slices.ContainsFunc(shows, func(x struct{ key, label string }) bool { return x.key == show }) {
		show = "all"
	}
	v := &changeList{Show: show}
	p.ChangeList = v
	var paths []string
	for _, x := range shows {
		n := 0
		for p, f := range st.Files {
			if inShow(f, x.key) {
				n++
				if x.key == show {
					paths = append(paths, p)
				}
			}
		}
		v.Tabs = append(v.Tabs, showTab{Label: x.label, Href: changesHref(x.key, ""), Count: n, Current: x.key == show})
	}
	slices.Sort(paths)
	// One file, folded on the list for its length; it must be one listed,
	// as the path goes to git
	if one := q.Get("file"); one != "" {
		if !slices.Contains(paths, one) {
			w.WriteHeader(http.StatusNotFound)
			s.render(w, s.newPage(r, snap, "_mini/changes", "notfound"))
			return
		}
		paths = []string{one}
		v.One = true
		v.All = changesHref(show, "")
	}

	which := workspace.DiffAll
	switch show {
	case "staged":
		which = workspace.DiffStaged
	case "unstaged":
		which = workspace.DiffUnstaged
	}
	patches := map[string]*diff.File{}
	if show != "untracked" {
		var specs []string
		if v.One {
			specs = []string{paths[0]}
			if from := st.Files[paths[0]].From; from != "" {
				specs = append(specs, from)
			}
		}
		parsed := diff.Parse(s.ws.Diff(st, which, specs...))
		for i := range parsed {
			patches[parsed[i].Path()] = &parsed[i]
		}
	}

	budget := maxPageLines
	for _, rel := range paths {
		f := st.Files[rel]
		cf := &changedFile{
			Path:   rel,
			From:   f.From,
			Letter: f.Letter,
			Label:  strings.ToUpper(statusLabels[f.Letter][:1]) + statusLabels[f.Letter][1:],
			Anchor: diffAnchor(rel),
		}
		if show == "all" {
			cf.Staged = inShow(f, "staged")
			cf.Unstaged = inShow(f, "unstaged")
		}
		if f.Letter != "D" {
			cf.Href = href(rel)
		}
		var df *diff.File
		if f.X == "?" {
			df = s.untrackedPatch(rel, cf)
		} else if df = patches[rel]; df == nil {
			cf.Notice = "No change in the content to show."
		}
		if df != nil {
			cf.Added, cf.Deleted = df.Stat()
			lines := 0
			for _, h := range df.Hunks {
				lines += len(h.Lines)
			}
			switch {
			case df.Binary:
				cf.Binary = true
				cf.Notice = "Binary file not shown."
			case len(df.Hunks) == 0 && df.OldMode != "":
				cf.Notice = "File mode changed from " + df.OldMode + " to " + df.NewMode + "."
			case len(df.Hunks) == 0:
				cf.Notice = "Empty file."
			case !v.One && (lines > maxDiffLines || lines > budget):
				cf.Load = changesHref(show, rel)
			default:
				budget -= lines
				cf.Diff = s.diffTable(st, which, df)
			}
		}
		cf.Blocks = statBlocks(cf.Added, cf.Deleted)
		v.Added += cf.Added
		v.Deleted += cf.Deleted
		v.Files = append(v.Files, cf)
	}
	s.render(w, p)
}

func changesHref(show, file string) string {
	q := url.Values{}
	if show != "all" {
		q.Set("show", show)
	}
	if file != "" {
		q.Set("file", file)
	}
	if len(q) == 0 {
		return "/_mini/changes"
	}
	return "/_mini/changes?" + q.Encode()
}

func diffAnchor(rel string) string {
	sum := sha256.Sum256([]byte(rel))
	return "diff-" + hex.EncodeToString(sum[:6])
}

// untrackedPatch makes the patch of a file git does not track: every line
// added. A binary file or one too large to show has none.
func (s *Server) untrackedPatch(rel string, cf *changedFile) *diff.File {
	df := &diff.File{NewPath: rel}
	info, err := s.ws.FS().Stat(rel)
	if err != nil || !info.Mode().IsRegular() {
		return df
	}
	if info.Size() > maxRender {
		cf.Notice = "This file is too large to show."
		return nil
	}
	b, err := fs.ReadFile(s.ws.FS().FS(), rel)
	if err != nil {
		return df
	}
	text, _, ok := decodeText(b)
	if !ok {
		df.Binary = true
		return df
	}
	lines := splitLines(string(text))
	if len(lines) == 0 {
		return df
	}
	h := diff.Hunk{NewStart: 1, NewLines: len(lines)}
	for i, l := range lines {
		h.Lines = append(h.Lines, diff.Line{Kind: diff.Added, Text: l, New: i + 1})
	}
	if !strings.HasSuffix(string(text), "\n") {
		h.Lines[len(h.Lines)-1].NoNewline = true
	}
	df.Hunks = []diff.Hunk{h}
	return df
}

// splitLines splits text into its lines, without their newlines.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// statBlocks are the five squares GitHub draws by a file's lines added
// and deleted: as many of each as their share, or one a line when fewer.
func statBlocks(added, deleted int) []string {
	g, r := added, deleted
	if t := added + deleted; t > 5 {
		g = (5*added + t/2) / t
		r = 5 - g
	}
	out := make([]string, 5)
	for i := range out {
		switch {
		case i < g:
			out[i] = "g"
		case i < g+r:
			out[i] = "r"
		}
	}
	return out
}

// sides reads the file before and after the change, to highlight them.
type sides struct {
	old, new         []template.HTML
	oldText, newText []string
}

func (s *Server) readSides(st *workspace.Status, which string, df *diff.File) sides {
	var sd sides
	read := func(rev, rel string) ([]byte, bool) {
		if rel == "" {
			return nil, false
		}
		if rev == "worktree" {
			info, err := s.ws.FS().Stat(rel)
			if err != nil || info.Size() > maxRender || !info.Mode().IsRegular() {
				return nil, false
			}
			b, err := fs.ReadFile(s.ws.FS().FS(), rel)
			return b, err == nil
		}
		b, ok := s.ws.Blob(st, rev, rel)
		return b, ok && len(b) <= maxRender
	}
	oldRev, newRev := "HEAD", "worktree"
	switch which {
	case workspace.DiffStaged:
		newRev = ""
	case workspace.DiffUnstaged:
		oldRev = ""
	}
	name := path.Base(df.Path())
	if b, ok := read(oldRev, df.OldPath); ok {
		sd.old, sd.oldText = highlightLines(name, b), splitLines(string(b))
	}
	if b, ok := read(newRev, df.NewPath); ok {
		sd.new, sd.newText = highlightLines(name, b), splitLines(string(b))
	}
	return sd
}

// lineHTML is a line of the diff, highlighted when the file read has the
// same line there: it may have changed since git read it.
func lineHTML(l diff.Line, hl []template.HTML, text []string, n int) string {
	if n > 0 && n <= len(hl) && n <= len(text) && text[n-1] == l.Text {
		return string(hl[n-1])
	}
	return template.HTMLEscapeString(l.Text)
}

// diffTable shows a file's patch as GitHub's unified view does: the two
// line numbers, then the line.
func (s *Server) diffTable(st *workspace.Status, which string, df *diff.File) template.HTML {
	sd := s.readSides(st, which, df)
	var b strings.Builder
	b.WriteString(`<table class="diff chroma">`)
	num := func(n int, cls string) {
		b.WriteString(`<td class="num` + cls + `">`)
		if n > 0 {
			b.WriteString(strconv.Itoa(n))
		}
		b.WriteString("</td>")
	}
	for _, h := range df.Hunks {
		b.WriteString(`<tr class="hunk"><td class="num"></td><td class="num"></td><td class="text">`)
		b.WriteString(template.HTMLEscapeString("@@ -" + strconv.Itoa(h.OldStart) + "," + strconv.Itoa(h.OldLines) +
			" +" + strconv.Itoa(h.NewStart) + "," + strconv.Itoa(h.NewLines) + " @@ " + h.Section))
		b.WriteString("</td></tr>")
		for _, l := range h.Lines {
			var cls, sign, text string
			switch l.Kind {
			case diff.Added:
				cls, sign, text = " add", "+", lineHTML(l, sd.new, sd.newText, l.New)
			case diff.Deleted:
				cls, sign, text = " del", "-", lineHTML(l, sd.old, sd.oldText, l.Old)
			default:
				cls, sign, text = "", " ", lineHTML(l, sd.new, sd.newText, l.New)
			}
			b.WriteString(`<tr>`)
			num(l.Old, cls)
			num(l.New, cls)
			b.WriteString(`<td class="text` + cls + `"><span class="sign">` + sign + `</span>` + text)
			if l.NoNewline {
				b.WriteString(`<span class="no-newline" title="No newline at end of file">⏎</span>`)
			}
			b.WriteString("</td></tr>")
		}
	}
	b.WriteString("</table>")
	return template.HTML(b.String())
}
