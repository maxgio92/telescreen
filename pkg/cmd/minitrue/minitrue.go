// Package minitrue runs the personal-watch producer: poll the watches,
// enqueue hits. The systemd timer runs it every 10 minutes; the agent,
// prompt, tool allowlist, and timeout come from ~/.config/minitrue.env
// with the defaults below.
package minitrue

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/agentrun"
	"github.com/maxgio92/telescreen/internal/recdep"
)

const (
	envFile       = "minitrue.env"
	defaultPrompt = "/minitrue produce"
	defaultTools  = "Bash Read Write Glob Grep mcp__plugin_slack_slack__slack_search_public_and_private mcp__plugin_slack_slack__slack_read_thread mcp__linear-server__list_issues mcp__linear-server__list_comments"
)

// New returns the minitrue subcommand.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "minitrue",
		Short: "run the personal-watch producer once (poll Slack/GitHub/Linear, enqueue hits)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run()
		},
	}
}

func run() error {
	root, err := recdep.StateRoot()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	vars, err := agentrun.ParseEnvFile(filepath.Join(home, ".config", envFile))
	if err != nil {
		return err
	}
	inv, err := agentrun.Resolve(vars, "MINITRUE", defaultPrompt, defaultTools, filepath.Join(root, "produce.log"))
	if err != nil {
		return err
	}
	return agentrun.Exec(inv)
}
