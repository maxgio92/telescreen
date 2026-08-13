package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	if e.stale != "" || e.staleLine != "" {
		t.Errorf("stale = %q, staleLine = %q, want empty", e.stale, e.staleLine)
	}
}

func TestParseEntryStale(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\nstale merged 2026-08-14T09:00:00Z\n"
	e := parseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.stale != "merged" {
		t.Errorf("stale = %q, want merged", e.stale)
	}
	if e.staleLine != "stale merged 2026-08-14T09:00:00Z" {
		t.Errorf("staleLine = %q", e.staleLine)
	}
	// The marker is parsed out; the preview shown in the list stays the
	// summary line and never picks up the stale line.
	if e.summary != "please review" {
		t.Errorf("summary = %q, want %q", e.summary, "please review")
	}
	if e.url != "https://github.com/o/r/pull/7" {
		t.Errorf("url = %q", e.url)
	}
	// The detail pane composes from the body, so the marker still shows.
	if !strings.Contains(e.detail("/p"), "stale merged 2026-08-14T09:00:00Z") {
		t.Errorf("detail lost the stale line:\n%s", e.detail("/p"))
	}
}

func TestParseEntryStaleWithoutURL(t *testing.T) {
	e := parseEntry("bogus.md", "[x] y: z\nstale merged 2026-08-14T09:00:00Z")
	if e.stale != "merged" {
		t.Errorf("stale = %q, want merged", e.stale)
	}
	if e.url != "" {
		t.Errorf("url = %q, want empty (marker is not a URL)", e.url)
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
	if !slices.Equal(states, want) {
		t.Errorf("states = %v, want %v", states, want)
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

func TestDetail(t *testing.T) {
	body := "[slack] wes: go for it\nhttps://example.com/thread/123\nseen 2026-08-11T14:23:02Z\n\nfirst preview line\nsecond preview line\n"
	e := parseEntry("20260811T142302Z-slack-wes-go-for-it.md", body)
	got := e.detail("/state/recdep/todo/20260811T142302Z-slack-wes-go-for-it.md")
	want := "[slack] wes: go for it\n" +
		"\nfirst preview line\nsecond preview line\n" +
		"/state/recdep/todo/20260811T142302Z-slack-wes-go-for-it.md\n" +
		"https://example.com/thread/123\n" +
		"seen 2026-08-11T14:23:02Z"
	if got != want {
		t.Errorf("detail =\n%q\nwant\n%q", got, want)
	}
}

func TestDetailMalformed(t *testing.T) {
	e := parseEntry("bogus.md", "just one line, no header")
	got := e.detail("/state/recdep/inbox/bogus.md")
	want := "just one line, no header\n/state/recdep/inbox/bogus.md"
	if got != want {
		t.Errorf("detail = %q, want %q", got, want)
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

func TestLoadStateSortsFreshBeforeStale(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "inbox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fresh := "[x] y: z\nhttp://u\nseen t\n"
	stale := "[x] y: z\nhttp://u\nseen t\nstale merged 2026-08-14T09:00:00Z\n"
	files := map[string]string{
		"20260811T100000Z-github-a-old-fresh.md": fresh,
		"20260813T100000Z-github-c-new-stale.md": stale,
		"20260812T100000Z-github-b-old-stale.md": stale,
		"20260814T100000Z-github-d-new-fresh.md": fresh,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := loadState(root, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range list {
		got = append(got, e.name)
	}
	want := []string{
		"20260814T100000Z-github-d-new-fresh.md",
		"20260811T100000Z-github-a-old-fresh.md",
		"20260813T100000Z-github-c-new-stale.md",
		"20260812T100000Z-github-b-old-stale.md",
	}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
