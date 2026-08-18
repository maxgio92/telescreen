package root

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/maxgio92/telescreen/internal/config"
)

var (
	ctrlS = tea.KeyMsg{Type: tea.KeyCtrlS}
	ctrlE = tea.KeyMsg{Type: tea.KeyCtrlE}
)

func TestQuickReplyOpensOnSelectedRecord(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, _ := seedModel(t, "tube", name)
	m.width, m.height = 80, 24
	m = press(t, m, key("r"))
	if !m.quick {
		t.Fatal("r did not open the popup")
	}
	view := m.View()
	if !strings.Contains(view, "reply to "+name) {
		t.Errorf("popup lacks the record name:\n%s", view)
	}
	if !strings.Contains(view, quickHelpLine) {
		t.Errorf("popup lacks the chord line:\n%s", view)
	}
}

func TestQuickReplySubmitCarriesRuleGuidanceThenTyped(t *testing.T) {
	builtin := actionRules
	t.Cleanup(func() { actionRules = builtin })
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	applyConfig(config.Config{Speakwrite: config.Speakwrite{Actions: []config.Action{
		{Source: "slack", Action: "slack-reply", Guidance: "casual register"},
	}}})
	m.width, m.height = 80, 24
	m = press(t, m, key("r"))
	m = press(t, m, key("say yes"))
	m = press(t, m, ctrlS)
	if m.quick {
		t.Error("submit left the popup open")
	}
	if want := "dictated " + name; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent"))
	if err != nil {
		t.Fatal(err)
	}
	want := "entry " + filepath.Join(root, "tube", name) + "\naction slack-reply\n\nguidance:\ncasual register\nsay yes\n"
	if string(got) != want {
		t.Errorf("intent = %q, want %q", got, want)
	}
	if !m.pending[name] {
		t.Error("submitted entry is not tracked as pending")
	}
	if !strings.Contains(m.View(), "[dictated]") {
		t.Errorf("row lacks [dictated] after the quick submit:\n%s", m.View())
	}
}

func TestQuickReplyEnterInsertsNewline(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = press(t, m, key("r"))
	m = press(t, m, key("a"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, key("b"))
	if got := m.quickInput.Value(); got != "a\nb" {
		t.Errorf("textarea value = %q, want %q", got, "a\nb")
	}
	if !m.quick {
		t.Error("enter closed the popup")
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("enter submitted: %d files in intents/", len(names))
	}
}

func TestQuickReplyEscDiscards(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = press(t, m, key("r"))
	m = press(t, m, key("never mind"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.quick {
		t.Error("esc did not close the popup")
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("esc wrote %d files in intents/", len(names))
	}
}

// TestQuickReplyEmptySubmit pins the empty-stance rule: with no rule
// guidance an empty ctrl+s no-ops; with rule guidance it submits, because
// composeGuidance makes the rule text alone a valid stance.
func TestQuickReplyEmptySubmit(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"

	t.Run("no rule guidance", func(t *testing.T) {
		m, root := seedModel(t, "tube", name)
		m = press(t, m, key("r"))
		m = press(t, m, ctrlS)
		if !m.quick {
			t.Error("empty submit closed the popup")
		}
		if m.status == "" {
			t.Error("empty submit left no status message")
		}
		names, err := os.ReadDir(filepath.Join(root, "intents"))
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 0 {
			t.Errorf("empty submit wrote %d files in intents/", len(names))
		}
	})

	t.Run("rule guidance", func(t *testing.T) {
		builtin := actionRules
		t.Cleanup(func() { actionRules = builtin })
		m, root := seedModel(t, "tube", name)
		applyConfig(config.Config{Speakwrite: config.Speakwrite{Actions: []config.Action{
			{Source: "slack", Action: "slack-reply", Guidance: "casual register"},
		}}})
		m = press(t, m, key("r"))
		m = press(t, m, ctrlS)
		if m.quick {
			t.Error("guided empty submit left the popup open")
		}
		got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent"))
		if err != nil {
			t.Fatal(err)
		}
		want := "entry " + filepath.Join(root, "tube", name) + "\naction slack-reply\n\nguidance:\ncasual register\n"
		if string(got) != want {
			t.Errorf("intent = %q, want %q", got, want)
		}
	})
}

// TestQuickReplyGlobalKeysInert pins the popup as a modal: tab, q, and x
// type into the textarea or do nothing, never switch views, quit, or arm
// a delete.
func TestQuickReplyGlobalKeysInert(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = press(t, m, key("r"))
	for _, msg := range []tea.Msg{tea.KeyMsg{Type: tea.KeyTab}, key("q"), key("x")} {
		nm, cmd := m.Update(msg)
		m = nm.(model)
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("%v quit while the popup was open", msg)
			}
		}
	}
	if !m.quick || m.view != 0 || m.armed != "" {
		t.Errorf("global keys changed state: quick=%v view=%d armed=%q", m.quick, m.view, m.armed)
	}
	if got := inState(t, root, name); !slices.Equal(got, []string{"tube"}) {
		t.Errorf("global keys moved the entry: in %v, want [tube]", got)
	}
}

// TestQuickReplyHeightInvariant keeps the popup usable inside the
// terminal: the detail pane yields and the list shrinks before the popup
// loses its chord line, down to tiny heights.
func TestQuickReplyHeightInvariant(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {80, 20}, {40, 10}, {40, 8}} {
		m := heightModel(200, size.w, size.h)
		m = press(t, m, key("r"))
		if !m.quick {
			t.Fatal("r did not open the popup")
		}
		view := m.View()
		if got := strings.Count(view, "\n") + 1; got > m.height {
			t.Errorf("%dx%d: view has %d lines, want <= %d:\n%s", size.w, size.h, got, m.height, view)
		}
		// The chord line closes the popup block; clampHeight cuts from
		// the bottom, so its presence proves the whole popup survived.
		if !strings.Contains(view, fitWidth(quickHelpLine, size.w)) {
			t.Errorf("%dx%d: popup lacks the chord line:\n%s", size.w, size.h, view)
		}
		// The tab header predates the popup and overflows narrow widths
		// on its own; the popup rows below it must fit.
		for _, l := range strings.Split(view, "\n")[1:] {
			if w := lipgloss.Width(l); w > m.width {
				t.Errorf("%dx%d: row is %d columns: %q", size.w, size.h, w, l)
			}
		}
	}
}

