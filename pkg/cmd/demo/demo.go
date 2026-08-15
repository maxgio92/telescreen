// Package demo seeds one sample record into the tube and opens the
// screen, so a fresh install shows something without any agent
// enrolled. Re-running is a no-op while a demo record sits in any
// drawer, so a record the user already moved does not respawn.
package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// suffix identifies the demo record in any drawer, whatever its stamp.
const suffix = "-github-demo-42.md"

// New returns the demo subcommand. openTUI is the root command's TUI
// runner, injected because root imports this package.
func New(openTUI func(root string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "demo",
		Short: "seed one sample record and open the screen",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := recdep.StateRoot()
			if err != nil {
				return err
			}
			if err := seed(root, time.Now().UTC()); err != nil {
				return err
			}
			return openTUI(root)
		},
	}
}

// seed writes the sample record into tube, unless a demo record
// already sits in one of the four drawers.
func seed(root string, now time.Time) error {
	for _, s := range recdep.States {
		entries, err := os.ReadDir(filepath.Join(root, s))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), suffix) {
				return nil
			}
		}
	}
	body := fmt.Sprintf(`[github] julia: review requested on demo#42: feat(ministry): ration the chocolate
https://github.com/example/demo/pull/42
seen %s

the ration goes from 30 grammes to 20. the announcement says it went up.
`, now.Format(time.RFC3339))
	name := now.Format(recdep.StampLayout) + suffix
	return os.WriteFile(filepath.Join(root, "tube", name), []byte(body), 0o600)
}
