package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// loadState reads one state dir and returns its entries newest first.
func loadState(root, state string) ([]recdep.Entry, error) {
	dir := filepath.Join(root, state)
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []recdep.Entry
	for _, d := range names {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, d.Name()))
		if err != nil {
			continue // raced with a move; the next refresh catches up
		}
		out = append(out, recdep.ParseEntry(d.Name(), string(body)))
	}
	// Fresh entries before stale ones, newest first within each group.
	sort.Slice(out, func(i, j int) bool {
		if si, sj := out[i].Stale != "", out[j].Stale != ""; si != sj {
			return sj
		}
		return out[i].Name > out[j].Name
	})
	return out, nil
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
