// The / search filter. / opens a one-line input on the status row; enter
// applies the query as the active filter across every drawer, esc cancels
// the input. With a filter active esc clears it and / reopens the input
// pre-filled. m.lists stays the full truth; every view derives its
// visible slice through matches.

package root

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/maxgio92/telescreen/internal/recdep"
)

// matches reports whether e matches query as a case-insensitive
// substring of its who, summary, source, any metadata value, or body.
// An empty query matches everything.
func matches(e recdep.Entry, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	for _, s := range []string{e.Who, e.Summary, e.Source, e.Body} {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	for _, p := range e.Meta {
		if strings.Contains(strings.ToLower(p.Value), q) {
			return true
		}
	}
	return false
}

// visible returns the view's list filtered by the active query. The
// cursor and viewport index this slice, never m.lists directly.
func (m model) visible(view int) []recdep.Entry {
	list := m.lists[view]
	if m.search == "" {
		return list
	}
	var out []recdep.Entry
	for _, e := range list {
		if matches(e, m.search) {
			out = append(out, e)
		}
	}
	return out
}

// clampCursors keeps every view's cursor inside its visible slice: a
// reload or a new filter can shrink a list under the cursor.
func (m *model) clampCursors() {
	for i := range m.lists {
		if n := len(m.visible(i)); m.cursor[i] >= n {
			m.cursor[i] = max(0, n-1)
		}
	}
}

// openSearch handles the / key in a list view: it opens the input on the
// status row, pre-filled with the active query for editing.
func (m model) openSearch() (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Width = searchWidth(m.width)
	ti.SetValue(m.search)
	ti.CursorEnd()
	cmd := ti.Focus()
	m.searchInput = ti
	m.searching = true
	return m, cmd
}

// handleSearchKey drives the input: enter applies the query (empty
// clears), esc closes without touching the active filter. Everything
// else feeds the input, so the global keys are inert.
func (m model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.searching = false
		return m, nil
	case "enter":
		m.searching = false
		m.search = strings.TrimSpace(m.searchInput.Value())
		m.clampCursors()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// searchWidth sizes the input's scroll window under the prompt so the
// status row never exceeds the terminal.
func searchWidth(width int) int {
	return max(1, width-2)
}

// statusRow renders the status line: the open search input, a status
// message, or the active filter with its clear hint. The input view
// carries styling escapes, so its width is bounded by searchWidth
// instead of fitWidth.
func (m model) statusRow() string {
	switch {
	case m.searching:
		return m.searchInput.View()
	case m.status != "":
		return fitWidth(m.status, m.width)
	case m.search != "":
		return fitWidth("/"+m.search+"  esc clears", m.width)
	}
	return ""
}