// TestQuickReplyFromReader opens the popup over the reader: the invariant
// holds and ctrl+s writes the same intent as the list path.
func TestQuickReplyFromReader(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, root := seedLongDraftModel(t, name, 40, 80, 30)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, key("r"))
	if !m.quick || !m.reader {
		t.Fatalf("r in the reader: quick=%v reader=%v", m.quick, m.reader)
	}
	view := m.View()
	if !strings.Contains(view, "reply to "+name) {
		t.Errorf("reader popup lacks the record name:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got > m.height {
		t.Errorf("reader with popup has %d lines, want <= %d", got, m.height)
	}
	m = press(t, m, key("looks fine"))
	m = press(t, m, ctrlS)
	got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), "\nguidance:\nlooks fine\n") {
		t.Errorf("intent lacks the typed stance: %q", got)
	}
}

// TestQuickReplyPinsRecordAcrossReload guards the submit target: a
// reload while the popup is open (minitrue writes on a timer) shifts the
// newest-first list under the cursor; ctrl+s must still write the intent
// for the record the popup opened on, never the new arrival.
func TestQuickReplyPinsRecordAcrossReload(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	newer := "20260815T090000Z-slack-kai-hold-on.md"
	m, root := seedModel(t, "tube", name)
	m.width, m.height = 80, 24
	m = press(t, m, key("r"))
	m = press(t, m, key("say yes"))
	body := "[slack] kai: hold on\nhttps://example.com\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, "tube", newer), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, fsEventMsg{})
	if !strings.Contains(m.View(), "reply to "+name) {
		t.Errorf("popup title swapped after the reload:\n%s", m.View())
	}
	m = press(t, m, ctrlS)
	if _, err := os.Stat(filepath.Join(root, "intents", newer+".intent")); err == nil {
		t.Error("submit wrote the intent for the swapped record")
	}
	got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "entry " + filepath.Join(root, "tube", name) + "\n"; !strings.HasPrefix(string(got), want) {
		t.Errorf("intent = %q, want prefix %q", got, want)
	}
}

// TestQuickReplyPinnedRecordVanishes discards the popup when the pinned
// record leaves the view: the key falls through to the list and nothing
// is written.
func TestQuickReplyPinnedRecordVanishes(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = press(t, m, key("r"))
	m = press(t, m, key("say yes"))
	if err := os.Remove(filepath.Join(root, "tube", name)); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, fsEventMsg{})
	m = press(t, m, ctrlS)
	if m.quick {
		t.Error("popup stayed open after the pinned record vanished")
	}
	names, err := os.ReadDir(filepath.Join(root, "intents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("vanish wrote %d files in intents/", len(names))
	}
}

func TestQuickReplyClosedViewsDoNothing(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	t.Run("memoryhole", func(t *testing.T) {
		m, _ := seedModel(t, "files", name)
		m.view = memoryHoleView
		m = press(t, m, key("r"))
		if m.quick {
			t.Error("r opened the popup in the memory hole")
		}
	})
	t.Run("files", func(t *testing.T) {
		m, _ := seedModel(t, "files", name)
		m = press(t, m, key("r"))
		if m.quick {
			t.Error("r opened the popup in files")
		}
	})
	t.Run("empty list", func(t *testing.T) {
		m := model{width: 80, height: 24}
		m = press(t, m, key("r"))
		if m.quick {
			t.Error("r opened the popup on an empty list")
		}
	})
}

// TestQuickReplyEscalatesToEditor pins ctrl+e: the popup closes, the
// editor command runs, and the draft intent carries any pre-fill first
// and the typed text last.
func TestQuickReplyEscalatesToEditor(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"

	t.Run("typed only", func(t *testing.T) {
		m, root := seedModel(t, "tube", name)
		m = press(t, m, key("r"))
		m = press(t, m, key("later"))
		nm, cmd := m.Update(ctrlE)
		m = nm.(model)
		if m.quick {
			t.Error("ctrl+e left the popup open")
		}
		if cmd == nil {
			t.Fatal("ctrl+e returned no editor command")
		}
		got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent.tmp"))
		if err != nil {
			t.Fatal(err)
		}
		want := "entry " + filepath.Join(root, "tube", name) + "\naction slack-reply\n\nguidance:\nlater\n"
		if string(got) != want {
			t.Errorf("draft intent = %q, want %q", got, want)
		}
	})

	t.Run("pending pre-fill first", func(t *testing.T) {
		m, root := seedModel(t, "tube", name)
		pending := "entry x\naction slack-reply\n\nguidance:\nearlier\n"
		if err := os.WriteFile(filepath.Join(root, "intents", name+".intent"), []byte(pending), 0o644); err != nil {
			t.Fatal(err)
		}
		m.reload()
		m = press(t, m, key("r"))
		m = press(t, m, key("later"))
		m = press(t, m, ctrlE)
		got, err := os.ReadFile(filepath.Join(root, "intents", name+".intent.tmp"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(got), "\nguidance:\nearlier\nlater\n") {
			t.Errorf("draft intent orders the pre-fill and typed text wrong: %q", got)
		}
	})
}
