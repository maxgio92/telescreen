package agentrun

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maxgio92/telescreen/internal/config"
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

func TestParseEnvFileValueKeepsSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minitrue.env")
	if err := os.WriteFile(path, []byte("MINITRUE_ARGS=exec --full-auto {prompt}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vars, err := ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := vars["MINITRUE_ARGS"]; got != "exec --full-auto {prompt}" {
		t.Errorf("MINITRUE_ARGS = %q, want the spaces after the first = kept", got)
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
	inv, err := Resolve(config.Component{}, map[string]string{"MINITRUE_PROMPT": ""}, "MINITRUE", "/minitrue produce", "Bash Read", "/log")
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

func TestResolveArgsTemplate(t *testing.T) {
	multiline := "Draft a reply.\n\nKeep it short."
	cases := []struct {
		name string
		vars map[string]string
		want []string
	}{
		{
			name: "absent key keeps the claude shape",
			vars: map[string]string{},
			want: []string{"claude", "-p", "/produce", "--allowedTools", "Bash Read"},
		},
		{
			name: "empty key keeps the claude shape",
			vars: map[string]string{"MINITRUE_ARGS": ""},
			want: []string{"claude", "-p", "/produce", "--allowedTools", "Bash Read"},
		},
		{
			name: "codex shape keeps the prompt one argument",
			vars: map[string]string{
				"MINITRUE_AGENT":  "codex",
				"MINITRUE_ARGS":   "exec {prompt}",
				"MINITRUE_PROMPT": multiline,
			},
			want: []string{"codex", "exec", multiline},
		},
		{
			name: "template without {tools} ignores the allowlist",
			vars: map[string]string{"MINITRUE_ARGS": "-p {prompt}"},
			want: []string{"claude", "-p", "/produce"},
		},
		{
			name: "braces short of a placeholder stay verbatim",
			vars: map[string]string{"MINITRUE_ARGS": "--flag={prompt} {prompts} {prompt}"},
			want: []string{"claude", "--flag={prompt}", "{prompts}", "/produce"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := Resolve(config.Component{}, tc.vars, "MINITRUE", "/produce", "Bash Read", "/log")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(inv.Argv, tc.want) {
				t.Errorf("argv = %q, want %q", inv.Argv, tc.want)
			}
		})
	}
}

func TestResolveArgsFileWinsOverEnv(t *testing.T) {
	t.Setenv("MINITRUE_ARGS", "run {prompt}")
	inv, err := Resolve(config.Component{}, map[string]string{"MINITRUE_ARGS": "exec {prompt}"}, "MINITRUE", "/produce", "Bash", "/log")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "exec", "/produce"}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %q, want the env file template %q", inv.Argv, want)
	}
}

func TestResolveEnvFileWins(t *testing.T) {
	t.Setenv("MINITRUE_AGENT", "fromenv")
	vars := map[string]string{
		"MINITRUE_AGENT":   "fromfile",
		"MINITRUE_TIMEOUT": "30",
		"SLACK_USER_ID":    "U123",
	}
	inv, err := Resolve(config.Component{}, vars, "MINITRUE", "/minitrue produce", "Bash", "/log")
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
		if _, err := Resolve(config.Component{}, map[string]string{"MINITRUE_TIMEOUT": v}, "MINITRUE", "p", "t", "/log"); err == nil {
			t.Errorf("Resolve accepted timeout %q", v)
		}
	}
}

// TestResolvePrecedence pins the layer order per field: telescreen.yaml
// wins, then the env file, then the process environment, then the
// built-in default.
func TestResolvePrecedence(t *testing.T) {
	instructions := filepath.Join(t.TempDir(), "produce.md")
	if err := os.WriteFile(instructions, []byte("from instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		cfg     config.Component
		vars    map[string]string
		env     map[string]string
		want    []string
		timeout time.Duration
	}{
		{
			name:    "defaults with everything unset",
			want:    []string{"claude", "-p", "/produce", "--allowedTools", "Bash"},
			timeout: 600 * time.Second,
		},
		{
			name:    "process env beats the default",
			env:     map[string]string{"MINITRUE_AGENT": "fromenv", "MINITRUE_TIMEOUT": "10"},
			want:    []string{"fromenv", "-p", "/produce", "--allowedTools", "Bash"},
			timeout: 10 * time.Second,
		},
		{
			name:    "env file beats the process env",
			env:     map[string]string{"MINITRUE_AGENT": "fromenv", "MINITRUE_TIMEOUT": "10"},
			vars:    map[string]string{"MINITRUE_AGENT": "fromfile", "MINITRUE_TIMEOUT": "20"},
			want:    []string{"fromfile", "-p", "/produce", "--allowedTools", "Bash"},
			timeout: 20 * time.Second,
		},
		{
			name: "config beats the env file on every field",
			cfg:  config.Component{Agent: "fromconfig", Args: "run {prompt} {tools}", Instructions: instructions, AllowedTools: "Read", Timeout: 30},
			env:  map[string]string{"MINITRUE_AGENT": "fromenv"},
			vars: map[string]string{
				"MINITRUE_AGENT":         "fromfile",
				"MINITRUE_ARGS":          "exec {prompt}",
				"MINITRUE_PROMPT":        "from file",
				"MINITRUE_ALLOWED_TOOLS": "Bash",
				"MINITRUE_TIMEOUT":       "20",
			},
			want:    []string{"fromconfig", "run", "from instructions", "Read"},
			timeout: 30 * time.Second,
		},
		{
			name:    "unset config fields fall through per field",
			cfg:     config.Component{Agent: "fromconfig"},
			vars:    map[string]string{"MINITRUE_ARGS": "exec {prompt}", "MINITRUE_PROMPT": "from file"},
			want:    []string{"fromconfig", "exec", "from file"},
			timeout: 600 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if tc.vars == nil {
				tc.vars = map[string]string{}
			}
			inv, err := Resolve(tc.cfg, tc.vars, "MINITRUE", "/produce", "Bash", "/log")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(inv.Argv, tc.want) {
				t.Errorf("argv = %q, want %q", inv.Argv, tc.want)
			}
			if inv.Timeout != tc.timeout {
				t.Errorf("timeout = %v, want %v", inv.Timeout, tc.timeout)
			}
		})
	}
}

// TestResolveInstructions pins the prompt resolution: the instructions
// file content, multi-line included, becomes the prompt and beats the
// env prompt key; a missing path fails the run naming the path.
func TestResolveInstructions(t *testing.T) {
	multiline := "Draft a reply.\n\nKeep it short.\n"
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(path, []byte(multiline), 0o600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"MINITRUE_PROMPT": "from file"}
	inv, err := Resolve(config.Component{Instructions: path}, vars, "MINITRUE", "/produce", "Bash", "/log")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "-p", multiline, "--allowedTools", "Bash"}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %q, want the instructions content as the prompt", inv.Argv)
	}

	missing := filepath.Join(t.TempDir(), "absent.md")
	_, err = Resolve(config.Component{Instructions: missing}, map[string]string{}, "MINITRUE", "/produce", "Bash", "/log")
	if err == nil {
		t.Fatal("Resolve ran on a missing instructions file, want an error")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name the path %q", err, missing)
	}
}

// TestResolveInstructionsExpandsHome pins ~ expansion on the
// instructions path.
func TestResolveInstructionsExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "p.md"), []byte("home prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	inv, err := Resolve(config.Component{Instructions: "~/p.md"}, map[string]string{}, "MINITRUE", "/produce", "Bash", "/log")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Argv[2] != "home prompt" {
		t.Errorf("prompt = %q, want the ~ expanded file content", inv.Argv[2])
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
