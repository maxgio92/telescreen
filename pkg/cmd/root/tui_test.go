package root

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// seedModel creates a state root with one entry in state and returns a
// loaded model whose active view is that state.
func seedModel(t *testing.T, state, name string) (model, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
	m.view = slices.Index(recdep.States, state)
	return m, root
}

// inState returns the states that contain name.
func inState(t *testing.T, root, name string) []string {
	t.Helper()
	var got []string
	for _, s := range recdep.States {
		if _, err := os.Stat(filepath.Join(root, s, name)); err == nil {
			got = append(got, s)
		}
	}
	return got
}

// TestNewModelMalformedConfigFallsBack pins the startup wiring: a broken
// telescreen.yaml surfaces in the status line and the built-in action
// table stands. XDG_CONFIG_HOME isolation works on Linux only;
// os.UserConfigDir ignores it on darwin, where these tests would read
// the real config.
func TestNewModelMalformedConfigFallsBack(t *testing.T) {
	confdir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", confdir)
	if err := os.WriteFile(filepath.Join(confdir, "telescreen.yaml"), []byte("speakwrite:\n  actions: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	builtin := actionRules
	t.Cleanup(func() { actionRules = builtin })
	m := newModel(t.TempDir(), nil)
	if m.status == "" {
		t.Error("malformed config left no trace in the status line")
	}
	if got, _ := actionFor(recdep.Entry{Source: "slack", Who: "wes"}); got != "slack-reply" {
		t.Errorf("actionFor(slack) = %q, want the built-in slack-reply", got)
	}
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
			m.lists[0] = []recdep.Entry{
				{Name: "a.md", Source: "slack", Summary: long, Stale: tt.stale},
				{Name: "b.md", Source: "github", Summary: long, Stale: tt.stale},
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
	m.lists[0] = []recdep.Entry{
		{Name: "a.md", Source: "github", Summary: "please review", Stale: ""},
		{Name: "b.md", Source: "github", Summary: "old ask", Stale: "merged"},
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
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "x", Stale: "already-reviewed"}}
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
	m.lists[0] = []recdep.Entry{
		{Name: "a.md", Source: "github", Summary: "drafted", Mark: "draft"},
		{Name: "b.md", Source: "github", Summary: "dictated", Mark: "dictated"},
		{Name: "c.md", Source: "github", Summary: "posted", Mark: "published"},
		{Name: "d.md", Source: "github", Summary: "dropped", Mark: "discarded"},
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
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "x", Mark: "draft"}}
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
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "old draft", Stale: "merged", Mark: "draft"}}
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
	if want := "press x again to delete " + name + " permanently"; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}

	m = press(t, m, key("x"))
	if got := inState(t, root, name); got != nil {
		t.Errorf("after second x: entry in %v, want gone", got)
	}
	if want := "deleted " + name; m.status != want {
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
	if want := "press x again to delete " + name + " permanently"; m.status != want {
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
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "slack", Summary: "still visible?"}}
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

// heightModel builds a model with n entries in the tube view; every entry
// carries a multi-line preview so the detail pane would overflow uncapped.
func heightModel(n, width, height int) model {
	m := model{width: width, height: height}
	preview := strings.Repeat("preview line that runs on and on to test the detail budget\n", 8)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("e-%03d.md", i)
		body := fmt.Sprintf("[slack] wes: summary-%03d\nhttps://example.com/%d\nseen 2026-08-15\n%s", i, i, preview)
		m.lists[0] = append(m.lists[0], recdep.ParseEntry(name, body))
	}
	return m
}

// TestViewNeverExceedsHeight pins the overflow bug: a full list plus a
// tall detail preview used to render more lines than the terminal, which
// clips from the top and hides row zero.
func TestViewNeverExceedsHeight(t *testing.T) {
	m := heightModel(200, 80, 20)
	view := m.View()
	if got := strings.Count(view, "\n") + 1; got > m.height {
		t.Errorf("view has %d lines, want <= %d:\n%s", got, m.height, view)
	}
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[headerLines], "summary-000") {
		t.Errorf("first list row with cursor 0 = %q, want summary-000", lines[headerLines])
	}
}

