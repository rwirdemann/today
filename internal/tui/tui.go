// Package tui implements the Bubble Tea interface for today's tasks and the Inbox.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"

	"github.com/rwirdemann/today/internal/daily"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	paneOnStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Underline(true)
	paneOffStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sepStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// sideBySideMin is the terminal width from which both panes are shown side by side.
const sideBySideMin = 90

type focus int

const (
	focusToday focus = iota
	focusInbox
)

type mode int

const (
	modeList mode = iota
	modeAdd
	modePick    // choosing the Inbox section for a task moved from today
	modeConfirm // confirming a delete
)

type (
	loadedMsg struct {
		today    *daily.Note
		fallback bool
		inbox    *daily.Note
		err      error
		status   string
	}
	fileChangedMsg struct{}
	tickMsg        time.Time
	errMsg         struct{ err error }
)

type pane struct {
	note   *daily.Note
	cursor int // index into visible tasks
}

// Model is the Bubble Tea model.
type Model struct {
	dir       daily.Dir
	inboxPath string
	date      string

	today    pane
	inbox    pane
	fallback bool
	focus    focus
	hideDone bool

	mode       mode
	input      textinput.Model
	pickCursor int
	pickTask   daily.Task

	status        string
	err           error
	width, height int

	watcher *fsnotify.Watcher
	events  chan struct{}
}

