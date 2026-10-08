package server

import (
	"encoding/json"
	"strings"

	"github.com/babarot/gh-mini/internal/workspace"
)

// changesView is what changed since the last commit, as the top bar counts
// it. It is set in a git repository only, with nothing changed too, so
// that the page can show it once something does.
type changesView struct {
	Files   int
	Added   int
	Deleted int
	// ETag tells which status the page was rendered with, without the
	// quotes of the header
	ETag string
}

func newChangesView(st *workspace.Status, etag string) *changesView {
	if st == nil {
		return nil
	}
	return &changesView{Files: len(st.Files), Added: st.Added, Deleted: st.Deleted, ETag: strings.Trim(etag, `"`)}
}

// statusFor is what changed since the last commit as the viewer's settings
// show it, encoded, and an ETag of that: nil outside a git repository and
// with Changes off, and without untracked files when those are left out.
func (s *Server) statusFor(snap *workspace.Snapshot, settings map[string]string) (*workspace.Status, []byte, string) {
	st := snap.Status
	if st == nil || !st.Git || settings["changes"] == "false" {
		return nil, nil, ""
	}
	if settings["untracked"] != "false" {
		return st, snap.StatusJSON, snap.StatusETag
	}
	st = st.WithoutUntracked()
	b, _ := json.Marshal(st)
	return st, b, strings.TrimSuffix(snap.StatusETag, `"`) + `-tracked"`
}

// statusLabels name the letters of workspace.FileStatus.
var statusLabels = map[string]string{
	"M": "modified",
	"A": "added",
	"D": "deleted",
	"R": "renamed",
	"U": "untracked",
	"C": "conflict",
}

// entryStatus is how a file listed in a directory changed, or for a
// directory, how many files in it did and by how many lines.
type entryStatus struct {
	Letter  string
	Label   string
	Files   int
	Added   int
	Deleted int
	Binary  bool
}

// childStatus sums up the status by the entries of the directory rel: a
// file in it, or a directory with the files under it. The names are those
// of the entries, some of which may be gone, as a file deleted is.
func childStatus(st *workspace.Status, rel string) map[string]*entryStatus {
	out := map[string]*entryStatus{}
	if st == nil {
		return out
	}
	prefix := ""
	if rel != "." {
		prefix = rel + "/"
	}
	for p, f := range st.Files {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok {
			continue
		}
		name, _, dir := strings.Cut(rest, "/")
		e := out[name]
		if e == nil {
			e = &entryStatus{}
			out[name] = e
		}
		e.Files++
		e.Added += f.Added
		e.Deleted += f.Deleted
		if !dir {
			e.Letter, e.Label, e.Binary = f.Letter, statusLabels[f.Letter], f.Binary
		}
	}
	return out
}
