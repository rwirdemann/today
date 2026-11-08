// Package tui implements the Bubble Tea interface for today's tasks.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"scheduled/internal/daily"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type (
	loadedMsg struct {
		note     *daily.Note
		fallback bool
		err      error
	}
	fileChangedMsg struct{}
	tickMsg        time.Time
	errMsg         struct{ err error }
)

// Model is the Bubble Tea model.
type Model struct {
	dir      daily.Dir
	today    string
	note     *daily.Note
	fallback bool
	cursor   int // index into visible()
	hideDone bool
	adding   bool
	input    textinput.Model
	status   string
	err      error
	height   int
	watcher  *fsnotify.Watcher
	events   chan struct{}
}

// New creates a model for the daily notes in dir.
func New(dir string) (*Model, error) {
	ti := textinput.New()
	ti.Placeholder = "New task"
	ti.Prompt = "+ "
	m := &Model{dir: daily.Dir{Path: dir}, today: daily.Today(), input: ti, events: make(chan struct{}, 1)}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	m.watcher = w
	go m.watch()
	return m, nil
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
			if filepath.Ext(ev.Name) != ".md" {
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

func (m *Model) load() tea.Cmd {
	dir, today := m.dir, m.today
	return func() tea.Msg {
		n, fb, err := dir.Load(today)
		return loadedMsg{n, fb, err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.waitForChange(), tick())
}

func (m *Model) visible() []daily.Task {
	if m.note == nil {
		return nil
	}
	if !m.hideDone {
		return m.note.Tasks
	}
	var out []daily.Task
	for _, t := range m.note.Tasks {
		if !t.Done {
			out = append(out, t)
		}
	}
	return out
}

func (m *Model) clampCursor() {
	n := len(m.visible())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.input.Width = msg.Width - 4
		return m, nil

	case loadedMsg:
		m.note, m.fallback, m.err = msg.note, msg.fallback, msg.err
		m.clampCursor()
		return m, nil

	case fileChangedMsg:
		return m, tea.Batch(m.load(), m.waitForChange())

	case tickMsg:
		// Handle midnight rollover.
		if d := daily.Today(); d != m.today {
			m.today = d
			return m, tea.Batch(m.load(), tick())
		}
		return m, tick()

	case errMsg:
		m.status = errStyle.Render(msg.err.Error())
		return m, m.load()

	case tea.KeyMsg:
		if m.adding {
			return m.updateAdding(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m *Model) updateAdding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.adding = false
		m.input.Reset()
		return m, nil
	case tea.KeyEnter:
		text := m.input.Value()
		m.adding = false
		m.input.Reset()
		dir, today := m.dir, m.today
		return m, func() tea.Msg {
			if err := dir.AddTask(today, text); err != nil {
				return errMsg{err}
			}
			n, fb, err := dir.Load(today)
			return loadedMsg{n, fb, err}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	tasks := m.visible()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(tasks)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(tasks) - 1
		m.clampCursor()
	case "h":
		m.hideDone = !m.hideDone
		m.clampCursor()
	case "r":
		return m, m.load()
	case "a":
		m.adding = true
		return m, m.input.Focus()
	case " ", "x", "enter":
		if len(tasks) == 0 {
			return m, nil
		}
		t, path := tasks[m.cursor], m.note.Path
		dir, today := m.dir, m.today
		return m, func() tea.Msg {
			if err := daily.Toggle(path, t); err != nil {
				return errMsg{err}
			}
			n, fb, err := dir.Load(today)
			return loadedMsg{n, fb, err}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	var b strings.Builder

	weekday := ""
	if t, err := time.Parse(daily.DateLayout, m.today); err == nil {
		weekday = weekdays[t.Weekday()] + ", "
	}
	b.WriteString(titleStyle.Render("Tasks · "+weekday+m.today) + "\n")

	switch {
	case m.err != nil:
		b.WriteString(errStyle.Render("Error: "+m.err.Error()) + "\n")
	case m.note == nil:
		b.WriteString(warnStyle.Render("No daily note found. Press 'a' to create today's note.") + "\n")
	case m.fallback:
		b.WriteString(warnStyle.Render(fmt.Sprintf("No note for today, showing %s", m.note.Date)) + "\n")
	}
	b.WriteString("\n")

	tasks := m.visible()
	open, done := 0, 0
	if m.note != nil {
		for _, t := range m.note.Tasks {
			if t.Done {
				done++
			} else {
				open++
			}
		}
	}

	if m.note != nil && len(tasks) == 0 {
		if m.hideDone && done > 0 {
			b.WriteString(helpStyle.Render("  All done 🎉") + "\n")
		} else {
			b.WriteString(helpStyle.Render("  No tasks") + "\n")
		}
	}

	// Simple scrolling: keep cursor visible within available rows.
	rows := m.height - 7
	if rows < 3 {
		rows = len(tasks)
	}
	start := 0
	if m.cursor >= rows {
		start = m.cursor - rows + 1
	}
	end := min(len(tasks), start+rows)

	for i := start; i < end; i++ {
		t := tasks[i]
		box := "[ ]"
		if t.Done {
			box = "[x]"
		}
		line := strings.Repeat(" ", t.Indent) + box + " " + t.Text
		if t.Done {
			line = doneStyle.Render(line)
		}
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("▸ ")
			if !t.Done {
				line = cursorStyle.Render(strings.Repeat(" ", t.Indent)+box+" ") + t.Text
			}
		}
		b.WriteString(prefix + line + "\n")
	}

	b.WriteString("\n")
	if m.note != nil {
		b.WriteString(helpStyle.Render(fmt.Sprintf("%d open · %d done", open, done)) + "\n")
	}
	if m.adding {
		b.WriteString(m.input.View() + "\n")
		b.WriteString(helpStyle.Render("enter save · esc cancel"))
	} else {
		if m.status != "" {
			b.WriteString(m.status + "\n")
		}
		hide := "hide done"
		if m.hideDone {
			hide = "show done"
		}
		b.WriteString(helpStyle.Render("j/k move · space toggle · a add · h " + hide + " · r reload · q quit"))
	}
	return b.String()
}

var weekdays = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
