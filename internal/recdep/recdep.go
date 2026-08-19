// Package recdep implements the queue contract in docs/contracts/recdep.md: the state
// directories, the entry file format with its marker sections, and the
// two writes the contract sanctions (the marker append and the rename
// between states). The telescreen TUI and the thinkpol actor both build
// on it.
package recdep

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// States are the queue subdirectories, in view order.
var States = []string{"tube", "desk", "upsub", "files"}

// IntentsDir holds speakwrite dictation intents and publish approvals
// next to the states.
const IntentsDir = "intents"

// StampLayout is the UTC timestamp prefix of entry filenames.
const StampLayout = "20060102T150405Z"

// Entry is one parsed queue entry file.
type Entry struct {
	Name    string // filename, e.g. 20260811T142145Z-slack-wes-topic.md
	Source  string // tag from the header line, e.g. slack, github, linear
	Who     string
	Summary string
	URL     string
	Body    string
	TS      time.Time
	// Seen is the value of the third line's "seen <time>" marker, verbatim;
	// empty when the line is missing or misplaced.
	Seen string
	// Meta holds the metadata lines that follow the seen line, in file
	// order: structured provider facts such as org/repo (github),
	// channel or dm (slack), project/ticket (linear).
	Meta []MetaPair
	// Stale is the reason from a trailing "stale <reason> <time>" marker
	// appended by the producer's revalidation pass; empty when fresh.
	// StaleLine keeps the raw marker line for the detail pane.
	Stale     string
	StaleLine string
	// Mark is the kind of the last speakwrite marker section, one of
	// dictated, draft, published, discarded; empty when none. Sections
	// start with a "--- <kind> <time>" line appended by the drafting
	// runner (discarded by the consumer). MarkTime keeps the marker's
	// time verbatim.
	Mark     string
	MarkTime string
}

// MetaPair is one metadata line, split at the first space.
type MetaPair struct {
	Key   string
	Value string
}

// MetaValue returns the value of the entry's last metadata line with
// the given key (duplicates are last-wins, matching export), or "".
// MetaValue is the lookup seam for metadata-scoped rule matching;
// export iterates Meta directly.
func (e Entry) MetaValue(key string) string {
	v := ""
	for _, p := range e.Meta {
		if p.Key == key {
			v = p.Value
		}
	}
	return v
}

// CutMeta splits a metadata line into its key and value. ok is false
// when the line does not match the grammar: a lowercase [a-z_]+ key,
// one space, a non-empty value.
func CutMeta(line string) (MetaPair, bool) {
	key, value, found := strings.Cut(line, " ")
	if !found || key == "" || value == "" || value[0] == ' ' {
		return MetaPair{}, false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && r != '_' {
			return MetaPair{}, false
		}
	}
	return MetaPair{Key: key, Value: value}, true
}

// markerKinds are the only speakwrite marker kinds docs/contracts/recdep.md recognizes.
// Draft and guidance text is appended verbatim and can contain "--- "
// lines of its own (unified diffs quote "--- a/file"), so a "--- " line
// counts as a marker only when its kind is one of these.
var markerKinds = map[string]bool{
	"dictated":  true,
	"draft":     true,
	"published": true,
	"discarded": true,
}

// MarkerKind returns the marker kind when line is a speakwrite marker
// line, and "" otherwise. The preview quotes third-party text verbatim,
// so a marker also needs a parseable RFC 3339 time to count.
func MarkerKind(line string) string {
	if !strings.HasPrefix(line, "--- ") {
		return ""
	}
	f := strings.Fields(line)
	if len(f) < 3 || !markerKinds[f[1]] {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, f[2]); err != nil {
		return ""
	}
	return f[1]
}

