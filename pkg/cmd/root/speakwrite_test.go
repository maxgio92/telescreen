package root

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/maxgio92/telescreen/internal/config"
	"github.com/maxgio92/telescreen/internal/recdep"
)

// lastDictated is renderIntent's pre-fill source, shortened for tests.
func lastDictated(body string) string {
	g, _ := recdep.LastSection(body, "dictated")
	return g
}

func TestActionFor(t *testing.T) {
	tests := []struct {
		name string
		e    recdep.Entry
		want string
	}{
		{"review requested", recdep.ParseEntry(
			"20260813T172742Z-github-review-requested-demo-42.md",
			"[github] alice: review requested: fix the widget (#42)\nhttps://example.com/pr/42\nseen now\n",
		), "review"},
		{"bot findings", recdep.Entry{Source: "github", Who: "dastardly[bot]"}, "vet-findings"},
		{"github human", recdep.Entry{Source: "github", Who: "alice"}, "pr-reply"},
		{"slack", recdep.Entry{Source: "slack", Who: "wes"}, "slack-reply"},
		{"linear", recdep.Entry{Source: "linear", Who: "chuck"}, "linear-comment"},
		{"unknown source", recdep.Entry{Source: "carrier-pigeon", Who: "alice"}, "respond"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actionFor(tt.e); got != tt.want {
				t.Errorf("actionFor(%q, %q) = %q, want %q", tt.e.Source, tt.e.Who, got, tt.want)
			}
		})
	}
}

func TestActionForCustomTable(t *testing.T) {
	builtin := actionRules
	t.Cleanup(func() { actionRules = builtin })
	applyConfig(config.Config{Actions: []config.Action{
		{Source: "slack", Action: "respond"},
		{Source: "github", WhoSuffix: "[bot]", Action: "review"},
	}})
	if got := actionFor(recdep.Entry{Source: "slack", Who: "wes"}); got != "respond" {
		t.Errorf("actionFor(slack) = %q, want %q", got, "respond")
	}
	if got := actionFor(recdep.Entry{Source: "github", Who: "dastardly[bot]"}); got != "review" {
		t.Errorf("actionFor(github bot) = %q, want %q", got, "review")
	}
	// The custom table replaces the built-ins entirely: a source only
	// the built-ins knew falls to the default.
	if got := actionFor(recdep.Entry{Source: "linear", Who: "chuck"}); got != "respond" {
		t.Errorf("actionFor(linear) = %q, want %q", got, "respond")
	}
	// An empty config keeps the current table: the custom rule must
	// survive, distinguishing kept-custom from a wiped table (respond)
	// and from the built-ins (vet-findings).
	applyConfig(config.Config{})
	if got := actionFor(recdep.Entry{Source: "github", Who: "dastardly[bot]"}); got != "review" {
		t.Errorf("actionFor(github bot) after empty config = %q, want %q", got, "review")
	}
}

func TestRenderIntent(t *testing.T) {
	fresh := recdep.ParseEntry("a.md", "[slack] wes: go for it\nhttps://example.com\nseen now\n")
	want := "entry /q/tube/a.md\naction slack-reply\n\nguidance:\n\n"
	if got := renderIntent("/q/tube/a.md", fresh, lastDictated(fresh.Body)); got != want {
		t.Errorf("fresh intent = %q, want %q", got, want)
	}
}

func TestRenderIntentPrefillsLastGuidance(t *testing.T) {
	body := strings.Join([]string{
		"[github] alice: please review",
		"https://example.com/pr/1",
		"seen now",
		"",
		"--- dictated 2026-08-13T10:00:00Z",
		"first round guidance",
		"",
		"--- draft 2026-08-13T10:05:00Z",
		"the old draft",
		"",
		"--- dictated 2026-08-14T09:00:00Z",
		"agree with the finding",
		"push back on the nit",
		"",
		"--- draft 2026-08-14T09:05:00Z",
		"the new draft",
	}, "\n")
	e := recdep.ParseEntry("b.md", body)
	got := renderIntent("/q/desk/b.md", e, lastDictated(e.Body))
	want := "entry /q/desk/b.md\naction pr-reply\n\nguidance:\nagree with the finding\npush back on the nit\n"
	if got != want {
		t.Errorf("re-dictation intent = %q, want %q", got, want)
	}
}

