package config

import (
	"os"
	"path/filepath"
	"testing"
)

// write puts a config.yaml under a fresh XDG_CONFIG_HOME and points the
// process at it.
func write(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "recdep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recdep", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadValid(t *testing.T) {
	write(t, `actions:
  - source: github
    name_contains: -review-requested-
    action: review
  - source: slack
    who_suffix: "[bot]"
    action: respond
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want := Config{Actions: []Action{
		{Source: "github", NameContains: "-review-requested-", Action: "review"},
		{Source: "slack", WhoSuffix: "[bot]", Action: "respond"},
	}}
	if len(c.Actions) != len(want.Actions) {
		t.Fatalf("Load() = %+v, want %+v", c, want)
	}
	for i := range want.Actions {
		if c.Actions[i] != want.Actions[i] {
			t.Errorf("actions[%d] = %+v, want %+v", i, c.Actions[i], want.Actions[i])
		}
	}
}

func TestLoadAbsentFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(c.Actions) != 0 {
		t.Errorf("Load() = %+v, want zero value", c)
	}
}

func TestLoadMalformed(t *testing.T) {
	write(t, "actions: [\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want parse error")
	}
}

func TestLoadUnknownKey(t *testing.T) {
	write(t, "actions:\n  - sourec: github\n    action: review\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want unknown-field error")
	}
}

func TestLoadMissingAction(t *testing.T) {
	write(t, "actions:\n  - source: slack\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want required-field error")
	}
}
