package thinkpol

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maxgio92/telescreen/internal/publish"
	"github.com/maxgio92/telescreen/internal/recdep"
)

const entryName = "20260814T090000Z-github-review-demo-1.md"

// draftedBody is an entry with a dictated and a draft section, the shape
// the TUI approves.
func draftedBody(url string) string {
	return strings.Join([]string{
		"[github] alice: please review",
		url,
		"seen now",
		"",
		"review requested",
		"",
		"--- dictated 2026-08-14T09:00:00Z",
		"agree with the finding",
		"",
		"--- draft 2026-08-14T09:05:00Z",
		"the draft text",
		"",
	}, "\n")
}

// seed creates a state root with one entry in state and its approval in
// intents/, and returns the root.
func seed(t *testing.T, state, body string) string {
	t.Helper()
	root := t.TempDir()
	for _, s := range append(slices.Clone(recdep.States), recdep.IntentsDir) {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entryPath := filepath.Join(root, state, entryName)
	if err := os.WriteFile(entryPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	approval := filepath.Join(root, recdep.IntentsDir, entryName+".publish")
	if err := os.WriteFile(approval, []byte("entry "+entryPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// fakeGH swaps publish.GHRun for a fake and returns pointers to the
// recorded call. err is what the fake returns alongside stdout.
func fakeGH(t *testing.T, stdout string, err error) (gotArgs *[]string, gotStdin *string) {
	t.Helper()
	var args []string
	var stdin string
	orig := publish.GHRun
	publish.GHRun = func(a []string, in string) (string, error) {
		args, stdin = a, in
		return stdout, err
	}
	t.Cleanup(func() { publish.GHRun = orig })
	return &args, &stdin
}

func approvalExists(t *testing.T, root string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, recdep.IntentsDir, entryName+".publish"))
	return err == nil
}

func readLog(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "publish.log"))
	if err != nil {
		t.Fatalf("publish.log: %v", err)
	}
	return string(b)
}

func TestDrainPublishesApprovedDraft(t *testing.T) {
	root := seed(t, "tube", draftedBody("https://github.com/o/r/pull/1"))
	commentURL := "https://github.com/o/r/pull/1#issuecomment-9"
	gotArgs, gotStdin := fakeGH(t, commentURL+"\n", nil)

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"pr", "comment", "1", "--repo", "o/r", "--body-file", "-"}
	if !slices.Equal(*gotArgs, wantArgs) {
		t.Errorf("gh args = %v, want %v", *gotArgs, wantArgs)
	}
	if *gotStdin != "the draft text" {
		t.Errorf("gh stdin = %q, want the draft text", *gotStdin)
	}
	if approvalExists(t, root) {
		t.Error("approval survived a successful publish")
	}
	moved, err := os.ReadFile(filepath.Join(root, "upsub", entryName))
	if err != nil {
		t.Fatalf("entry not in upsub: %v", err)
	}
	if !strings.Contains(string(moved), "--- published ") || !strings.Contains(string(moved), commentURL) {
		t.Errorf("entry lacks the published marker with the comment URL:\n%s", moved)
	}
	if _, err := os.Stat(filepath.Join(root, "tube", entryName)); !os.IsNotExist(err) {
		t.Errorf("entry still in tube: %v", err)
	}
	if !strings.Contains(readLog(t, root), commentURL) {
		t.Errorf("log lacks the comment URL:\n%s", readLog(t, root))
	}
}

func TestDrainRefusesDiscardedDraft(t *testing.T) {
	body := draftedBody("https://github.com/o/r/pull/1") + "\n--- discarded 2026-08-14T10:00:00Z\n"
	root := seed(t, "desk", body)
	gotArgs, _ := fakeGH(t, "", nil)

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	if *gotArgs != nil {
		t.Errorf("gh was called on a discarded draft: %v", *gotArgs)
	}
	if approvalExists(t, root) {
		t.Error("approval survived a refusal")
	}
	after, err := os.ReadFile(filepath.Join(root, "desk", entryName))
	if err != nil {
		t.Fatalf("entry moved on a refusal: %v", err)
	}
	if string(after) != body {
		t.Errorf("refusal changed the entry:\n%s", after)
	}
	if !strings.Contains(readLog(t, root), "discarded") {
		t.Errorf("log lacks the refusal reason:\n%s", readLog(t, root))
	}
}

