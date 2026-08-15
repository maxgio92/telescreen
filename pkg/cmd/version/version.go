// Package version prints the release version goreleaser stamps at
// build time, falling back to the module version the Go toolchain
// embeds in go-installed binaries.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// version is stamped by goreleaser through ldflags.
var version = "dev"

// Version returns the stamped version, then the embedded module
// version, then "dev" on source builds.
func Version() string {
	return resolve(version, buildInfoVersion())
}

// buildInfoVersion returns the module version the toolchain embedded,
// or "" when build info is unavailable.
func buildInfoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

// resolve picks the reported version, without a v prefix: the stamped
// value wins, then the embedded module version, then "dev". A
// "(devel)" or "+dirty" module version means a source build and maps
// to "dev".
func resolve(stamped, buildInfo string) string {
	if stamped != "" && stamped != "dev" {
		return strings.TrimPrefix(stamped, "v")
	}
	if buildInfo != "" && buildInfo != "(devel)" && !strings.HasSuffix(buildInfo, "+dirty") {
		return strings.TrimPrefix(buildInfo, "v")
	}
	return "dev"
}

// New returns the version subcommand.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "telescreen "+Version())
		},
	}
}
