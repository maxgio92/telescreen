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

func TestParseEntryMarkers(t *testing.T) {
	head := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n"
	tests := []struct {
		name     string
		section  string
		mark     string
		markTime string
	}{
		{"dictated", "--- dictated 2026-08-14T09:00:00Z\nagree with the finding\n", "dictated", "2026-08-14T09:00:00Z"},
		{"draft", "--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n", "draft", "2026-08-14T10:00:00Z"},
		{"published", "--- published 2026-08-14T11:00:00Z https://github.com/o/r/pull/7#c1\n", "published", "2026-08-14T11:00:00Z"},
		{"discarded", "--- discarded 2026-08-14T12:00:00Z\n", "discarded", "2026-08-14T12:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := parseEntry("20260812T100000Z-github-alice-please-review.md", head+"\n"+tt.section)
			if e.mark != tt.mark {
				t.Errorf("mark = %q, want %q", e.mark, tt.mark)
			}
			if e.markTime != tt.markTime {
				t.Errorf("markTime = %q, want %q", e.markTime, tt.markTime)
			}
			// The marker follows the preview and leaves the header fields alone.
			if e.summary != "please review" {
				t.Errorf("summary = %q, want %q", e.summary, "please review")
			}
			if e.url != "https://github.com/o/r/pull/7" {
				t.Errorf("url = %q", e.url)
			}
		})
	}
}

// TestParseEntryMarkerKindsOnly pins that a "--- " line inside draft
// text (a quoted unified diff, for example) is not a marker: only the
// four kinds in RECDEP.md count.
func TestParseEntryMarkerKindsOnly(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nSee the diff:\n--- a/entry.go\n+++ b/entry.go\n"
	e := parseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.mark != "draft" {
		t.Errorf("mark = %q, want draft", e.mark)
	}
	if e.markTime != "2026-08-14T10:00:00Z" {
		t.Errorf("markTime = %q", e.markTime)
	}
}

// TestParseEntryMarkerWithoutURL pins that on a minimal entry a
// speakwrite marker on line 2 is not taken as the URL, the same
// collision handling the stale marker gets.
func TestParseEntryMarkerWithoutURL(t *testing.T) {
	e := parseEntry("bogus.md", "[x] y: z\n--- dictated 2026-08-14T09:00:00Z\nagree with the finding")
	if e.mark != "dictated" {
		t.Errorf("mark = %q, want dictated", e.mark)
	}
	if e.url != "" {
		t.Errorf("url = %q, want empty (marker is not a URL)", e.url)
	}
}

func TestParseEntryLastMarkerWins(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- dictated 2026-08-14T09:00:00Z\nagree with the finding\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n"
	e := parseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.mark != "draft" {
		t.Errorf("mark = %q, want draft", e.mark)
	}
	if e.markTime != "2026-08-14T10:00:00Z" {
		t.Errorf("markTime = %q", e.markTime)
	}

	// The discard flow: the consumer's discarded marker supersedes the
	// draft, so the tag disappears.
	e = parseEntry("x.md", body+"\n--- discarded 2026-08-14T11:00:00Z\n")
	if e.mark != "discarded" {
		t.Errorf("mark after discard = %q, want discarded", e.mark)
	}
}

// TestParseEntryMarkersAfterStale pins the other ordering: the producer
// stamped stale first, the runner appended sections after it.
func TestParseEntryMarkersAfterStale(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n" +
		"stale merged 2026-08-14T08:00:00Z\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n"
	e := parseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.stale != "merged" {
		t.Errorf("stale = %q, want merged", e.stale)
	}
	if e.mark != "draft" {
		t.Errorf("mark = %q, want draft", e.mark)
	}
}

func TestParseEntryStaleWithMarkers(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n" +
		"stale merged 2026-08-14T11:00:00Z\n"
	e := parseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.stale != "merged" {
		t.Errorf("stale = %q, want merged", e.stale)
	}
	if e.mark != "draft" {
		t.Errorf("mark = %q, want draft", e.mark)
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
	want := []string{"tube", "desk", "upsub", "files"}
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
	if err := os.WriteFile(filepath.Join(root, "tube", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := moveEntry(root, name, "tube", "desk"); err != nil {
		t.Fatalf("moveEntry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tube", name)); !os.IsNotExist(err) {
		t.Errorf("entry still in tube: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "desk", name))
	if err != nil {
		t.Fatalf("entry not in desk: %v", err)
	}
	if string(got) != body {
		t.Errorf("body changed after move")
	}

	if err := moveEntry(root, name, "tube", "desk"); err == nil {
		t.Error("moveEntry of a missing file succeeded, want error")
	}

	for _, step := range []struct{ from, to string }{
		{"desk", "upsub"},
		{"upsub", "files"},
		{"files", "upsub"},
		{"upsub", "desk"},
		{"desk", "tube"},
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
	got := e.detail("/state/recdep/desk/20260811T142302Z-slack-wes-go-for-it.md")
	want := "[slack] wes: go for it\n" +
		"\nfirst preview line\nsecond preview line\n" +
		"/state/recdep/desk/20260811T142302Z-slack-wes-go-for-it.md\n" +
		"https://example.com/thread/123\n" +
		"seen 2026-08-11T14:23:02Z"
	if got != want {
		t.Errorf("detail =\n%q\nwant\n%q", got, want)
	}
}

// TestDetailWithMarkers pins that marker sections after the preview pass
// through detail's lines[3:] unchanged and the path, URL, and seen lines
// still land at the end.
func TestDetailWithMarkers(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n"
	e := parseEntry("20260812T100000Z-github-alice-please-review.md", body)
	got := e.detail("/state/recdep/desk/20260812T100000Z-github-alice-please-review.md")
	want := "[github] alice: please review\n" +
		"\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n" +
		"/state/recdep/desk/20260812T100000Z-github-alice-please-review.md\n" +
		"https://github.com/o/r/pull/7\n" +
		"seen 2026-08-12T10:00:00Z"
	if got != want {
		t.Errorf("detail =\n%q\nwant\n%q", got, want)
	}
}

func TestDetailMalformed(t *testing.T) {
	e := parseEntry("bogus.md", "just one line, no header")
	got := e.detail("/state/recdep/tube/bogus.md")
	want := "just one line, no header\n/state/recdep/tube/bogus.md"
	if got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestLoadStateSortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tube")
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
	list, err := loadState(root, "tube")
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
	dir := filepath.Join(root, "tube")
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
	list, err := loadState(root, "tube")
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
