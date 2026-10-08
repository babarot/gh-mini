package version

import (
	"runtime/debug"
	"testing"
)

func TestFormat(t *testing.T) {
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "80bb60c1234567890abcdef"}
	for _, tt := range []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"no revision", nil, "1.2.3"},
		{"revision", []debug.BuildSetting{rev, {Key: "vcs.modified", Value: "false"}}, "1.2.3 (80bb60c)"},
		{"dirty", []debug.BuildSetting{rev, {Key: "vcs.modified", Value: "true"}}, "1.2.3 (80bb60c-dirty)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := format("1.2.3", tt.settings); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
