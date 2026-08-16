package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	minitrueskill "github.com/maxgio92/telescreen/minitrue"
)

// isolate points HOME, the config dir, and the state root into temp
// dirs and swaps systemctl for a recorder.
func isolate(t *testing.T) (home string, calls *[][]string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	var got [][]string
	orig := systemctl
	systemctl = func(args ...string) error {
		got = append(got, args)
		return nil
	}
	t.Cleanup(func() { systemctl = orig })
	return home, &got
}

func execute(t *testing.T, args ...string) string {
	t.Helper()
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestInstallAll(t *testing.T) {
	home, calls := isolate(t)
	out := execute(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	unitsDir := filepath.Join(home, ".config", "systemd", "user")
	for unit, component := range map[string]string{
		"minitrue.service":   "minitrue",
		"speakwrite.service": "speakwrite",
		"thinkpol.service":   "thinkpol",
	} {
		b, err := os.ReadFile(filepath.Join(unitsDir, unit))
		if err != nil {
			t.Fatalf("%s: %v", unit, err)
		}
		if want := "ExecStart=" + exe + " " + component + "\n"; !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %q:\n%s", unit, want, b)
		}
	}
	for _, unit := range []string{"minitrue.timer", "speakwrite.path", "thinkpol.path"} {
		if _, err := os.Stat(filepath.Join(unitsDir, unit)); err != nil {
			t.Errorf("%s: %v", unit, err)
		}
	}
	skill, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "minitrue", "SKILL.md"))
	if err != nil {
		t.Fatalf("minitrue skill: %v", err)
	}
	if !bytes.Equal(skill, minitrueskill.SkillMD) {
		t.Error("written minitrue skill differs from the embedded one")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "speakwrite", "SKILL.md")); err != nil {
		t.Errorf("speakwrite skill: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(home, ".local", "state", "recdep", "tube"),
		filepath.Join(home, ".local", "state", "recdep", "intents"),
	} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s: %v", dir, err)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(home, ".config", "telescreen.yaml"))
	if err != nil {
		t.Fatalf("telescreen.yaml: %v", err)
	}
	if want := "minitrue:\n  agent: claude\nspeakwrite:\n  agent: claude\n"; string(cfg) != want {
		t.Errorf("telescreen.yaml = %q, want %q", cfg, want)
	}
	want := [][]string{
		{"daemon-reload"},
		{"enable", "--now", "minitrue.timer"},
		{"enable", "--now", "speakwrite.path"},
		{"enable", "--now", "thinkpol.path"},
	}
	if len(*calls) != len(want) {
		t.Fatalf("systemctl calls = %v, want %v", *calls, want)
	}
	for i, c := range want {
		if strings.Join((*calls)[i], " ") != strings.Join(c, " ") {
			t.Errorf("systemctl call %d = %v, want %v", i, (*calls)[i], c)
		}
	}
	if !strings.Contains(out, "wrote ") {
		t.Errorf("install printed nothing about the writes:\n%s", out)
	}
}

func TestInstallOneComponent(t *testing.T) {
	home, calls := isolate(t)
	execute(t, "thinkpol")

	unitsDir := filepath.Join(home, ".config", "systemd", "user")
	for _, unit := range []string{"thinkpol.service", "thinkpol.path"} {
		if _, err := os.Stat(filepath.Join(unitsDir, unit)); err != nil {
			t.Errorf("%s: %v", unit, err)
		}
	}
	if _, err := os.Stat(filepath.Join(unitsDir, "minitrue.service")); !os.IsNotExist(err) {
		t.Errorf("thinkpol install wrote the minitrue unit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("thinkpol install wrote a skill: %v", err)
	}
	want := [][]string{{"daemon-reload"}, {"enable", "--now", "thinkpol.path"}}
	if len(*calls) != len(want) {
		t.Fatalf("systemctl calls = %v, want %v", *calls, want)
	}
}

func TestInstallDryRunWritesNothing(t *testing.T) {
	home, calls := isolate(t)
	out := execute(t, "--dry-run")

	if !strings.Contains(out, "would write ") {
		t.Errorf("dry run printed no plan:\n%s", out)
	}
	for _, want := range []string{"would seed minitrue in ", "would seed speakwrite in "} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run plan lacks %q:\n%s", want, out)
		}
	}
	for _, dir := range []string{
		filepath.Join(home, ".config", "systemd"),
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".local", "state", "recdep"),
		filepath.Join(home, ".config", "telescreen.yaml"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("dry run created %s: %v", dir, err)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("dry run called systemctl: %v", *calls)
	}
}

func TestInstallRejectsUnknownComponent(t *testing.T) {
	isolate(t)
	cmd := New()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"bigbrother"})
	if err := cmd.Execute(); err == nil {
		t.Error("install accepted an unknown component")
	}
}

// TestInstallSeedsMissingConfigKey pins the key-level upsert: a
// telescreen.yaml with only speakwrite gets minitrue appended and its
// existing bytes stay untouched, comments included.
func TestInstallSeedsMissingConfigKey(t *testing.T) {
	home, _ := isolate(t)
	existing := "# my choices\nspeakwrite:\n  agent: codex   # keep this\n  args: exec {prompt}\n"
	cfgPath := filepath.Join(home, ".config", "telescreen.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	out := execute(t)
	if !strings.Contains(out, "seeded minitrue in "+cfgPath) {
		t.Errorf("install did not report the seed:\n%s", out)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := existing + "minitrue:\n  agent: claude\n"; string(b) != want {
		t.Errorf("telescreen.yaml = %q, want %q", b, want)
	}
}

// TestInstallSeedsWithoutTrailingNewline pins the append onto a file
// lacking a final newline: the seeded key starts on its own line and
// the result stays valid YAML.
func TestInstallSeedsWithoutTrailingNewline(t *testing.T) {
	home, _ := isolate(t)
	existing := "speakwrite:\n  agent: codex"
	cfgPath := filepath.Join(home, ".config", "telescreen.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	execute(t)
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := existing + "\nminitrue:\n  agent: claude\n"; string(b) != want {
		t.Errorf("telescreen.yaml = %q, want %q", b, want)
	}
}

// TestInstallKeepsFullConfig pins that a file with both keys survives a
// re-install, --force included, byte for byte.
func TestInstallKeepsFullConfig(t *testing.T) {
	home, _ := isolate(t)
	existing := "minitrue:\n  agent: codex\nspeakwrite:\n  agent: codex\n"
	cfgPath := filepath.Join(home, ".config", "telescreen.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	execute(t)
	execute(t, "--force")
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != existing {
		t.Errorf("telescreen.yaml = %q, want it untouched", b)
	}
}

// TestInstallKeepsEditedSkill pins that a re-install never clobbers a
// user-edited skill without --force.
func TestInstallKeepsEditedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	systemctl = func(...string) error { return nil }

	var out strings.Builder
	if err := run(&out, []string{"minitrue"}, false, false); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(home, ".claude", "skills", "minitrue", "SKILL.md")
	if err := os.WriteFile(skill, []byte("my tweaked skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(&out, []string{"minitrue"}, false, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "my tweaked skill\n" {
		t.Error("re-install clobbered the edited skill")
	}
	if err := run(&out, []string{"minitrue"}, false, true); err != nil {
		t.Fatal(err)
	}
	if b, _ = os.ReadFile(skill); string(b) == "my tweaked skill\n" {
		t.Error("--force kept the edited skill, want the embedded one restored")
	}
}
