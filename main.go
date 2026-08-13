// Command telescreen is a dashboard for the recdep file
// queue: four states (inbox, todo, waiting, done), one markdown file per entry.
// It only reads and renames files under the state dir; the producer is a
// separate process.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
)

type fsEventMsg struct{}

// Layout constants shared by View and the mouse hit tests: the header is
// the tab line plus a blank line, and the detail pane is fixed height.
const (
	headerLines = 2
	detailLines = 6
)

type model struct {
	root    string
	watcher *fsnotify.Watcher
	view    int
	cursor  [4]int
	lists   [4][]entry
	width   int
	height  int
	status  string
}

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)
	tabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	rowSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	sourceStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	ageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	detailStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(lipgloss.Color("241"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func newModel(root string, w *fsnotify.Watcher) model {
	m := model{root: root, watcher: w}
	m.reload()
	return m
}

func (m *model) reload() {
	for i, s := range states {
		list, err := loadState(m.root, s)
		if err != nil {
			m.status = err.Error()
			continue
		}
		m.lists[i] = list
		if m.cursor[i] >= len(list) {
			m.cursor[i] = max(0, len(list)-1)
		}
	}
}

func (m model) selected() (entry, bool) {
	list := m.lists[m.view]
	if len(list) == 0 {
		return entry{}, false
	}
	return list[m.cursor[m.view]], true
}

func watchCmd(w *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return nil
				}
				return fsEventMsg{}
			case _, ok := <-w.Errors:
				if !ok {
					return nil
				}
				return fsEventMsg{}
			}
		}
	}
}

func (m model) Init() tea.Cmd {
	return watchCmd(m.watcher)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case fsEventMsg:
		m.reload()
		return m, watchCmd(m.watcher)
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.view = (m.view + 1) % len(states)
	case "shift+tab":
		m.view = (m.view + len(states) - 1) % len(states)
	case "1", "2", "3", "4":
		m.view = int(msg.String()[0] - '1')
	case "j", "down":
		if m.cursor[m.view] < len(m.lists[m.view])-1 {
			m.cursor[m.view]++
		}
	case "k", "up":
		if m.cursor[m.view] > 0 {
			m.cursor[m.view]--
		}
	case "o", "enter":
		if e, ok := m.selected(); ok && e.url != "" {
			if err := exec.Command("xdg-open", e.url).Start(); err != nil {
				m.status = err.Error()
			}
		}
	case "y":
		if e, ok := m.selected(); ok && e.url != "" {
			if err := copyToClipboard(e.url); err != nil {
				m.status = err.Error()
			} else {
				m.status = "copied " + e.url
			}
		}
	case "r":
		m.move("inbox", "todo")
	case "w":
		m.move("todo", "waiting")
	case "a":
		m.move("todo", "archive")
		m.move("waiting", "archive")
	case "u":
		m.move("archive", "waiting")
		m.move("waiting", "todo")
		m.move("todo", "inbox")
	}
	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if m.cursor[m.view] > 0 {
			m.cursor[m.view]--
		}
	case msg.Button == tea.MouseButtonWheelDown:
		if m.cursor[m.view] < len(m.lists[m.view])-1 {
			m.cursor[m.view]++
		}
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if msg.Y == 0 {
			if v := viewAtX(msg.X, m.tabLabels()); v >= 0 {
				m.view = v
			}
			return m, nil
		}
		start, rows := m.listViewport()
		if i := rowAtY(msg.Y, headerLines, start, len(m.lists[m.view]), rows); i >= 0 {
			m.cursor[m.view] = i
		}
	}
	return m, nil
}

// tabLabels returns the header labels before styling. Styling does not
// change their width, so hit tests on these match the rendered header.
func (m model) tabLabels() []string {
	labels := make([]string, len(states))
	for i, s := range states {
		labels[i] = fmt.Sprintf("%d %s (%d)", i+1, s, len(m.lists[i]))
	}
	return labels
}

