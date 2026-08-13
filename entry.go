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
var states = []string{"inbox", "todo", "waiting", "archive"}

type entry struct {
	name    string // filename, e.g. 20260811T142145Z-slack-wes-topic.md
	source  string // tag from the header line, e.g. slack, github, linear
	who     string
	summary string
	url     string
	body    string
	ts      time.Time
}

const stampLayout = "20060102T150405Z"

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
	if len(lines) > 1 {
		e.url = strings.TrimSpace(lines[1])
	}
	return e
}

// stateRoot returns the queue root and creates the four state dirs.
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
	sort.Slice(out, func(i, j int) bool { return out[i].name > out[j].name })
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
