// Package version prints the release version goreleaser stamps at
// build time.
package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is stamped by goreleaser through ldflags.
var version = "dev"

// Version returns the stamped version, "dev" on unstamped builds.
func Version() string {
	return version
}

// New returns the version subcommand.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "telescreen "+version)
		},
	}
}
