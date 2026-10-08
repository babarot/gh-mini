package workspace

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// emptyTree is git's tree with nothing in it, what a repository with no
// commit yet is compared with.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

const (
	// maxCounted is how many untracked files have their lines counted;
	// the rest are listed without them.
	maxCounted = 2000
	// maxCountSize is the largest untracked file whose lines are counted.
	maxCountSize = 1 << 20
)

// Status is what changed in the working tree since HEAD, or on a branch,
// since it left its base: the commits since and what is not committed.
type Status struct {
	// Git is false outside a git repository, where nothing is listed.
	Git   bool                   `json:"git"`
	Files map[string]*FileStatus `json:"files"`
	// Added and Deleted are the lines of all the files.
	Added   int `json:"added"`
	Deleted int `json:"deleted"`
	// Base is the base the changes are counted from, such as "main", or
	// "" for those since HEAD.
	Base string `json:"base,omitempty"`
	// head is set when HEAD has a commit to compare with.
	head bool
	// rev is the commit compared with: HEAD, where the branch left the
	// base, or the empty tree before the first commit
	rev string
}

// Rev is the commit the working tree is compared with.
func (st *Status) Rev() string {
	return st.rev
}

// FileStatus is how a file differs from HEAD, as git status tells it.
type FileStatus struct {
	// X and Y are git's status letters for the index and for the working
	// tree: M, A, D, R, C or T, or . for no change. Both are ? for a file
	// git does not track.
	X string `json:"x"`
	Y string `json:"y"`
	// Letter is the one letter that tells how the file changed, as a
	// file tree shows it: M modified, A added, D deleted, R renamed, U
	// untracked, or C in a conflict.
	Letter string `json:"letter"`
	// Unmerged is set for a file in a merge conflict.
	Unmerged bool `json:"unmerged,omitempty"`
	// From is the old path of a file renamed or copied.
	From string `json:"from,omitempty"`
	// Added and Deleted count lines, none for a binary file or an
	// untracked one past the files counted.
	Added   int  `json:"added"`
	Deleted int  `json:"deleted"`
	Binary  bool `json:"binary,omitempty"`
	// Uncounted is set on an untracked file whose lines were not counted.
	Uncounted bool `json:"uncounted,omitempty"`
	// Committed and Uncommitted tell, of the changes since the base,
	// whether a commit since has some and the working tree some more.
	Committed   bool `json:"committed,omitempty"`
	Uncommitted bool `json:"uncommitted,omitempty"`
}

// WithoutUntracked is the status with the files git does not track left
// out, and the lines counted without them.
func (st *Status) WithoutUntracked() *Status {
	out := *st
	out.Files = make(map[string]*FileStatus, len(st.Files))
	out.Added, out.Deleted = 0, 0
	for p, f := range st.Files {
		if f.X == "?" {
			continue
		}
		out.Files[p] = f
		out.Added += f.Added
		out.Deleted += f.Deleted
	}
	return &out
}

func (f *FileStatus) letter() string {
	switch {
	case f.Unmerged:
		return "C"
	case f.X == "?":
		return "U"
	case f.Y == "D":
		// Gone from the working tree, whatever the index has
		return "D"
	}
	l := f.X
	if l == "." {
		l = f.Y
	}
	switch l {
	case "T":
		return "M"
	case "C":
		// A copy is a file added
		return "A"
	}
	return l
}

