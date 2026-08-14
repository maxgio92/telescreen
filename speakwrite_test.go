package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionFor(t *testing.T) {
	tests := []struct {
		name string
		e    entry
		want string
	}{
		{"review requested", parseEntry(
			"20260813T172742Z-github-review-requested-demo-42.md",
			"[github] alice: review requested: fix the widget (#42)\nhttps://example.com/pr/42\nseen now\n",
		), "review"},
		{"bot findings", entry{source: "github", who: "dastardly[bot]"}, "vet-findings"},
		{"github human", entry{source: "github", who: "alice"}, "pr-reply"},
		{"slack", entry{source: "slack", who: "wes"}, "slack-reply"},
		{"linear", entry{source: "linear", who: "chuck"}, "linear-comment"},
		{"unknown source", entry{source: "carrier-pigeon", who: "alice"}, "respond"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actionFor(tt.e); got != tt.want {
				t.Errorf("actionFor(%q, %q) = %q, want %q", tt.e.source, tt.e.who, got, tt.want)
			}
		})
	}
}

func TestRenderIntent(t *testing.T) {
	fresh := parseEntry("a.md", "[slack] wes: go for it\nhttps://example.com\nseen now\n")
	want := "entry /q/inbox/a.md\naction slack-reply\n\nguidance:\n\n"
	if got := renderIntent("/q/inbox/a.md", fresh, dictatedGuidance(fresh.body)); got != want {
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
	e := parseEntry("b.md", body)
	got := renderIntent("/q/todo/b.md", e, dictatedGuidance(e.body))
	want := "entry /q/todo/b.md\naction pr-reply\n\nguidance:\nagree with the finding\npush back on the nit\n"
	if got != want {
		t.Errorf("re-dictation intent = %q, want %q", got, want)
	}
}

func TestGuidanceForPrefersPendingIntent(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, intentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	e := parseEntry(name, "[slack] wes: go for it\nhttps://example.com\nseen now\n")
	if got := guidanceFor(root, e); got != "" {
		t.Fatalf("guidance without a pending intent = %q, want empty", got)
	}
	pending := "entry /q/inbox/" + name + "\naction slack-reply\n\nguidance:\nsay yes, but after the freeze\n"
	if err := os.WriteFile(filepath.Join(root, intentsDir, name+".intent"), []byte(pending), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := guidanceFor(root, e); got != "say yes, but after the freeze" {
		t.Errorf("guidance with a pending intent = %q", got)
	}
}

func TestActionForReviewRequestedNeedsGitHub(t *testing.T) {
	slack := parseEntry(
		"20260813T172742Z-slack-review-requested-foo.md",
		"[slack] wes: review requested: foo\nhttps://example.com\nseen now\n",
	)
	if got := actionFor(slack); got != "slack-reply" {
		t.Errorf("slack entry with a review-requested slug = %q, want slack-reply", got)
	}
	tagged := parseEntry(
		"20260813T131405Z-github-review-requested-77.md",
		"[github-review-requested] ampleforth: review requested on PR 77\nhttps://example.com/pr/77\nseen now\n",
	)
	if got := actionFor(tagged); got != "review" {
		t.Errorf("github-review-requested header = %q, want review", got)
	}
}

func TestDictatedGuidanceRunsToEndOfBody(t *testing.T) {
	body := "[github] alice: hi\nurl\nseen now\n\n--- dictated 2026-08-14T09:00:00Z\ntail guidance\n"
	if got := dictatedGuidance(strings.TrimRight(body, "\n")); got != "tail guidance" {
		t.Errorf("dictatedGuidance = %q, want %q", got, "tail guidance")
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
	for _, state := range []string{"inbox", "todo", "waiting"} {
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
	for _, view := range []string{"archive", "memoryhole"} {
		t.Run(view, func(t *testing.T) {
			m, root := seedModel(t, "archive", name)
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
	m, root := seedModel(t, "inbox", name)
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
			m, root := seedModel(t, "inbox", name)
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

func TestPendingIntentShowsDictatedTag(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "inbox", name)
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