// ParseEntry builds an Entry from a queue filename and its body.
// Body format: line 1 "[<source>] <who>: <summary>", line 2 the link URL,
// line 3 "seen <time>". Missing pieces degrade to empty fields.
func ParseEntry(name, body string) Entry {
	e := Entry{Name: name, Body: strings.TrimRight(body, "\n")}
	if len(name) >= len(StampLayout) {
		if ts, err := time.Parse(StampLayout, name[:len(StampLayout)]); err == nil {
			e.TS = ts
		}
	}
	lines := strings.Split(e.Body, "\n")
	if len(lines) > 0 {
		head := lines[0]
		if strings.HasPrefix(head, "[") {
			if end := strings.Index(head, "]"); end > 0 {
				e.Source = head[1:end]
				head = strings.TrimSpace(head[end+1:])
			}
		}
		if who, summary, ok := strings.Cut(head, ": "); ok {
			e.Who, e.Summary = who, summary
		} else {
			e.Summary = head
		}
	}
	if len(lines) > 2 {
		if v, ok := strings.CutPrefix(lines[2], "seen "); ok {
			e.Seen = v
		}
	}
	// Metadata lines sit between the seen line and the blank line. A
	// stale or marker line stops the scan (stale lands there on a
	// previewless record), and so does the first malformed line: the
	// parser tolerates it, verify flags it.
	if e.Seen != "" {
		for _, line := range lines[3:] {
			if line == "" || strings.HasPrefix(line, "stale ") || MarkerKind(line) != "" {
				break
			}
			p, ok := CutMeta(line)
			if !ok {
				break
			}
			e.Meta = append(e.Meta, p)
		}
	}
	// Everything from the first "--- " line onward is marker sections,
	// not preview. The last marker wins for presentation.
	for _, line := range lines {
		if kind := MarkerKind(line); kind != "" {
			e.Mark, e.MarkTime = kind, ""
			if f := strings.Fields(line); len(f) >= 3 {
				e.MarkTime = f[2]
			}
		}
	}
	// The stale line sits at the very end, or just above the marker
	// sections when the runner appended after the revalidation pass.
	staleAt := len(lines) - 1
	if !strings.HasPrefix(lines[staleAt], "stale ") {
		for i, line := range lines {
			if MarkerKind(line) != "" {
				for staleAt = i - 1; staleAt > 0 && lines[staleAt] == ""; staleAt-- {
				}
				break
			}
		}
	}
	if staleAt > 0 && strings.HasPrefix(lines[staleAt], "stale ") {
		e.StaleLine = lines[staleAt]
		if f := strings.Fields(lines[staleAt]); len(f) >= 2 {
			e.Stale = f[1]
		}
	}
	// The URL line can collide with the stale marker or a speakwrite
	// marker on a minimal entry; the marker wins and the entry has no URL.
	if len(lines) > 1 && lines[1] != e.StaleLine && MarkerKind(lines[1]) == "" {
		e.URL = strings.TrimSpace(lines[1])
	}
	return e
}

// Detail renders the pane body for an entry stored at path: the content
// line, the preview, then the entry's file path (so the full file is one
// cat or one agent handle away), the URL, the metadata lines, and the
// seen line last.
func (e Entry) Detail(path string) string {
	lines := strings.Split(e.Body, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[2], "seen ") {
		return e.Body + "\npath " + path
	}
	// path and url carry labels in the seen line's style, so the two
	// look-alike lines cannot be confused.
	out := []string{lines[0]}
	out = append(out, lines[3+len(e.Meta):]...)
	out = append(out, "path "+path, "url "+lines[1])
	for _, p := range e.Meta {
		out = append(out, p.Key+" "+p.Value)
	}
	out = append(out, lines[2])
	return strings.Join(out, "\n")
}

// DetailTail is the number of labeled lines Detail appends after the
// preview (path, url, metadata, seen), so the TUI protects them when
// capping the pane. A body without a well-placed seen line gets only
// the path line.
func (e Entry) DetailTail() int {
	lines := strings.Split(e.Body, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[2], "seen ") {
		return 1
	}
	return 3 + len(e.Meta)
}

// LastSection returns the text of the body's last marker section of the
// given kind: the lines after its marker line up to the next marker or
// the end, trailing blank lines dropped. ok reports whether the body has
// such a marker at all.
func LastSection(body, kind string) (text string, ok bool) {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if MarkerKind(line) == kind {
			start = i + 1
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if MarkerKind(lines[i]) != "" {
			end = i
			break
		}
	}
	g := lines[start:end]
	for len(g) > 0 && strings.TrimSpace(g[len(g)-1]) == "" {
		g = g[:len(g)-1]
	}
	return strings.Join(g, "\n"), true
}

// StateRoot returns the queue root and creates the four state dirs plus
// the intents dir the speakwrite dictation writes into. New dirs are
// 0700: the store is private to the user. Existing dirs keep their
// modes (verify warns instead of chmodding).
func StateRoot() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	root := filepath.Join(base, "recdep")
	for _, s := range append(slices.Clone(States), IntentsDir) {
		if err := os.MkdirAll(filepath.Join(root, s), 0o700); err != nil {
			return "", err
		}
	}
	return root, nil
}

// MoveEntry renames one entry file between state dirs under root.
func MoveEntry(root, name, from, to string) error {
	return os.Rename(filepath.Join(root, from, name), filepath.Join(root, to, name))
}

// DraftDiscarded reports whether a discarded marker sits after the last
// draft section, meaning the human withdrew the draft an approval may
// still point at.
func DraftDiscarded(body string) bool {
	lastDraft, lastDiscarded := -1, -1
	for i, line := range strings.Split(body, "\n") {
		switch MarkerKind(line) {
		case "draft":
			lastDraft = i
		case "discarded":
			lastDiscarded = i
		}
	}
	return lastDiscarded > lastDraft && lastDiscarded >= 0
}

// AppendMarker appends a marker line to an entry file with the contract's
// newline discipline: the marker must start its own line, so prepend a
// newline when the file lacks a trailing one.
func AppendMarker(path, marker string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) > 0 && !bytes.HasSuffix(body, []byte("\n")) {
		marker = "\n" + marker
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0) // no O_CREATE: the mode is unused
	if err != nil {
		return err
	}
	if _, err := f.WriteString(marker); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