// gitPrefix is where dir is in its repository, such as "docs/", or "" at
// its top; ok is false outside a repository.
func gitPrefix(dir string) (prefix string, ok bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-prefix").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// skipPathspecs keeps names left out of the tree out of what git looks
// at, so that git does not go through a large directory only for it to
// be dropped.
func skipPathspecs(skip []string) []string {
	var specs []string
	for _, name := range skip {
		if name == ".git" {
			continue
		}
		name = globEscape(name)
		specs = append(specs, ":(exclude,glob)**/"+name, ":(exclude,glob)**/"+name+"/**")
	}
	return specs
}

func globEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`*?[]\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// gitStatus reads what changed under root, which is prefix in its
// repository. An untracked directory is one entry to git; untracked
// lists the files under one, from the tree.
func gitStatus(root, prefix string, skip []string, untracked func(dir string) []string) *Status {
	st := &Status{Git: true, Files: map[string]*FileStatus{}}
	specs := append([]string{"."}, skipPathspecs(skip)...)
	// No optional locks: a status that refreshed the index would write it,
	// and the watcher would see the write and ask again
	out, err := exec.Command("git", append([]string{"--no-optional-locks", "-C", root,
		"status", "--porcelain=v2", "-z", "--branch", "--renames",
		"--ignore-submodules=none", "--untracked-files=normal", "--"}, specs...)...).Output()
	if err != nil {
		return st
	}
	head := parseStatus(out, prefix, st.Files)
	var dirs []string
	for p, f := range st.Files {
		if f.X == "?" && strings.HasSuffix(p, "/") {
			delete(st.Files, p)
			dirs = append(dirs, strings.TrimSuffix(p, "/"))
		}
	}
	for _, d := range dirs {
		// Another repository in this one is untracked as a whole: its
		// files are its own
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(d), ".git")); err == nil {
			continue
		}
		for _, p := range untracked(d) {
			st.Files[p] = &FileStatus{X: "?", Y: "?"}
		}
	}

	st.head = head
	st.rev = "HEAD"
	if !head {
		st.rev = emptyTree
	}
	out, err = exec.Command("git", append([]string{"--no-optional-locks", "-C", root,
		"diff-index", "--numstat", "-z", "-M", "--relative", st.rev, "--"}, specs...)...).Output()
	if err == nil {
		addNumstat(out, st.Files)
	}
	countUntracked(root, st.Files)
	for _, f := range st.Files {
		f.Letter = f.letter()
		st.Added += f.Added
		st.Deleted += f.Deleted
	}
	return st
}

// gitBaseStatus reads what changed under root since the branch left its
// base at mb, as a pull request shows it: the working tree against mb,
// with the files git does not track yet. head is what changed since the
// last commit, which tells the changes not committed. A file committed
// and then changed back has no change to show, and is not listed.
func gitBaseStatus(root, base, mb string, skip []string, head *Status) *Status {
	st := &Status{Git: true, Files: map[string]*FileStatus{}, Base: base, head: true, rev: mb}
	specs := append([]string{"."}, skipPathspecs(skip)...)
	git := func(args ...string) ([]byte, error) {
		return exec.Command("git", append(append([]string{"--no-optional-locks", "-C", root}, args...), append([]string{"--"}, specs...)...)...).Output()
	}
	out, err := git("diff-index", "--name-status", "-z", "-M", "--relative", mb)
	if err != nil {
		return nil
	}
	parseNameStatus(out, st.Files)
	if out, err := git("diff-index", "--numstat", "-z", "-M", "--relative", mb); err == nil {
		addNumstat(out, st.Files)
	}
	committed := map[string]bool{}
	// Without renames found, so that a file renamed has both its names
	if out, err := git("diff-tree", "-r", "--name-only", "-z", "--relative", mb, "HEAD"); err == nil {
		for _, p := range strings.Split(string(out), "\x00") {
			committed[p] = p != ""
		}
	}
	for p, f := range head.Files {
		if f.X == "?" {
			c := *f
			st.Files[p] = &c
		}
	}
	// diff-index tells a file changed by its stat in the index alone, and
	// gh-mini never refreshes the index: a file saved again as it was is
	// listed, which git status, comparing contents, does not list. A file
	// differs from mb only if a commit since or the working tree changed it
	for p := range st.Files {
		if !committed[p] && head.Files[p] == nil {
			delete(st.Files, p)
		}
	}
	for p, f := range st.Files {
		f.Committed = committed[p]
		f.X, f.Y = ".", "."
		if hf := head.Files[p]; hf != nil {
			f.X, f.Y, f.Unmerged, f.Uncommitted = hf.X, hf.Y, hf.Unmerged, true
		}
		if f.Unmerged {
			f.Letter = "C"
		}
		st.Added += f.Added
		st.Deleted += f.Deleted
	}
	return st
}

// parseNameStatus reads git diff-index --name-status -z into files, by
// the letters a file tree shows. A rename or a copy comes with a score,
// as R100, and the old path before the new.
func parseNameStatus(out []byte, files map[string]*FileStatus) {
	recs := strings.Split(string(out), "\x00")
	for i := 0; i+1 < len(recs); i += 2 {
		l, p := recs[i], recs[i+1]
		if l == "" {
			break
		}
		f := &FileStatus{}
		switch l[0] {
		case 'R', 'C':
			if i+2 >= len(recs) {
				return
			}
			f.From, p = p, recs[i+2]
			i++
			f.Letter = "R"
			if l[0] == 'C' {
				// A copy is a file added
				f.Letter = "A"
			}
		case 'T':
			f.Letter = "M"
		case 'A', 'D', 'M':
			f.Letter = l[:1]
		default:
			// U, an unmerged path, which the status tells
			f.Letter = "M"
		}
		files[p] = f
	}
}

// parseStatus reads git status --porcelain=v2 -z --branch into files,
// with paths made relative to root, which is prefix in the repository,
// and tells whether HEAD has a commit.
func parseStatus(out []byte, prefix string, files map[string]*FileStatus) (head bool) {
	rel := func(p string) (string, bool) {
		if !strings.HasPrefix(p, prefix) {
			return "", false
		}
		return p[len(prefix):], true
	}
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		r := recs[i]
		switch {
		case strings.HasPrefix(r, "# branch.oid "):
			head = r != "# branch.oid (initial)"
		case strings.HasPrefix(r, "1 "):
			// 1 XY sub mH mI mW hH hI path
			f := strings.SplitN(r, " ", 9)
			if p, ok := rel(f[8]); ok && len(f) == 9 {
				files[p] = &FileStatus{X: f[1][:1], Y: f[1][1:]}
			}
		case strings.HasPrefix(r, "2 "):
			// 2 XY sub mH mI mW hH hI Xscore path, then the old path
			f := strings.SplitN(r, " ", 10)
			i++
			if p, ok := rel(f[len(f)-1]); ok && len(f) == 10 && i < len(recs) {
				fs := &FileStatus{X: f[1][:1], Y: f[1][1:]}
				if from, ok := rel(recs[i]); ok {
					fs.From = from
				}
				files[p] = fs
			}
		case strings.HasPrefix(r, "u "):
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			f := strings.SplitN(r, " ", 11)
			if p, ok := rel(f[len(f)-1]); ok && len(f) == 11 {
				files[p] = &FileStatus{X: f[1][:1], Y: f[1][1:], Unmerged: true}
			}
		case strings.HasPrefix(r, "? "):
			if p, ok := rel(r[2:]); ok {
				files[p] = &FileStatus{X: "?", Y: "?"}
			}
		}
	}
	return head
}

// addNumstat adds the lines of git diff-index --numstat -z to the files it
// lists. A rename comes as "added\tdeleted\t" with the old and new paths
// in the two fields after it. A rename the index has may come as a
// deletion and an addition instead, as the working tree is compared.
func addNumstat(out []byte, files map[string]*FileStatus) {
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		f := strings.SplitN(recs[i], "\t", 3)
		if len(f) != 3 {
			continue
		}
		p := f[2]
		if p == "" && i+2 < len(recs) {
			p = recs[i+2]
			i += 2
		}
		fs := files[p]
		if fs == nil {
			continue
		}
		if f[0] == "-" {
			fs.Binary = true
			continue
		}
		fs.Added, _ = strconv.Atoi(f[0])
		fs.Deleted, _ = strconv.Atoi(f[1])
	}
}

// countUntracked counts the lines of untracked files, as many as
// maxCounted; a large file or one with a NUL is taken for binary.
func countUntracked(root string, files map[string]*FileStatus) {
	n := 0
	for p, f := range files {
		if f.X != "?" {
			continue
		}
		if n >= maxCounted {
			f.Uncounted = true
			continue
		}
		n++
		lines, binary := countFile(filepath.Join(root, filepath.FromSlash(p)))
		f.Added, f.Binary = lines, binary
	}
}

func countFile(name string) (lines int, binary bool) {
	file, err := os.Open(name)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	if fi, err := file.Stat(); err != nil || !fi.Mode().IsRegular() {
		return 0, false
	} else if fi.Size() > maxCountSize {
		return 0, true
	}
	b, err := io.ReadAll(file)
	if err != nil {
		return 0, false
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return 0, true
	}
	lines = bytes.Count(b, []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		lines++
	}
	return lines, false
}

// untrackedFiles lists the files in the tree under dir, but for those git
// ignores.
func (s *Snapshot) untrackedFiles(dir string) []string {
	n, ok := s.dirs[dir]
	if !ok {
		return nil
	}
	var out []string
	var walk func(n *Node)
	walk = func(n *Node) {
		for _, c := range n.Children {
			switch {
			case c.Ignored:
			case c.Dir && !c.Lazy:
				walk(c)
			default:
				// A file, or a symlink to a directory, which git takes
				// for a file
				out = append(out, c.Path)
			}
		}
	}
	walk(n)
	return out
}
