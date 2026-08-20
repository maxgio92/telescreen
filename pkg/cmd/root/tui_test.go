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
	meta := []recdep.MetaPair{{Key: "channel", Value: "#" + long}, {Key: "repo", Value: long}}
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
				{Name: "a.md", Source: "slack", Summary: long, Stale: tt.stale, Meta: meta},
				{Name: "b.md", Source: "github", Summary: long, Stale: tt.stale, Meta: meta},
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

// statusCol slices the status column out of a stripped row: it starts
// after the age, source, and metadata prefix and runs statusWidth
// columns; the offset derives from the same constants the view uses.
func statusCol(row string) string {
	off := 14 + contextWidth + 1
	return row[off : off+statusWidth]
}

// TestViewRendersStatusColumn pins the column between metadata and
// summary: draft and dictated show their word, published and discarded
// leave it blank, and the summary sits at column 40 on every row.
func TestViewRendersStatusColumn(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []recdep.Entry{
		{Name: "a.md", Source: "github", Summary: "review one", Mark: "draft"},
		{Name: "b.md", Source: "github", Summary: "review two", Mark: "dictated"},
		{Name: "c.md", Source: "github", Summary: "review three", Mark: "published"},
		{Name: "d.md", Source: "github", Summary: "review four", Mark: "discarded"},
	}
	lines := strings.Split(m.View(), "\n")
	wants := []string{"draft   ", "dictated", "        ", "        "}
	for i, want := range wants {
		row := stripANSI(lines[headerLines+i])
		if got := statusCol(row); got != want {
			t.Errorf("row %d status column = %q, want %q", i, got, want)
		}
		if !strings.HasPrefix(row[40:], m.lists[0][i].Summary) {
			t.Errorf("row %d summary not at column 40: %q", i, row)
		}
	}
}

// TestViewRendersStaleStatusDimmed pins the stale row: the column shows
// the bare word (the reason stays in the detail pane) and the row keeps
// the muted style.
func TestViewRendersStaleStatusDimmed(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []recdep.Entry{
		{Name: "a.md", Source: "github", Summary: "please review", Stale: ""},
		{Name: "b.md", Source: "github", Summary: "old ask", Stale: "merged"},
	}
	lines := strings.Split(m.View(), "\n")
	if got := statusCol(stripANSI(lines[headerLines])); got != strings.Repeat(" ", statusWidth) {
		t.Errorf("fresh row status column = %q, want blank", got)
	}
	staleRow := lines[headerLines+1]
	if got := statusCol(stripANSI(staleRow)); got != "stale   " {
		t.Errorf("stale row status column = %q, want %q", got, "stale   ")
	}
	if strings.Contains(stripANSI(staleRow), "merged") {
		t.Errorf("stale row carries the reason: %q", staleRow)
	}
	if staleRow != tabInactive.Render(stripANSI(staleRow)) {
		t.Errorf("stale row is not rendered with the muted style: %q", staleRow)
	}
}

// TestViewStatusMarkBeatsStale pins the priority on a stale draft: the
// mark is the actionable state, so it wins the column.
func TestViewStatusMarkBeatsStale(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "old ask", Stale: "merged", Mark: "draft"}}
	row := stripANSI(strings.Split(m.View(), "\n")[headerLines])
	if got := statusCol(row); got != "draft   " {
		t.Errorf("status column = %q, want draft to win over stale", got)
	}
	// The pending-map branch carries the same priority: a fresh intent
	// on a stale row shows dictated.
	m.lists[0][0].Mark = ""
	m.pending = map[string]bool{"a.md": true}
	row = stripANSI(strings.Split(m.View(), "\n")[headerLines])
	if got := statusCol(row); got != "dictated" {
		t.Errorf("status column = %q, want dictated to win over stale", got)
	}
}

