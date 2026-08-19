// Package verify lints the queue against the docs/contracts/recdep.md grammar without
// writing anything: one line per finding, exit code 1 when findings
// exist. Loose permissions and look-alike marker or stale lines in the
// verbatim preview are reported as warnings and leave the exit code
// alone.
package verify

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// abandonedAge is how old an .intent.tmp must be to count as an
// abandoned dictation.
const abandonedAge = 24 * time.Hour

// New returns the verify subcommand.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "lint the queue against the docs/contracts/recdep.md grammar",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := recdep.StateRoot()
			if err != nil {
				return err
			}
			return run(cmd.OutOrStdout(), root, time.Now())
		},
	}
}

// run lints the queue under root and prints one line per finding. It
// returns an error, so the command exits 1, when findings exist.
func run(out io.Writer, root string, now time.Time) error {
	findings, warnings, records, err := lint(root, now)
	if err != nil {
		return err
	}
	for _, f := range findings {
		_, _ = fmt.Fprintln(out, f)
	}
	for _, w := range warnings {
		_, _ = fmt.Fprintln(out, w)
	}
	if len(findings) > 0 {
		return errors.New("the queue has findings")
	}
	_, _ = fmt.Fprintf(out, "%d records, clean\n", records)
	return nil
}

// lint walks the drawers and the intents dir and returns the findings,
// the permission warnings, and the record count.
func lint(root string, now time.Time) (findings, warnings []string, records int, err error) {
	dirs := append(append([]string{}, recdep.States...), recdep.IntentsDir)
	for _, d := range dirs {
		info, err := os.Stat(filepath.Join(root, d))
		if err != nil {
			return nil, nil, 0, err
		}
		if mode := info.Mode().Perm(); mode != 0o700 {
			warnings = append(warnings, fmt.Sprintf("%s: warning: directory mode %04o, want 0700", d, mode))
		}
	}
	for _, drawer := range recdep.States {
		f, w, n, err := lintDrawer(root, drawer)
		if err != nil {
			return nil, nil, 0, err
		}
		findings = append(findings, f...)
		warnings = append(warnings, w...)
		records += n
	}
	f, err := lintIntents(root, now)
	if err != nil {
		return nil, nil, 0, err
	}
	findings = append(findings, f...)
	return findings, warnings, records, nil
}

