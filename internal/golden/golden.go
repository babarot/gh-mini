// Package golden compares test output with files kept in testdata. Run
// the tests with UPDATE_GOLDEN=1 to write the files from the current
// output after a deliberate change, and read their diff before committing
// it. It is an environment variable rather than a flag so that
// go test ./... passes it to the packages without golden files too.
package golden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Check compares got with the file at path, or writes it there with
// UPDATE_GOLDEN=1.
func Check(t *testing.T, path string, got []byte) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run UPDATE_GOLDEN=1 go test to write it)", err)
	}
	if string(got) == string(want) {
		return
	}
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := range max(len(g), len(w)) {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			t.Fatalf("%s differs from line %d:\n got: %s\nwant: %s\nRun UPDATE_GOLDEN=1 go test and read the diff if the change is meant.", path, i+1, gl, wl)
		}
	}
}
