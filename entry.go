package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// states are the queue subdirectories, in view order.
var states = []string{"tube", "desk", "upsub", "files"}

// intentsDir holds speakwrite dictation intents next to the states.
const intentsDir = "intents"

type entry struct {
	name    string // filename, e.g. 20260811T142145Z-slack-wes-topic.md
	source  string // tag from the header line, e.g. slack, github, linear
	who     string
	summary string
	url     string
	body    string
	ts      time.Time
	// stale is the reason from a trailing "stale <reason> <time>" marker
	// appended by the producer's revalidation pass; empty when fresh.
	// staleLine keeps the raw marker line for the detail pane.
	stale     string
	staleLine string
	// mark is the kind of the last speakwrite marker section, one of
	// dictated, draft, published, discarded; empty when none. Sections
	// start with a "--- <kind> <time>" line appended by the drafting
	// runner (discarded by the consumer). markTime keeps the marker's
	// time verbatim.
	mark     string
	markTime string
}

const stampLayout = "20060102T150405Z"

// markerKinds are the only speakwrite marker kinds RECDEP.md recognizes.
// Draft and guidance text is appended verbatim and can contain "--- "
// lines of its own (unified diffs quote "--- a/file"), so a "--- " line
// counts as a marker only when its kind is one of these.
var markerKinds = map[string]bool{
	"dictated":  true,
	"draft":     true,
	"published": true,
	"discarded": true,
}

// markerKind returns the marker kind when line is a speakwrite marker
// line, and "" otherwise. The preview quotes third-party text verbatim,
// so a marker also needs a parseable RFC 3339 time to count.
func markerKind(line string) string {
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

// parseEntry builds an entry from a queue filename and its body.
// Body format: line 1 "[<source>] <who>: <summary>", line 2 the link URL,
// line 3 "seen <time>". Missing pieces degrade to empty fields.
func parseEntry(name, body string) entry {
	e := entry{name: name, body: strings.TrimRight(body, "\n")}
	if len(name) >= len(stampLayout) {
		if ts, err := time.Parse(stampLayout, name[:len(stampLayout)]); err == nil {
			e.ts = ts
		}
	}
	lines := strings.Split(e.body, "\n")
	if len(lines) > 0 {
		head := lines[0]
		if strings.HasPrefix(head, "[") {
			if end := strings.Index(head, "]"); end > 0 {
				e.source = head[1:end]
				head = strings.TrimSpace(head[end+1:])
			}
		}
		if who, summary, ok := strings.Cut(head, ": "); ok {
			e.who, e.summary = who, summary
		} else {
			e.summary = head
		}
	}
	// Everything from the first "--- " line onward is marker sections,
	// not preview. The last marker wins for presentation.
	for _, line := range lines {
		if kind := markerKind(line); kind != "" {
			e.mark, e.markTime = kind, ""
			if f := strings.Fields(line); len(f) >= 3 {
				e.markTime = f[2]
			}
		}
	}
	// The stale line sits at the very end, or just above the marker
	// sections when the runner appended after the revalidation pass.
	staleAt := len(lines) - 1
	if !strings.HasPrefix(lines[staleAt], "stale ") {
		for i, line := range lines {
			if markerKind(line) != "" {
				for staleAt = i - 1; staleAt > 0 && lines[staleAt] == ""; staleAt-- {
				}
				break
			}
		}
	}
	if staleAt > 0 && strings.HasPrefix(lines[staleAt], "stale ") {
		e.staleLine = lines[staleAt]
		if f := strings.Fields(lines[staleAt]); len(f) >= 2 {
			e.stale = f[1]
		}
	}
	// The URL line can collide with the stale marker or a speakwrite
	// marker on a minimal entry; the marker wins and the entry has no URL.
	if len(lines) > 1 && lines[1] != e.staleLine && markerKind(lines[1]) == "" {
		e.url = strings.TrimSpace(lines[1])
	}
	return e
}

// detail renders the pane body for an entry stored at path: the content
// line, the preview, then the entry's file path (so the full file is one
// cat or one agent handle away), the URL, and the seen line last.
func (e entry) detail(path string) string {
	lines := strings.Split(e.body, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[2], "seen ") {
		return e.body + "\npath " + path
	}
	// path and url carry labels in the seen line's style, so the two
	// look-alike lines cannot be confused.
	out := []string{lines[0]}
	out = append(out, lines[3:]...)
	out = append(out, "path "+path, "url "+lines[1], lines[2])
	return strings.Join(out, "\n")
}

// stateRoot returns the queue root and creates the four state dirs plus
// the intents dir the speakwrite dictation writes into.
func stateRoot() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	root := filepath.Join(base, "recdep")
	for _, s := range states {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Join(root, intentsDir), 0o755); err != nil {
		return "", err
	}
	return root, nil
}

// loadState reads one state dir and returns its entries newest first.
func loadState(root, state string) ([]entry, error) {
	dir := filepath.Join(root, state)
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []entry
	for _, d := range names {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, d.Name()))
		if err != nil {
			continue // raced with a move; the next refresh catches up
		}
		out = append(out, parseEntry(d.Name(), string(body)))
	}
	// Fresh entries before stale ones, newest first within each group.
	sort.Slice(out, func(i, j int) bool {
		if si, sj := out[i].stale != "", out[j].stale != ""; si != sj {
			return sj
		}
		return out[i].name > out[j].name
	})
	return out, nil
}

// moveEntry renames one entry file between state dirs under root.
func moveEntry(root, name, from, to string) error {
	return os.Rename(filepath.Join(root, from, name), filepath.Join(root, to, name))
}

// age renders a compact duration since ts, e.g. 42m, 3h, 2d.
func age(ts time.Time, now time.Time) string {
	if ts.IsZero() {
		return "?"
	}
	d := now.Sub(ts)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