// TestViewDropsStatusOnNarrowWidth pins that below contextMinWidth the
// status column disappears with the metadata column and the summary
// recovers their budget.
func TestViewDropsStatusOnNarrowWidth(t *testing.T) {
	m := model{width: 59, height: 24}
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "review", Stale: "merged", Mark: "draft"}}
	row := stripANSI(strings.Split(m.View(), "\n")[headerLines])
	if strings.Contains(row, "draft") || strings.Contains(row, "stale") {
		t.Errorf("narrow row still carries a status word: %q", row)
	}
	if !strings.HasPrefix(row[14:], "review") {
		t.Errorf("summary did not recover the columns' budget: %q", row)
	}
	if w := lipgloss.Width(row); w > m.width {
		t.Errorf("row width = %d, want <= %d", w, m.width)
	}
}

func TestContextFor(t *testing.T) {
	tests := []struct {
		name string
		e    recdep.Entry
		want string
	}{
		{"github repo", recdep.Entry{Source: "github", Meta: []recdep.MetaPair{{Key: "org", Value: "example"}, {Key: "repo", Value: "demo"}}}, "demo"},
		{"github without repo", recdep.Entry{Source: "github", Meta: []recdep.MetaPair{{Key: "org", Value: "example"}}}, ""},
		{"slack channel", recdep.Entry{Source: "slack", Meta: []recdep.MetaPair{{Key: "channel", Value: "#general"}}}, "#general"},
		{"slack dm fallback", recdep.Entry{Source: "slack", Meta: []recdep.MetaPair{{Key: "dm", Value: "wes,julia"}}}, "wes,julia"},
		{"linear ticket", recdep.Entry{Source: "linear", Meta: []recdep.MetaPair{{Key: "ticket", Value: "DEMO-1"}}}, "DEMO-1"},
		{"linear project fallback", recdep.Entry{Source: "linear", Meta: []recdep.MetaPair{{Key: "project", Value: "queue"}}}, "queue"},
		{"other source", recdep.Entry{Source: "mail", Meta: []recdep.MetaPair{{Key: "repo", Value: "demo"}}}, ""},
		{"no metadata", recdep.Entry{Source: "github"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextFor(tt.e); got != tt.want {
				t.Errorf("contextFor = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestViewRendersContextColumn pins the column between source and
// summary: values show, a record without the keys keeps the summary
// aligned, and a long value truncates at contextWidth.
func TestViewRendersContextColumn(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []recdep.Entry{
		{Name: "a.md", Source: "github", Summary: "review", Meta: []recdep.MetaPair{{Key: "repo", Value: "demo"}}},
		{Name: "b.md", Source: "slack", Summary: "thread", Meta: []recdep.MetaPair{{Key: "channel", Value: "#general"}}},
		{Name: "c.md", Source: "slack", Summary: "direct", Meta: []recdep.MetaPair{{Key: "dm", Value: "wes"}}},
		{Name: "d.md", Source: "linear", Summary: "assigned", Meta: []recdep.MetaPair{{Key: "ticket", Value: "DEMO-1"}}},
		{Name: "e.md", Source: "github", Summary: "bare"},
		{Name: "f.md", Source: "github", Summary: "long", Meta: []recdep.MetaPair{{Key: "repo", Value: strings.Repeat("r", 30)}}},
	}
	lines := strings.Split(m.View(), "\n")
	wants := []string{"demo", "#general", "wes", "DEMO-1", "", strings.Repeat("r", 16)}
	for i, want := range wants {
		row := stripANSI(lines[headerLines+i])
		if want != "" && !strings.Contains(row, want+" ") {
			t.Errorf("row %d lacks context %q: %q", i, want, row)
		}
		// The summary starts after the fixed 40-column prefix on every
		// row, populated or empty.
		if got := m.lists[0][i].Summary; !strings.HasPrefix(row[40:], got[:4]) {
			t.Errorf("row %d summary not at column 40: %q", i, row)
		}
		if w := lipgloss.Width(lines[headerLines+i]); w > m.width {
			t.Errorf("row %d width = %d, want <= %d", i, w, m.width)
		}
	}
	if strings.Contains(stripANSI(lines[headerLines+5]), strings.Repeat("r", 17)) {
		t.Errorf("long context not truncated at %d: %q", contextWidth, lines[headerLines+5])
	}
}

// TestViewDropsContextOnNarrowWidth pins that below width 60 the column
// disappears whole instead of starving the summary.
func TestViewDropsContextOnNarrowWidth(t *testing.T) {
	m := model{width: 59, height: 24}
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "review", Meta: []recdep.MetaPair{{Key: "repo", Value: "demo"}}}}
	row := stripANSI(strings.Split(m.View(), "\n")[headerLines])
	if strings.Contains(row, "demo") {
		t.Errorf("narrow row still carries the context column: %q", row)
	}
	if len(row) < 20 || !strings.HasPrefix(row[14:], "review") {
		t.Errorf("summary did not recover the column's budget: %q", row)
	}
}

// TestViewRendersColumnHeader pins the header row above the list: each
// label sits at the offset its column uses.
func TestViewRendersColumnHeader(t *testing.T) {
	m := model{width: 80, height: 24}
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "review"}}
	row := stripANSI(strings.Split(m.View(), "\n")[headerLines-1])
	if !strings.HasPrefix(row, " age  source ") {
		t.Errorf("header = %q, want the ' age  source ' prefix", row)
	}
	if got := strings.Index(row, "metadata"); got != 14 {
		t.Errorf("metadata label at column %d, want 14: %q", got, row)
	}
	if got := strings.Index(row, "status"); got != 14+contextWidth+1 {
		t.Errorf("status label at column %d, want %d: %q", got, 14+contextWidth+1, row)
	}
	if got := strings.Index(row, "summary"); got != 14+contextWidth+1+statusWidth+1 {
		t.Errorf("summary label at column %d, want %d: %q", got, 14+contextWidth+1+statusWidth+1, row)
	}
}

// TestViewColumnHeaderDropsMetadataOnNarrowWidth keeps the header in step
// with the rows: below contextMinWidth the metadata and status labels
// disappear with their columns and summary moves up.
func TestViewColumnHeaderDropsMetadataOnNarrowWidth(t *testing.T) {
	m := model{width: 59, height: 24}
	m.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "review"}}
	row := stripANSI(strings.Split(m.View(), "\n")[headerLines-1])
	if strings.Contains(row, "metadata") || strings.Contains(row, "status") {
		t.Errorf("narrow header still names a dropped column: %q", row)
	}
	if got := strings.Index(row, "summary"); got != 14 {
		t.Errorf("summary label at column %d, want 14: %q", got, row)
	}
}