// listViewport returns the scroll offset and row count of the list area,
// matching what View renders.
func (m model) listViewport() (start, rows int) {
	rows = max(3, m.height-detailLines-5)
	if m.cursor[m.view] >= rows {
		start = m.cursor[m.view] - rows + 1
	}
	return start, rows
}

// rowAtY maps a click at line y to a list index, or -1 when the click is
// above the list, past its end, or below the visible rows.
func rowAtY(y, header, start, listLen, listRows int) int {
	if y < header || y >= header+listRows {
		return -1
	}
	i := start + y - header
	if i >= listLen {
		return -1
	}
	return i
}

// viewAtX maps a click at column x on the header to a tab index, or -1
// when x falls on a separator or past the last tab. Labels are joined with
// two spaces; lipgloss.Width ignores styling escapes.
func viewAtX(x int, labels []string) int {
	pos := 0
	for i, l := range labels {
		w := lipgloss.Width(l)
		if x >= pos && x < pos+w {
			return i
		}
		pos += w + 2
	}
	return -1
}

// move renames the selected entry when the active view matches from.
func (m *model) move(from, to string) {
	if states[m.view] != from {
		return
	}
	e, ok := m.selected()
	if !ok {
		return
	}
	if err := moveEntry(m.root, e.name, from, to); err != nil {
		m.status = err.Error()
		return
	}
	m.reload()
}

func copyToClipboard(text string) error {
	cmd := exec.Command("wl-copy")
	if _, err := exec.LookPath("wl-copy"); err != nil {
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func (m model) View() string {
	var b strings.Builder

	var tabs []string
	for i, label := range m.tabLabels() {
		if i == m.view {
			tabs = append(tabs, tabActive.Render(label))
		} else {
			tabs = append(tabs, tabInactive.Render(label))
		}
	}
	b.WriteString(strings.Join(tabs, "  ") + "\n\n")

	list := m.lists[m.view]
	now := time.Now().UTC()
	if len(list) == 0 {
		b.WriteString(tabInactive.Render("  (empty)") + "\n")
	}
	start, listRows := m.listViewport()
	for i := start; i < len(list) && i < start+listRows; i++ {
		e := list[i]
		tag := ""
		if e.stale != "" {
			tag = "  [stale: " + e.stale + "]"
		}
		// Drop the tag entirely when it alone would overflow the width, so a
		// narrow terminal never wraps a row and breaks the click mapping.
		if m.width > 14 && len([]rune(tag)) > m.width-14 {
			tag = ""
		}
		summary := e.summary
		if budget := m.width - 14 - len([]rune(tag)); m.width > 14 {
			if r := []rune(summary); len(r) > budget {
				summary = string(r[:max(0, budget)])
			}
		}
		switch {
		case i == m.cursor[m.view]:
			b.WriteString(rowSelected.Render(fmt.Sprintf("%4s  %-7s %s%s", age(e.ts, now), e.source, summary, tag)))
		case e.stale != "":
			b.WriteString(tabInactive.Render(fmt.Sprintf("%4s  %-7s %s%s", age(e.ts, now), e.source, summary, tag)))
		default:
			b.WriteString(ageStyle.Render(fmt.Sprintf("%4s", age(e.ts, now))) + "  " +
				sourceStyle.Render(fmt.Sprintf("%-7s", e.source)) + " " + summary)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if e, ok := m.selected(); ok {
		path := filepath.Join(m.root, states[m.view], e.name)
		b.WriteString(detailStyle.Width(max(20, m.width)).Render(e.detail(path)) + "\n")
	}
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString(helpStyle.Render("j/k/wheel move  click select  tab/shift+tab/1-4/click view  o open  y yank url  r read  w waiting  a archive  u undo  q quit"))
	return b.String()
}

func main() {
	once := flag.Bool("once", false, "print state counts and exit (no TUI)")
	flag.Parse()

	root, err := stateRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *once {
		for _, s := range states {
			list, err := loadState(root, s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Printf("%s %d\n", s, len(list))
		}
		return
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = w.Close() }()
	for _, s := range states {
		if err := w.Add(filepath.Join(root, s)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if _, err := tea.NewProgram(newModel(root, w), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
