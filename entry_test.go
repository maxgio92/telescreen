package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseEntry(t *testing.T) {
	body := "[slack] wes: go for it\nhttps://example.com/thread/123\nseen 2026-08-11T14:23:02Z\n"
	e := parseEntry("20260811T142302Z-slack-wes-go-for-it.md", body)

	want := time.Date(2026, 8, 11, 14, 23, 2, 0, time.UTC)
	if !e.ts.Equal(want) {
		t.Errorf("ts = %v, want %v", e.ts, want)
	}
	if e.source != "slack" {
		t.Errorf("source = %q, want slack", e.source)
	}
	if e.who != "wes" {
		t.Errorf("who = %q, want wes", e.who)
	}
	if e.summary != "go for it" {
		t.Errorf("summary = %q, want %q", e.summary, "go for it")
	}
	if e.url != "https://example.com/thread/123" {
		t.Errorf("url = %q", e.url)
	}
}

func TestParseEntryMalformed(t *testing.T) {
	e := parseEntry("bogus.md", "just one line, no header")
	if !e.ts.IsZero() {
		t.Errorf("ts = %v, want zero", e.ts)
	}
	if e.source != "" || e.url != "" {
		t.Errorf("source = %q, url = %q, want empty", e.source, e.url)
	}
	if e.summary != "just one line, no header" {
		t.Errorf("summary = %q", e.summary)
	}
}

func TestStates(t *testing.T) {
	want := []string{"inbox", "todo", "waiting", "archive"}
	if len(states) != len(want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
	for i, s := range want {
		if states[i] != s {
			t.Errorf("states[%d] = %q, want %q", i, states[i], s)
		}
	}
}

func TestMoveEntry(t *testing.T) {
	root := t.TempDir()
	for _, s := range states {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	body := "[slack] wes: go for it\nhttps://example.com\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, "inbox", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := moveEntry(root, name, "inbox", "todo"); err != nil {
		t.Fatalf("moveEntry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", name)); !os.IsNotExist(err) {
		t.Errorf("entry still in inbox: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "todo", name))
	if err != nil {
		t.Fatalf("entry not in todo: %v", err)
	}
	if string(got) != body {
		t.Errorf("body changed after move")
	}

	if err := moveEntry(root, name, "inbox", "todo"); err == nil {
		t.Error("moveEntry of a missing file succeeded, want error")
	}

	for _, step := range []struct{ from, to string }{
		{"todo", "waiting"},
		{"waiting", "archive"},
		{"archive", "waiting"},
		{"waiting", "todo"},
		{"todo", "inbox"},
	} {
		if err := moveEntry(root, name, step.from, step.to); err != nil {
			t.Fatalf("moveEntry %s -> %s: %v", step.from, step.to, err)
		}
		if _, err := os.Stat(filepath.Join(root, step.to, name)); err != nil {
			t.Fatalf("entry not in %s after move: %v", step.to, err)
		}
	}
}

func TestLoadStateSortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "inbox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"20260811T100000Z-slack-a-old.md",
		"20260812T100000Z-github-b-new.md",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("[x] y: z\nhttp://u\nseen t\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := loadState(root, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].name != "20260812T100000Z-github-b-new.md" {
		t.Errorf("first = %q, want the newest entry", list[0].name)
	}
}
