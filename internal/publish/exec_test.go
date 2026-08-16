package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxgio92/telescreen/internal/config"
)

// setRules points Rules at the given routing list for the test's
// duration.
func setRules(t *testing.T, rules []config.PublisherRule) {
	t.Helper()
	orig := Rules
	Rules = rules
	t.Cleanup(func() { Rules = orig })
}

// writeScript writes an executable stub into a temp dir and returns
// its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "post-stub")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRulesRouteEnterpriseHost(t *testing.T) {
	setRules(t, []config.PublisherRule{
		{Publisher: "github-pr", URLPrefix: "https://github.example.com/"},
	})
	rawURL := "https://github.example.com/o/r/pull/5"
	if name, ok := Match(rawURL); !ok || name != "github-pr" {
		t.Fatalf("Match(%q) = %q, %v; want github-pr, true", rawURL, name, ok)
	}
	var gotArgs []string
	orig := GHRun
	GHRun = func(args []string, _ string) (string, error) {
		gotArgs = args
		return "https://github.example.com/o/r/pull/5#issuecomment-1\n", nil
	}
	t.Cleanup(func() { GHRun = orig })
	name, permalink, err := Post(rawURL, "d")
	if err != nil {
		t.Fatal(err)
	}
	if name != "github-pr" {
		t.Errorf("publisher = %q, want github-pr", name)
	}
	want := []string{"pr", "comment", "5", "--repo", "github.example.com/o/r", "--body-file", "-"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Errorf("gh args = %v, want %v", gotArgs, want)
	}
	if want := "https://github.example.com/o/r/pull/5#issuecomment-1"; permalink != want {
		t.Errorf("permalink = %q, want %q", permalink, want)
	}
}

func TestRulesDisablePublisher(t *testing.T) {
	off := false
	setRules(t, []config.PublisherRule{
		{Publisher: "github-pr", Enabled: &off},
		{Publisher: "github-pr", URLPrefix: "https://github.example.com/"},
	})
	// Disabled in the built-in phase.
	if name, ok := Match("https://github.com/o/r/pull/1"); ok {
		t.Errorf("Match matched %q; a disabled publisher must not match", name)
	}
	// Disabled in the rule phase too: the prefix rule names the same
	// publisher and is skipped.
	if name, ok := Match("https://github.example.com/o/r/pull/1"); ok {
		t.Errorf("Match matched %q; a rule for a disabled publisher must not match", name)
	}
	// Other publishers keep matching.
	if name, ok := Match("https://linear.app/acme/issue/FUL-123"); !ok || name != "linear-issue" {
		t.Errorf("Match = %q, %v; want linear-issue, true", name, ok)
	}
}

func TestRulesNoMatchKeepsRefusal(t *testing.T) {
	setRules(t, []config.PublisherRule{
		{Publisher: "exec", URLPrefix: "https://forum.example.com/", Command: "true"},
	})
	if name, ok := Match("https://example.com/thread/1"); ok {
		t.Errorf("Match matched %q; want no publisher", name)
	}
	_, _, err := Post("https://example.com/thread/1", "d")
	if err == nil || !strings.Contains(err.Error(), "no publisher") {
		t.Errorf("err = %v, want a no-publisher error", err)
	}
}

func TestExecPost(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, `printf '%s\n' "$@" >"`+dir+`/argv"
cat >"`+dir+`/stdin"
printf 'https://forum.example.com/t/1/reply/2\n'
`)
	setRules(t, []config.PublisherRule{
		{Publisher: "exec", URLPrefix: "https://forum.example.com/", Command: script + " --to {url}"},
	})
	rawURL := "https://forum.example.com/t/1"
	name, permalink, err := Post(rawURL, "the draft\ntwo lines")
	if err != nil {
		t.Fatal(err)
	}
	if name != "exec" {
		t.Errorf("publisher = %q, want exec", name)
	}
	argv, err := os.ReadFile(filepath.Join(dir, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(argv)), "--to\n"+rawURL; got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
	stdin, err := os.ReadFile(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != "the draft\ntwo lines" {
		t.Errorf("stdin = %q, want the draft", stdin)
	}
	if want := "https://forum.example.com/t/1/reply/2"; permalink != want {
		t.Errorf("permalink = %q, want %q", permalink, want)
	}
}

func TestExecPostNonZeroExit(t *testing.T) {
	script := writeScript(t, "echo 'boom: token rejected' >&2\nexit 3\n")
	setRules(t, []config.PublisherRule{
		{Publisher: "exec", Command: script + " {url}"},
	})
	_, _, err := Post("https://forum.example.com/t/1", "d")
	if err == nil || !strings.Contains(err.Error(), "token rejected") {
		t.Errorf("err = %v, want the script's stderr", err)
	}
}

func TestExecPostNonURLOutputFallsBackToRecordURL(t *testing.T) {
	script := writeScript(t, "cat >/dev/null\necho done\n")
	setRules(t, []config.PublisherRule{
		{Publisher: "exec", Command: script},
	})
	rawURL := "https://forum.example.com/t/1"
	_, permalink, err := Post(rawURL, "d")
	if err != nil {
		t.Fatal(err)
	}
	if permalink != rawURL {
		t.Errorf("permalink = %q, want the record URL %q", permalink, rawURL)
	}
}
