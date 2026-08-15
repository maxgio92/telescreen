package demo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxgio92/telescreen/internal/recdep"
)

func TestSeed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, err := recdep.StateRoot()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 9, 5, 0, 0, time.UTC)
	if err := seed(root, now); err != nil {
		t.Fatal(err)
	}
	name := now.Format(recdep.StampLayout) + suffix
	body, err := os.ReadFile(filepath.Join(root, "tube", name))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(body), "\n")
	want := "[github] julia: review requested on demo#42: feat(ministry): ration the chocolate"
	if first != want {
		t.Errorf("first line = %q, want %q", first, want)
	}
	info, err := os.Stat(filepath.Join(root, "tube", name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("record mode = %v, want 0600", info.Mode().Perm())
	}

	if err := seed(root, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := countDemo(t, root, "tube"); n != 1 {
		t.Errorf("after re-seed: %d demo records in tube, want 1", n)
	}
}

func TestSeedSkipsWhenFiled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, err := recdep.StateRoot()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 9, 5, 0, 0, time.UTC)
	filed := filepath.Join(root, "files", now.Format(recdep.StampLayout)+suffix)
	if err := os.WriteFile(filed, []byte("moved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := seed(root, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := countDemo(t, root, "tube"); n != 0 {
		t.Errorf("seed with a filed copy: %d demo records in tube, want 0", n)
	}
}

func countDemo(t *testing.T, root, drawer string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, drawer))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			n++
		}
	}
	return n
}