// lintDrawer checks every file in one drawer against the entry grammar.
func lintDrawer(root, drawer string) (findings, warnings []string, records int, err error) {
	dir := filepath.Join(root, drawer)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, 0, err
	}
	report := func(name, format string, args ...any) {
		findings = append(findings, drawer+"/"+name+": "+fmt.Sprintf(format, args...))
	}
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") {
			report(name, "no .md suffix, the TUI ignores it")
			continue
		}
		records++
		info, err := d.Info()
		if err != nil {
			return nil, nil, 0, err
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			warnings = append(warnings, fmt.Sprintf("%s/%s: warning: mode %04o, want 0600", drawer, name, mode))
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, 0, err
		}
		e := recdep.ParseEntry(name, string(body))
		if e.TS.IsZero() {
			report(name, "filename lacks a parseable UTC stamp")
		}
		if e.Source == "" {
			report(name, "header line missing the [source] tag")
		}
		if e.Who == "" {
			report(name, "header line missing the \"who: summary\" shape")
		}
		lines := strings.Split(e.Body, "\n")
		if len(lines) >= 3 && !looksLikeURL(e.URL) {
			report(name, "second line is not a URL")
		}
		// The preview quotes third-party text verbatim, so "--- " and
		// "stale " lines there can be content: the stale grammar check
		// anchors to the one line ParseEntry recognizes, and look-alike
		// marker or stale lines in the preview are warnings, not findings.
		preview := lines
		for i, line := range lines {
			if recdep.MarkerKind(line) != "" {
				preview = lines[:i]
				break
			}
		}
		warn := func(format string, args ...any) {
			warnings = append(warnings, drawer+"/"+name+": warning: "+fmt.Sprintf(format, args...))
		}
		seenAt := -1
		staleLines := 0
		staleInPreview := false
		for i, line := range preview {
			if seenAt < 0 && strings.HasPrefix(line, "seen ") {
				seenAt = i
			}
			if strings.HasPrefix(line, "stale ") {
				staleLines++
			}
			if line == e.StaleLine && e.StaleLine != "" {
				staleInPreview = true
			}
			if strings.HasPrefix(line, "--- ") {
				warn("suspicious marker line %q: unknown kind or unparseable time", line)
			}
		}
		if e.StaleLine != "" && len(strings.Fields(e.StaleLine)) < 3 {
			// A trailing stale line after marker sections may be verbatim
			// section text, so only the pre-marker one is grammar.
			if staleInPreview {
				report(name, "stale line has fewer than 3 fields: %q", e.StaleLine)
			} else {
				warn("stale line has fewer than 3 fields: %q", e.StaleLine)
			}
		}
		switch {
		case seenAt < 0:
			report(name, "missing seen line")
		case seenAt != 2:
			report(name, "seen line not third (line %d)", seenAt+1)
		default:
			// Metadata lines sit between seen and the blank line; a stale
			// or marker line ends the region on a previewless record. Any
			// key is allowed, but the shape (lowercase key, one space,
			// non-empty value) is grammar.
			for _, line := range preview[3:] {
				if line == "" || strings.HasPrefix(line, "stale ") || recdep.MarkerKind(line) != "" {
					break
				}
				pair, ok := recdep.CutMeta(line)
				if !ok {
					report(name, "malformed metadata line %q", line)
					continue
				}
				switch pair.Key {
				case "seen", "path", "url":
					report(name, "reserved metadata key %q", pair.Key)
				}
			}
		}
		if staleLines > 1 {
			warn("more than one stale line (%d)", staleLines)
		}
	}
	return findings, warnings, records, nil
}

// lintIntents checks the intents dir: intents without an entry line,
// abandoned dictation temp files, and approvals whose entry resolves
// nowhere.
func lintIntents(root string, now time.Time) (findings []string, err error) {
	dir := filepath.Join(root, recdep.IntentsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	report := func(name, format string, args ...any) {
		findings = append(findings, recdep.IntentsDir+"/"+name+": "+fmt.Sprintf(format, args...))
	}
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		name := d.Name()
		switch {
		case strings.HasSuffix(name, ".intent.tmp"):
			info, err := d.Info()
			if err != nil {
				return nil, err
			}
			if now.Sub(info.ModTime()) > abandonedAge {
				report(name, "abandoned dictation (older than one day)")
			}
		case strings.HasSuffix(name, ".intent"):
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			if _, ok := entryPathOf(string(body)); !ok {
				report(name, "missing entry line")
			}
		case strings.HasSuffix(name, ".publish"):
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			recorded, ok := entryPathOf(string(body))
			if !ok {
				report(name, "missing entry line")
				continue
			}
			if !resolves(root, recorded) {
				report(name, "entry %s resolves nowhere", recorded)
			}
		}
	}
	return findings, nil
}

// entryPathOf parses the "entry <absolute path>" line of an intent or
// approval file.
func entryPathOf(body string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		if path, ok := strings.CutPrefix(line, "entry "); ok && path != "" {
			return path, true
		}
	}
	return "", false
}

// resolves reports whether an approval's recorded entry still exists:
// at the recorded path, or under any drawer by filename, the same
// fallback the thinkpol actor uses.
func resolves(root, recorded string) bool {
	if _, err := os.Stat(recorded); err == nil {
		return true
	}
	name := filepath.Base(recorded)
	for _, s := range recdep.States {
		if _, err := os.Stat(filepath.Join(root, s, name)); err == nil {
			return true
		}
	}
	return false
}

// looksLikeURL reports whether s parses as an absolute URL with a
// scheme, the shape the record grammar expects on line 2.
func looksLikeURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