func TestGuidanceForPrefersPendingIntent(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, recdep.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	e := recdep.ParseEntry(name, "[slack] wes: go for it\nhttps://example.com\nseen now\n")
	if got := guidanceFor(root, e); got != "" {
		t.Fatalf("guidance without a pending intent = %q, want empty", got)
	}
	pending := "entry /q/tube/" + name + "\naction slack-reply\n\nguidance:\nsay yes, but after the freeze\n"
	if err := os.WriteFile(filepath.Join(root, recdep.IntentsDir, name+".intent"), []byte(pending), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := guidanceFor(root, e); got != "say yes, but after the freeze" {
		t.Errorf("guidance with a pending intent = %q", got)
	}
}

func TestActionForReviewRequestedNeedsGitHub(t *testing.T) {
	slack := recdep.ParseEntry(
		"20260813T172742Z-slack-review-requested-foo.md",
		"[slack] wes: review requested: foo\nhttps://example.com\nseen now\n",
	)
	if got := actionFor(slack); got != "slack-reply" {
		t.Errorf("slack entry with a review-requested slug = %q, want slack-reply", got)
	}
	tagged := recdep.ParseEntry(
		"20260813T131405Z-github-review-requested-77.md",
		"[github-review-requested] ampleforth: review requested on PR 77\nhttps://example.com/pr/77\nseen now\n",
	)
	if got := actionFor(tagged); got != "review" {
		t.Errorf("github-review-requested header = %q, want review", got)
	}
}

func TestDictationSubmits(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		err     error
		want    bool
	}{
		{"saved with content", []byte("entry x\naction respond\n\nguidance:\nok\n"), nil, true},
		{"saved with empty guidance", []byte("entry x\naction respond\n\nguidance:\n"), nil, true},
		{"emptied file", []byte(""), nil, false},
		{"whitespace only", []byte(" \n\t\n"), nil, false},
		{"deleted file", nil, nil, false},
		{"editor exited nonzero", []byte("entry x\n"), errors.New("exit status 1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dictationSubmits(tt.content, tt.err); got != tt.want {
				t.Errorf("dictationSubmits(%q, %v) = %v, want %v", tt.content, tt.err, got, tt.want)
			}
		})
	}
}

func TestDictateWritesDraftIntent(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	for _, state := range []string{"tube", "desk", "upsub"} {
		t.Run(state, func(t *testing.T) {
			m, root := seedModel(t, state, name)
			_, cmd := m.Update(key("s"))
			if cmd == nil {
				t.Fatal("s returned no editor command")
			}
			got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent.tmp"))
			if err != nil {
				t.Fatal(err)
			}
			want := "entry " + filepath.Join(root, state, name) + "\naction slack-reply\n\nguidance:\n\n"
			if string(got) != want {
				t.Errorf("draft intent = %q, want %q", got, want)
			}
		})
	}
}

func TestDictateOutsideOpenStatesDoesNothing(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	for _, view := range []string{"files", "memoryhole"} {
		t.Run(view, func(t *testing.T) {
			m, root := seedModel(t, "files", name)
			if view == "memoryhole" {
				m.view = memoryHoleView
			}
			nm, cmd := m.Update(key("s"))
			m = nm.(model)
			if cmd != nil {
				t.Error("s outside the open states returned a command")
			}
			if m.status != "" {
				t.Errorf("s outside the open states set status %q", m.status)
			}
			names, err := os.ReadDir(filepath.Join(root, "intents"))
			if err != nil {
				t.Fatal(err)
			}
			if len(names) != 0 {
				t.Errorf("s outside the open states wrote %d intent files", len(names))
			}
		})
	}
}

