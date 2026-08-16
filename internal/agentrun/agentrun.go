// Package agentrun runs a headless agent the way the minitrue and
// speakwrite subcommands need: parameters from a plain KEY=value env
// file with per-role defaults, stdin from /dev/null, stdout and stderr
// appended to a log under the state root, and a hard timeout so the
// oneshot unit always completes.
package agentrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// defaultAgent, defaultArgs, and defaultTimeoutSeconds match every
// role. defaultArgs is the claude CLI shape; another agent overrides
// it with <PREFIX>_ARGS, a whitespace-split template where an element
// that is exactly {prompt} or {tools} becomes that value as one
// argument. No shell interpolation, no re-splitting.
const (
	defaultAgent          = "claude"
	defaultArgs           = "-p {prompt} --allowedTools {tools}"
	defaultTimeoutSeconds = 600
)

// Invocation is one resolved agent run.
type Invocation struct {
	// Argv is the full command line: the agent binary, then the args
	// template with {prompt} and {tools} substituted.
	Argv []string
	// Env holds the env file's KEY=value pairs, appended to the process
	// environment so the agent sees them (identity handles, tokens).
	Env []string
	// Timeout kills the agent when it hangs on teardown.
	Timeout time.Duration
	// Log is the file stdout and stderr append to.
	Log string
}

// ParseEnvFile reads a plain KEY=value env file, ignoring comment and
// blank lines and stripping matching surrounding quotes from values.
// A missing file yields an empty map: an absent config is a working one.
func ParseEnvFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	vars := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		vars[strings.TrimSpace(key)] = value
	}
	return vars, nil
}

// Resolve builds the invocation for one role from its env file vars and
// defaults. prefix names the role's variables (MINITRUE_AGENT and so
// on); a file value wins over a process environment value, and a key
// set empty in the file means the default, not the process value,
// matching the retired wrappers, which sourced the file over the
// environment before applying ${VAR:-default}.
func Resolve(vars map[string]string, prefix, defaultPrompt, defaultTools, logPath string) (Invocation, error) {
	get := func(suffix, fallback string) string {
		key := prefix + "_" + suffix
		if v, ok := vars[key]; ok {
			if v == "" {
				return fallback
			}
			return v
		}
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	seconds, err := strconv.Atoi(get("TIMEOUT", strconv.Itoa(defaultTimeoutSeconds)))
	if err != nil || seconds <= 0 {
		return Invocation{}, fmt.Errorf("%s_TIMEOUT must be a positive number of seconds: %q", prefix, get("TIMEOUT", ""))
	}
	env := make([]string, 0, len(vars))
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	argv := []string{get("AGENT", defaultAgent)}
	for _, el := range strings.Fields(get("ARGS", defaultArgs)) {
		switch el {
		case "{prompt}":
			argv = append(argv, get("PROMPT", defaultPrompt))
		case "{tools}":
			argv = append(argv, get("ALLOWED_TOOLS", defaultTools))
		default:
			argv = append(argv, el)
		}
	}
	return Invocation{
		Argv:    argv,
		Env:     env,
		Timeout: time.Duration(seconds) * time.Second,
		Log:     logPath,
	}, nil
}

// Exec runs one invocation. A nil Stdin gives the agent /dev/null, so
// the process always exits (claude -p can hang on teardown) and the
// oneshot unit completes. Tests replace it to record the invocation.
var Exec = func(inv Invocation) error {
	ctx, cancel := context.WithTimeout(context.Background(), inv.Timeout)
	defer cancel()
	log, err := os.OpenFile(inv.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	cmd := exec.CommandContext(ctx, inv.Argv[0], inv.Argv[1:]...)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Env = append(os.Environ(), inv.Env...)
	// SIGTERM first with a grace period, like coreutils timeout in the
	// retired wrappers, so the agent can flush before the SIGKILL.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	return cmd.Run()
}