func TestDrainRemovesOrphanApproval(t *testing.T) {
	root := seed(t, "tube", draftedBody("https://github.com/o/r/pull/1"))
	if err := os.Remove(filepath.Join(root, "tube", entryName)); err != nil {
		t.Fatal(err)
	}
	gotArgs, _ := fakeGH(t, "", nil)

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	if *gotArgs != nil {
		t.Errorf("gh was called on an orphan approval: %v", *gotArgs)
	}
	if approvalExists(t, root) {
		t.Error("orphan approval survived")
	}
	if !strings.Contains(readLog(t, root), "orphan") {
		t.Errorf("log lacks the orphan line:\n%s", readLog(t, root))
	}
}

func TestDrainRefusesNonGitHubURL(t *testing.T) {
	root := seed(t, "desk", draftedBody("https://example.com/thread/1"))
	gotArgs, _ := fakeGH(t, "", nil)

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	if *gotArgs != nil {
		t.Errorf("gh was called on a non-GitHub URL: %v", *gotArgs)
	}
	if approvalExists(t, root) {
		t.Error("approval survived a refusal")
	}
	if _, err := os.Stat(filepath.Join(root, "desk", entryName)); err != nil {
		t.Errorf("entry moved on a refusal: %v", err)
	}
	if !strings.Contains(readLog(t, root), "no publisher") {
		t.Errorf("log lacks the refusal reason:\n%s", readLog(t, root))
	}
}

func TestDrainGHFailureLeavesEntryUntouched(t *testing.T) {
	body := draftedBody("https://github.com/o/r/pull/1")
	root := seed(t, "tube", body)
	fakeGH(t, "", errors.New("gh: HTTP 502"))

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	if approvalExists(t, root) {
		t.Error("approval survived a failed post")
	}
	after, err := os.ReadFile(filepath.Join(root, "tube", entryName))
	if err != nil {
		t.Fatalf("entry moved on a failed post: %v", err)
	}
	if string(after) != body {
		t.Errorf("failed post changed the entry:\n%s", after)
	}
	if !strings.Contains(readLog(t, root), "HTTP 502") {
		t.Errorf("log lacks the gh error:\n%s", readLog(t, root))
	}
}

func TestDrainEntryInFilesGetsMarkerButNoMove(t *testing.T) {
	root := seed(t, "files", draftedBody("https://github.com/o/r/pull/1"))
	commentURL := "https://github.com/o/r/pull/1#issuecomment-9"
	fakeGH(t, commentURL+"\n", nil)

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	if approvalExists(t, root) {
		t.Error("approval survived a successful publish")
	}
	after, err := os.ReadFile(filepath.Join(root, "files", entryName))
	if err != nil {
		t.Fatalf("entry left files: %v", err)
	}
	if !strings.Contains(string(after), "--- published ") || !strings.Contains(string(after), commentURL) {
		t.Errorf("entry lacks the published marker:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(root, "upsub", entryName)); !os.IsNotExist(err) {
		t.Errorf("entry in files was moved to upsub: %v", err)
	}
}

// TestPublishRefusesDraftDiscardedThenRedictated pins the withdrawn-draft
// guard when a dictated section follows the discarded marker.
func TestPublishRefusesDraftDiscardedThenRedictated(t *testing.T) {
	body := draftedBody("https://github.com/o/r/pull/7") +
		"\n--- discarded 2026-08-14T10:05:00Z\n" +
		"\n--- dictated 2026-08-14T10:10:00Z\nnew guidance, no new draft yet\n"
	root := seed(t, "tube", body)
	gotArgs, _ := fakeGH(t, "", nil)

	if err := drain(root); err != nil {
		t.Fatal(err)
	}
	if *gotArgs != nil {
		t.Errorf("thinkpol posted a withdrawn draft: %v", *gotArgs)
	}
	if _, err := os.Stat(filepath.Join(root, recdep.IntentsDir, entryName+".publish")); !os.IsNotExist(err) {
		t.Error("refusal left the approval behind")
	}
}
