package recdep

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
	e := ParseEntry("20260811T142302Z-slack-wes-go-for-it.md", body)

	want := time.Date(2026, 8, 11, 14, 23, 2, 0, time.UTC)
	if !e.TS.Equal(want) {
		t.Errorf("ts = %v, want %v", e.TS, want)
	}
	if e.Source != "slack" {
		t.Errorf("source = %q, want slack", e.Source)
	}
	if e.Who != "wes" {
		t.Errorf("who = %q, want wes", e.Who)
	}
	if e.Summary != "go for it" {
		t.Errorf("summary = %q, want %q", e.Summary, "go for it")
	}
	if e.URL != "https://example.com/thread/123" {
		t.Errorf("url = %q", e.URL)
	}
	if e.Stale != "" || e.StaleLine != "" {
		t.Errorf("stale = %q, staleLine = %q, want empty", e.Stale, e.StaleLine)
	}
}

func TestParseEntryStale(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\nstale merged 2026-08-14T09:00:00Z\n"
	e := ParseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.Stale != "merged" {
		t.Errorf("stale = %q, want merged", e.Stale)
	}
	if e.StaleLine != "stale merged 2026-08-14T09:00:00Z" {
		t.Errorf("staleLine = %q", e.StaleLine)
	}
	// The marker is parsed out; the preview shown in the list stays the
	// summary line and never picks up the stale line.
	if e.Summary != "please review" {
		t.Errorf("summary = %q, want %q", e.Summary, "please review")
	}
	if e.URL != "https://github.com/o/r/pull/7" {
		t.Errorf("url = %q", e.URL)
	}
	// The detail pane composes from the body, so the marker still shows.
	if !strings.Contains(e.Detail("/p"), "stale merged 2026-08-14T09:00:00Z") {
		t.Errorf("detail lost the stale line:\n%s", e.Detail("/p"))
	}
}

