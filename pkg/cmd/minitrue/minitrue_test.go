package minitrue

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/maxgio92/telescreen/internal/agentrun"
)

// fakeExec swaps agentrun.Exec for a recorder and returns a pointer to
// the recorded invocation.
func fakeExec(t *testing.T) *agentrun.Invocation {
	t.Helper()
	var got agentrun.Invocation
	orig := agentrun.Exec
	agentrun.Exec = func(inv agentrun.Invocation) error {
		got = inv
		return nil
	}
	t.Cleanup(func() { agentrun.Exec = orig })
	return &got
}

// isolate points HOME and the state root into temp dirs and returns them.
func isolate(t *testing.T) (home, state string) {
	t.Helper()
	home = t.TempDir()
	state = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", state)
	return home, state
}

func TestRunDefaults(t *testing.T) {
	_, state := isolate(t)
	got := fakeExec(t)

	if err := run(); err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "-p", defaultPrompt, "--allowedTools", defaultTools}
	if !slices.Equal(got.Argv, want) {
		t.Errorf("argv = %v, want %v", got.Argv, want)
	}
	if got.Timeout != 600*time.Second {
		t.Errorf("timeout = %v, want 600s", got.Timeout)
	}
	if want := filepath.Join(state, "recdep", "produce.log"); got.Log != want {
		t.Errorf("log = %q, want %q", got.Log, want)
	}
	for _, s := range []string{"tube", "desk", "upsub", "files"} {
		if _, err := os.Stat(filepath.Join(state, "recdep", s)); err != nil {
			t.Errorf("state dir %s missing: %v", s, err)
		}
	}
}

func TestRunHonorsEnvFile(t *testing.T) {
	home, _ := isolate(t)
	got := fakeExec(t)
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "MINITRUE_AGENT=myagent\nMINITRUE_PROMPT=/other produce\nMINITRUE_ALLOWED_TOOLS=\"Read Grep\"\nMINITRUE_TIMEOUT=42\nSLACK_USER_ID=U123\n"
	if err := os.WriteFile(filepath.Join(home, ".config", envFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(); err != nil {
		t.Fatal(err)
	}
	want := []string{"myagent", "-p", "/other produce", "--allowedTools", "Read Grep"}
	if !slices.Equal(got.Argv, want) {
		t.Errorf("argv = %v, want %v", got.Argv, want)
	}
	if got.Timeout != 42*time.Second {
		t.Errorf("timeout = %v, want 42s", got.Timeout)
	}
	if !slices.Contains(got.Env, "SLACK_USER_ID=U123") {
		t.Errorf("env %v lacks the exported identity variable", got.Env)
	}
}
