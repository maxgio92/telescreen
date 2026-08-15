// Package export dumps every record in the four drawers to stdout as
// one JSON document, so scripts and agents read the queue without
// reimplementing the docs/contracts/recdep.md grammar. All parsing stays in
// internal/recdep; export only shapes the result.
package export

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// sectionKinds are the marker sections a record can carry, per the
// docs/contracts/recdep.md grammar.
var sectionKinds = []string{"dictated", "draft", "published", "discarded"}

// record is one drawer entry in the export document.
type record struct {
	Drawer   string            `json:"drawer"`
	File     string            `json:"file"`
	TS       string            `json:"ts,omitempty"`
	Source   string            `json:"source"`
	Who      string            `json:"who"`
	Summary  string            `json:"summary"`
	URL      string            `json:"url"`
	Seen     string            `json:"seen"`
	Stale    *stale            `json:"stale"`
	Marker   *marker           `json:"marker"`
	Sections map[string]string `json:"sections,omitempty"`
	Body     string            `json:"body"`
}

// stale is the record's stale marker, when the producer's revalidation
// pass appended one.
type stale struct {
	Reason string `json:"reason"`
}

// marker is the record's last speakwrite marker, when the body has one.
type marker struct {
	Kind string `json:"kind"`
	Time string `json:"time"`
}

// New returns the export subcommand.
func New() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "write every record in the four drawers to stdout as one JSON document",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output != "json" {
				return fmt.Errorf("unsupported output %q (supported: json)", output)
			}
			root, err := recdep.StateRoot()
			if err != nil {
				return err
			}
			return run(cmd.OutOrStdout(), root)
		},
	}
	cmd.Flags().StringVar(&output, "output", "json", "output format (json)")
	return cmd
}

// run writes the export document for the queue under root: drawers in
// view order, files newest first within each, like the TUI.
func run(out io.Writer, root string) error {
	records := []record{}
	for _, drawer := range recdep.States {
		dir := filepath.Join(root, drawer)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		var names []string
		for _, d := range entries {
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				continue
			}
			names = append(names, d.Name())
		}
		// ReadDir returns names sorted ascending; the TUI shows newest first.
		slices.Reverse(names)
		for _, name := range names {
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return err
			}
			records = append(records, toRecord(drawer, recdep.ParseEntry(name, string(body))))
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

// toRecord shapes one parsed entry into its export form.
func toRecord(drawer string, e recdep.Entry) record {
	r := record{
		Drawer:  drawer,
		File:    e.Name,
		Source:  e.Source,
		Who:     e.Who,
		Summary: e.Summary,
		URL:     e.URL,
		Seen:    e.Seen,
		Body:    e.Body,
	}
	if !e.TS.IsZero() {
		r.TS = e.TS.Format(time.RFC3339)
	}
	if e.Stale != "" {
		r.Stale = &stale{Reason: e.Stale}
	}
	if e.Mark != "" {
		r.Marker = &marker{Kind: e.Mark, Time: e.MarkTime}
	}
	for _, kind := range sectionKinds {
		if text, ok := recdep.LastSection(e.Body, kind); ok {
			if r.Sections == nil {
				r.Sections = map[string]string{}
			}
			r.Sections[kind] = text
		}
	}
	return r
}
