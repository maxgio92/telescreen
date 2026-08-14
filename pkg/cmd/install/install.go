// Package install enrolls the pipeline: it writes the embedded agent
// skills and systemd user units, creates the state and config dirs,
// and enables the units, so the telescreen binary is self-sufficient.
package install

import (
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/internal/recdep"
	minitrueskill "github.com/maxgio92/telescreen/minitrue"
	speakwriteskill "github.com/maxgio92/telescreen/speakwrite"
)

// units holds the systemd user unit templates. Each service template
// carries an "ExecStart={{exec}} <component>" line; install replaces
// the placeholder with the running binary's absolute path.
//
//go:embed units
var units embed.FS

// component is one enrollable role: its unit files, the unit systemctl
// enables, and its agent skill when it has one.
type component struct {
	name   string
	units  []string
	enable string
	skill  []byte
}

var components = []component{
	{"minitrue", []string{"minitrue.service", "minitrue.timer"}, "minitrue.timer", minitrueskill.SkillMD},
	{"speakwrite", []string{"speakwrite.service", "speakwrite.path"}, "speakwrite.path", speakwriteskill.SkillMD},
	{"thinkpol", []string{"thinkpol.service", "thinkpol.path"}, "thinkpol.path", nil},
}

// systemctl runs one systemctl --user invocation. Tests replace it.
var systemctl = func(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// New returns the install subcommand.
func New() *cobra.Command {
	var dryRun bool
	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.name
	}
	cmd := &cobra.Command{
		Use:       "install [component]...",
		Short:     "enroll the stack, or one component (" + strings.Join(names, ", ") + ")",
		Args:      cobra.OnlyValidArgs,
		ValidArgs: names,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.OutOrStdout(), args, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what install would write, without writing")
	return cmd
}

// selected returns the components to enroll: all of them without args,
// else the named ones in table order.
func selected(args []string) []component {
	if len(args) == 0 {
		return components
	}
	var out []component
	for _, c := range components {
		for _, a := range args {
			if a == c.name {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func run(out io.Writer, args []string, dryRun bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgDir := os.Getenv("XDG_CONFIG_HOME")
	if cfgDir == "" {
		cfgDir = filepath.Join(home, ".config")
	}
	unitsDir := filepath.Join(cfgDir, "systemd", "user")
	skillsDir := filepath.Join(home, ".claude", "skills")

	write := func(path string, content []byte) error {
		if dryRun {
			_, _ = fmt.Fprintln(out, "would write "+path)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "wrote "+path)
		return nil
	}

	// systemd requires quoting when the executable path carries spaces.
	if strings.ContainsAny(exe, " \t") {
		exe = `"` + exe + `"`
	}

	picked := selected(args)
	for _, c := range picked {
		if c.skill != nil {
			if err := write(filepath.Join(skillsDir, c.name, "SKILL.md"), c.skill); err != nil {
				return err
			}
		}
		for _, u := range c.units {
			b, err := units.ReadFile("units/" + u)
			if err != nil {
				return err
			}
			unit := strings.ReplaceAll(string(b), "{{exec}}", exe)
			if err := write(filepath.Join(unitsDir, u), []byte(unit)); err != nil {
				return err
			}
		}
	}
	if dryRun {
		// The plan states everything a real run does.
		_, _ = fmt.Fprintln(out, "would create the state dirs and "+filepath.Join(cfgDir, "recdep"))
		_, _ = fmt.Fprintln(out, "would run systemctl --user daemon-reload")
		for _, c := range picked {
			_, _ = fmt.Fprintln(out, "would enable "+c.enable)
		}
		return nil
	}
	if _, err := recdep.StateRoot(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfgDir, "recdep"), 0o755); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	for _, c := range picked {
		if err := systemctl("enable", "--now", c.enable); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "enabled "+c.enable)
	}
	return nil
}
