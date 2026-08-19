package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// write puts a telescreen.yaml under a fresh XDG_CONFIG_HOME and
// points the process at it.
func write(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "telescreen.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeOld puts a retired recdep/config.yaml under the current
// XDG_CONFIG_HOME.
func writeOld(t *testing.T, content string) {
	t.Helper()
	dir := os.Getenv("XDG_CONFIG_HOME")
	if err := os.MkdirAll(filepath.Join(dir, "recdep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recdep", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFull(t *testing.T) {
	write(t, `minitrue:
  agent: codex
  args: exec {prompt}
  instructions: ~/prompts/produce.md
  allowed_tools: Bash Read
  timeout: 300
speakwrite:
  agent: claude
  args: -p {prompt} --allowedTools {tools}
  instructions: /abs/draft.md
  allowed_tools: Bash Write
  timeout: 900
  actions:
    - source: github
      name_contains: -review-requested-
      action: review
    - source: slack
      who_suffix: "[bot]"
      action: respond
    - url_prefix: https://github.com/acme/
      author: alice
      action: pr-reply
      guidance: professional register
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	wantMinitrue := Component{Agent: "codex", Args: "exec {prompt}", Instructions: "~/prompts/produce.md", AllowedTools: "Bash Read", Timeout: 300}
	if c.Minitrue != wantMinitrue {
		t.Errorf("minitrue = %+v, want %+v", c.Minitrue, wantMinitrue)
	}
	wantSpeakwrite := Component{Agent: "claude", Args: "-p {prompt} --allowedTools {tools}", Instructions: "/abs/draft.md", AllowedTools: "Bash Write", Timeout: 900}
	if c.Speakwrite.Component != wantSpeakwrite {
		t.Errorf("speakwrite = %+v, want %+v", c.Speakwrite.Component, wantSpeakwrite)
	}
	wantActions := []Action{
		{Source: "github", NameContains: "-review-requested-", Action: "review"},
		{Source: "slack", WhoSuffix: "[bot]", Action: "respond"},
		{URLPrefix: "https://github.com/acme/", Author: "alice", Action: "pr-reply", Guidance: "professional register"},
	}
	if len(c.Speakwrite.Actions) != len(wantActions) {
		t.Fatalf("actions = %+v, want %+v", c.Speakwrite.Actions, wantActions)
	}
	for i := range wantActions {
		if !reflect.DeepEqual(c.Speakwrite.Actions[i], wantActions[i]) {
			t.Errorf("actions[%d] = %+v, want %+v", i, c.Speakwrite.Actions[i], wantActions[i])
		}
	}
}

func TestLoadPartial(t *testing.T) {
	write(t, "minitrue:\n  agent: codex\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if c.Minitrue.Agent != "codex" || c.Minitrue.Args != "" || c.Minitrue.Timeout != 0 {
		t.Errorf("minitrue = %+v, want only agent set", c.Minitrue)
	}
	if c.Speakwrite.Component != (Component{}) || len(c.Speakwrite.Actions) != 0 {
		t.Errorf("speakwrite = %+v, want zero value", c.Speakwrite)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	write(t, "")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if c.Minitrue != (Component{}) || c.Speakwrite.Component != (Component{}) || len(c.Speakwrite.Actions) != 0 {
		t.Errorf("Load() = %+v, want zero value", c)
	}
}

func TestLoadAbsentFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(c.Speakwrite.Actions) != 0 || c.Minitrue != (Component{}) {
		t.Errorf("Load() = %+v, want zero value", c)
	}
}

func TestLoadUnknownTopLevelKey(t *testing.T) {
	write(t, "actor:\n  agent: claude\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want unknown-key error")
	}
}

// TestLoadThinkpolIsNotAnAgent pins that the actor's key carries
// routing only, no agent fields.
func TestLoadThinkpolIsNotAnAgent(t *testing.T) {
	write(t, "thinkpol:\n  agent: claude\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want unknown-field error")
	}
}

func TestLoadPublishers(t *testing.T) {
	write(t, `thinkpol:
  publishers:
    - publisher: github-pr
      url_prefix: https://github.example.com/
    - publisher: slack-thread
      enabled: false
    - publisher: exec
      command: post-anywhere --to {url}
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	rules := c.Thinkpol.Publishers
	if len(rules) != 3 {
		t.Fatalf("publishers = %+v, want 3 rules", rules)
	}
	if rules[0].Publisher != "github-pr" || rules[0].URLPrefix != "https://github.example.com/" || !rules[0].On() {
		t.Errorf("publishers[0] = %+v, want an enabled github-pr routing rule", rules[0])
	}
	if rules[1].Publisher != "slack-thread" || rules[1].On() {
		t.Errorf("publishers[1] = %+v, want a disabled slack-thread rule", rules[1])
	}
	if rules[2].Publisher != "exec" || rules[2].Command != "post-anywhere --to {url}" || !rules[2].On() {
		t.Errorf("publishers[2] = %+v, want the exec rule", rules[2])
	}
}

func TestLoadPublisherCommandOnBuiltin(t *testing.T) {
	write(t, "thinkpol:\n  publishers:\n    - publisher: github-pr\n      command: post {url}\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want a command-forbidden error")
	}
}

func TestLoadExecWithoutCommand(t *testing.T) {
	write(t, "thinkpol:\n  publishers:\n    - publisher: exec\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want a command-required error")
	}
}

func TestLoadUnknownPublisher(t *testing.T) {
	write(t, "thinkpol:\n  publishers:\n    - publisher: mastodon\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want an unknown-publisher error")
	}
}

func TestLoadUnknownField(t *testing.T) {
	write(t, "minitrue:\n  prompt: /minitrue produce\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want unknown-field error")
	}
}

func TestLoadUnknownActionField(t *testing.T) {
	write(t, "speakwrite:\n  actions:\n    - sourec: github\n      action: review\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want unknown-field error")
	}
}

