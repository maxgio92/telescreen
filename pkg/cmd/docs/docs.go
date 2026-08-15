// Package docs generates the CLI reference from the command tree
// itself, so the reference cannot drift from the binary.
package docs

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// New returns the hidden docs subcommand. It renders one markdown page
// per command into the target directory. Being hidden, it stays out of
// the generated tree.
func New() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:    "docs",
		Short:  "generate the CLI reference as markdown",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return doc.GenMarkdownTree(cmd.Root(), dir)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "docs/reference", "directory the markdown pages land in")
	return cmd
}
