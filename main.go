// Command telescreen is the single binary of the pipeline: the
// dashboard on the bare command, the minitrue producer, the speakwrite
// agent, the thinkpol actor, and the installer as
// subcommands. See pkg/cmd for each command.
package main

import (
	"fmt"
	"os"

	"github.com/maxgio92/telescreen/pkg/cmd/root"
)

func main() {
	if err := root.New().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
