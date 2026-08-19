package root

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/maxgio92/telescreen/internal/recdep"
)

func TestMatches(t *testing.T) {
	e := recdep.Entry{
		Source:  "slack",
		Who:     "myfriend",
		Summary: "go for it",
		Meta:    []recdep.MetaPair{{Key: "channel", Value: "#General"}},
		Body:    "[slack] myfriend: go for it\nhttps://example.com\nseen now\n\nthe preview paragraph",
	}
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"empty query matches", "", true},
		{"who", "myfriend", true},
		{"who case-insensitive", "MyFriend", true},
		{"summary", "for it", true},
		{"source", "slack", true},
		{"metadata value case-insensitive", "#general", true},
		{"body preview", "preview paragraph", true},
		{"no match", "elsewhere", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matches(e, tt.query); got != tt.want {
				t.Errorf("matches(e, %q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// searchModel builds a tube model with three parsed entries whose who,
// summary, metadata, and body differ, so each match target is separable.
func searchModel() model {
	m := model{width: 80, height: 24}
	bodies := []struct{ name, body string }{
		{"20260819T090300Z-slack-c.md", "[slack] casey: ping about lunch\nhttps://example.com/3\nseen now\nchannel #random\n\nnothing else here"},
		{"20260819T090200Z-github-b.md", "[github] blake: please review\nhttps://example.com/2\nseen now\nrepo demo\n\nthe body mentions myfriend once"},
		{"20260819T090100Z-slack-a.md", "[slack] myfriend: go for it\nhttps://example.com/1\nseen now\nchannel #general\n\njust a preview"},
	}
	for _, b := range bodies {
		m.lists[0] = append(m.lists[0], recdep.ParseEntry(b.name, b.body))
	}
	return m
}

// enterSearch opens the input and types query.
func enterSearch(t *testing.T, m model, query string) model {
	t.Helper()
	m = press(t, m, key("/"))
	if !m.searching {
		t.Fatal("/ did not open the search input")
	}
	for _, r := range query {
		m = press(t, m, key(string(r)))
	}
	return m
}

// applySearch opens the input, types query, and applies it with enter.
func applySearch(t *testing.T, m model, query string) model {
	t.Helper()
	m = enterSearch(t, m, query)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.searching {
		t.Fatal("enter did not close the search input")
	}
	return m
}

// TestSearchInputSwallowsKeys pins the input open on /: a j types j
// instead of moving the cursor, and the input renders on the status row
// with the / prefix.
func TestSearchInputSwallowsKeys(t *testing.T) {
	m := searchModel()
	m = enterSearch(t, m, "j")
	if m.cursor[0] != 0 {
		t.Errorf("j moved the cursor to %d with the input open", m.cursor[0])
	}
	if got := m.searchInput.Value(); got != "j" {
		t.Errorf("input value = %q, want j", got)
	}
	lines := strings.Split(m.View(), "\n")
	status := stripANSI(lines[len(lines)-2])
	if !strings.HasPrefix(status, "/j") {
		t.Errorf("status row = %q, want the /j input", status)
	}
}

// TestSearchAppliesAcrossDrawers pins enter: each list shrinks to its
// matches and every tab shows matched over total.
func TestSearchAppliesAcrossDrawers(t *testing.T) {
	m := searchModel()
	m.lists[1] = []recdep.Entry{
		{Name: "d.md", Source: "github", Summary: "myfriend opened a pr"},
		{Name: "e.md", Source: "github", Summary: "unrelated"},
	}
	m = applySearch(t, m, "myfriend")
	if got := len(m.visible(0)); got != 2 {
		t.Fatalf("tube visible = %d, want 2 (who and body matches)", got)
	}
	view := stripANSI(m.View())
	for _, want := range []string{"1 tube (2/3)", "2 desk (1/2)", "3 upsub (0/0)"} {
		if !strings.Contains(view, want) {
			t.Errorf("tabs lack %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "ping about lunch") {
		t.Errorf("a non-matching row still renders:\n%s", view)
	}
	if !strings.Contains(view, "/myfriend  esc clears") {
		t.Errorf("status row lacks the filter hint:\n%s", view)
	}
}

// TestSearchMatchesMetadataValue pins the metadata target end to end:
// the channel value filters the list.
func TestSearchMatchesMetadataValue(t *testing.T) {
	m := applySearch(t, searchModel(), "#general")
	if got := len(m.visible(0)); got != 1 {
		t.Fatalf("visible = %d, want 1", got)
	}
	if m.visible(0)[0].Who != "myfriend" {
		t.Errorf("metadata match selected %q", m.visible(0)[0].Who)
	}
}

// TestSearchEscCancelsInput pins esc with the input open: the active
// filter stays what it was.
func TestSearchEscCancelsInput(t *testing.T) {
	m := applySearch(t, searchModel(), "myfriend")
	m = enterSearch(t, m, "xyz")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching {
		t.Error("esc did not close the input")
	}
	if m.search != "myfriend" {
		t.Errorf("esc changed the active filter to %q", m.search)
	}
}

// TestSearchEscClearsActiveFilter pins esc with the input closed: the
// filter clears and the full list returns.
func TestSearchEscClearsActiveFilter(t *testing.T) {
	m := applySearch(t, searchModel(), "myfriend")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.search != "" {
		t.Errorf("esc left the filter %q", m.search)
	}
	if got := len(m.visible(0)); got != 3 {
		t.Errorf("visible after clear = %d, want 3", got)
	}
	if v := stripANSI(m.View()); !strings.Contains(v, "1 tube (3)") {
		t.Errorf("tabs did not return to plain counts:\n%s", v)
	}
}

// TestSearchReopensPrefilled pins / with a filter active: the input
// carries the query for editing.
func TestSearchReopensPrefilled(t *testing.T) {
	m := applySearch(t, searchModel(), "myfriend")
	m = press(t, m, key("/"))
	if got := m.searchInput.Value(); got != "myfriend" {
		t.Errorf("reopened input = %q, want myfriend", got)
	}
}

// TestSearchEmptyEnterClears pins the empty query: enter on a cleared
// input drops the filter.
func TestSearchEmptyEnterClears(t *testing.T) {
	m := applySearch(t, searchModel(), "myfriend")
	m = press(t, m, key("/"))
	for range len("myfriend") {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.search != "" {
		t.Errorf("empty enter left the filter %q", m.search)
	}
}

// TestSearchInertOutsideLists keeps / off the memory hole, the reader,
// and the popup.
func TestSearchInertOutsideLists(t *testing.T) {
	m := searchModel()
	m.view = memoryHoleView
	m = press(t, m, key("/"))
	if m.searching {
		t.Error("/ opened the input in the memory hole")
	}
	m = searchModel()
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, key("/"))
	if m.searching {
		t.Error("/ opened the input in the reader")
	}
	m = searchModel()
	m = press(t, m, key("r"))
	m = press(t, m, key("/"))
	if m.searching {
		t.Error("/ opened the input over the popup")
	}
	if got := m.quickInput.Value(); got != "/" {
		t.Errorf("popup swallowed / into %q, want it typed", got)
	}
}

// TestSearchTriageMovesFilteredSelection pins triage on a filtered list:
// t moves the visible selection's file, not the record the raw index
// would name.
func TestSearchTriageMovesFilteredSelection(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	other := "20260812T090000Z-github-alice-review.md"
	body := "[github] alice: please review\nhttps://example.com/x\nseen now\n"
	if err := os.WriteFile(filepath.Join(root, "tube", other), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reload()
	// Newest first: other sits at index 0, the wes record at index 1.
	m = applySearch(t, m, "wes")
	if got := len(m.visible(0)); got != 1 {
		t.Fatalf("visible = %d, want 1", got)
	}
	m = press(t, m, key("t"))
	if got := inState(t, root, name); !slices.Equal(got, []string{"desk"}) {
		t.Errorf("t on the filtered selection: %s in %v, want [desk]", name, got)
	}
	if got := inState(t, root, other); !slices.Equal(got, []string{"tube"}) {
		t.Errorf("t moved the hidden record: %s in %v, want [tube]", other, got)
	}
}

// TestSearchCursorClampsWhenFilterEmpties pins the clamp: a deep cursor
// snaps into the shrunk slice and an emptied view renders (empty).
func TestSearchCursorClampsWhenFilterEmpties(t *testing.T) {
	m := searchModel()
	m.cursor[0] = 2
	m = applySearch(t, m, "casey")
	if m.cursor[0] != 0 {
		t.Errorf("cursor = %d, want 0 after the filter shrank the list", m.cursor[0])
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = applySearch(t, m, "nothing-matches-this")
	if got := len(m.visible(0)); got != 0 {
		t.Fatalf("visible = %d, want 0", got)
	}
	if _, ok := m.selected(); ok {
		t.Error("selected() returned an entry on an emptied view")
	}
	if v := stripANSI(m.View()); !strings.Contains(v, "(empty)") {
		t.Errorf("emptied view lacks the (empty) placeholder:\n%s", v)
	}
}

// TestSearchReloadKeepsFilter pins reload under a filter: a new arrival
// that matches appears, one that does not stays hidden.
func TestSearchReloadKeepsFilter(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = applySearch(t, m, "wes")
	arrival := "20260812T090000Z-slack-wes-follow-up.md"
	if err := os.WriteFile(filepath.Join(root, "tube", arrival), []byte("[slack] wes: follow up\nhttps://example.com/y\nseen now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	miss := "20260812T091000Z-github-alice-review.md"
	if err := os.WriteFile(filepath.Join(root, "tube", miss), []byte("[github] alice: please review\nhttps://example.com/z\nseen now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reload()
	if m.search != "wes" {
		t.Fatalf("reload dropped the filter: %q", m.search)
	}
	if got := len(m.visible(0)); got != 2 {
		t.Errorf("visible after reload = %d, want 2", got)
	}
	if v := stripANSI(m.View()); strings.Contains(v, "please review") {
		t.Errorf("a non-matching arrival renders:\n%s", v)
	}
}

// TestSearchQuickPinSurvivesReload pins the popup's name pin under an
// active filter: a matching arrival shifts the visible slice and ctrl+s
// still writes the pinned record's intent. The input cannot open over
// the popup (it swallows /), so the filter itself cannot change there.
func TestSearchQuickPinSurvivesReload(t *testing.T) {
	name := "20260811T142302Z-slack-wes-go-for-it.md"
	m, root := seedModel(t, "tube", name)
	m = applySearch(t, m, "wes")
	m = press(t, m, key("r"))
	if !m.quick || m.quickName != name {
		t.Fatalf("popup not pinned to %s", name)
	}
	arrival := "20260812T090000Z-slack-wes-follow-up.md"
	if err := os.WriteFile(filepath.Join(root, "tube", arrival), []byte("[slack] wes: follow up\nhttps://example.com/y\nseen now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reload()
	for _, r := range "ack" {
		m = press(t, m, key(string(r)))
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, err := os.Stat(filepath.Join(root, "intents", name+".intent")); err != nil {
		t.Errorf("ctrl+s did not write the pinned record's intent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "intents", arrival+".intent")); !os.IsNotExist(err) {
		t.Errorf("ctrl+s wrote the arrival's intent: %v", err)
	}
}

// TestSearchViewInvariants sweeps the height and width budgets with the
// input open, a long query typed, and a long active filter.
func TestSearchViewInvariants(t *testing.T) {
	long := strings.Repeat("verylongquery", 20)
	tests := []struct {
		name string
		m    func() model
	}{
		{"input open long query", func() model {
			return enterSearch(t, searchModel(), long)
		}},
		{"active long filter", func() model {
			m := searchModel()
			m.search = long
			return m
		}},
		{"input open tiny terminal", func() model {
			m := searchModel()
			m.width, m.height = 40, 10
			return enterSearch(t, m, long)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m()
			view := m.View()
			if got := strings.Count(view, "\n") + 1; got > m.height {
				t.Errorf("view has %d lines, want <= %d", got, m.height)
			}
			// The tab row overflows tiny terminals on main already; at
			// width 80 it fits and the (n/m) labels are what this change
			// widens, so only the tiny case skips it.
			lines := strings.Split(view, "\n")
			start := 0
			if m.width < 60 {
				start = 1
			}
			for _, line := range lines[start:] {
				if w := lipgloss.Width(line); w > m.width {
					t.Errorf("row is %d columns at width %d: %q", w, m.width, line)
				}
			}
		})
	}
}

// TestHelpLineNamesSearch keeps the help line in step with the README
// keys table.
func TestHelpLineNamesSearch(t *testing.T) {
	if !strings.Contains(helpLine, "/ search") {
		t.Errorf("helpLine lacks %q: %q", "/ search", helpLine)
	}
}
