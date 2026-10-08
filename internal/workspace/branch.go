package workspace

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// BranchInfo is where the branch checked out stands against its base, the
// default branch of origin, and against its own branch on origin.
type BranchInfo struct {
	// Branch is the branch checked out, or on a detached HEAD, the tag
	// there or the commit, read with the rest: the snapshot's may be
	// older.
	Branch string `json:"branch"`
	// Base is the base's name, such as "main".
	Base string `json:"base"`
	// OnBase is set when the base itself is checked out.
	OnBase bool `json:"onBase"`
	// Detached is set when HEAD is no branch, as at a tag.
	Detached bool `json:"detached"`
	// MergeBase is the commit where HEAD left the base, abbreviated.
	MergeBase string `json:"mergeBase"`
	// Ahead counts the commits HEAD has that the base has not, Behind
	// those the base has that HEAD has not. The base is origin's, as of
	// the last fetch.
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Upstream is the branch on a remote the branch follows, such as
	// "origin/feature", or "" when it has none, and Unpushed counts the
	// commits it has not.
	Upstream string `json:"upstream,omitempty"`
	Unpushed int    `json:"unpushed"`
	// mergeBase is MergeBase in full, and head HEAD's commit
	mergeBase string
	head      string
}

// Head is HEAD's commit.
func (b *BranchInfo) Head() string {
	return b.head
}

// gitBase finds the base: the branch origin/HEAD names, the default
// branch of origin as cloned, or else origin's main or master. origin/HEAD
// may name a branch gone since, renamed on origin and pruned here. Only
// the refs are read, not the remote's configuration.
func gitBase(dir string) (name, ref string) {
	if ref := gitOutput(dir, "symbolic-ref", "-q", "refs/remotes/origin/HEAD"); ref != "" && gitOutput(dir, "rev-parse", "--verify", "-q", ref) != "" {
		return strings.TrimPrefix(ref, "refs/remotes/origin/"), ref
	}
	for _, name := range []string{"main", "master"} {
		ref := "refs/remotes/origin/" + name
		if gitOutput(dir, "rev-parse", "--verify", "-q", ref) != "" {
			return name, ref
		}
	}
	return "", ""
}

// gitBranchInfo reads where HEAD stands, or nil without a base or a commit.
func gitBranchInfo(dir string) *BranchInfo {
	name, ref := gitBase(dir)
	if name == "" {
		return nil
	}
	mb := gitOutput(dir, "merge-base", ref, "HEAD")
	if mb == "" {
		// No commit yet, or histories that never met
		return nil
	}
	b := &BranchInfo{Base: name, mergeBase: mb, MergeBase: mb[:min(len(mb), 7)]}
	branch := gitOutput(dir, "symbolic-ref", "-q", "--short", "HEAD")
	b.Branch = gitBranch(dir)
	b.head = gitOutput(dir, "rev-parse", "HEAD")
	b.Detached = branch == ""
	b.OnBase = branch == name
	if ahead, behind, ok := strings.Cut(gitOutput(dir, "rev-list", "--left-right", "--count", "HEAD..."+ref), "\t"); ok {
		b.Ahead, _ = strconv.Atoi(ahead)
		b.Behind, _ = strconv.Atoi(behind)
	}
	if !b.Detached {
		b.Upstream = gitOutput(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
		if b.Upstream != "" {
			b.Unpushed, _ = strconv.Atoi(gitOutput(dir, "rev-list", "--count", "@{upstream}..HEAD"))
		}
	}
	return b
}

// Branch returns where the branch checked out stands against its base,
// or nil outside a repository, without a base, or with changes off.
func (w *Workspace) Branch() *BranchInfo {
	if !w.git {
		return nil
	}
	return w.branches.get(w.opts.Root)
}

// branchCache keeps the branch's standing while the refs stay as they
// are, read as the commits' are, and the branch checked out: the refs
// tell HEAD's commit only, the same for a branch just made from another.
// Reading the refs again on every page picks up a fetch or a push the
// watcher did not see.
type branchCache struct {
	mu   sync.Mutex
	key  string
	info *BranchInfo
}

func (c *branchCache) get(dir string) *BranchInfo {
	key := gitOutput(dir, "show-ref", "--head") + "\x00" + gitOutput(dir, "symbolic-ref", "-q", "HEAD")
	c.mu.Lock()
	defer c.mu.Unlock()
	if key != c.key {
		c.key, c.info = key, gitBranchInfo(dir)
	}
	return c.info
}

// BranchCommits lists the commits since the branch left its base, the
// latest first, as many as limit, each telling whether a remote branch
// has it.
func (w *Workspace) BranchCommits(limit int) []Commit {
	b := w.Branch()
	if b == nil || b.OnBase {
		return nil
	}
	dir := w.opts.Root
	out := gitOutput(dir, "log", "-n", strconv.Itoa(limit), "--format=%H%x00%an%x00%ae%x00%at%x00%s", b.mergeBase+"..HEAD")
	if out == "" {
		return nil
	}
	unpushed := map[string]bool{}
	for _, sha := range strings.Fields(gitOutput(dir, "rev-list", b.mergeBase+"..HEAD", "--not", "--remotes")) {
		unpushed[sha] = true
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\x00", 5)
		if len(f) != 5 {
			continue
		}
		sec, _ := strconv.ParseInt(f[3], 10, 64)
		commits = append(commits, Commit{SHA: f[0], Author: f[1], Email: f[2], Time: time.Unix(sec, 0), Subject: f[4], Pushed: !unpushed[f[0]]})
	}
	return commits
}
