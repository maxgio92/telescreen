package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// seedModel creates a state root with one entry in state and returns a
// loaded model whose active view is that state.
func seedModel(t *testing.T, state, name string) (model, string) {
	t.Helper()
	root := t.TempDir()
	for _, s := range watchedDirs {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	body := "[slack] wes: go for it\nhttps://example.com\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, state, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newModel(root, nil)
	m.view = slices.Index(states, state)
	return m, root
}

// inState returns the states that contain name.
func inState(t *testing.T, root, name string) []string {
	t.Helper()
	var got []string
	for _, s := range states {
		if _, err := os.Stat(filepath.Join(root, s, name)); err == nil {
			got = append(got, s)
		}
	}
	return got
}

func TestRowAtY(t *testing.T) {
	tests := []struct {
		name                             string
		y, headerLines, start, len, rows int
		want                             int
	}{
		{"first row", 2, 2, 0, 5, 10, 0},
		{"last row", 6, 2, 0, 5, 10, 4},
		{"above the list", 1, 2, 0, 5, 10, -1},
		{"below the last row", 7, 2, 0, 5, 10, -1},
		{"scrolled offset", 3, 2, 4, 20, 10, 5},
		{"below the viewport", 12, 2, 4, 20, 10, -1},
		{"empty list", 2, 2, 0, 0, 10, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowAtY(tt.y, tt.headerLines, tt.start, tt.len, tt.rows); got != tt.want {
				t.Errorf("rowAtY(%d, %d, %d, %d, %d) = %d, want %d",
					tt.y, tt.headerLines, tt.start, tt.len, tt.rows, got, tt.want)
			}
		})
	}
}

