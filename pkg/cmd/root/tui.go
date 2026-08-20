package root

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/maxgio92/telescreen/internal/config"
	"github.com/maxgio92/telescreen/internal/recdep"
)

type fsEventMsg struct{}

// Layout constants shared by View and the mouse hit tests: the header is
// the tab line, a blank line, and the column header row; the detail pane
// is fixed height.
const (
	headerLines = 3
	detailLines = 6
)

// views are the tabs in order: the four state directories plus the
// memory hole, a virtual view with no directory that is always empty.
var views = append(slices.Clone(recdep.States), "memoryhole")

// watchedDirs are the directories under the state root the model watches
// and the tests seed: the four states plus the intents drop box.
var watchedDirs = append(slices.Clone(recdep.States), recdep.IntentsDir)

// memoryHoleView is the index of the virtual fifth view.
var memoryHoleView = len(recdep.States)

const epitaph = "The past was erased, the erasure was forgotten."

type model struct {
	root    string
	watcher *fsnotify.Watcher
	view    int
	cursor  [4]int
	lists   [4][]recdep.Entry
	width   int
	height  int
	status  string
	// armed holds the entry name the last x keypress selected for
	// incineration; any other key or mouse press clears it.
	armed string
	// pubArmed holds the entry name the last p keypress selected for
	// publication; any other key or mouse press clears it.
	pubArmed string
	// pending holds the entry names with an intent file waiting in
	// recdep/intents/; their rows show dictated in the status column
	// before the runner writes the marker.
	pending map[string]bool
	// reader is the full-record view enter opens on the selected entry;
	// readerScroll is its top line in the wrapped body. The reader
	// follows the cursor: a reload can swap the body, and the
	// name-armed publish gate keeps p p from approving a swapped
	// record.
	reader       bool
	readerScroll int
	// quick is the quick-reply popup r opens on the selected entry;
	// quickInput is its textarea. The popup layers over the list or the
	// reader and swallows every key until it closes. quickName pins the
	// record the popup opened on: a reload can shift the newest-first
	// list under the cursor, so keys re-resolve the pin by name before
	// the popup writes anything, like the editor path pins by e.Name.
	quick      bool
	quickName  string
	quickInput textarea.Model
	// search is the active filter query; every list view shows only
	// matching records while it is set. searching is the / input open on
	// the status row; it swallows every key until enter or esc.
	search      string
	searching   bool
	searchInput textinput.Model
	// drafted is the fsEventMsg-turn snapshot of record names whose last
	// mark is draft. Only the fsEventMsg case reads and advances it, so
	// keypress-driven reloads (dictation, discard, delete, move, quick
	// reply) cannot consume a fresh-draft transition before the fs event
	// that alerts on it arrives.
	drafted map[string]bool
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
	// A malformed config never kills the dashboard: the built-in action
	// map stands in and the parse error shows once in the status line.
	cfg, err := config.Load()
	if err != nil {
		m.status = err.Error()
	} else {
		applyConfig(cfg)
	}
	m.reload()
	// Record the drafts already on disk so opening the screen over
	// existing drafts never alerts.
	m.drafted = m.draftedSet()
	return m
}

func (m *model) reload() {
	for i, s := range recdep.States {
		list, err := loadState(m.root, s)
		if err != nil {
			m.status = err.Error()
			continue
		}
		m.lists[i] = list
	}
	m.clampCursors()
	m.pending = map[string]bool{}
	if names, err := os.ReadDir(filepath.Join(m.root, recdep.IntentsDir)); err == nil {
		for _, d := range names {
			if name, ok := strings.CutSuffix(d.Name(), ".intent"); ok {
				m.pending[name] = true
			}
		}
	}
}

// draftedSet returns the record names whose last mark is draft in the
// loaded lists.
func (m *model) draftedSet() map[string]bool {
	drafted := map[string]bool{}
	for _, list := range m.lists {
		for _, e := range list {
			if e.Mark == "draft" {
				drafted[e.Name] = true
			}
		}
	}
	return drafted
}

// draftAlert advances the drafted snapshot and, when new drafts appeared
// since the previous fsEventMsg turn, sets the status line and reports
// that the bell should ring. Called only from the fsEventMsg case so no
// other reload path can consume the transition or clobber the message.
func (m *model) draftAlert() bool {
	prev := m.drafted
	m.drafted = m.draftedSet()
	var fresh []string
	for name := range m.drafted {
		if !prev[name] {
			fresh = append(fresh, name)
		}
	}
	// The alert outranks whatever status the turn left behind: the
	// arrival is the newest fact and any keypress clears it anyway.
	switch {
	case len(fresh) == 1:
		m.status = "draft ready: " + fresh[0]
	case len(fresh) > 1:
		m.status = fmt.Sprintf("drafts ready: %d", len(fresh))
	}
	return len(fresh) > 0
}

