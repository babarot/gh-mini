// Package version holds the release version. tagpr updates it.
package version

import "runtime/debug"

// Version is the current release.
const Version = "0.0.0"

// String is the version with the commit it was built from, when the build
// knows it: a build in a clone does, go install of a release does not.
func String() string {
	return format(Version, buildSettings())
}

// Revision is the full hash of the commit built from, or "".
func Revision() string {
	return setting(buildSettings(), "vcs.revision")
}

func buildSettings() []debug.BuildSetting {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info.Settings
}

func format(version string, settings []debug.BuildSetting) string {
	rev := setting(settings, "vcs.revision")
	if rev == "" {
		return version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if setting(settings, "vcs.modified") == "true" {
		rev += "-dirty"
	}
	return version + " (" + rev + ")"
}

func setting(settings []debug.BuildSetting, key string) string {
	for _, s := range settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
