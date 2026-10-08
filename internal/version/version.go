// Package version holds the release version. tagpr updates it.
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version is the current release.
const Version = "0.1.0"

// String is the version with the commit it was built from, when the build
// knows it: a build in a clone does. go install does not, but it tells the
// version of the module installed, a tag or one naming the commit.
func String() string {
	return format(Version, buildInfo())
}

// Revision is the hash of the commit built from, or "": the full one from
// a clone, the short one in the version of a module installed untagged.
func Revision() string {
	info := buildInfo()
	if rev := setting(info, "vcs.revision"); rev != "" {
		return rev
	}
	if m := pseudoRevision.FindStringSubmatch(info.Main.Version); m != nil {
		return m[1]
	}
	return ""
}

// pseudoRevision is the commit at the end of a pseudo-version, as in
// v0.0.0-20261008123456-b78655f1abcd.
var pseudoRevision = regexp.MustCompile(`[-.]\d{14}-([0-9a-f]{12})(?:\+incompatible)?$`)

func buildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return &debug.BuildInfo{}
	}
	return info
}

func format(version string, info *debug.BuildInfo) string {
	rev := setting(info, "vcs.revision")
	if rev == "" {
		// Installed with go install, the module's version is known
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
		return version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if setting(info, "vcs.modified") == "true" {
		rev += "-dirty"
	}
	return version + " (" + rev + ")"
}

func setting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