// bellCmd rings the terminal once. Bubbletea has no bell API, so this is
// a one-shot command writing BEL straight to stdout: BEL moves no
// cursor, and a one-byte interleaved write is harmless with the
// standard renderer. A \a in View would re-ring on every render.
func bellCmd() tea.Msg {
	fmt.Print("\a")
	return nil
}

func (m model) selected() (recdep.Entry, bool) {
	if m.view >= len(recdep.States) {
		return recdep.Entry{}, false
	}
	list := m.visible(m.view)
	if len(list) == 0 {
		return recdep.Entry{}, false
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
		if m.quick {
			m.quickInput.SetWidth(max(10, m.width-4))
			m.quickInput.SetHeight(quickInputHeight(m.height))
		}
		if m.searching {
			m.searchInput.Width = searchWidth(m.width)
		}
	case fsEventMsg:
		m.reload()
		if m.draftAlert() {
			return m, tea.Batch(watchCmd(m.watcher), bellCmd)
		}
		return m, watchCmd(m.watcher)
	case editorDoneMsg:
		m.finishDictation(msg)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	armed := m.armed
	pubArmed := m.pubArmed
	m.armed = ""
	m.pubArmed = ""
	m.status = ""
	if m.searching {
		return m.handleSearchKey(msg)
	}
	if m.quick {
		if i := slices.IndexFunc(m.visible(m.view), func(e recdep.Entry) bool { return e.Name == m.quickName }); i >= 0 {
			m.cursor[m.view] = i
			return m.handleQuickKey(msg)
		}
		// The pinned record vanished under the popup; discard it like the
		// reader.
		m.quick = false
	}
	if m.reader {
		if _, ok := m.selected(); ok {
			return m.handleReaderKey(msg, pubArmed)
		}
		// The read record vanished under the reader (moved or deleted by a
		// reload); the view already fell back to the list, so keys do too.
		m.reader = false
		m.readerScroll = 0
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.view = (m.view + 1) % len(views)
	case "shift+tab":
		m.view = (m.view + len(views) - 1) % len(views)
	case "1", "2", "3", "4", "5":
		m.view = int(msg.String()[0] - '1')
	case "j", "down":
		if m.view < len(recdep.States) && m.cursor[m.view] < len(m.visible(m.view))-1 {
			m.cursor[m.view]++
		}
	case "k", "up":
		if m.view < len(recdep.States) && m.cursor[m.view] > 0 {
			m.cursor[m.view]--
		}
	case "g":
		if m.view < len(recdep.States) && len(m.visible(m.view)) > 0 {
			m.cursor[m.view] = 0
		}
	case "G":
		if m.view < len(recdep.States) && len(m.visible(m.view)) > 0 {
			m.cursor[m.view] = len(m.visible(m.view)) - 1
		}
	case "enter":
		if _, ok := m.selected(); ok {
			m.reader = true
			m.readerScroll = 0
		}
	case "o":
		if e, ok := m.selected(); ok && e.URL != "" {
			if err := exec.Command("xdg-open", e.URL).Start(); err != nil {
				m.status = err.Error()
			}
		}
	case "y":
		if e, ok := m.selected(); ok && e.URL != "" {
			if err := copyToClipboard(e.URL); err != nil {
				m.status = err.Error()
			} else {
				m.status = "copied " + e.URL
			}
		}
	case "t":
		m.move("tube", "desk")
	case "u":
		m.move("tube", "upsub")
		m.move("desk", "upsub")
	case "f":
		m.move("tube", "files")
		m.move("desk", "files")
		m.move("upsub", "files")
	case "b":
		m.move("files", "upsub")
		m.move("upsub", "desk")
		m.move("desk", "tube")
	case "/":
		if m.view < len(recdep.States) {
			return m.openSearch()
		}
	case "esc":
		m.search = ""
	case "s":
		return m.dictate("")
	case "r":
		return m.openQuick()
	case "p":
		m.publish(pubArmed)
	case "D":
		m.discard()
	case "x":
		m.incinerate(armed)
	}
	return m, nil
}

// incinerate is the one destructive action: in the files view, the first
// x on an entry arms it and a second consecutive x removes the file. In the
// book the memory hole rides to the incinerators; nothing returns.
func (m *model) incinerate(armed string) {
	if m.view >= len(recdep.States) || recdep.States[m.view] != "files" {
		return
	}
	e, ok := m.selected()
	if !ok {
		return
	}
	if armed != e.Name {
		m.armed = e.Name
		m.status = "press x again to delete " + e.Name + " permanently"
		return
	}
	if err := os.Remove(filepath.Join(m.root, "files", e.Name)); err != nil {
		m.status = err.Error()
		return
	}
	m.reload()
	m.status = "deleted " + e.Name
}

// handleReaderKey drives the full-record view: scroll keys, close on
// q/esc, and the read-record actions (p p approve, D discard, s dictate)
// without leaving the reader. Every other key is inert.
func (m model) handleReaderKey(msg tea.KeyMsg, pubArmed string) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		m.reader = false
		m.readerScroll = 0
		return m, nil
	case "j", "down":
		m.readerScroll++
	case "k", "up":
		m.readerScroll--
	case "pgdown", " ":
		m.readerScroll += m.readerRows()
	case "pgup":
		m.readerScroll -= m.readerRows()
	case "g":
		m.readerScroll = 0
	case "G":
		m.readerScroll = len(m.readerBody())
	case "s":
		return m.dictate("")
	case "r":
		return m.openQuick()
	case "p":
		m.publish(pubArmed)
	case "D":
		m.discard()
	}
	m.clampReaderScroll()
	return m, nil
}

