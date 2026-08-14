// Package speakwrite runs the drafting runner: consume dictation
// intents and append drafts; publishing is thinkpol's job. The systemd
// path unit runs it when an intent lands; the agent, prompt, tool
// allowlist, and timeout come from ~/.config/speakwrite.env with the
// defaults below.
package speakwrite

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/agentrun"
	"github.com/maxgio92/telescreen/internal/recdep"
)

const (
	envFile       = "speakwrite.env"
	defaultPrompt = "/speakwrite draft"
	defaultTools  = "Bash Read Write Glob Grep mcp__plugin_slack_slack__slack_read_thread mcp__linear-server__get_issue mcp__linear-server__list_comments"
)

// New returns the speakwrite subcommand.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "speakwrite",
		Short: "run the drafting runner once (consume dictation intents, append drafts)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run()
		},
	}
}

// hasIntent reports whether any name is a dictation intent. Without one
// the run exits fast and never starts the agent.
func hasIntent(names []string) bool {
	for _, n := range names {
		if strings.HasSuffix(n, ".intent") {
			return true
		}
	}
	return false
}

func run() error {
	root, err := recdep.StateRoot()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(root, recdep.IntentsDir))
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !hasIntent(names) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	vars, err := agentrun.ParseEnvFile(filepath.Join(home, ".config", envFile))
	if err != nil {
		return err
	}
	inv, err := agentrun.Resolve(vars, "SPEAKWRITE", defaultPrompt, defaultTools, filepath.Join(root, "draft.log"))
	if err != nil {
		return err
	}
	return agentrun.Exec(inv)
}
