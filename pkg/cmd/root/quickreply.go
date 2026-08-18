// The quick-reply popup. r opens a small textarea over the list or the
// reader for dictating a stance without leaving the TUI. ctrl+s submits
// the same intent file the editor path writes; ctrl+e escalates into the
// editor carrying the typed text; esc discards.

package root

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/maxgio92/telescreen/internal/recdep"
)

var quickStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("241"))

const quickHelpLine = "enter newline  ctrl+s submit  ctrl+e editor  esc cancel"

// openQuick handles the r key: on a selected record in an open state it
// opens the quick-reply popup. Files and the memory hole are closed to
// dictation, so the popup is too.
func (m model) openQuick() (tea.Model, tea.Cmd) {
	if m.view >= len(recdep.States) || recdep.States[m.view] == "files" {
		return m, nil
	}
	e, ok := m.selected()
	if !ok {
		return m, nil
	}
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Placeholder = "stance for " + e.Name
	ta.SetWidth(max(10, m.width-4))
	// Five visible lines, shrunk on short terminals; the textarea
	// scrolls internally past its window. The budget subtracts the
	// chrome around the popup: header 2, blank 1, status 1, help 1,
	// and the popup's own title, border, and chord lines (4). The
	// detail pane hides while the popup is open, so it costs nothing.
	ta.SetHeight(quickInputHeight(m.height))
	cmd := ta.Focus()
	m.quickInput = ta
	m.quick = true
	m.quickName = e.Name
	return m, cmd
}

// handleQuickKey drives the popup: ctrl+s submits, ctrl+e escalates into
// the editor, esc discards. Everything else feeds the textarea, so the
// global keys are inert and enter inserts a newline.
func (m model) handleQuickKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.quick = false
		return m, nil
	case "ctrl+s":
		return m.submitQuick()
	case "ctrl+e":
		typed := strings.TrimSpace(m.quickInput.Value())
		m.quick = false
		return m.dictate(typed)
	}
	var cmd tea.Cmd
	m.quickInput, cmd = m.quickInput.Update(msg)
	return m, cmd
}

// submitQuick writes the intent the editor path would: same renderIntent,
// tmp plus rename, same status. An empty stance submits only when the
// matched rule carries guidance: composeGuidance then emits the rule text
// alone, a valid stance; with no rule guidance an empty intent would say
// nothing, so it no-ops.
func (m model) submitQuick() (tea.Model, tea.Cmd) {
	// handleKey re-resolves the pinned record before routing here, so
	// the selection always holds; the guard stays as a seam check.
	e, ok := m.selected()
	if !ok {
		m.quick = false
		return m, nil
	}
	typed := strings.TrimSpace(m.quickInput.Value())
	if typed == "" {
		if _, rule := actionFor(e); rule == "" {
			m.status = "empty stance: type something or esc"
			return m, nil
		}
	}
	// A pending or last-dictated stance survives under the typed text,
	// like the editor path; a quick reply refines, never erases.
	stance := composeGuidance(guidanceFor(m.root, e), typed)
	entryPath := filepath.Join(m.root, recdep.States[m.view], e.Name)
	intent := filepath.Join(m.root, recdep.IntentsDir, e.Name+".intent")
	if err := os.WriteFile(intent+".tmp", []byte(renderIntent(entryPath, e, stance)), 0o600); err != nil {
		m.status = err.Error()
		return m, nil
	}
	if err := os.Rename(intent+".tmp", intent); err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.quick = false
	m.reload()
	m.status = "dictated " + e.Name
	return m, nil
}

// quickInputHeight sizes the textarea so the popup block and the chrome
// around it fit the terminal: 9 rows of chrome (header 2, blank 1, status
// 1, help 1, popup title, border, and chord 4), never above five lines,
// never below one.
func quickInputHeight(height int) int {
	return max(1, min(5, height-9))
}

// quickLines is the popup's height budget: title, bordered textarea, and
// the popup help line. Zero while closed, so the list and reader budgets
// subtract it unconditionally.
func (m model) quickLines() int {
	if !m.quick {
		return 0
	}
	return m.quickInput.Height() + 4
}

// viewQuick renders the popup block: the record name, the bordered
// textarea, and the chord line.
func (m model) viewQuick() string {
	var b strings.Builder
	b.WriteString(tabActive.Render(fitWidth("reply to "+m.quickName, m.width)) + "\n")
	b.WriteString(quickStyle.Render(m.quickInput.View()) + "\n")
	b.WriteString(helpStyle.Render(fitWidth(quickHelpLine, m.width)) + "\n")
	return b.String()
}