// readerRows is the body row budget of the reader: the height minus the
// name, status, and help lines, and the popup block when it is open.
func (m model) readerRows() int {
	return max(1, m.height-3-m.quickLines())
}

// readerBody returns the selected record's full detail wrapped at the
// terminal width, one rendered line per element, so scroll offsets count
// wrapped lines against the height budget.
func (m model) readerBody() []string {
	e, ok := m.selected()
	if !ok {
		return nil
	}
	path := filepath.Join(m.root, recdep.States[m.view], e.Name)
	w := max(20, m.width)
	return strings.Split(lipgloss.NewStyle().Width(w).Render(e.Detail(path)), "\n")
}

func (m *model) clampReaderScroll() {
	m.readerScroll = min(m.readerScroll, len(m.readerBody())-m.readerRows())
	m.readerScroll = max(0, m.readerScroll)
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.quick || m.searching {
		return m, nil
	}
	m.status = ""
	m.armed = ""
	m.pubArmed = ""
	if m.reader {
		if _, ok := m.selected(); !ok {
			m.reader = false
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.readerScroll--
		case tea.MouseButtonWheelDown:
			m.readerScroll++
		}
		m.clampReaderScroll()
		return m, nil
	}
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if m.view < len(recdep.States) && m.cursor[m.view] > 0 {
			m.cursor[m.view]--
		}
	case msg.Button == tea.MouseButtonWheelDown:
		if m.view < len(recdep.States) && m.cursor[m.view] < len(m.visible(m.view))-1 {
			m.cursor[m.view]++
		}
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if msg.Y == 0 {
			if v := viewAtX(msg.X, m.tabLabels()); v >= 0 {
				m.view = v
			}
			return m, nil
		}
		if m.view >= len(recdep.States) {
			return m, nil
		}
		start, rows := m.listViewport()
		if i := rowAtY(msg.Y, headerLines, start, len(m.visible(m.view)), rows); i >= 0 {
			m.cursor[m.view] = i
		}
	}
	return m, nil
}

// tabLabels returns the header labels before styling. Styling does not
// change their width, so hit tests on these match the rendered header.
func (m model) tabLabels() []string {
	labels := make([]string, len(views))
	for i, s := range views {
		switch {
		case i >= len(recdep.States):
			labels[i] = fmt.Sprintf("%d %s", i+1, s)
		case m.search != "":
			// Matched over total, so cross-drawer hits show at a glance.
			labels[i] = fmt.Sprintf("%d %s (%d/%d)", i+1, s, len(m.visible(i)), len(m.lists[i]))
		default:
			labels[i] = fmt.Sprintf("%d %s (%d)", i+1, s, len(m.lists[i]))
		}
	}
	return labels
}