func TestFinishDictationSubmitsOnCleanExit(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	nm, _ := m.Update(key("s"))
	nm, _ = nm.(model).Update(editorDoneMsg{name: name})
	m = nm.(model)
	if want := "dictated " + name; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	if _, err := os.Stat(filepath.Join(root, "intents", name+".intent")); err != nil {
		t.Errorf("submitted intent missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "intents", name+".intent.tmp")); !os.IsNotExist(err) {
		t.Errorf("draft intent still present after submit: %v", err)
	}
	if !m.pending[name] {
		t.Error("submitted entry is not tracked as pending")
	}
}

func TestFinishDictationCancels(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	tests := []struct {
		name string
		msg  editorDoneMsg
		// prepare mutates the draft intent between editor start and exit.
		prepare func(t *testing.T, tmp string)
	}{
		{"editor exited nonzero", editorDoneMsg{name: name, err: errors.New("exit status 1")}, func(*testing.T, string) {}},
		{"file emptied", editorDoneMsg{name: name}, func(t *testing.T, tmp string) {
			if err := os.WriteFile(tmp, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"file deleted", editorDoneMsg{name: name}, func(t *testing.T, tmp string) {
			if err := os.Remove(tmp); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, root := seedModel(t, "tube", name)
			nm, _ := m.Update(key("s"))
			tt.prepare(t, filepath.Join(root, "intents", name+".intent.tmp"))
			nm, _ = nm.(model).Update(tt.msg)
			m = nm.(model)
			if m.status != "dictation cancelled" {
				t.Errorf("status = %q, want %q", m.status, "dictation cancelled")
			}
			names, err := os.ReadDir(filepath.Join(root, "intents"))
			if err != nil {
				t.Fatal(err)
			}
			if len(names) != 0 {
				t.Errorf("cancel left %d files in intents/", len(names))
			}
		})
	}
}

// seedDraftModel creates a state root with one drafted entry in desk and
// returns a loaded model on that view. url is the entry's link line.
func seedDraftModel(t *testing.T, name, url string) (model, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	for _, s := range watchedDirs {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	body := strings.Join([]string{
		"[github] alice: please review",
		url,
		"seen now",
		"",
		"--- dictated 2026-08-14T09:00:00Z",
		"agree with the finding",
		"",
		"--- draft 2026-08-14T09:05:00Z",
		"the draft text",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "desk", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, nil)
	m.view = 1
	m.width, m.height = 80, 24
	return m, root
}

func TestPublishArmsThenApproves(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	approval := filepath.Join(root, "intents", name+".publish")

	m = press(t, m, key("p"))
	if want := "publish to https://github.com/o/r/pull/1: press p again to approve"; m.status != want {
		t.Errorf("status after first p = %q, want %q", m.status, want)
	}
	if _, err := os.Stat(approval); !os.IsNotExist(err) {
		t.Fatalf("first p already wrote the approval: %v", err)
	}

	m = press(t, m, key("p"))
	if want := "publish approved: " + name; m.status != want {
		t.Errorf("status after second p = %q, want %q", m.status, want)
	}
	got, err := os.ReadFile(approval)
	if err != nil {
		t.Fatal(err)
	}
	want := "entry " + filepath.Join(root, "desk", name) + "\n"
	if string(got) != want {
		t.Errorf("approval = %q, want %q", got, want)
	}
}

func TestPublishGitHubIssueOnlyHints(t *testing.T) {
	name := "20260814T090000Z-github-issue.md"
	m, _ := seedDraftModel(t, name, "https://github.com/o/r/issues/7")
	m = press(t, m, key("p"))
	if m.pubArmed != "" {
		t.Errorf("p armed a github issue draft: %q", m.pubArmed)
	}
	if !strings.Contains(m.status, "no publisher") {
		t.Errorf("status = %q, want the no-publisher hint", m.status)
	}
}

// TestPublishArmsOnPublishableTargets pins the p gate to the publisher
// table: every table entry's URL shape arms.
func TestPublishArmsOnPublishableTargets(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"20260814T090000Z-slack-wes-thread.md", "https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456"},
		{"20260814T090000Z-linear-ful-123.md", "https://linear.app/acme/issue/FUL-123/fix-the-thing"},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			m, root := seedDraftModel(t, c.name, c.url)
			m = press(t, m, key("p"))
			if m.pubArmed != c.name {
				t.Fatalf("p did not arm on %s: status %q", c.url, m.status)
			}
			m = press(t, m, key("p"))
			if want := "publish approved: " + c.name; m.status != want {
				t.Errorf("status = %q, want %q", m.status, want)
			}
			if _, err := os.Stat(filepath.Join(root, "intents", c.name+".publish")); err != nil {
				t.Errorf("p p wrote no approval: %v", err)
			}
		})
	}
}

func TestPublishMousePressDisarms(t *testing.T) {
	name := "20260814T090000Z-github-pr.md"
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	m = press(t, m, key("p"))
	if m.pubArmed != name {
		t.Fatalf("p did not arm: %q", m.pubArmed)
	}
	m = press(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: 1, X: 0})
	m = press(t, m, key("p"))
	if got := m.pubArmed; got != name {
		t.Errorf("p after a mouse press should re-arm, got armed %q", got)
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("mouse-interrupted p p published anyway: %d files", len(names))
	}
}

