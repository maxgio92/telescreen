package export

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// seed creates a state root with the standard dirs and returns it.
func seed(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, s := range append([]string{recdep.IntentsDir}, recdep.States...) {
		if err := os.MkdirAll(filepath.Join(root, s), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// write puts one record file into a drawer.
func write(t *testing.T, root, drawer, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, drawer, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func export(t *testing.T, root string) []record {
	t.Helper()
	var out bytes.Buffer
	if err := run(&out, root); err != nil {
		t.Fatal(err)
	}
	var records []record
	if err := json.Unmarshal(out.Bytes(), &records); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	return records
}

func TestExportFieldsAndOrder(t *testing.T) {
	root := seed(t)
	write(t, root, "tube", "20260814T090000Z-github-alice-review.md",
		"[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-14T09:00:01Z\n\npreview line\n")
	write(t, root, "tube", "20260814T100000Z-slack-bob-ping.md",
		"[slack] bob: ping\nhttps://example.com/t/1\nseen 2026-08-14T10:00:01Z\nstale merged 2026-08-15T09:00:00Z\n")
	write(t, root, "desk", "20260813T080000Z-github-carol-drafted.md",
		"[github] carol: needs a reply\nhttps://github.com/o/r/pull/9\nseen 2026-08-13T08:00:01Z\n\n--- dictated 2026-08-13T09:00:00Z\nbe brief\n\n--- draft 2026-08-13T09:05:00Z\nthe draft text\n")
	// Non-.md files never export.
	write(t, root, "desk", "notes.txt", "not a record\n")

	records := export(t, root)
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}

	// Drawer order tube, desk; filename descending within a drawer.
	if records[0].File != "20260814T100000Z-slack-bob-ping.md" || records[0].Drawer != "tube" {
		t.Errorf("records[0] = %s/%s", records[0].Drawer, records[0].File)
	}
	if records[1].File != "20260814T090000Z-github-alice-review.md" || records[1].Drawer != "tube" {
		t.Errorf("records[1] = %s/%s", records[1].Drawer, records[1].File)
	}
	if records[2].Drawer != "desk" {
		t.Errorf("records[2].Drawer = %s, want desk", records[2].Drawer)
	}

	fresh := records[1]
	if fresh.TS != "2026-08-14T09:00:00Z" {
		t.Errorf("ts = %q", fresh.TS)
	}
	if fresh.Source != "github" || fresh.Who != "alice" || fresh.Summary != "please review" {
		t.Errorf("header fields = %q %q %q", fresh.Source, fresh.Who, fresh.Summary)
	}
	if fresh.URL != "https://github.com/o/r/pull/7" {
		t.Errorf("url = %q", fresh.URL)
	}
	if fresh.Seen != "2026-08-14T09:00:01Z" {
		t.Errorf("seen = %q", fresh.Seen)
	}
	if fresh.Stale != nil || fresh.Marker != nil || fresh.Sections != nil {
		t.Errorf("fresh record carries stale/marker/sections: %+v", fresh)
	}

	stale := records[0]
	if stale.Stale == nil || stale.Stale.Reason != "merged" {
		t.Errorf("stale = %+v, want reason merged", stale.Stale)
	}

	drafted := records[2]
	if drafted.Marker == nil || drafted.Marker.Kind != "draft" || drafted.Marker.Time != "2026-08-13T09:05:00Z" {
		t.Errorf("marker = %+v", drafted.Marker)
	}
	if drafted.Sections["dictated"] != "be brief" || drafted.Sections["draft"] != "the draft text" {
		t.Errorf("sections = %+v", drafted.Sections)
	}
	if _, ok := drafted.Sections["published"]; ok {
		t.Errorf("sections carries a published key the body lacks")
	}
}

func TestExportMalformedDegrades(t *testing.T) {
	root := seed(t)
	write(t, root, "files", "not-a-stamp.md", "no header shape here\n")

	records := export(t, root)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	r := records[0]
	if r.TS != "" {
		t.Errorf("ts = %q, want omitted", r.TS)
	}
	if r.Source != "" || r.Who != "" || r.URL != "" || r.Seen != "" {
		t.Errorf("malformed record grew fields: %+v", r)
	}
	if r.Summary != "no header shape here" {
		t.Errorf("summary = %q", r.Summary)
	}
	if r.Body != "no header shape here" {
		t.Errorf("body = %q", r.Body)
	}
	// ts must be absent from the document, not an empty string.
	var out bytes.Buffer
	if err := run(&out, root); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte(`"ts"`)) {
		t.Errorf("document carries a ts key for an unparsed stamp:\n%s", out.String())
	}
}

func TestExportEmptyQueue(t *testing.T) {
	root := seed(t)
	var out bytes.Buffer
	if err := run(&out, root); err != nil {
		t.Fatal(err)
	}
	var records []record
	if err := json.Unmarshal(out.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
	// An empty queue is an empty array, not null.
	if !bytes.Contains(bytes.TrimSpace(out.Bytes()), []byte("[]")) {
		t.Errorf("output = %q, want []", out.String())
	}
}

func TestExportRejectsUnknownOutput(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cmd := New()
	cmd.SetArgs([]string{"--output", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want an error for --output yaml")
	}
	if got := err.Error(); !strings.Contains(got, "json") {
		t.Errorf("error %q does not list the supported outputs", got)
	}
}
