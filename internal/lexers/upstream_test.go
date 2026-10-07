package lexers

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// upstream holds the SHA-256 of chroma's lexer each file here is made from.
// When chroma changes one, carry its change over into the file here and
// update the hash, along with the version in the file's comment.
var upstream = map[string]string{
	"bash.xml": "1c6a428f37ade5adddbb7a30e0f2c3699039168527b59e6136018f618ee806d0",
	"nix.xml":  "c1d41400345d3e48933756683c6fdfa2c943cbc250d542a6af9a9881ed3805db",
}

func TestUpstreamUnchanged(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/alecthomas/chroma/v2").Output()
	if err != nil {
		t.Fatalf("finding chroma: %v", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "lexers", "embedded")

	paths, _ := filepath.Glob("*.xml")
	for _, p := range paths {
		want, ok := upstream[p]
		if !ok {
			t.Errorf("%s: no hash of chroma's lexer recorded", p)
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Errorf("%s: chroma has no such lexer: %v", p, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("chroma's %s changed (sha256 %s); carry the change over into %s", p, got, p)
		}
	}
}
