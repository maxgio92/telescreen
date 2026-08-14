package speakwrite

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/maxgio92/telescreen/internal/agentrun"
)

func TestHasIntent(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  bool
	}{
		{"empty", nil, false},
		{"only approvals", []string{"a.publish", "b.publish"}, false},
		{"one intent", []string{"a.publish", "b.intent"}, true},
	}
	for _, tt := range tests {
		if got := hasIntent(tt.names); got != tt.want {
			t.Errorf("%s: hasIntent(%v) = %v, want %v", tt.name, tt.names, got, tt.want)
		}
	}
}

// fakeExec swaps agentrun.Exec for a recorder and returns pointers to
// the recorded invocation and the call flag.
func fakeExec(t *testing.T) (*agentrun.Invocation, *bool) {
	t.Helper()
	var got agentrun.Invocation
	var called bool
	orig := agentrun.Exec
	agentrun.Exec = func(inv agentrun.Invocation) error {
		got, called = inv, true
		return nil
	}
	t.Cleanup(func() { agentrun.Exec = orig })
	return &got, &called
}

func isolate(t *testing.T) (home, state string) {
	t.Helper()
	home = t.TempDir()
	state = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", state)
	return home, state
}

func TestRunExitsFastWithoutIntents(t *testing.T) {
	isolate(t)
	_, called := fakeExec(t)

	if err := run(); err != nil {
		t.Fatal(err)
	}
	if *called {
		t.Error("run started the agent with no intent queued")
	}
}

func TestRunDrainsQueuedIntent(t *testing.T) {
	_, state := isolate(t)
	got, called := fakeExec(t)
	intents := filepath.Join(state, "recdep", "intents")
	if err := os.MkdirAll(intents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(intents, "x.intent"), []byte("entry /e\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(); err != nil {
		t.Fatal(err)
	}
	if !*called {
		t.Fatal("run never started the agent")
	}
	want := []string{"claude", "-p", defaultPrompt, "--allowedTools", defaultTools}
	if !slices.Equal(got.Argv, want) {
		t.Errorf("argv = %v, want %v", got.Argv, want)
	}
	if want := filepath.Join(state, "recdep", "draft.log"); got.Log != want {
		t.Errorf("log = %q, want %q", got.Log, want)
	}
}
