// Package install enrolls the pipeline: it writes the embedded agent
// skills and systemd user units, creates the state and config dirs,
// and enables the units, so the telescreen binary is self-sufficient.
package install

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
// enables, its agent skill when it has one, and whether it owns a
// telescreen.yaml key (thinkpol does not: the actor is deterministic).
type component struct {
	name      string
	units     []string
	enable    string
	skill     []byte
	configKey bool
}

var components = []component{
	{"minitrue", []string{"minitrue.service", "minitrue.timer"}, "minitrue.timer", minitrueskill.SkillMD, true},
	{"speakwrite", []string{"speakwrite.service", "speakwrite.path"}, "speakwrite.path", speakwriteskill.SkillMD, true},
	{"thinkpol", []string{"thinkpol.service", "thinkpol.path"}, "thinkpol.path", nil, false},
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
	var force bool
	cmd := &cobra.Command{
		Use:       "install [component]...",
		Short:     "enroll the stack, or one component (" + strings.Join(names, ", ") + ")",
		Args:      cobra.OnlyValidArgs,
		ValidArgs: names,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.OutOrStdout(), args, dryRun, force)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what install would write, without writing")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite installed skills the user may have edited")
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

func run(out io.Writer, args []string, dryRun, force bool) error {
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
			// A skill is a user file once installed: the embed only seeds
			// it, and edits survive re-installs unless force says otherwise.
			path := filepath.Join(skillsDir, c.name, "SKILL.md")
			if _, err := os.Stat(path); err == nil && !force {
				_, _ = fmt.Fprintln(out, "kept "+path+" (exists; --force overwrites)")
			} else if err := write(path, c.skill); err != nil {
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
	if err := seedConfig(out, filepath.Join(cfgDir, "telescreen.yaml"), picked, dryRun); err != nil {
		return err
	}
	if dryRun {
		// The plan states everything a real run does.
		_, _ = fmt.Fprintln(out, "would create the state dirs")
		_, _ = fmt.Fprintln(out, "would run systemctl --user daemon-reload")
		for _, c := range picked {
			_, _ = fmt.Fprintln(out, "would enable "+c.enable)
		}
		return nil
	}
	if _, err := recdep.StateRoot(); err != nil {
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

// seedConfig upserts telescreen.yaml at the key level: each picked
// component's key is seeded with {agent: claude} only when absent, and
// existing keys and values are never modified or removed. Only the
// picked components seed, so a component-scoped install creates the
// file with its own key alone; the next install appends the rest. A
// missing section is appended as text at the end instead of a
// yaml.Node round-trip: an append never rewrites existing bytes, so
// user comments and formatting survive for free. The append assumes a
// block-style top-level mapping, so the result is re-parsed and rolled
// back when it does not decode. --force does not reach here; it
// covers skills only.
func seedConfig(out io.Writer, path string, picked []component, dryRun bool) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	present := map[string]bool{}
	if len(b) > 0 {
		var top map[string]any
		if err := yaml.Unmarshal(b, &top); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for k := range top {
			present[k] = true
		}
	}
	add := ""
	var seeded []string
	for _, c := range picked {
		if !c.configKey || present[c.name] {
			continue
		}
		add += c.name + ":\n  agent: claude\n"
		seeded = append(seeded, c.name)
		if dryRun {
			_, _ = fmt.Fprintln(out, "would seed "+c.name+" in "+path)
		}
	}
	if add == "" || dryRun {
		return nil
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		add = "\n" + add
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(add); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var check map[string]any
	if err := yaml.Unmarshal(after, &check); err != nil {
		_ = os.WriteFile(path, b, 0o644)
		return fmt.Errorf("%s: the seeded keys do not parse against the existing style, restored the file: %w", path, err)
	}
	for _, name := range seeded {
		_, _ = fmt.Fprintln(out, "seeded "+name+" in "+path)
	}
	return nil
}
