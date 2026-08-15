//go:build e2e

// Package e2e drives the built telescreen binary through the whole
// record life defined in docs/contracts/recdep.md: produce, dictate,
// draft, approve, publish. TestMain builds the binary once with
// coverage instrumentation; each test runs it against a scratch state
// root and stub executables, never the network beyond localhost.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// binPath is the coverage-instrumented telescreen binary TestMain builds.
var binPath string

// coverDir collects the binary's GOCOVERDIR profiles across every run:
// the GOCOVERDIR the caller exported, else a per-run temp dir.
var coverDir string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "telescreen-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	coverDir = os.Getenv("GOCOVERDIR")
	if coverDir == "" {
		coverDir = filepath.Join(tmp, "covdata")
	}
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	binPath = filepath.Join(tmp, "telescreen")
	build := exec.Command("go", "build", "-cover", "-o", binPath, ".")
	build.Dir = ".."
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

// scratch is one hermetic run environment: a scratch HOME, config dir,
// state root, and a bin dir prepended to PATH for stub executables.
type scratch struct {
	home      string
	configDir string
	stateHome string
	binDir    string
	root      string // <stateHome>/recdep
	extraEnv  []string
}

// newScratch lays out the run environment under t.TempDir.
func newScratch(t *testing.T) *scratch {
	t.Helper()
	base := t.TempDir()
	s := &scratch{
		home:      filepath.Join(base, "home"),
		stateHome: filepath.Join(base, "state"),
		binDir:    filepath.Join(base, "bin"),
	}
	s.configDir = filepath.Join(s.home, ".config")
	s.root = filepath.Join(s.stateHome, "recdep")
	for _, d := range []string{s.configDir, s.stateHome, s.binDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// env builds the child environment: hermetic HOME and XDG dirs, the
// stub bin dir first on PATH, the shared GOCOVERDIR, and any extras.
func (s *scratch) env() []string {
	return append([]string{
		"HOME=" + s.home,
		"XDG_STATE_HOME=" + s.stateHome,
		"XDG_CONFIG_HOME=" + s.configDir,
		"GOCOVERDIR=" + coverDir,
		"PATH=" + s.binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, s.extraEnv...)
}

// run executes the binary with args and returns its combined output and
// exit code.
func (s *scratch) run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = s.env()
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("telescreen %v: %v\n%s", args, err, out)
		}
		code = exitErr.ExitCode()
	}
	return string(out), code
}

// writeScript writes an executable stub into the scratch bin dir and
// returns its path.
func (s *scratch) writeScript(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(s.binDir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeRecord writes a contract-conforming entry into the given drawer
// and returns its absolute path. The drawers exist after the first
// binary run; tests create them up front instead.
func (s *scratch) writeRecord(t *testing.T, drawer, name, body string) string {
	t.Helper()
	dir := filepath.Join(s.root, drawer)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeIntent writes a file into intents/ and returns its path.
func (s *scratch) writeIntent(t *testing.T, name, body string) string {
	t.Helper()
	dir := filepath.Join(s.root, "intents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeFile writes a private file, the mode the contract recommends.
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}

// readFile fails the test when path is unreadable.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// exists reports whether path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