func TestParseEntryStaleWithoutURL(t *testing.T) {
	e := ParseEntry("bogus.md", "[x] y: z\nstale merged 2026-08-14T09:00:00Z")
	if e.Stale != "merged" {
		t.Errorf("stale = %q, want merged", e.Stale)
	}
	if e.URL != "" {
		t.Errorf("url = %q, want empty (marker is not a URL)", e.URL)
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
			e := ParseEntry("20260812T100000Z-github-alice-please-review.md", head+"\n"+tt.section)
			if e.Mark != tt.mark {
				t.Errorf("mark = %q, want %q", e.Mark, tt.mark)
			}
			if e.MarkTime != tt.markTime {
				t.Errorf("markTime = %q, want %q", e.MarkTime, tt.markTime)
			}
			// The marker follows the preview and leaves the header fields alone.
			if e.Summary != "please review" {
				t.Errorf("summary = %q, want %q", e.Summary, "please review")
			}
			if e.URL != "https://github.com/o/r/pull/7" {
				t.Errorf("url = %q", e.URL)
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
	e := ParseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.Mark != "draft" {
		t.Errorf("mark = %q, want draft", e.Mark)
	}
	if e.MarkTime != "2026-08-14T10:00:00Z" {
		t.Errorf("markTime = %q", e.MarkTime)
	}
}

// TestParseEntryMarkerWithoutURL pins that on a minimal entry a
// speakwrite marker on line 2 is not taken as the URL, the same
// collision handling the stale marker gets.
func TestParseEntryMarkerWithoutURL(t *testing.T) {
	e := ParseEntry("bogus.md", "[x] y: z\n--- dictated 2026-08-14T09:00:00Z\nagree with the finding")
	if e.Mark != "dictated" {
		t.Errorf("mark = %q, want dictated", e.Mark)
	}
	if e.URL != "" {
		t.Errorf("url = %q, want empty (marker is not a URL)", e.URL)
	}
}

func TestParseEntryLastMarkerWins(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- dictated 2026-08-14T09:00:00Z\nagree with the finding\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n"
	e := ParseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.Mark != "draft" {
		t.Errorf("mark = %q, want draft", e.Mark)
	}
	if e.MarkTime != "2026-08-14T10:00:00Z" {
		t.Errorf("markTime = %q", e.MarkTime)
	}

	// The discard flow: the consumer's discarded marker supersedes the
	// draft, so the tag disappears.
	e = ParseEntry("x.md", body+"\n--- discarded 2026-08-14T11:00:00Z\n")
	if e.Mark != "discarded" {
		t.Errorf("mark after discard = %q, want discarded", e.Mark)
	}
}

// TestParseEntryMarkersAfterStale pins the other ordering: the producer
// stamped stale first, the runner appended sections after it.
func TestParseEntryMarkersAfterStale(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n" +
		"stale merged 2026-08-14T08:00:00Z\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n"
	e := ParseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.Stale != "merged" {
		t.Errorf("stale = %q, want merged", e.Stale)
	}
	if e.Mark != "draft" {
		t.Errorf("mark = %q, want draft", e.Mark)
	}
}

func TestParseEntryStaleWithMarkers(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n" +
		"stale merged 2026-08-14T11:00:00Z\n"
	e := ParseEntry("20260812T100000Z-github-alice-please-review.md", body)
	if e.Stale != "merged" {
		t.Errorf("stale = %q, want merged", e.Stale)
	}
	if e.Mark != "draft" {
		t.Errorf("mark = %q, want draft", e.Mark)
	}
}

func TestParseEntryMalformed(t *testing.T) {
	e := ParseEntry("bogus.md", "just one line, no header")
	if !e.TS.IsZero() {
		t.Errorf("ts = %v, want zero", e.TS)
	}
	if e.Source != "" || e.URL != "" {
		t.Errorf("source = %q, url = %q, want empty", e.Source, e.URL)
	}
	if e.Summary != "just one line, no header" {
		t.Errorf("summary = %q", e.Summary)
	}
}

func TestStates(t *testing.T) {
	want := []string{"tube", "desk", "upsub", "files"}
	if !slices.Equal(States, want) {
		t.Errorf("states = %v, want %v", States, want)
	}
}

func TestMoveEntry(t *testing.T) {
	root := t.TempDir()
	for _, s := range States {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	body := "[slack] wes: go for it\nhttps://example.com\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, "tube", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MoveEntry(root, name, "tube", "desk"); err != nil {
		t.Fatalf("MoveEntry: %v", err)
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

	if err := MoveEntry(root, name, "tube", "desk"); err == nil {
		t.Error("MoveEntry of a missing file succeeded, want error")
	}

	for _, step := range []struct{ from, to string }{
		{"desk", "upsub"},
		{"upsub", "files"},
		{"files", "upsub"},
		{"upsub", "desk"},
		{"desk", "tube"},
	} {
		if err := MoveEntry(root, name, step.from, step.to); err != nil {
			t.Fatalf("MoveEntry %s -> %s: %v", step.from, step.to, err)
		}
		if _, err := os.Stat(filepath.Join(root, step.to, name)); err != nil {
			t.Fatalf("entry not in %s after move: %v", step.to, err)
		}
	}
}

func TestDetail(t *testing.T) {
	body := "[slack] wes: go for it\nhttps://example.com/thread/123\nseen 2026-08-11T14:23:02Z\n\nfirst preview line\nsecond preview line\n"
	e := ParseEntry("20260811T142302Z-slack-wes-go-for-it.md", body)
	got := e.Detail("/state/recdep/desk/20260811T142302Z-slack-wes-go-for-it.md")
	want := "[slack] wes: go for it\n" +
		"\nfirst preview line\nsecond preview line\n" +
		"path /state/recdep/desk/20260811T142302Z-slack-wes-go-for-it.md\n" +
		"url https://example.com/thread/123\n" +
		"seen 2026-08-11T14:23:02Z"
	if got != want {
		t.Errorf("detail =\n%q\nwant\n%q", got, want)
	}
}

// TestDetailWithMarkers pins that marker sections after the preview pass
// through Detail's lines[3:] unchanged and the path, URL, and seen lines
// still land at the end.
func TestDetailWithMarkers(t *testing.T) {
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-12T10:00:00Z\n\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n"
	e := ParseEntry("20260812T100000Z-github-alice-please-review.md", body)
	got := e.Detail("/state/recdep/desk/20260812T100000Z-github-alice-please-review.md")
	want := "[github] alice: please review\n" +
		"\nreview requested\n\n" +
		"--- draft 2026-08-14T10:00:00Z\nThanks, fixed in the follow-up.\n" +
		"path /state/recdep/desk/20260812T100000Z-github-alice-please-review.md\n" +
		"url https://github.com/o/r/pull/7\n" +
		"seen 2026-08-12T10:00:00Z"
	if got != want {
		t.Errorf("detail =\n%q\nwant\n%q", got, want)
	}
}

func TestDetailMalformed(t *testing.T) {
	e := ParseEntry("bogus.md", "just one line, no header")
	got := e.Detail("/state/recdep/tube/bogus.md")
	want := "just one line, no header\npath /state/recdep/tube/bogus.md"
	if got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestLastSection(t *testing.T) {
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
	got, ok := LastSection(body, "dictated")
	if !ok || got != "agree with the finding\npush back on the nit" {
		t.Errorf("dictated section = %q, %v", got, ok)
	}
	got, ok = LastSection(body, "draft")
	if !ok || got != "the new draft" {
		t.Errorf("draft section = %q, %v", got, ok)
	}
	if _, ok := LastSection(body, "published"); ok {
		t.Error("LastSection found a published section in a body without one")
	}
}

func TestLastSectionRunsToEndOfBody(t *testing.T) {
	body := "[github] alice: hi\nurl\nseen now\n\n--- dictated 2026-08-14T09:00:00Z\ntail guidance\n"
	got, ok := LastSection(strings.TrimRight(body, "\n"), "dictated")
	if !ok || got != "tail guidance" {
		t.Errorf("LastSection = %q, %v, want %q", got, ok, "tail guidance")
	}
}

func TestAppendMarkerWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e.md")
	if err := os.WriteFile(path, []byte("[x] y: z\nhttp://u\nseen t\n\npreview without newline"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendMarker(path, "--- discarded 2026-08-14T12:00:00Z\n"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "newline\n--- discarded") {
		t.Errorf("marker does not start its own line:\n%s", b)
	}
	if ParseEntry("e.md", string(b)).Mark != "discarded" {
		t.Errorf("parser missed the appended marker:\n%s", b)
	}
}
