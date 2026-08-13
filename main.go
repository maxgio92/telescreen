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
	for i, s := range states {
		label := fmt.Sprintf("%d %s (%d)", i+1, s, len(m.lists[i]))
		if i == m.view {
			tabs = append(tabs, tabActive.Render(label))
		} else {
			tabs = append(tabs, tabInactive.Render(label))
		}
	}
	b.WriteString(strings.Join(tabs, "  ") + "\n\n")

	list := m.lists[m.view]
	detailLines := 6
	listRows := max(3, m.height-detailLines-5)
	now := time.Now().UTC()
	if len(list) == 0 {
		b.WriteString(tabInactive.Render("  (empty)") + "\n")
	}
	start := 0
	if m.cursor[m.view] >= listRows {
		start = m.cursor[m.view] - listRows + 1
	}
	for i := start; i < len(list) && i < start+listRows; i++ {
		e := list[i]
		summary := e.summary
		if m.width > 14 {
			if r := []rune(summary); len(r) > m.width-14 {
				summary = string(r[:m.width-14])
			}
		}
		if i == m.cursor[m.view] {
			b.WriteString(rowSelected.Render(fmt.Sprintf("%4s  %-7s %s", age(e.ts, now), e.source, summary)))
		} else {
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
	b.WriteString(helpStyle.Render("j/k move  tab/1-4 view  o open  y yank url  r read  w waiting  a archive  u undo  q quit"))
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

	if _, err := tea.NewProgram(newModel(root, w), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