func TestLoadBadTimeout(t *testing.T) {
	for _, v := range []string{"-5", "soon"} {
		write(t, "minitrue:\n  timeout: "+v+"\n")
		if _, err := Load(); err == nil {
			t.Errorf("Load() accepted timeout %q", v)
		}
	}
}

func TestLoadMissingAction(t *testing.T) {
	write(t, "speakwrite:\n  actions:\n    - source: slack\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want required-field error")
	}
}

func TestLoadMetaRule(t *testing.T) {
	write(t, `speakwrite:
  actions:
    - source: slack
      metadata:
        channel: "#incidents"
      action: slack-reply
      guidance: urgent register
    - metadata:
        org: acme
        repo: widgets
      action: review
`)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	wantActions := []Action{
		{Source: "slack", Meta: map[string]string{"channel": "#incidents"}, Action: "slack-reply", Guidance: "urgent register"},
		{Meta: map[string]string{"org": "acme", "repo": "widgets"}, Action: "review"},
	}
	if !reflect.DeepEqual(c.Speakwrite.Actions, wantActions) {
		t.Errorf("actions = %+v, want %+v", c.Speakwrite.Actions, wantActions)
	}
}

func TestLoadMetaReservedKey(t *testing.T) {
	for _, k := range []string{"stale", "seen", "path", "url"} {
		write(t, "speakwrite:\n  actions:\n    - metadata:\n        "+k+": x\n      action: respond\n")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Errorf("Load() with metadata key %q error = %v, want the reserved-key rejection", k, err)
		}
	}
}

func TestLoadMetaMalformedKey(t *testing.T) {
	for _, k := range []string{"Channel", "chan-nel", "chan nel", "chan1", `""`} {
		write(t, "speakwrite:\n  actions:\n    - metadata:\n        "+k+": x\n      action: respond\n")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "lowercase") {
			t.Errorf("Load() with metadata key %q error = %v, want the key-shape rejection", k, err)
		}
	}
}

func TestLoadMetaEmptyValue(t *testing.T) {
	write(t, "speakwrite:\n  actions:\n    - metadata:\n        channel: \"\"\n      action: respond\n")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "value is required") {
		t.Errorf("Load() error = %v, want the empty-value rejection", err)
	}
}

// TestLoadOldPathFallback pins the migration path: with telescreen.yaml
// absent, the retired recdep/config.yaml still loads and its actions
// map to speakwrite.actions.
func TestLoadOldPathFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeOld(t, "actions:\n  - source: slack\n    action: respond\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want := Action{Source: "slack", Action: "respond"}
	if len(c.Speakwrite.Actions) != 1 || !reflect.DeepEqual(c.Speakwrite.Actions[0], want) {
		t.Errorf("actions = %+v, want [%+v]", c.Speakwrite.Actions, want)
	}
}

func TestLoadOldPathValidates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeOld(t, "actions:\n  - source: slack\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want required-field error from the old file")
	}
}

func TestLoadNewFileWins(t *testing.T) {
	write(t, "speakwrite:\n  actions:\n    - source: github\n      action: review\n")
	writeOld(t, "actions:\n  - source: slack\n    action: respond\n")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(c.Speakwrite.Actions) != 1 || c.Speakwrite.Actions[0].Source != "github" {
		t.Errorf("actions = %+v, want the new file's rule", c.Speakwrite.Actions)
	}
}

func TestLoadMalformed(t *testing.T) {
	write(t, "speakwrite: [\n")
	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want parse error")
	}
}

// TestLoadRejectsDisabledRuleWithPrefix pins that enabled: false plus
// url_prefix errors at load instead of sitting inert.
func TestLoadRejectsDisabledRuleWithPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	yamlBody := "thinkpol:\n  publishers:\n    - publisher: slack-thread\n      enabled: false\n      url_prefix: https://x.example.com/\n"
	if err := os.WriteFile(filepath.Join(dir, "telescreen.yaml"), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "enabled: false cannot carry url_prefix") {
		t.Errorf("Load() error = %v, want the disabled-with-prefix rejection", err)
	}
}
