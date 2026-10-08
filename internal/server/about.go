package server

import (
	"os"
	"path/filepath"
	"strings"
)

const repoURL = "https://github.com/babarot/gh-mini"

// aboutView is what the About dialog shows.
type aboutView struct {
	Version string
	// CommitURL links the commit built from, when the build knows it
	CommitURL string
	// Serving is the root, under ~ when in the home directory
	Serving string
	RepoURL string
}

func (s *Server) about() aboutView {
	a := aboutView{Version: s.opts.Version, Serving: s.serving, RepoURL: repoURL}
	if s.opts.Revision != "" {
		a.CommitURL = repoURL + "/commit/" + s.opts.Revision
	}
	return a
}

// homeDir is the home directory with its symlinks resolved, as the root
// is, or "" when there is none.
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	return home
}

// shortenHome writes root under home as ~/..., so that the About dialog
// does not show the full path to whoever reaches the server.
func shortenHome(root, home string) string {
	if home == "" || home == string(filepath.Separator) {
		return root
	}
	if root == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(root, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return root
}