// listViewport returns the scroll offset and row count of the list area,
// matching what View renders.
func (m model) listViewport() (start, rows int) {
	// With the popup open the detail pane and the column header hide and
	// the popup takes the room; the list gives up every row before the
	// popup loses its chord line. Mouse hit tests never see that branch:
	// the popup swallows clicks.
	if m.quick {
		rows = max(0, m.height-5-m.quickLines())
	} else {
		rows = max(3, m.height-detailLines-6)
	}
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
	if m.view >= len(recdep.States) || recdep.States[m.view] != from {
		return
	}
	e, ok := m.selected()
	if !ok {
		return
	}
	if err := recdep.MoveEntry(m.root, e.Name, from, to); err != nil {
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

// contextWidth is the fixed width of the metadata context column.
const contextWidth = 16

// statusWidth is the fixed width of the status column, sized to
// "dictated", the longest word.
const statusWidth = 8

// contextMinWidth is the narrowest terminal that affords the metadata
// and status columns. They share one threshold: two steps would buy
// nine columns of summary in a narrow band, not worth the second rule.
// The columns and their gaps cost 26 on top of the 14-column age and
// source prefix; below width 60 the summary would keep fewer than 20
// columns, so narrower terminals drop both columns whole.
const contextMinWidth = 60

// contextFor picks the most identifying metadata value for the list
// row: repo for github, channel (or dm) for slack, ticket (or project)
// for linear. Any other source carries nothing.
func contextFor(e recdep.Entry) string {
	switch e.Source {
	case "github":
		return e.MetaValue("repo")
	case "slack":
		if v := e.MetaValue("channel"); v != "" {
			return v
		}
		return e.MetaValue("dm")
	case "linear":
		if v := e.MetaValue("ticket"); v != "" {
			return v
		}
		return e.MetaValue("project")
	}
	return ""
}

// statusFor picks the row's status word. Staleness and the mark are
// parsed independently; a stale row with a draft or dictated mark still
// wants approval or discard, so the actionable state shows. A pending
// intent counts as dictated before the runner writes the marker.
// Published and discarded rows carry no word; the stale reason stays
// in the detail pane.
func (m model) statusFor(e recdep.Entry) string {
	mark := e.Mark
	if m.pending[e.Name] {
		mark = "dictated"
	}
	switch mark {
	case "draft", "dictated":
		return mark
	}
	if e.Stale != "" {
		return "stale"
	}
	return ""
}

func (m model) View() string {
	if m.reader {
		if v, ok := m.viewReader(); ok {
			return v
		}
	}
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

	if m.view == memoryHoleView {
		b.WriteString(tabInactive.Render("  "+epitaph) + "\n")
		b.WriteString(fitWidth(m.status, m.width) + "\n")
		b.WriteString(helpStyle.Render(fitWidth(helpLine, m.width)))
		return clampHeight(b.String(), m.height)
	}

	list := m.visible(m.view)
	now := time.Now().UTC()
	// prefix is the row budget spent before the summary: age (4), gaps,
	// source (7), and the context and status columns when the terminal
	// affords them.
	prefix := 14
	showContext := m.width >= contextMinWidth
	if showContext {
		prefix += contextWidth + 1 + statusWidth + 1
	}
	// The column header names the row columns at their offsets; the popup
	// hides it with the detail pane.
	if !m.quick {
		header := fmt.Sprintf("%4s  %-7s ", "age", "source")
		if showContext {
			header += fmt.Sprintf("%-*s %-*s ", contextWidth, "metadata", statusWidth, "status")
		}
		header += "summary"
		b.WriteString(tabInactive.Render(fitWidth(header, m.width)) + "\n")
	}
	if len(list) == 0 {
		b.WriteString(tabInactive.Render("  (empty)") + "\n")
	}
	start, listRows := m.listViewport()
	for i := start; i < len(list) && i < start+listRows; i++ {
		e := list[i]
		ctx, status := "", ""
		if showContext {
			c := []rune(contextFor(e))
			if len(c) > contextWidth {
				c = c[:contextWidth]
			}
			// Pad by rune count: %-*s pads by bytes and a multibyte
			// value would shift the summary left on its row.
			ctx = string(c) + strings.Repeat(" ", contextWidth-len(c)) + " "
			s := m.statusFor(e)
			status = s + strings.Repeat(" ", statusWidth-len([]rune(s))) + " "
		}
		summary := e.Summary
		if m.width > prefix {
			if r := []rune(summary); len(r) > m.width-prefix {
				summary = string(r[:m.width-prefix])
			}
		}
		switch {
		case i == m.cursor[m.view]:
			b.WriteString(rowSelected.Render(fmt.Sprintf("%4s  %-7s %s%s%s", age(e.TS, now), e.Source, ctx, status, summary)))
		case e.Stale != "":
			b.WriteString(tabInactive.Render(fmt.Sprintf("%4s  %-7s %s%s%s", age(e.TS, now), e.Source, ctx, status, summary)))
		default:
			row := ageStyle.Render(fmt.Sprintf("%4s", age(e.TS, now))) + "  " +
				sourceStyle.Render(fmt.Sprintf("%-7s", e.Source)) + " "
			if ctx != "" {
				row += sourceStyle.Render(ctx)
			}
			b.WriteString(row + status + summary)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	// The popup replaces the detail pane: both describe the selected
	// record, and on short terminals the pane's six rows would push the
	// popup's chord line past the height budget.
	if e, ok := m.selected(); ok && !m.quick {
		path := filepath.Join(m.root, recdep.States[m.view], e.Name)
		w := max(20, m.width)
		detail := detailStyle.Width(w).Render(capDetail(e.Detail(path), w, detailLines-1, e.DetailTail()))
		if dl := strings.Split(detail, "\n"); len(dl) > detailLines {
			detail = strings.Join(dl[:detailLines], "\n")
		}
		b.WriteString(detail + "\n")
	}
	if m.quick {
		b.WriteString(m.viewQuick())
	}
	// The status row always renders, blank when empty, so the list never
	// jumps when a message comes and goes and the height budget is constant.
	// The search input and the active filter hint share its line.
	b.WriteString(m.statusRow() + "\n")
	b.WriteString(helpStyle.Render(fitWidth(helpLine, m.width)))
	return clampHeight(b.String(), m.height)
}

// fitWidth truncates s to one terminal row. clampHeight counts newline
// lines, so a row wider than the terminal would still wrap visually and
// clip the top of the screen.
func fitWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	if r := []rune(s); len(r) > width {
		return string(r[:width])
	}
	return s
}

// clampHeight drops trailing lines past height. The terminal clips from
// the top when the view overflows, hiding the first list row; cutting the
// bottom keeps the list and cursor visible instead.
func clampHeight(view string, height int) string {
	if height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) <= height {
		return view
	}
	return strings.Join(lines[:height], "\n")
}

// capDetail bounds the detail content to rows rendered lines at width,
// counting lipgloss wrapping. Preview lines get cut first: the summary
// line and the labeled tail (path, url, metadata, seen: the last tail
// lines, per Entry.DetailTail) carry the most and survive as long as
// they fit.
func capDetail(content string, width, rows, tail int) string {
	wrap := lipgloss.NewStyle().Width(width)
	h := func(s string) int { return lipgloss.Height(wrap.Render(s)) }
	if h(content) <= rows {
		return content
	}
	lines := strings.Split(content, "\n")
	tail = max(1, len(lines)-tail)
	out := []string{lines[0]}
	used := h(lines[0])
	for _, l := range lines[tail:] {
		used += h(l)
	}
	for _, l := range lines[1:tail] {
		lh := h(l)
		if used+lh > rows {
			break
		}
		used += lh
		out = append(out, l)
	}
	return strings.Join(append(out, lines[tail:]...), "\n")
}

// viewReader renders the full-record view: the record name on top, the
// wrapped body window, then the status and help lines. ok is false with
// no selection, and View falls back to the list.
func (m model) viewReader() (string, bool) {
	e, ok := m.selected()
	if !ok {
		return "", false
	}
	var b strings.Builder
	b.WriteString(tabActive.Render(fitWidth(e.Name, m.width)) + "\n")
	body := m.readerBody()
	rows := m.readerRows()
	start := min(m.readerScroll, max(0, len(body)-rows))
	for i := start; i < len(body) && i < start+rows; i++ {
		b.WriteString(body[i] + "\n")
	}
	if m.quick {
		b.WriteString(m.viewQuick())
	}
	b.WriteString(fitWidth(m.status, m.width) + "\n")
	b.WriteString(helpStyle.Render(fitWidth(readerHelpLine, m.width)))
	return clampHeight(b.String(), m.height), true
}

const helpLine = "j/k g/G move tab/1-5 view enter read o open y yank t take u up f file b back s dictate r reply p approve D discard x delete / search q quit"

const readerHelpLine = "j/k scroll  space/pgup/pgdn page  g/G top/bottom  s dictate  r reply  p approve  D discard  q close"

// onceCounts renders one "<state> <count>" line per real state directory;
// the virtual memory hole never appears here.
func onceCounts(root string) (string, error) {
	var b strings.Builder
	for _, s := range recdep.States {
		list, err := loadState(root, s)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s %d\n", s, len(list))
	}
	return b.String(), nil
}

// runTUI watches the state dirs under root and runs the dashboard until
// the user switches it off.
func runTUI(root string) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	for _, s := range watchedDirs {
		if err := w.Add(filepath.Join(root, s)); err != nil {
			return err
		}
	}
	_, err = tea.NewProgram(newModel(root, w), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
