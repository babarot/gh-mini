package workspace

import (
	"os/exec"
)

// Which side of the changes a diff shows.
const (
	// DiffAll is the working tree against HEAD, staged or not.
	DiffAll = "all"
	// DiffStaged is the index against HEAD, what a commit would take.
	DiffStaged = "staged"
	// DiffUnstaged is the working tree against the index.
	DiffUnstaged = "unstaged"
)

// Diff returns the patches of the files changed under the root, or of the
// files at paths, as git diff gives them. Untracked files are not in it.
// With ignoreSpace, changes of whitespace alone are left out, as git diff
// -w leaves them.
func (w *Workspace) Diff(st *Status, which string, ignoreSpace bool, paths ...string) []byte {
	if !w.git {
		return nil
	}
	base := st.rev
	// Paths names are taken literally; the root's own pathspecs keep what
	// is skipped out
	args := []string{"--no-optional-locks", "-c", "core.quotePath=false", "-C", w.opts.Root}
	specs := append([]string{"."}, skipPathspecs(w.opts.Skip)...)
	if len(paths) > 0 {
		args = append(args, "--literal-pathspecs")
		specs = paths
	}
	// Plumbing, which reads no diff.* settings of the user's, such as
	// diff.noprefix
	diff := []string{"-p", "-M", "--relative", "--no-color", "--no-ext-diff"}
	if ignoreSpace {
		diff = append(diff, "--ignore-all-space")
	}
	switch which {
	case DiffStaged:
		args = append(append(append(args, "diff-index", "--cached"), diff...), base)
	case DiffUnstaged:
		args = append(append(args, "diff-files"), diff...)
	default:
		args = append(append(append(args, "diff-index"), diff...), base)
	}
	out, err := exec.Command("git", append(append(args, "--"), specs...)...).Output()
	if err != nil {
		return nil
	}
	return out
}

// Blob returns a file under the root as a commit has it, such as the one
// the status compares with, or as the index has it for rev "".
func (w *Workspace) Blob(st *Status, rev, rel string) ([]byte, bool) {
	if !w.git || rev == emptyTree || (rev == "HEAD" && !st.head) {
		return nil, false
	}
	// ./ makes the path relative to the root rather than the repository
	out, err := exec.Command("git", "--no-optional-locks", "-C", w.opts.Root,
		"cat-file", "blob", rev+":./"+rel).Output()
	if err != nil {
		return nil, false
	}
	return out, true
}
