package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxgio92/telescreen/pkg/cmd/root"
)

// TestDocsGenerates runs the docs command against a temp dir and checks
// the root page lands without the date-stamped footer, so a cobra
// upgrade flipping DisableAutoGenTag semantics fails here first.
func TestDocsGenerates(t *testing.T) {
	dir := t.TempDir()
	cmd := root.New()
	cmd.SetArgs([]string{"docs", "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("docs: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "telescreen.md"))
	if err != nil {
		t.Fatalf("root page: %v", err)
	}
	page := string(b)
	if strings.Contains(page, "Auto generated") {
		t.Error("root page carries the auto-generated footer; the reference would churn on every run")
	}
	if strings.Contains(page, "telescreen docs") {
		t.Error("the hidden docs command leaked into the generated tree")
	}
	for _, sub := range []string{"install", "minitrue", "speakwrite", "thinkpol", "export", "verify", "version"} {
		if !strings.Contains(page, "telescreen "+sub) {
			t.Errorf("root page misses subcommand %q", sub)
		}
	}
}