func TestPublishNonPublishableDraftOnlyHints(t *testing.T) {
	name := "20260814T090000Z-forum-thread.md"
	m, root := seedDraftModel(t, name, "https://example.com/thread/1")

	for i := 0; i < 2; i++ {
		m = press(t, m, key("p"))
		if want := "no publisher for this record's target; y copies the url"; m.status != want {
			t.Errorf("status after p %d = %q, want %q", i+1, m.status, want)
		}
		if m.pubArmed != "" {
			t.Errorf("p %d armed a non-publishable draft: %q", i+1, m.pubArmed)
		}
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("p on a non-publishable draft wrote %d files in intents/", len(names))
	}
}

func TestPublishFreshEntryDoesNothing(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = press(t, m, key("p"))
	m = press(t, m, key("p"))
	if m.status != "" || m.pubArmed != "" {
		t.Errorf("p on a fresh entry set status %q, pubArmed %q", m.status, m.pubArmed)
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("p on a fresh entry wrote %d files in intents/", len(names))
	}
}

func TestPublishDisarmsOnOtherKey(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	approval := filepath.Join(root, "intents", name+".publish")

	m = press(t, m, key("p"))
	m = press(t, m, key("k"))
	if m.pubArmed != "" {
		t.Fatalf("k left the entry armed: %q", m.pubArmed)
	}
	m = press(t, m, key("p"))
	if _, err := os.Stat(approval); !os.IsNotExist(err) {
		t.Fatalf("p after disarm approved instead of re-arming: %v", err)
	}
	m = press(t, m, key("p"))
	if _, err := os.Stat(approval); err != nil {
		t.Errorf("second clean arm did not approve: %v", err)
	}
	if want := "publish approved: " + name; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
}

func TestDiscardAppendsMarkerOnce(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, _ := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	if !strings.Contains(m.View(), "[draft]") {
		t.Fatal("seeded row lacks [draft]")
	}

	m = press(t, m, key("D"))
	if m.status != "draft discarded" {
		t.Errorf("status = %q, want %q", m.status, "draft discarded")
	}
	if strings.Contains(m.View(), "[draft]") {
		t.Errorf("discarded row still carries [draft]:\n%s", m.View())
	}

	// A second D is a no-op: the last marker is discarded, not draft.
	m = press(t, m, key("D"))
	e, ok := m.selected()
	if !ok {
		t.Fatal("entry disappeared")
	}
	if got := strings.Count(e.Body, "--- discarded "); got != 1 {
		t.Errorf("discarded markers = %d, want 1\nbody:\n%s", got, e.Body)
	}
	if e.Mark != "discarded" {
		t.Errorf("mark = %q, want discarded", e.Mark)
	}
}

func TestDiscardRevokesPendingPublishApproval(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")

	m = press(t, m, key("p"))
	m = press(t, m, key("p"))
	if _, err := os.Stat(filepath.Join(root, "intents", name+".publish")); err != nil {
		t.Fatalf("p p did not approve: %v", err)
	}

	m = press(t, m, key("D"))
	if m.status != "draft discarded" {
		t.Errorf("status = %q, want %q", m.status, "draft discarded")
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("D left %d files in intents/, want 0", len(names))
	}
}

func TestDiscardFreshEntryDoesNothing(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	before, err := os.ReadFile(filepath.Join(root, "tube", name))
	if err != nil {
		t.Fatal(err)
	}
	m = press(t, m, key("D"))
	if m.status != "" {
		t.Errorf("D on a fresh entry set status %q", m.status)
	}
	after, err := os.ReadFile(filepath.Join(root, "tube", name))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("D on a fresh entry changed the file:\n%s", after)
	}
}

func TestPendingIntentShowsDictatedTag(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m.width, m.height = 80, 24
	if strings.Contains(m.View(), "[dictated]") {
		t.Fatal("row shows [dictated] before any intent exists")
	}
	if err := os.WriteFile(filepath.Join(root, "intents", name+".intent"), []byte("entry x\naction respond\n\nguidance:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reload()
	if !strings.Contains(m.View(), "[dictated]") {
		t.Errorf("row lacks [dictated] with a pending intent:\n%s", m.View())
	}
}
