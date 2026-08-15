package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxgio92/telescreen/internal/recdep"
)

var now = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

// cleanBody is a record that passes every check.
const cleanBody = "[github] alice: please review\nhttps://github.com/o/r/pull/7\nseen 2026-08-14T09:00:01Z\n\npreview line\n"

// seed creates a state root with the standard dirs at 0700 and returns it.
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

// write puts one file into a dir under root at 0600.
func write(t *testing.T, root, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// verifyOut runs verify against root and returns the output and whether
// findings made it fail.
func verifyOut(t *testing.T, root string) (string, bool) {
	t.Helper()
	var out bytes.Buffer
	err := run(&out, root, now)
	return out.String(), err != nil
}

// wantFinding asserts one finding line containing want, prefixed with
// dir/name, and a failing run.
func wantFinding(t *testing.T, root, dir, name, want string) {
	t.Helper()
	out, failed := verifyOut(t, root)
	if !failed {
		t.Errorf("run passed, want findings; output:\n%s", out)
	}
	prefix := dir + "/" + name + ": "
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, want) {
			return
		}
	}
	t.Errorf("no %q finding for %s%s; output:\n%s", want, prefix, want, out)
}

// wantWarning asserts one warning line containing want, prefixed with
// dir/name, and a passing run.
func wantWarning(t *testing.T, root, dir, name, want string) {
	t.Helper()
	out, failed := verifyOut(t, root)
	if failed {
		t.Errorf("run failed, want warnings only; output:\n%s", out)
	}
	prefix := dir + "/" + name + ": warning: "
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, want) {
			return
		}
	}
	t.Errorf("no %q warning for %s; output:\n%s", want, prefix, out)
}

func TestVerifyClean(t *testing.T) {
	root := seed(t)
	write(t, root, "tube", "20260814T090000Z-github-alice-review.md", cleanBody)
	write(t, root, "desk", "20260813T080000Z-slack-bob-ping.md",
		"[slack] bob: ping\nhttps://example.com/t/1\nseen 2026-08-13T08:00:01Z\n\n--- dictated 2026-08-13T09:00:00Z\nbe brief\n\n--- draft 2026-08-13T09:05:00Z\nthe draft text\n")
	// A stale record with one well-formed marker line stays clean.
	write(t, root, "upsub", "20260812T070000Z-linear-carol-ticket.md",
		"[linear] carol: ticket moved\nhttps://linear.app/t/1\nseen 2026-08-12T07:00:01Z\nstale closed 2026-08-14T07:00:00Z\n")
	write(t, root, recdep.IntentsDir, "20260813T080000Z-slack-bob-ping.md.intent",
		"entry "+filepath.Join(root, "desk", "20260813T080000Z-slack-bob-ping.md")+"\naction slack-reply\n\nguidance:\n\n")
	write(t, root, recdep.IntentsDir, "20260813T080000Z-slack-bob-ping.md.publish",
		"entry "+filepath.Join(root, "desk", "20260813T080000Z-slack-bob-ping.md")+"\n")

	out, failed := verifyOut(t, root)
	if failed {
		t.Fatalf("clean queue failed:\n%s", out)
	}
	if !strings.Contains(out, "3 records, clean") {
		t.Errorf("output = %q, want the summary line", out)
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("clean queue warned:\n%s", out)
	}
}

func TestVerifyFilenameFindings(t *testing.T) {
	root := seed(t)
	write(t, root, "tube", "not-a-stamp.md", cleanBody)
	wantFinding(t, root, "tube", "not-a-stamp.md", "parseable UTC stamp")

	write(t, root, "tube", "notes.txt", "whatever\n")
	wantFinding(t, root, "tube", "notes.txt", "no .md suffix")
}

func TestVerifyHeaderFindings(t *testing.T) {
	root := seed(t)
	write(t, root, "desk", "20260814T090000Z-github-a-nosource.md",
		"alice: no tag here\nhttps://github.com/o/r/pull/7\nseen 2026-08-14T09:00:01Z\n")
	wantFinding(t, root, "desk", "20260814T090000Z-github-a-nosource.md", "[source] tag")

	write(t, root, "desk", "20260814T090100Z-github-a-nowho.md",
		"[github] just a summary\nhttps://github.com/o/r/pull/7\nseen 2026-08-14T09:01:01Z\n")
	wantFinding(t, root, "desk", "20260814T090100Z-github-a-nowho.md", "who: summary")
}

func TestVerifyURLAndSeenFindings(t *testing.T) {
	root := seed(t)
	// Three lines, the second blank: no URL, and seen sits third.
	write(t, root, "tube", "20260814T090000Z-github-a-nourl.md",
		"[github] alice: no url\n\nseen 2026-08-14T09:00:01Z\n")
	wantFinding(t, root, "tube", "20260814T090000Z-github-a-nourl.md", "second line is not a URL")

	write(t, root, "tube", "20260814T090100Z-github-a-noseen.md",
		"[github] alice: no seen\nhttps://github.com/o/r/pull/7\n\npreview\n")
	wantFinding(t, root, "tube", "20260814T090100Z-github-a-noseen.md", "missing seen line")

	write(t, root, "tube", "20260814T090200Z-github-a-seenlate.md",
		"[github] alice: seen late\nhttps://github.com/o/r/pull/7\npreview\nseen 2026-08-14T09:02:01Z\n")
	wantFinding(t, root, "tube", "20260814T090200Z-github-a-seenlate.md", "seen line not third")
}