// New creates a model for the daily notes in dir and the inbox file at inboxPath.
func New(dir, inboxPath string) (*Model, error) {
	ti := textinput.New()
	ti.Placeholder = "New task"
	ti.Prompt = "+ "
	m := &Model{
		dir:       daily.Dir{Path: dir},
		inboxPath: inboxPath,
		date:      daily.Today(),
		input:     ti,
		events:    make(chan struct{}, 1),
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, d := range uniq(dir, filepath.Dir(inboxPath)) {
		if err := w.Add(d); err != nil {
			w.Close()
			return nil, fmt.Errorf("watch %s: %w", d, err)
		}
	}
	m.watcher = w
	go m.watch()
	return m, nil
}

func uniq(a, b string) []string {
	if filepath.Clean(a) == filepath.Clean(b) {
		return []string{a}
	}
	return []string{a, b}
}

// Close releases the file watcher.
func (m *Model) Close() { m.watcher.Close() }

// watch forwards relevant fs events, debounced, into m.events.
func (m *Model) watch() {
	var timer *time.Timer
	for {
		select {
		case ev, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			name := filepath.Clean(ev.Name)
			if filepath.Ext(name) != ".md" ||
				(filepath.Dir(name) != filepath.Clean(m.dir.Path) && name != filepath.Clean(m.inboxPath)) {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(150*time.Millisecond, func() {
				select {
				case m.events <- struct{}{}:
				default:
				}
			})
		case _, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (m *Model) waitForChange() tea.Cmd {
	return func() tea.Msg {
		<-m.events
		return fileChangedMsg{}
	}
}

func loadAll(dir daily.Dir, date, inboxPath, status string) loadedMsg {
	n, fb, err := dir.Load(date)
	msg := loadedMsg{today: n, fallback: fb, err: err, status: status}
	if in, ierr := daily.Parse(inboxPath, ""); ierr == nil {
		msg.inbox = in
	}
	return msg
}

func (m *Model) load() tea.Cmd { return m.do("", nil) }

// do runs op (if any) in the background and reloads both files afterwards.
func (m *Model) do(status string, op func() error) tea.Cmd {
	dir, date, inboxPath := m.dir, m.date, m.inboxPath
	return func() tea.Msg {
		if op != nil {
			if err := op(); err != nil {
				return errMsg{err}
			}
		}
		return loadAll(dir, date, inboxPath, status)
	}
}

// carryOver moves open todos from earlier notes into today's note
// (creating it if needed) and then reloads.
func (m *Model) carryOver() tea.Cmd {
	dir, date, inboxPath := m.dir, m.date, m.inboxPath
	return func() tea.Msg {
		res, err := dir.CarryOver(date)
		if err != nil {
			return errMsg{fmt.Errorf("carry over: %w", err)}
		}
		var status string
		if res.Moved > 0 {
			status = fmt.Sprintf("%d open task(s) carried over from %d note(s)", res.Moved, res.Sources)
		} else if res.Created {
			status = "Created today's note"
		}
		return loadAll(dir, date, inboxPath, status)
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.waitForChange(), tick())
}

func (m *Model) visible(p *pane) []daily.Task {
	if p.note == nil {
		return nil
	}
	if !m.hideDone {
		return p.note.Tasks
	}
	var out []daily.Task
	for _, t := range p.note.Tasks {
		if !t.Done {
			out = append(out, t)
		}
	}
	return out
}

func (m *Model) clamp(p *pane) {
	n := len(m.visible(p))
	if p.cursor >= n {
		p.cursor = n - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (m *Model) focused() *pane {
	if m.focus == focusInbox {
		return &m.inbox
	}
	return &m.today
}

// current returns the task under the cursor in the focused pane.
func (m *Model) current() (daily.Task, bool) {
	p := m.focused()
	tasks := m.visible(p)
	if len(tasks) == 0 {
		return daily.Task{}, false
	}
	return tasks[p.cursor], true
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(msg.Width-4, 20) // bubbles textinput panics on tiny/negative widths
		return m, nil

	case loadedMsg:
		m.today.note, m.fallback, m.err = msg.today, msg.fallback, msg.err
		m.inbox.note = msg.inbox
		if msg.status != "" {
			m.status = msg.status
		}
		m.clamp(&m.today)
		m.clamp(&m.inbox)
		return m, nil

	case fileChangedMsg:
		return m, tea.Batch(m.load(), m.waitForChange())

	case tickMsg:
		// Handle midnight rollover.
		if d := daily.Today(); d != m.date {
			m.date = d
			return m, tea.Batch(m.load(), tick())
		}
		return m, tick()

	case errMsg:
		m.status = errStyle.Render(msg.err.Error())
		return m, m.load()

	case tea.KeyMsg:
		switch m.mode {
		case modeAdd:
			return m.updateAdding(msg)
		case modePick:
			return m.updatePicking(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m *Model) updateAdding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeList
		m.input.Reset()
		return m, nil
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		m.mode = modeList
		m.input.Reset()
		if text == "" {
			return m, nil
		}
		if m.focus == focusInbox {
			section := ""
			if t, ok := m.current(); ok {
				section = t.Section
			} else if m.inbox.note != nil && len(m.inbox.note.Sections) > 0 {
				section = m.inbox.note.Sections[0]
			}
			path := m.inboxPath
			return m, m.do("", func() error { return daily.InsertInSection(path, section, "- [ ] "+text) })
		}
		dir, date := m.dir, m.date
		return m, m.do("", func() error { return dir.AddTask(date, text) })
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) sections() []string {
	if m.inbox.note == nil {
		return nil
	}
	return m.inbox.note.Sections
}

func (m *Model) updatePicking(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	secs := m.sections()
	switch k := msg.String(); k {
	case "esc", "q":
		m.mode = modeList
	case "j", "down":
		if m.pickCursor < len(secs)-1 {
			m.pickCursor++
		}
	case "k", "up":
		if m.pickCursor > 0 {
			m.pickCursor--
		}
	case "enter":
		m.mode = modeList
		return m, m.moveToInbox(secs[m.pickCursor])
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			if i := int(k[0] - '1'); i < len(secs) {
				m.mode = modeList
				return m, m.moveToInbox(secs[i])
			}
		}
	}
	return m, nil
}

func (m *Model) moveToInbox(section string) tea.Cmd {
	t, src, dst := m.pickTask, m.today.note.Path, m.inboxPath
	label := "Inbox"
	if section != "" {
		label += " › " + section
	}
	return m.do("Moved to "+label, func() error { return daily.MoveToSection(src, t, dst, section) })
}

func (m *Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeList
	if msg.String() != "d" && msg.String() != "y" {
		m.status = ""
		return m, nil
	}
	t, ok := m.current()
	if !ok {
		return m, nil
	}
	path := m.focused().note.Path
	return m, m.do("Deleted", func() error { return daily.Delete(path, t) })
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	p := m.focused()
	tasks := m.visible(p)
	key := msg.String()

	// Keys that work in both panes.
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.focus = 1 - m.focus
		return m, nil
	case "j", "down":
		if p.cursor < len(tasks)-1 {
			p.cursor++
		}
		return m, nil
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return m, nil
	case "g", "home":
		p.cursor = 0
		return m, nil
	case "G", "end":
		p.cursor = len(tasks) - 1
		m.clamp(p)
		return m, nil
	case "h":
		m.hideDone = !m.hideDone
		m.clamp(&m.today)
		m.clamp(&m.inbox)
		return m, nil
	case "c":
		return m, m.carryOver()
	case "r":
		return m, m.load()
	case "a":
		m.mode = modeAdd
		return m, m.input.Focus()
	case " ", "x", "enter":
		t, ok := m.current()
		if !ok {
			return m, nil
		}
		path := p.note.Path
		return m, m.do("", func() error { return daily.Toggle(path, t) })
	}

	if m.focus == focusToday {
		switch key {
		case "ctrl+up", "K", "ctrl+down", "J":
			other := p.cursor - 1
			if key == "ctrl+down" || key == "J" {
				other = p.cursor + 1
			}
			if other < 0 || other >= len(tasks) {
				return m, nil
			}
			a, b, path := tasks[p.cursor], tasks[other], p.note.Path
			p.cursor = other // cursor follows the moved task
			return m, m.do("", func() error { return daily.Swap(path, a, b) })
		case "left", "i":
			t, ok := m.current()
			if !ok {
				return m, nil
			}
			m.pickTask = t
			if len(m.sections()) == 0 {
				return m, m.moveToInbox("")
			}
			m.pickCursor = 0
			m.mode = modePick
		}
		return m, nil
	}

	// Inbox pane.
	switch key {
	case "right", "l", "t":
		t, ok := m.current()
		if !ok {
			return m, nil
		}
		dir, date, src := m.dir, m.date, p.note.Path
		return m, m.do("Moved to today", func() error { return dir.MoveToDay(src, t, date) })
	case "d":
		if _, ok := m.current(); ok {
			m.mode = modeConfirm
		}
	}
	return m, nil
}

// --- view ---

func (m *Model) View() string {
	var b strings.Builder

	weekday := ""
	if t, err := time.Parse(daily.DateLayout, m.date); err == nil {
		weekday = weekdays[t.Weekday()] + ", "
	}
	b.WriteString(titleStyle.Render("Tasks · "+weekday+m.date) + "\n")
	switch {
	case m.err != nil:
		b.WriteString(errStyle.Render("Error: "+m.err.Error()) + "\n")
	case m.today.note == nil:
		b.WriteString(warnStyle.Render("No daily note found. Press 'a' to create today's note.") + "\n")
	case m.fallback:
		b.WriteString(warnStyle.Render(fmt.Sprintf("No note for today, showing %s", m.today.note.Date)) + "\n")
	}
	b.WriteString("\n")

	footer := m.footer()
	header := strings.Count(b.String(), "\n")
	avail := 0 // 0 = unlimited
	if m.height > 0 {
		avail = max(m.height-header-strings.Count(footer, "\n")-2, 4)
	}

	todayTitle, inboxTitle := m.paneTitles()
	if m.width == 0 || m.width >= sideBySideMin {
		colW := 0
		if m.width > 0 {
			colW = (m.width - 3) / 2
		}
		left := m.renderPane(inboxTitle, m.inboxRows(), m.focus == focusInbox, colW, avail)
		right := m.renderPane(todayTitle, m.todayRows(), m.focus == focusToday, colW, avail)
		n := max(strings.Count(left, "\n"), strings.Count(right, "\n")) + 1
		sep := sepStyle.Render(strings.TrimSuffix(strings.Repeat(" │ \n", n), "\n"))
		if colW > 0 {
			left = lipgloss.NewStyle().Width(colW).Render(left)
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right) + "\n")
	} else {
		half := 0
		if avail > 0 {
			half = max((avail-1)/2, 2)
		}
		b.WriteString(m.renderPane(inboxTitle, m.inboxRows(), m.focus == focusInbox, m.width, half) + "\n")
		b.WriteString(sepStyle.Render(strings.Repeat("─", max(m.width, 10))) + "\n")
		b.WriteString(m.renderPane(todayTitle, m.todayRows(), m.focus == focusToday, m.width, half) + "\n")
	}

	b.WriteString("\n" + footer)
	return b.String()
}

func (m *Model) paneTitles() (string, string) {
	count := func(n *daily.Note) (open, done int) {
		if n == nil {
			return
		}
		for _, t := range n.Tasks {
			if t.Done {
				done++
			} else {
				open++
			}
		}
		return
	}
	o, d := count(m.today.note)
	today := fmt.Sprintf("Heute · %d open · %d done", o, d)
	o, _ = count(m.inbox.note)
	inbox := fmt.Sprintf("Inbox · %d open", o)
	return today, inbox
}

func (m *Model) footer() string {
	var b strings.Builder
	switch m.mode {
	case modeAdd:
		target := "today"
		if m.focus == focusInbox {
			target = "Inbox"
			if t, ok := m.current(); ok && t.Section != "" {
				target += " › " + t.Section
			}
		}
		b.WriteString(helpStyle.Render("Add to "+target) + "\n")
		b.WriteString(m.input.View() + "\n")
		b.WriteString(helpStyle.Render("enter save · esc cancel"))
	case modePick:
		b.WriteString(warnStyle.Render("Move “"+m.pickTask.Text+"” to Inbox section:") + "\n")
		for i, s := range m.sections() {
			line := fmt.Sprintf("%d %s", i+1, s)
			if i == m.pickCursor {
				b.WriteString(cursorStyle.Render("▸ "+line) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
		b.WriteString(helpStyle.Render("j/k select · enter/1-9 move · esc cancel"))
	case modeConfirm:
		t, _ := m.current()
		b.WriteString(warnStyle.Render("Delete “" + t.Text + "” from Inbox? d/y = yes, any other key = no"))
	default:
		if m.status != "" {
			b.WriteString(m.status + "\n")
		}
		hide := "hide done"
		if m.hideDone {
			hide = "show done"
		}
		var help string
		if m.focus == focusToday {
			help = "tab inbox · j/k move · ctrl+↑/↓ reorder · space toggle · a add · ← to inbox · c carry over · h " + hide + " · q quit"
		} else {
			help = "tab today · j/k move · space toggle · a add · d delete · → to today · h " + hide + " · q quit"
		}
		style := helpStyle
		if m.width > 0 {
			style = style.Width(m.width)
		}
		b.WriteString(style.Render(help))
	}
	return b.String()
}

type row struct {
	text   string
	cursor bool
}

func taskText(t daily.Task, cursor, focused bool) string {
	box := "[ ]"
	if t.Done {
		box = "[x]"
	}
	indent := strings.Repeat(" ", t.Indent)
	prefix := "  "
	if cursor && focused {
		prefix = cursorStyle.Render("▸ ")
	} else if cursor {
		prefix = helpStyle.Render("› ")
	}
	switch {
	case t.Done:
		return prefix + doneStyle.Render(indent+box+" "+t.Text)
	case cursor && focused:
		return prefix + cursorStyle.Render(indent+box+" ") + t.Text
	default:
		return prefix + indent + box + " " + t.Text
	}
}

func (m *Model) todayRows() []row {
	tasks := m.visible(&m.today)
	if m.today.note != nil && len(tasks) == 0 {
		if m.hideDone && len(m.today.note.Tasks) > 0 {
			return []row{{text: helpStyle.Render("  All done 🎉")}}
		}
		return []row{{text: helpStyle.Render("  No tasks")}}
	}
	rows := make([]row, len(tasks))
	for i, t := range tasks {
		c := i == m.today.cursor
		rows[i] = row{taskText(t, c, m.focus == focusToday), c}
	}
	return rows
}

func (m *Model) inboxRows() []row {
	if m.inbox.note == nil {
		return []row{{text: helpStyle.Render("  No Inbox file (" + filepath.Base(m.inboxPath) + ")")}}
	}
	tasks := m.visible(&m.inbox)
	if len(tasks) == 0 {
		return []row{{text: helpStyle.Render("  Inbox empty")}}
	}
	var rows []row
	section := "\x00"
	for i, t := range tasks {
		if t.Section != section {
			section = t.Section
			if section != "" {
				rows = append(rows, row{text: sectionStyle.Render(section)})
			}
		}
		c := i == m.inbox.cursor
		rows = append(rows, row{taskText(t, c, m.focus == focusInbox), c})
	}
	return rows
}

// renderPane renders a titled list, truncated to width and scrolled so the
// cursor row stays visible within height (0 = unlimited).
func (m *Model) renderPane(title string, rows []row, focused bool, width, height int) string {
	style := paneOffStyle
	if focused {
		style = paneOnStyle
	}
	lines := []string{style.Render(title), ""}

	limit := max(len(rows), 1)
	if height > 0 {
		limit = max(height-2, 1)
	}
	cur := 0
	for i, r := range rows {
		if r.cursor {
			cur = i
		}
	}
	start := 0
	if cur >= limit {
		start = cur - limit + 1
	}
	// Show the section header above the cursor when scrolled.
	if start > 0 && start == cur && cur > 0 {
		start--
	}
	end := min(len(rows), start+limit)
	for _, r := range rows[start:end] {
		lines = append(lines, r.text)
	}
	if width > 0 {
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, width, "…")
		}
	}
	return strings.Join(lines, "\n")
}

var weekdays = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
