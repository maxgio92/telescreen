// Package thinkpol executes recorded publish approvals, the acting
// layer defined in docs/contracts/thinkpol.md. It drains recdep/intents/*.publish
// oldest first, posts each approved draft through its publisher,
// appends the published marker, and renames the entry to upsub/. It
// never composes text and never judges a draft; the human approved,
// thinkpol acts.
package thinkpol

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/config"
	"github.com/maxgio92/telescreen/internal/publish"
	"github.com/maxgio92/telescreen/internal/recdep"
)

// New returns the thinkpol subcommand.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "thinkpol",
		Short: "run the actor once (execute publish approvals deterministically)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// A malformed config fails the run: the actor must never
			// post with routing rules it could not read.
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			publish.Rules = cfg.Thinkpol.Publishers
			root, err := recdep.StateRoot()
			if err != nil {
				return err
			}
			return drain(root)
		},
	}
}

// logLine appends one line to recdep/publish.log (created when missing)
// and mirrors it to stdout, so a disappeared approval is always
// explained somewhere.
func logLine(root, format string, args ...any) {
	line := time.Now().UTC().Format(time.RFC3339) + " " + fmt.Sprintf(format, args...)
	fmt.Println(line)
	f, err := os.OpenFile(filepath.Join(root, "publish.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// entryPathOf parses the approval's "entry <absolute path>" line.
func entryPathOf(approval string) (string, bool) {
	for _, line := range strings.Split(approval, "\n") {
		if path, ok := strings.CutPrefix(line, "entry "); ok {
			return path, true
		}
	}
	return "", false
}

// resolve locates the entry: the recorded path when it still exists
// inside one of the four drawers under root, else the same filename in
// the drawers. It returns the drawer and the path, or ok=false when the
// entry exists nowhere.
func resolve(root, recorded string) (state, path string, ok bool) {
	if _, err := os.Stat(recorded); err == nil {
		state := filepath.Base(filepath.Dir(recorded))
		if filepath.Dir(filepath.Dir(recorded)) == root && slices.Contains(recdep.States, state) {
			return state, recorded, true
		}
	}
	name := filepath.Base(recorded)
	for _, s := range recdep.States {
		candidate := filepath.Join(root, s, name)
		if _, err := os.Stat(candidate); err == nil {
			return s, candidate, true
		}
	}
	return "", "", false
}

// execute runs one approval file per the docs/contracts/thinkpol.md procedure. The
// approval is always removed: posted, refused, or failed, nothing
// retries silently. Only an unreadable approval or entry returns an
// error and leaves the file for the next run.
func execute(root, approval string) error {
	aname := filepath.Base(approval)
	b, err := os.ReadFile(approval)
	if err != nil {
		return err
	}
	recorded, ok := entryPathOf(string(b))
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "refused %s: malformed approval, no entry line", aname)
		return nil
	}
	state, path, ok := resolve(root, recorded)
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "orphan %s: entry %s missing from every drawer", aname, recorded)
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	e := recdep.ParseEntry(filepath.Base(path), string(body))
	if recdep.DraftDiscarded(e.Body) {
		_ = os.Remove(approval)
		logLine(root, "refused %s: draft discarded", aname)
		return nil
	}
	draft, ok := recdep.LastSection(e.Body, "draft")
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "refused %s: no draft section in %s", aname, path)
		return nil
	}
	if _, ok := publish.Match(e.URL); !ok {
		// Refusal, not failure: nothing was attempted, so no failure record
		// is filed. The log line and the surviving entry already tell the
		// operator the URL matched no publisher.
		_ = os.Remove(approval)
		logLine(root, "refused %s: no publisher for %q", aname, e.URL)
		return nil
	}
	pubName, commentURL, err := publish.Post(e.URL, draft)
	if err != nil {
		_ = os.Remove(approval)
		logLine(root, "failed %s: %v", aname, err)
		writeFailureRecord(root, e, err)
		return nil
	}
	// The post happened; a failure past this point is logged, and the
	// approval still goes so nothing double-posts.
	marker := "--- published " + time.Now().UTC().Format(time.RFC3339) + " " + commentURL + "\n"
	if err := recdep.AppendMarker(path, marker); err != nil {
		logLine(root, "posted %s but the marker append failed: %v", aname, err)
	}
	if state != "upsub" && state != "files" {
		if err := recdep.MoveEntry(root, e.Name, state, "upsub"); err != nil {
			logLine(root, "posted %s but the move to upsub failed: %v", aname, err)
		}
	}
	_ = os.Remove(approval)
	logLine(root, "published %s via %s: %s", aname, pubName, commentURL)
	return nil
}

// writeFailureRecord files one record into tube/ so the screen shows the
// failed post: the original's URL (so o opens the subject), a "record"
// metadata line naming it, the original's own metadata, and the error's
// first line as the preview. It is an ordinary record with no draft and
// no approval, so it never triggers further thinkpol action by itself.
// One record per consumed approval: re-approving and failing again is a
// new event and files another.
func writeFailureRecord(root string, e recdep.Entry, postErr error) {
	now := time.Now().UTC()
	// The original slug is the filename minus its stamp when the prefix
	// really is one (ParseEntry's rule); a stampless hand-made name
	// stays whole. The source tag stays in the slug so two originals
	// differing only by source cannot collide on the stamped name.
	slug := strings.TrimSuffix(e.Name, ".md")
	if len(slug) > len(recdep.StampLayout) {
		if _, err := time.Parse(recdep.StampLayout, slug[:len(recdep.StampLayout)]); err == nil && slug[len(recdep.StampLayout)] == '-' {
			slug = slug[len(recdep.StampLayout)+1:]
		}
	}
	name := now.Format(recdep.StampLayout) + "-thinkpol-publish-failed-" + slug + ".md"
	reason, _, _ := strings.Cut(postErr.Error(), "\n")
	lines := []string{
		"[thinkpol] actor: publish failed: " + e.Name,
		e.URL,
		"seen " + now.Format(time.RFC3339),
		"record " + e.Name,
	}
	for _, p := range e.Meta {
		lines = append(lines, p.Key+" "+p.Value)
	}
	lines = append(lines, "", reason, "")
	// O_EXCL: a second failure in the same second must not truncate the
	// first record.
	f, err := os.OpenFile(filepath.Join(root, "tube", name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		logLine(root, "failed to file the failure record for %s: %v", e.Name, err)
		return
	}
	if _, err := f.WriteString(strings.Join(lines, "\n")); err != nil {
		logLine(root, "failed to file the failure record for %s: %v", e.Name, err)
	}
	_ = f.Close()
}

// drain executes every pending approval. Approval names start with the
// entry's UTC stamp, so the sorted glob walks entries oldest first (the
// posts are independent, so approval age does not matter).
func drain(root string) error {
	approvals, err := filepath.Glob(filepath.Join(root, recdep.IntentsDir, "*.publish"))
	if err != nil {
		return err
	}
	for _, a := range approvals {
		if err := execute(root, a); err != nil {
			return err
		}
	}
	return nil
}