// TestViewShowsFirstRowAfterScrollBack scrolls deep into the list, walks
// the cursor back to zero, and expects row zero on screen.
func TestViewShowsFirstRowAfterScrollBack(t *testing.T) {
	m := heightModel(200, 80, 20)
	for range 150 {
		m = press(t, m, key("j"))
	}
	for range 150 {
		m = press(t, m, key("k"))
	}
	if m.cursor[0] != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor[0])
	}
	view := m.View()
	if !strings.Contains(view, "summary-000") {
		t.Errorf("view with cursor 0 lacks the first record:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got > m.height {
		t.Errorf("view has %d lines, want <= %d", got, m.height)
	}
}

// TestViewStatusRowIsConstant keeps the line count identical with and
// without a status message so the list never jumps.
func TestViewStatusRowIsConstant(t *testing.T) {
	m := heightModel(200, 80, 20)
	without := strings.Count(m.View(), "\n") + 1
	m.status = strings.Repeat("a long status message ", 3)
	with := strings.Count(m.View(), "\n") + 1
	if with != without {
		t.Errorf("line count with status = %d, without = %d, want equal", with, without)
	}
	if with > m.height {
		t.Errorf("view has %d lines, want <= %d", with, m.height)
	}
}

// TestViewFitsStateMatrix sweeps the states that once broke the budget:
// tiny terminal, empty list, memory hole, status on and off.
func TestViewFitsStateMatrix(t *testing.T) {
	tests := []struct {
		name string
		m    model
	}{
		{"tiny terminal full list", heightModel(200, 40, 10)},
		{"tiny terminal with status", func() model {
			m := heightModel(200, 40, 10)
			m.status = "something happened"
			return m
		}()},
		{"empty list", model{width: 80, height: 20}},
		{"memory hole", model{width: 80, height: 20, view: memoryHoleView, status: "gone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Count(tt.m.View(), "\n") + 1; got > tt.m.height {
				t.Errorf("view has %d lines, want <= %d", got, tt.m.height)
			}
		})
	}
}

// TestCapDetailCutsPreviewFirst keeps the summary and the path, url, and
// seen tail while the preview absorbs the cut.
func TestCapDetailCutsPreviewFirst(t *testing.T) {
	content := "summary line\npreview 1\npreview 2\npreview 3\npreview 4\npath /tmp/x\nurl https://example.com\nseen 2026-08-15"
	got := capDetail(content, 80, 5)
	if h := strings.Count(got, "\n") + 1; h > 5 {
		t.Errorf("capDetail height = %d, want <= 5", h)
	}
	for _, want := range []string{"summary line", "path /tmp/x", "url https://example.com", "seen 2026-08-15"} {
		if !strings.Contains(got, want) {
			t.Errorf("capDetail dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "preview 2") {
		t.Errorf("capDetail kept a preview line past the budget:\n%s", got)
	}
}

// TestViewRowsNeverWiderThanTerminal guards the width side of the height
// invariant: a row wider than the terminal wraps visually and clips the
// top even when the newline count fits.
func TestViewRowsNeverWiderThanTerminal(t *testing.T) {
	m := model{width: 80, height: 20}
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "slack", Summary: "s"}}
	m.status = strings.Repeat("e", 200)
	for _, line := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("row is %d columns at width %d: %q", w, m.width, line)
		}
	}
}

// TestHelpLineSpeaksDocumentedVerbs pins the help line to the verbs the
// README keys table documents for each key.
func TestHelpLineSpeaksDocumentedVerbs(t *testing.T) {
	for _, want := range []string{"t take", "u up", "f file", "s dictate", "p approve", "D discard", "x delete"} {
		if !strings.Contains(helpLine, want) {
			t.Errorf("helpLine lacks %q: %q", want, helpLine)
		}
	}
}