func TestVerifyMarkerAndStaleWarnings(t *testing.T) {
	root := seed(t)
	// Look-alike marker and stale lines can be verbatim preview content,
	// so they warn without flipping the exit code.
	write(t, root, "desk", "20260814T090000Z-github-a-badmarker.md",
		cleanBody+"\n--- posted 2026-08-14T10:00:00Z\n")
	wantWarning(t, root, "desk", "20260814T090000Z-github-a-badmarker.md", "suspicious marker line")

	write(t, root, "desk", "20260814T090100Z-github-a-badtime.md",
		cleanBody+"\n--- draft not-a-time\n")
	wantWarning(t, root, "desk", "20260814T090100Z-github-a-badtime.md", "suspicious marker line")

	write(t, root, "desk", "20260814T090200Z-github-a-doublestale.md",
		cleanBody+"stale merged 2026-08-15T09:00:00Z\nstale closed 2026-08-15T10:00:00Z\n")
	wantWarning(t, root, "desk", "20260814T090200Z-github-a-doublestale.md", "more than one stale line")

	// The stale line ParseEntry recognizes is grammar: too few fields is
	// a finding.
	write(t, root, "desk", "20260814T090300Z-github-a-shortstale.md",
		cleanBody+"stale merged\n")
	wantFinding(t, root, "desk", "20260814T090300Z-github-a-shortstale.md", "fewer than 3 fields")
}

func TestVerifyPreviewQuotesStayClean(t *testing.T) {
	root := seed(t)
	// A GitHub review comment quoting a diff hunk, plus a real trailing
	// stale marker: the parser treats the record as clean, so verify must
	// exit 0.
	write(t, root, "tube", "20260814T090000Z-github-alice-hunk.md",
		"[github] alice: quoted a hunk\nhttps://github.com/o/r/pull/7\nseen 2026-08-14T09:00:01Z\n\n--- a/main.go\n+++ b/main.go\nstale bread\nmore preview\nstale merged 2026-08-15T09:00:00Z\n")

	out, failed := verifyOut(t, root)
	if failed {
		t.Fatalf("preview quoting a diff hunk failed:\n%s", out)
	}
	if !strings.Contains(out, "1 records, clean") {
		t.Errorf("no summary line; output:\n%s", out)
	}
}

func TestVerifySectionTextStaysClean(t *testing.T) {
	root := seed(t)
	// A drafted diff reply: the section text quotes "--- a/file" lines
	// and a "stale ..." line. docs/contracts/recdep.md sanctions those as section text,
	// so verify must not flag them.
	write(t, root, "desk", "20260814T090000Z-github-alice-diff.md",
		cleanBody+"\n--- draft 2026-08-14T10:00:00Z\nquoting the change:\n--- a/main.go\n+++ b/main.go\nstale comment kept for context\nstale two\n")

	out, failed := verifyOut(t, root)
	if failed {
		t.Fatalf("drafted diff reply flagged:\n%s", out)
	}
	if !strings.Contains(out, "1 records, clean") {
		t.Errorf("no summary line; output:\n%s", out)
	}
}

func TestVerifyIntentsFindings(t *testing.T) {
	root := seed(t)
	write(t, root, recdep.IntentsDir, "a.intent", "action respond\n\nguidance:\n\n")
	wantFinding(t, root, recdep.IntentsDir, "a.intent", "missing entry line")

	tmp := filepath.Join(root, recdep.IntentsDir, "b.intent.tmp")
	write(t, root, recdep.IntentsDir, "b.intent.tmp", "entry /nowhere\n")
	old := now.Add(-25 * time.Hour)
	if err := os.Chtimes(tmp, old, old); err != nil {
		t.Fatal(err)
	}
	wantFinding(t, root, recdep.IntentsDir, "b.intent.tmp", "abandoned dictation")

	write(t, root, recdep.IntentsDir, "c.md.publish", "entry "+filepath.Join(root, "desk", "c.md")+"\n")
	wantFinding(t, root, recdep.IntentsDir, "c.md.publish", "resolves nowhere")
}

func TestVerifyPublishResolvesByBasename(t *testing.T) {
	root := seed(t)
	name := "20260814T090000Z-github-alice-review.md"
	write(t, root, "upsub", name, cleanBody)
	// The recorded path points at desk; the entry moved to upsub. The
	// basename fallback the actor uses still resolves it.
	write(t, root, recdep.IntentsDir, name+".publish", "entry "+filepath.Join(root, "desk", name)+"\n")

	out, failed := verifyOut(t, root)
	if failed {
		t.Fatalf("moved entry reported unresolved:\n%s", out)
	}
}

func TestVerifyPermissionWarnings(t *testing.T) {
	root := seed(t)
	name := "20260814T090000Z-github-alice-review.md"
	write(t, root, "tube", name, cleanBody)
	if err := os.Chmod(filepath.Join(root, "tube", name), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "desk"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, failed := verifyOut(t, root)
	if failed {
		t.Fatalf("warnings flipped the exit code:\n%s", out)
	}
	if !strings.Contains(out, "tube/"+name+": warning: mode 0644, want 0600") {
		t.Errorf("no file mode warning; output:\n%s", out)
	}
	if !strings.Contains(out, "desk: warning: directory mode 0755, want 0700") {
		t.Errorf("no directory mode warning; output:\n%s", out)
	}
	if !strings.Contains(out, "1 records, clean") {
		t.Errorf("no summary line; output:\n%s", out)
	}
}
