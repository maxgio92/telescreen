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
		filepath.Join(home, ".config", "recdep"),
	} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s: %v", dir, err)
		}
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
	for _, dir := range []string{
		filepath.Join(home, ".config", "systemd"),
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".local", "state", "recdep"),
		filepath.Join(home, ".config", "recdep"),
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
