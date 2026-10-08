package version

import (
	"runtime/debug"
	"testing"
)

func TestFormat(t *testing.T) {
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "80bb60c1234567890abcdef"}
	for _, tt := range []struct {
		name     string
		module   string
		settings []debug.BuildSetting
		want     string
	}{
		{"no revision", "", nil, "1.2.3"},
		{"revision", "(devel)", []debug.BuildSetting{rev, {Key: "vcs.modified", Value: "false"}}, "1.2.3 (80bb60c)"},
		{"dirty", "(devel)", []debug.BuildSetting{rev, {Key: "vcs.modified", Value: "true"}}, "1.2.3 (80bb60c-dirty)"},
		{"installed from a tag", "v1.4.0", nil, "1.4.0"},
		{"installed untagged", "v0.0.0-20261008123456-b78655f1abcd", nil, "0.0.0-20261008123456-b78655f1abcd"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := &debug.BuildInfo{Main: debug.Module{Version: tt.module}, Settings: tt.settings}
			if got := format("1.2.3", info); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPseudoRevision(t *testing.T) {
	for v, want := range map[string]string{
		"v0.0.0-20261008123456-b78655f1abcd":              "b78655f1abcd",
		"v1.2.4-0.20261008123456-b78655f1abcd":            "b78655f1abcd",
		"v2.0.0-20261008123456-b78655f1abcd+incompatible": "b78655f1abcd",
		"v1.4.0":  "",
		"(devel)": "",
	} {
		got := ""
		if m := pseudoRevision.FindStringSubmatch(v); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", v, got, want)
		}
	}
}