// TestViewColumnHeaderAbsentOutsideLists keeps the header off the views
// that render no list: the memory hole and the reader.
func TestViewColumnHeaderAbsentOutsideLists(t *testing.T) {
	mh := model{width: 80, height: 24, view: memoryHoleView}
	if v := stripANSI(mh.View()); strings.Contains(v, " age  source ") {
		t.Errorf("memory hole view renders the column header:\n%s", v)
	}
	m, _ := seedModel(t, "tube", "20260811T142302Z-slack-wes-go-for-it.md")
	m.width, m.height = 80, 24
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.reader {
		t.Fatal("enter did not open the reader")
	}
	if v := stripANSI(m.View()); strings.Contains(v, " age  source ") {
		t.Errorf("reader renders the column header:\n%s", v)
	}
	// The quick-reply popup hides the header directly, not only via the
	// height clip.
	qm := model{width: 80, height: 24}
	qm.lists[0] = []recdep.Entry{{Name: "a.md", Source: "github", Summary: "s"}}
	qm = press(t, qm, key("r"))
	if strings.Contains(stripANSI(qm.View()), "age  source") {
		t.Error("the popup view still renders the column header")
	}
}

// TestViewEmptyListRendersUnderHeader keeps the placeholder on the first
// row slot, under the column header.
func TestViewEmptyListRendersUnderHeader(t *testing.T) {
	m := model{width: 80, height: 20}
	lines := strings.Split(m.View(), "\n")
	if got := stripANSI(lines[headerLines]); !strings.Contains(got, "(empty)") {
		t.Errorf("line %d = %q, want the (empty) placeholder", headerLines, got)
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

// TestListJumpKeys drives g/G on a deep list: G lands the cursor on the
// last record with its row on screen, g returns to the first.
func TestListJumpKeys(t *testing.T) {
	m := heightModel(200, 80, 20)
	m = press(t, m, key("G"))
	if m.cursor[0] != 199 {
		t.Fatalf("cursor after G = %d, want 199", m.cursor[0])
	}
	if view := m.View(); !strings.Contains(view, "summary-199") {
		t.Errorf("view after G lacks the last record:\n%s", view)
	}
	m = press(t, m, key("g"))
	if m.cursor[0] != 0 {
		t.Fatalf("cursor after g = %d, want 0", m.cursor[0])
	}
	if view := m.View(); !strings.Contains(view, "summary-000") {
		t.Errorf("view after g lacks the first record:\n%s", view)
	}
}

// TestListJumpKeysPerView keeps the jump scoped to the open view: G in
// tube leaves desk's cursor alone.
func TestListJumpKeysPerView(t *testing.T) {
	m := heightModel(200, 80, 20)
	m.lists[1] = m.lists[0]
	m.cursor[1] = 3
	m = press(t, m, key("G"))
	if m.cursor[0] != 199 || m.cursor[1] != 3 {
		t.Errorf("cursors = %d/%d, want 199/3", m.cursor[0], m.cursor[1])
	}
}

// TestListJumpKeysEmptyList keeps g/G inert on an empty drawer.
func TestListJumpKeysEmptyList(t *testing.T) {
	m := heightModel(0, 80, 20)
	for _, k := range []string{"g", "G"} {
		m = press(t, m, key(k))
		if m.cursor[0] != 0 {
			t.Errorf("cursor after %q on empty list = %d, want 0", k, m.cursor[0])
		}
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
	got := capDetail(content, 80, 5, 3)
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

// TestCapDetailProtectsMetadataTail keeps the metadata lines with the
// path, url, and seen tail while the preview absorbs the cut.
func TestCapDetailProtectsMetadataTail(t *testing.T) {
	e := recdep.ParseEntry("20260815T100000Z-github-x.md",
		"[github] alice: please review\n"+
			"https://github.com/o/r/pull/7\n"+
			"seen 2026-08-15T10:00:00Z\n"+
			"org o\n"+
			"repo r\n"+
			"\n"+
			"preview 1\npreview 2\npreview 3\npreview 4\n")
	got := capDetail(e.Detail("/tmp/x"), 80, 7, e.DetailTail())
	if h := strings.Count(got, "\n") + 1; h > 7 {
		t.Errorf("capDetail height = %d, want <= 7", h)
	}
	for _, want := range []string{"org o", "repo r", "path /tmp/x", "seen 2026-08-15T10:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("capDetail dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "preview 1") {
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
	for _, want := range []string{"j/k g/G move", "enter read", "t take", "u up", "f file", "s dictate", "p approve", "D discard", "x delete"} {
		if !strings.Contains(helpLine, want) {
			t.Errorf("helpLine lacks %q: %q", want, helpLine)
		}
	}
}

// seedLongDraftModel builds a drafted desk entry whose draft is n numbered
// lines, on a loaded model sized width x height with the reader closed.
func seedLongDraftModel(t *testing.T, name string, n, width, height int) (model, string) {
	t.Helper()
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	var draft strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&draft, "draft-line-%03d\n", i)
	}
	body := "[github] alice: please review\nhttps://github.com/o/r/pull/1\nseen now\n\n--- draft 2026-08-14T09:05:00Z\n" + draft.String()
	if err := os.WriteFile(filepath.Join(root, "desk", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m.width, m.height = width, height
	return m, root
}

// TestReaderShowsTopOfLongDraft opens the reader on a 40-line draft at
// height 30: the top window shows, the budget holds, no row overflows.
func TestReaderShowsTopOfLongDraft(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, _ := seedLongDraftModel(t, name, 40, 80, 30)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.reader {
		t.Fatal("enter did not open the reader")
	}
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Errorf("reader has %d lines, want <= %d", len(lines), m.height)
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w > m.width {
			t.Errorf("reader row is %d columns at width %d: %q", w, m.width, l)
		}
	}
	if !strings.Contains(view, name) {
		t.Errorf("reader lacks the record name on top:\n%s", view)
	}
	if !strings.Contains(view, "draft-line-001") {
		t.Errorf("reader window lacks the draft top:\n%s", view)
	}
	if strings.Contains(view, "draft-line-040") {
		t.Errorf("reader shows the draft bottom without scrolling:\n%s", view)
	}
}

// TestReaderScrollsToBottom pins G and the line keys: the last draft line
// comes on screen and the height budget still holds.
func TestReaderScrollsToBottom(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, _ := seedLongDraftModel(t, name, 40, 80, 30)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, key("G"))
	view := m.View()
	if !strings.Contains(view, "draft-line-040") {
		t.Errorf("G did not reach the draft bottom:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got > m.height {
		t.Errorf("reader has %d lines, want <= %d", got, m.height)
	}
	m = press(t, m, key("g"))
	if !strings.Contains(m.View(), "draft-line-001") {
		t.Error("g did not return to the top")
	}
	for range 50 {
		m = press(t, m, key("j"))
	}
	if !strings.Contains(m.View(), "draft-line-040") {
		t.Error("j past the end did not pin the bottom")
	}
	m = press(t, m, key("g"))
	m = press(t, m, key(" "))
	if strings.Contains(m.View(), "draft-line-001") {
		t.Error("space did not page down")
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.View(), "draft-line-002") {
		t.Error("esc did not close the reader")
	}
}

// TestReaderClosesBackToList pins q: the list view returns unchanged.
func TestReaderClosesBackToList(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, _ := seedLongDraftModel(t, name, 40, 80, 30)
	before := m.View()
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, key("j"))
	m = press(t, m, key("q"))
	if m.reader {
		t.Fatal("q did not close the reader")
	}
	if got := m.View(); got != before {
		t.Errorf("list after the reader differs:\n%s\nwant:\n%s", got, before)
	}
}

// TestReaderClosesWhenRecordVanishes pins the reader on a record that a
// reload removed: keys fall back to the list, so q quits instead of
// silently clearing a reader the screen no longer shows.
func TestReaderClosesWhenRecordVanishes(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "desk", name)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.reader {
		t.Fatal("enter did not open the reader")
	}
	if err := os.Remove(filepath.Join(root, "desk", name)); err != nil {
		t.Fatal(err)
	}
	m.reload()
	nm, cmd := m.Update(key("q"))
	m = nm.(model)
	if m.reader {
		t.Error("q on a vanished record left the reader flag set")
	}
	if cmd == nil {
		t.Fatal("q on a vanished record did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q returned %T, want tea.QuitMsg", cmd())
	}
}

// TestReaderPublishArmsThenApproves pins p p inside the reader: the arm
// status renders in the reader and the approval lands like the list path.
func TestReaderPublishArmsThenApproves(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	approval := filepath.Join(root, "intents", name+".publish")

	m = press(t, m, key("p"))
	if want := "approve posting to https://github.com/o/r/pull/1: press p again"; m.status != want {
		t.Errorf("status after first p = %q, want %q", m.status, want)
	}
	if !strings.Contains(m.View(), "press p again") {
		t.Errorf("reader does not render the arm status:\n%s", m.View())
	}
	if _, err := os.Stat(approval); !os.IsNotExist(err) {
		t.Fatalf("first p already wrote the approval: %v", err)
	}

	m = press(t, m, key("p"))
	if !m.reader {
		t.Error("approving closed the reader")
	}
	got, err := os.ReadFile(approval)
	if err != nil {
		t.Fatal(err)
	}
	if want := "entry " + filepath.Join(root, "desk", name) + "\n"; string(got) != want {
		t.Errorf("approval = %q, want %q", got, want)
	}
}

// TestReaderInertKeysStayInReader pins that list-only keys do nothing in
// the reader: no move, no view switch, no quit command.
func TestReaderInertKeysStayInReader(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, root := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	for _, k := range []string{"t", "u", "f", "b", "x", "1", "5", "o"} {
		m = press(t, m, key(k))
	}
	if !m.reader || m.view != 1 {
		t.Errorf("inert keys changed the reader state: reader=%v view=%d", m.reader, m.view)
	}
	if got := inState(t, root, name); !slices.Equal(got, []string{"desk"}) {
		t.Errorf("inert keys moved the entry: in %v, want [desk]", got)
	}
}

// appendDraft rewrites the record in state with a draft marker appended.
func appendDraft(t *testing.T, root, state, name string) {
	t.Helper()
	path := filepath.Join(root, state, name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := append(body, []byte("\n--- draft 2026-08-14T09:05:00Z\nthe draft text\n")...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fsEvent drives one fsEventMsg through Update and returns the model and
// the command.
func fsEvent(t *testing.T, m model) (model, tea.Cmd) {
	t.Helper()
	nm, cmd := m.Update(fsEventMsg{})
	return nm.(model), cmd
}

// TestFsEventFreshDraftAlerts pins the notification: a reload where a
// record gains a draft marker sets the status line and batches the bell
// command alongside the watch resubscription.
func TestFsEventFreshDraftAlerts(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "desk", name)
	appendDraft(t, root, "desk", name)
	m, cmd := fsEvent(t, m)
	if want := "draft ready: " + name; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	if cmd == nil {
		t.Fatal("fresh draft returned no command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Errorf("fresh draft cmd = %T of %d, want a two-command tea.BatchMsg", cmd(), len(batch))
	}
}

// TestFsEventInitialLoadSilent pins the baseline: opening the screen over
// an existing draft never alerts.
func TestFsEventInitialLoadSilent(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, _ := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	if m.status != "" {
		t.Errorf("initial load over a draft set status %q, want silence", m.status)
	}
}

// TestFsEventNoTransitionSilent pins a reload that changes nothing: a
// draft already on screen stays quiet.
func TestFsEventNoTransitionSilent(t *testing.T) {
	name := "20260814T090000Z-github-review-demo-1.md"
	m, _ := seedDraftModel(t, name, "https://github.com/o/r/pull/1")
	m, _ = fsEvent(t, m)
	if m.status != "" {
		t.Errorf("no-transition reload set status %q, want silence", m.status)
	}
}

// TestKeypressReloadKeepsDraftAlert pins the window between a draft
// landing on disk and the fs event that announces it: a keypress-driven
// reload in that window (dictation, discard, delete, move, quick reply
// all call reload and overwrite the status) must not consume the
// transition, so the next fsEventMsg still alerts and rings the bell.
func TestKeypressReloadKeepsDraftAlert(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "desk", name)
	appendDraft(t, root, "desk", name)
	m.reload()
	m.status = "deleted something-else.md"
	m, cmd := fsEvent(t, m)
	if want := "draft ready: " + name; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	if cmd == nil {
		t.Fatal("draft after a keypress reload returned no command")
	}
	if batch, ok := cmd().(tea.BatchMsg); !ok || len(batch) != 2 {
		t.Errorf("cmd = %T, want a two-command tea.BatchMsg carrying the bell", cmd())
	}
}

// TestFsEventTwoFreshDraftsCount pins the plural form: two drafts landing
// in one reload alert with the count, not a name.
func TestFsEventTwoFreshDraftsCount(t *testing.T) {
	a := "20260811T142302Z-slack-wes-go-for-it.md"
	b := "20260811T150000Z-slack-julia-follow-up.md"
	m, root := seedModel(t, "desk", a)
	body := "[slack] julia: follow up\nhttps://example.com/2\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, "desk", b), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	appendDraft(t, root, "desk", a)
	appendDraft(t, root, "desk", b)
	m, cmd := fsEvent(t, m)
	if want := "drafts ready: 2"; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	if cmd == nil {
		t.Fatal("two fresh drafts returned no command")
	}
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Errorf("two fresh drafts cmd = %T, want a tea.BatchMsg carrying the bell", cmd())
	}
}

// TestReaderPlainRecordShowsBody pins the reader on a record with no
// draft: the plain body and the path line render.
func TestReaderPlainRecordShowsBody(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m.width, m.height = 80, 24
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	if !strings.Contains(view, "[slack] wes: go for it") {
		t.Errorf("reader lacks the record body:\n%s", view)
	}
	// The path wraps at the terminal width; assert on its head.
	if !strings.Contains(view, "path "+root) {
		t.Errorf("reader lacks the path line:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got > m.height {
		t.Errorf("reader has %d lines, want <= %d", got, m.height)
	}
}
