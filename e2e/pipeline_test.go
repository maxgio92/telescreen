//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recordName and recordBody are one conforming github record per the
// docs/contracts/recdep.md grammar: header, URL, seen line, metadata,
// blank, preview.
const (
	recordName = "20260815T100000Z-github-alice-please-review.md"
	recordBody = "[github] alice: please review\n" +
		"https://github.com/o/r/pull/7\n" +
		"seen 2026-08-15T10:00:00Z\n" +
		"org o\n" +
		"repo r\n" +
		"\n" +
		"review requested on o/r#7\n"
)

// TestPipeline walks one record through its whole life: produced into
// tube/, dictated, drafted by the speakwrite stub, approved, published
// through a fake gh, then verified and exported.
func TestPipeline(t *testing.T) {
	s := newScratch(t)
	entryPath := s.writeRecord(t, "tube", recordName, recordBody)

	// The dictation intent the TUI would write.
	s.writeIntent(t, recordName+".intent",
		"entry "+entryPath+"\naction review\n\nguidance:\nsay thanks and promise a review\n")

	// The agent stub does what the speakwrite skill does: append a
	// dictated and a draft section, remove the intent.
	agent := s.writeScript(t, "agent-stub", `intent=$(ls "$XDG_STATE_HOME"/recdep/intents/*.intent)
entry=$(sed -n 's/^entry //p' "$intent")
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
{
  printf -- '--- dictated %s\n' "$now"
  sed -n '/^guidance:$/,$p' "$intent" | tail -n +2
  printf -- '\n--- draft %s\n' "$now"
  printf 'Thanks, taking a look now.\n'
} >>"$entry"
rm "$intent"
`)
	if err := writeFile(filepath.Join(s.configDir, "speakwrite.env"), "SPEAKWRITE_AGENT="+agent+"\n"); err != nil {
		t.Fatal(err)
	}

	out, code := s.run(t, "speakwrite")
	if code != 0 {
		t.Fatalf("speakwrite exited %d:\n%s", code, out)
	}
	body := readFile(t, entryPath)
	if !strings.Contains(body, "--- draft ") {
		t.Fatalf("no draft marker after speakwrite:\n%s", body)
	}
	if exists(filepath.Join(s.root, "intents", recordName+".intent")) {
		t.Error("intent survived the speakwrite run")
	}

	// Approve and publish through a fake gh on PATH.
	s.writeIntent(t, recordName+".publish", "entry "+entryPath+"\n")
	argvFile := filepath.Join(t.TempDir(), "gh-argv")
	stdinFile := filepath.Join(t.TempDir(), "gh-stdin")
	s.writeScript(t, "gh", `printf '%s\n' "$@" >"$GH_ARGV_FILE"
cat >"$GH_STDIN_FILE"
printf 'https://github.com/o/r/pull/7#issuecomment-1\n'
`)
	s.extraEnv = []string{"GH_ARGV_FILE=" + argvFile, "GH_STDIN_FILE=" + stdinFile}

	out, code = s.run(t, "thinkpol")
	if code != 0 {
		t.Fatalf("thinkpol exited %d:\n%s", code, out)
	}
	upsubPath := filepath.Join(s.root, "upsub", recordName)
	if exists(entryPath) || !exists(upsubPath) {
		t.Fatalf("record did not move to upsub (tube: %v, upsub: %v)", exists(entryPath), exists(upsubPath))
	}
	body = readFile(t, upsubPath)
	if !strings.Contains(body, "--- published ") || !strings.Contains(body, "https://github.com/o/r/pull/7#issuecomment-1") {
		t.Errorf("no published marker with the comment URL:\n%s", body)
	}
	log := readFile(t, filepath.Join(s.root, "publish.log"))
	if !strings.Contains(log, "published "+recordName+".publish via github-pr") {
		t.Errorf("publish.log does not name the record:\n%s", log)
	}
	if exists(filepath.Join(s.root, "intents", recordName+".publish")) {
		t.Error("approval survived the thinkpol run")
	}
	argv := splitLines(readFile(t, argvFile))
	wantArgv := []string{"pr", "comment", "7", "--repo", "o/r", "--body-file", "-"}
	if strings.Join(argv, " ") != strings.Join(wantArgv, " ") {
		t.Errorf("gh argv = %v, want %v", argv, wantArgv)
	}
	if got := readFile(t, stdinFile); !strings.Contains(got, "Thanks, taking a look now.") {
		t.Errorf("gh stdin = %q, want the draft text", got)
	}

	// The resulting store is clean.
	out, code = s.run(t, "verify")
	if code != 0 {
		t.Errorf("verify exited %d:\n%s", code, out)
	}

	out, code = s.run(t, "export", "--output", "json")
	if code != 0 {
		t.Fatalf("export exited %d:\n%s", code, out)
	}
	var records []struct {
		Drawer string            `json:"drawer"`
		File   string            `json:"file"`
		Meta   map[string]string `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("export output is not JSON: %v\n%s", err, out)
	}
	found := false
	for _, r := range records {
		if r.File == recordName {
			found = true
			if r.Drawer != "upsub" {
				t.Errorf("exported drawer = %q, want upsub", r.Drawer)
			}
			if r.Meta["repo"] != "r" {
				t.Errorf("exported meta = %v, want repo r", r.Meta)
			}
		}
	}
	if !found {
		t.Errorf("export lacks %s:\n%s", recordName, out)
	}
}

// TestSlackPublish publishes a drafted slack record through an httptest
// chat.postMessage, rerouted with SLACK_API_BASE.
func TestSlackPublish(t *testing.T) {
	s := newScratch(t)
	name := "20260815T110000Z-slack-wes-thread.md"
	body := "[slack] wes: what about this\n" +
		"https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456\n" +
		"seen 2026-08-15T11:00:00Z\n" +
		"\n" +
		"a thread reply\n" +
		"--- draft 2026-08-15T11:30:00Z\n" +
		"the slack draft\n"
	entryPath := s.writeRecord(t, "desk", name, body)
	s.writeIntent(t, name+".publish", "entry "+entryPath+"\n")

	var mu sync.Mutex
	var gotPath, gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &gotBody); err != nil {
			t.Errorf("request body is not flat JSON: %v: %q", err, b)
		}
		_, _ = w.Write([]byte(`{"ok": true, "ts": "1755000099.000200"}`))
	}))
	defer srv.Close()
	s.extraEnv = []string{"SLACK_API_BASE=" + srv.URL, "SLACK_TOKEN=xoxp-e2e-dummy"}

	out, code := s.run(t, "thinkpol")
	if code != 0 {
		t.Fatalf("thinkpol exited %d:\n%s", code, out)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/chat.postMessage" {
		t.Errorf("posted to %q, want /chat.postMessage", gotPath)
	}
	if gotAuth != "Bearer xoxp-e2e-dummy" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	want := map[string]string{
		"channel":   "C0A86EX00GH",
		"thread_ts": "1755000000.123456",
		"text":      "the slack draft",
	}
	for k, v := range want {
		if gotBody[k] != v {
			t.Errorf("body[%s] = %q, want %q", k, gotBody[k], v)
		}
	}
	published := readFile(t, filepath.Join(s.root, "upsub", name))
	permalink := "https://acme.slack.com/archives/C0A86EX00GH/p1755000099000200"
	if !strings.Contains(published, "--- published ") || !strings.Contains(published, permalink) {
		t.Errorf("no published marker with the permalink:\n%s", published)
	}
}

// TestThinkpolRefusesDiscardedDraft covers the negative path. Current
// behavior per pkg/cmd/thinkpol: a discarded-after-draft record is
// refused, the approval is consumed (removed), the record stays in its
// drawer untouched, and publish.log records the refusal.
func TestThinkpolRefusesDiscardedDraft(t *testing.T) {
	s := newScratch(t)
	name := "20260815T120000Z-github-bob-nudge.md"
	body := "[github] bob: nudge\n" +
		"https://github.com/o/r/pull/9\n" +
		"seen 2026-08-15T12:00:00Z\n" +
		"\n" +
		"a nudge\n" +
		"--- draft 2026-08-15T12:30:00Z\n" +
		"a withdrawn draft\n" +
		"--- discarded 2026-08-15T12:45:00Z\n"
	entryPath := s.writeRecord(t, "tube", name, body)
	s.writeIntent(t, name+".publish", "entry "+entryPath+"\n")

	out, code := s.run(t, "thinkpol")
	if code != 0 {
		t.Fatalf("thinkpol exited %d:\n%s", code, out)
	}
	if !exists(entryPath) {
		t.Error("record left tube on a refused publish")
	}
	if got := readFile(t, entryPath); got != body {
		t.Errorf("record changed on a refused publish:\n%s", got)
	}
	if exists(filepath.Join(s.root, "intents", name+".publish")) {
		t.Error("approval survived the refusal")
	}
	log := readFile(t, filepath.Join(s.root, "publish.log"))
	if !strings.Contains(log, "refused "+name+".publish: draft discarded") {
		t.Errorf("publish.log does not record the refusal:\n%s", log)
	}
}

// TestExecPublish publishes through a configured exec publisher: a
// telescreen.yaml routing rule points the record's URL shape at a stub
// script, which receives the URL as an argument and the draft on
// stdin, and prints the permalink.
func TestExecPublish(t *testing.T) {
	s := newScratch(t)
	name := "20260816T100000Z-forum-carol-question.md"
	rawURL := "https://forum.example.com/t/42"
	body := "[forum] carol: a question\n" +
		rawURL + "\n" +
		"seen 2026-08-16T10:00:00Z\n" +
		"\n" +
		"a forum question\n" +
		"--- draft 2026-08-16T10:30:00Z\n" +
		"the forum draft\n"
	entryPath := s.writeRecord(t, "desk", name, body)
	s.writeIntent(t, name+".publish", "entry "+entryPath+"\n")

	argvFile := filepath.Join(t.TempDir(), "post-argv")
	stdinFile := filepath.Join(t.TempDir(), "post-stdin")
	script := s.writeScript(t, "forum-post", `printf '%s\n' "$@" >"$POST_ARGV_FILE"
cat >"$POST_STDIN_FILE"
printf 'https://forum.example.com/t/42/reply/7\n'
`)
	s.extraEnv = []string{"POST_ARGV_FILE=" + argvFile, "POST_STDIN_FILE=" + stdinFile}
	if err := writeFile(filepath.Join(s.configDir, "telescreen.yaml"),
		"thinkpol:\n  publishers:\n    - publisher: exec\n      url_prefix: https://forum.example.com/\n      command: "+script+" --to {url}\n"); err != nil {
		t.Fatal(err)
	}

	out, code := s.run(t, "thinkpol")
	if code != 0 {
		t.Fatalf("thinkpol exited %d:\n%s", code, out)
	}
	upsubPath := filepath.Join(s.root, "upsub", name)
	if exists(entryPath) || !exists(upsubPath) {
		t.Fatalf("record did not move to upsub (desk: %v, upsub: %v)", exists(entryPath), exists(upsubPath))
	}
	published := readFile(t, upsubPath)
	permalink := "https://forum.example.com/t/42/reply/7"
	if !strings.Contains(published, "--- published ") || !strings.Contains(published, permalink) {
		t.Errorf("no published marker with the permalink:\n%s", published)
	}
	argv := splitLines(readFile(t, argvFile))
	if strings.Join(argv, " ") != "--to "+rawURL {
		t.Errorf("script argv = %v, want [--to %s]", argv, rawURL)
	}
	if got := readFile(t, stdinFile); !strings.Contains(got, "the forum draft") {
		t.Errorf("script stdin = %q, want the draft text", got)
	}
	log := readFile(t, filepath.Join(s.root, "publish.log"))
	if !strings.Contains(log, "published "+name+".publish via exec") {
		t.Errorf("publish.log does not name the exec publisher:\n%s", log)
	}
}

// splitLines splits the stub's one-argument-per-line record without
// mangling arguments that contain spaces.
func splitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
