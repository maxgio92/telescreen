package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

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
	if list[0].Name != "20260812T100000Z-github-b-new.md" {
		t.Errorf("first = %q, want the newest entry", list[0].Name)
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
		got = append(got, e.Name)
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
