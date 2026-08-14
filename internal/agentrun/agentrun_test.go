package agentrun

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestParseEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minitrue.env")
	content := "# identity\n\nSLACK_USER_ID=U123\nGH_LOGIN='octo'\nMINITRUE_AGENT=\"myagent\"\nnot a pair\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	vars, err := ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"SLACK_USER_ID": "U123", "GH_LOGIN": "octo", "MINITRUE_AGENT": "myagent"}
	if len(vars) != len(want) {
		t.Errorf("vars = %v, want %v", vars, want)
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("vars[%s] = %q, want %q", k, vars[k], v)
		}
	}
}

func TestParseEnvFileMissingIsEmpty(t *testing.T) {
	vars, err := ParseEnvFile(filepath.Join(t.TempDir(), "absent.env"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 0 {
		t.Errorf("vars = %v, want empty", vars)
	}
}

func TestResolveDefaults(t *testing.T) {
	// An empty value falls back to the default, like ${VAR:-default}.
	inv, err := Resolve(map[string]string{"MINITRUE_PROMPT": ""}, "MINITRUE", "/minitrue produce", "Bash Read", "/log")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "-p", "/minitrue produce", "--allowedTools", "Bash Read"}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %v, want %v", inv.Argv, want)
	}
	if inv.Timeout != 600*time.Second {
		t.Errorf("timeout = %v, want 600s", inv.Timeout)
	}
	if inv.Log != "/log" {
		t.Errorf("log = %q, want /log", inv.Log)
	}
}

func TestResolveEnvFileWins(t *testing.T) {
	t.Setenv("MINITRUE_AGENT", "fromenv")
	vars := map[string]string{
		"MINITRUE_AGENT":   "fromfile",
		"MINITRUE_TIMEOUT": "30",
		"SLACK_USER_ID":    "U123",
	}
	inv, err := Resolve(vars, "MINITRUE", "/minitrue produce", "Bash", "/log")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Argv[0] != "fromfile" {
		t.Errorf("agent = %q, want the env file value", inv.Argv[0])
	}
	if inv.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", inv.Timeout)
	}
	if !slices.Contains(inv.Env, "SLACK_USER_ID=U123") {
		t.Errorf("env %v lacks the exported file variable", inv.Env)
	}
}

func TestResolveRejectsBadTimeout(t *testing.T) {
	for _, v := range []string{"soon", "0", "-5"} {
		if _, err := Resolve(map[string]string{"MINITRUE_TIMEOUT": v}, "MINITRUE", "p", "t", "/log"); err == nil {
			t.Errorf("Resolve accepted timeout %q", v)
		}
	}
}

// TestExecReal exercises the production Exec with a shell instead of an
// agent: log lines append across runs, env-file vars reach the child,
// and the deadline kills a hung child.
func TestExecReal(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	inv := Invocation{
		Argv:    []string{"sh", "-c", "echo run-$DEMO_VAR"},
		Env:     []string{"DEMO_VAR=one"},
		Timeout: 5 * time.Second,
		Log:     logPath,
	}
	if err := Exec(inv); err != nil {
		t.Fatal(err)
	}
	inv.Env = []string{"DEMO_VAR=two"}
	if err := Exec(inv); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "run-one\nrun-two\n" {
		t.Errorf("log = %q, want two appended runs", got)
	}

	hung := Invocation{
		Argv:    []string{"sh", "-c", "sleep 30"},
		Timeout: 200 * time.Millisecond,
		Log:     logPath,
	}
	start := time.Now()
	if err := Exec(hung); err == nil {
		t.Error("hung child exited cleanly, want a timeout error")
	}
	if time.Since(start) > 15*time.Second {
		t.Error("timeout did not cut the hung child short")
	}
}