func TestViewAtX(t *testing.T) {
	// Widths 6, 6, 7: tabs span [0,6), [8,14), [16,23).
	labels := []string{"1 tube", "2 desk", "3 upsub"}
	tests := []struct {
		name string
		x    int
		want int
	}{
		{"first tab start", 0, 0},
		{"first tab end", 5, 0},
		{"separator", 6, -1},
		{"separator second column", 7, -1},
		{"second tab", 8, 1},
		{"third tab", 16, 2},
		{"past the last tab", 23, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := viewAtX(tt.x, labels); got != tt.want {
				t.Errorf("viewAtX(%d) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}

func TestViewAtXStyledLabels(t *testing.T) {
	styled := []string{
		tabActive.Render("1 tube"),
		tabInactive.Render("2 desk"),
	}
	if w := lipgloss.Width(styled[0]); w != 6 {
		t.Fatalf("styled label width = %d, want 6", w)
	}
	if got := viewAtX(8, styled); got != 1 {
		t.Errorf("viewAtX(8, styled) = %d, want 1", got)
	}
}

func TestViewRowsFitWidth(t *testing.T) {
	long := strings.Repeat("x", 100)
	tests := []struct {
		name  string
		stale string
	}{
		{"fresh long summary", ""},
		{"stale long summary", "already-reviewed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{width: 80, height: 24}
			m.lists[0] = []entry{
				{name: "a.md", source: "slack", summary: long, stale: tt.stale},
				{name: "b.md", source: "github", summary: long, stale: tt.stale},
			}
			// The cursor stays on index 0, so both the selected and the
			// plain row style are covered. List rows start after the header.
			lines := strings.Split(m.View(), "\n")
			for i := headerLines; i < headerLines+len(m.lists[0]); i++ {
				if w := lipgloss.Width(lines[i]); w > m.width {
					t.Errorf("row %d width = %d, want <= %d: %q", i-headerLines, w, m.width, lines[i])
				}
			}
		})
	}
}

func TestViewRendersStaleTagDimmed(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []entry{
		{name: "a.md", source: "github", summary: "please review", stale: ""},
		{name: "b.md", source: "github", summary: "old ask", stale: "merged"},
	}
	view := m.View()
	if !strings.Contains(view, "[stale: merged]") {
		t.Errorf("view lacks the stale tag:\n%s", view)
	}
	staleRow := strings.Split(view, "\n")[headerLines+1]
	if staleRow != tabInactive.Render(stripANSI(staleRow)) {
		t.Errorf("stale row is not rendered with the muted style: %q", staleRow)
	}
}

// TestViewDropsTagOnNarrowWidth pins that a tag wider than the row budget
// disappears instead of wrapping the row.
func TestViewDropsTagOnNarrowWidth(t *testing.T) {
	m := model{width: 30, height: 24}
	m.lists[0] = []entry{{name: "a.md", source: "github", summary: "x", stale: "already-reviewed"}}
	lines := strings.Split(m.View(), "\n")
	row := lines[headerLines]
	if strings.Contains(row, "[stale:") {
		t.Errorf("narrow row still carries the tag: %q", row)
	}
	if w := lipgloss.Width(row); w > m.width {
		t.Errorf("row width = %d, want <= %d", w, m.width)
	}
}

func TestViewRendersSpeakwriteTags(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []entry{
		{name: "a.md", source: "github", summary: "drafted", mark: "draft"},
		{name: "b.md", source: "github", summary: "dictated", mark: "dictated"},
		{name: "c.md", source: "github", summary: "posted", mark: "published"},
		{name: "d.md", source: "github", summary: "dropped", mark: "discarded"},
	}
	lines := strings.Split(m.View(), "\n")
	if !strings.Contains(lines[headerLines], "[draft]") {
		t.Errorf("draft row lacks the tag: %q", lines[headerLines])
	}
	if !strings.Contains(lines[headerLines+1], "[dictated]") {
		t.Errorf("dictated row lacks the tag: %q", lines[headerLines+1])
	}
	for i := headerLines + 2; i < headerLines+4; i++ {
		if row := stripANSI(lines[i]); strings.Contains(row, "[") {
			t.Errorf("published/discarded row carries a tag: %q", row)
		}
	}
}

// TestViewDropsDraftTagOnNarrowWidth pins that the speakwrite tag follows
// the same drop-whole discipline as the stale tag.
func TestViewDropsDraftTagOnNarrowWidth(t *testing.T) {
	m := model{width: 20, height: 24}
	m.lists[0] = []entry{{name: "a.md", source: "github", summary: "x", mark: "draft"}}
	lines := strings.Split(m.View(), "\n")
	row := lines[headerLines]
	if strings.Contains(row, "[draft]") {
		t.Errorf("narrow row still carries the tag: %q", row)
	}
	if w := lipgloss.Width(row); w > m.width {
		t.Errorf("row width = %d, want <= %d", w, m.width)
	}
}

func TestViewRendersStaleAndDraftTags(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []entry{{name: "a.md", source: "github", summary: "old draft", stale: "merged", mark: "draft"}}
	row := strings.Split(m.View(), "\n")[headerLines]
	if !strings.Contains(row, "[stale: merged]  [draft]") {
		t.Errorf("row lacks stale-then-draft tags: %q", row)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(model)
}

func TestIncinerateArmsThenDeletes(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "files", name)

	m = press(t, m, key("x"))
	if got := inState(t, root, name); !slices.Equal(got, []string{"files"}) {
		t.Fatalf("after first x: entry in %v, want [files]", got)
	}
	if want := "6079 Smith W.! Yes, you! Press x again to incinerate."; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}

	m = press(t, m, key("x"))
	if got := inState(t, root, name); got != nil {
		t.Errorf("after second x: entry in %v, want gone", got)
	}
	if want := "incinerated " + name; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
}

func TestIncinerateDisarmsOnOtherKey(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "files", name)

	m = press(t, m, key("x"))
	m = press(t, m, key("j"))
	m = press(t, m, key("x"))
	if got := inState(t, root, name); !slices.Equal(got, []string{"files"}) {
		t.Fatalf("x j x deleted the entry: in %v, want [files]", got)
	}
	if want := "6079 Smith W.! Yes, you! Press x again to incinerate."; m.status != want {
		t.Errorf("status = %q, want %q (re-armed)", m.status, want)
	}

	m = press(t, m, key("x"))
	if got := inState(t, root, name); got != nil {
		t.Errorf("x after re-arm: entry in %v, want gone", got)
	}
}

func TestIncinerateOutsideFilesDoesNothing(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	for _, state := range []string{"tube", "desk", "upsub"} {
		t.Run(state, func(t *testing.T) {
			m, root := seedModel(t, state, name)
			m = press(t, m, key("x"))
			m = press(t, m, key("x"))
			if got := inState(t, root, name); !slices.Equal(got, []string{state}) {
				t.Errorf("x x in %s: entry in %v, want [%s]", state, got, state)
			}
			if m.status != "" || m.armed != "" {
				t.Errorf("x in %s set status %q, armed %q", state, m.status, m.armed)
			}
		})
	}

	// The memory hole view is where losing the states-bound guard would
	// panic instead of no-oping: seed files, press x from view 5.
	t.Run("memoryhole", func(t *testing.T) {
		m, root := seedModel(t, "files", name)
		m.view = memoryHoleView
		m = press(t, m, key("x"))
		m = press(t, m, key("x"))
		if got := inState(t, root, name); !slices.Equal(got, []string{"files"}) {
			t.Errorf("x x in memoryhole: entry in %v, want [files]", got)
		}
		if m.status != "" || m.armed != "" {
			t.Errorf("x in memoryhole set status %q, armed %q", m.status, m.armed)
		}
	})
}

func TestMemoryHoleRendersEpitaphOnly(t *testing.T) {
	m := model{width: 80, height: 24, view: memoryHoleView}
	m.lists[0] = []entry{{name: "a.md", source: "slack", summary: "still visible?"}}
	view := m.View()
	if !strings.Contains(view, epitaph) {
		t.Errorf("memory hole view lacks the epitaph:\n%s", view)
	}
	if strings.Contains(view, "still visible?") {
		t.Errorf("memory hole view renders a list entry:\n%s", view)
	}
	if !strings.Contains(view, "5 memoryhole") {
		t.Errorf("tab row lacks the countless memoryhole label:\n%s", view)
	}
}

func TestTabCyclesAcrossFiveViews(t *testing.T) {
	m := model{view: memoryHoleView}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.view != 0 {
		t.Errorf("tab from memoryhole: view = %d, want 0", m.view)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.view != memoryHoleView {
		t.Errorf("shift+tab from tube: view = %d, want %d", m.view, memoryHoleView)
	}
	for range views {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	}
	if m.view != memoryHoleView {
		t.Errorf("five tabs did not return to memoryhole: view = %d", m.view)
	}
}

func TestOnceCountsListsRealStatesOnly(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	_, root := seedModel(t, "tube", name)
	// A pending intent never shows up in the counts.
	if err := os.WriteFile(filepath.Join(root, "intents", name+".intent"), []byte("entry x\naction respond\n\nguidance:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := onceCounts(root)
	if err != nil {
		t.Fatal(err)
	}
	want := "tube 1\ndesk 0\nupsub 0\nfiles 0\n"
	if out != want {
		t.Errorf("onceCounts = %q, want %q", out, want)
	}
}

func TestHandleKeyMovesOnce(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	tests := []struct {
		key  string
		from string
		want string
	}{
		{"t", "tube", "desk"},
		{"u", "tube", "upsub"},
		{"u", "desk", "upsub"},
		{"f", "tube", "files"},
		{"f", "desk", "files"},
		{"f", "upsub", "files"},
		{"b", "files", "upsub"},
		{"b", "upsub", "desk"},
		{"b", "desk", "tube"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"-from-"+tt.from, func(t *testing.T) {
			m, root := seedModel(t, tt.from, name)
			_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			got := inState(t, root, name)
			if !slices.Equal(got, []string{tt.want}) {
				t.Errorf("after %q from %s: entry in %v, want [%s]", tt.key, tt.from, got, tt.want)
			}
		})
	}
}
