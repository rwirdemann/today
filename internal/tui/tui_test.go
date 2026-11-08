package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestFallbackToggleAndAdd(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "2026-11-06.md")
	os.WriteFile(old, []byte("# 2026-11-06\n\n- [ ] Fix pipelines\n- [x] Checklist\n"), 0o644)

	m, err := New(dir, filepath.Join(dir, "Inbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.date = "2026-11-08"
	m.Update(m.load()())

	v := m.View()
	for _, want := range []string{"showing 2026-11-06", "Fix pipelines", "1 open · 1 done"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q:\n%s", want, v)
		}
	}

	// Toggle first task in the fallback note.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m.Update(cmd())
	if b, _ := os.ReadFile(old); !strings.Contains(string(b), "- [x] Fix pipelines") {
		t.Fatalf("toggle not written: %s", b)
	}

	// Add creates today's note and switches to it.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	for _, r := range "Neu" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if b, _ := os.ReadFile(filepath.Join(dir, "2026-11-08.md")); string(b) != "# 2026-11-08\n\n- [ ] Neu\n" {
		t.Fatalf("today note: %q", b)
	}
	if v := m.View(); strings.Contains(v, "showing") || !strings.Contains(v, "Neu") {
		t.Fatalf("expected today's note:\n%s", v)
	}
}

func TestReorder(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "2026-11-08.md")
	os.WriteFile(p, []byte("# 2026-11-08\n\n- [ ] A\n- [x] B\n- [ ] C\n"), 0o644)
	m, err := New(dir, filepath.Join(dir, "Inbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.date = "2026-11-08"
	m.Update(m.load()())

	// Hide done, then move C up: it swaps with A (the visible neighbour).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m.Update(cmd())
	if b, _ := os.ReadFile(p); string(b) != "# 2026-11-08\n\n- [ ] C\n- [x] B\n- [ ] A\n" {
		t.Fatalf("got %q", b)
	}
	if m.today.cursor != 0 || m.visible(&m.today)[0].Text != "C" {
		t.Fatalf("cursor should follow C, cursor=%d", m.today.cursor)
	}
	// Moving up at the top is a no-op.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftUp}); cmd != nil {
		t.Fatal("expected no command at top")
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// press sends a key and runs the resulting command (if any) synchronously.
func press(m *Model, s string) {
	_, cmd := m.Update(key(s))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(tea.BatchMsg); !ok {
				m.Update(msg)
			}
		}
	}
}

func setup(t *testing.T, today, inbox string) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	if today != "" {
		os.WriteFile(filepath.Join(dir, "2026-11-08.md"), []byte(today), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "Inbox.md"), []byte(inbox), 0o644)
	m, err := New(dir, filepath.Join(dir, "Inbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.date = "2026-11-08"
	m.Update(m.load()())
	return m, dir
}

func file(t *testing.T, dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return string(b)
}

const testInbox = "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n\n## Privat\n\n- [ ] C\n"

func TestInboxToToday(t *testing.T) {
	m, dir := setup(t, "", testInbox)
	press(m, "tab")
	press(m, "down")
	press(m, "right") // move B to today, creating the note
	if got := file(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] B\n" {
		t.Fatalf("today %q", got)
	}
	if got := file(t, dir, "Inbox.md"); got != "# Inbox\n\n## Neo\n\n- [ ] A\n\n## Privat\n\n- [ ] C\n" {
		t.Fatalf("inbox %q", got)
	}
	if m.fallback || m.today.note == nil || len(m.today.note.Tasks) != 1 {
		t.Fatalf("today pane not reloaded")
	}
}

func TestTodayToInboxSection(t *testing.T) {
	m, dir := setup(t, "# 2026-11-08\n\n- [ ] T\n", testInbox)
	press(m, "left")
	if m.mode != modePick {
		t.Fatal("expected section picker")
	}
	if v := m.View(); !strings.Contains(v, "1 Neo") || !strings.Contains(v, "2 Privat") {
		t.Fatalf("picker not shown:\n%s", v)
	}
	press(m, "2")
	if got := file(t, dir, "Inbox.md"); got != testInbox+"- [ ] T\n" {
		t.Fatalf("inbox %q", got)
	}
	if got := file(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n" {
		t.Fatalf("today %q", got)
	}
}

func TestInboxAddDeleteToggle(t *testing.T) {
	m, dir := setup(t, "", testInbox)
	press(m, "tab")
	press(m, "down")
	press(m, "down") // C in Privat
	press(m, "a")
	for _, r := range "N" {
		m.Update(key(string(r)))
	}
	press(m, "enter")
	if got := file(t, dir, "Inbox.md"); got != testInbox+"- [ ] N\n" {
		t.Fatalf("add: %q", got)
	}
	press(m, " ") // toggle C
	if got := file(t, dir, "Inbox.md"); !strings.Contains(got, "- [x] C") {
		t.Fatalf("toggle: %q", got)
	}
	press(m, "d")
	press(m, "n") // cancel
	if !strings.Contains(file(t, dir, "Inbox.md"), "C") {
		t.Fatal("delete should have been cancelled")
	}
	press(m, "d")
	press(m, "d")
	if got := file(t, dir, "Inbox.md"); strings.Contains(got, "C") {
		t.Fatalf("delete: %q", got)
	}
}

func TestLayout(t *testing.T) {
	m, _ := setup(t, "# 2026-11-08\n\n- [ ] Today task\n", testInbox)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	v := m.View()
	var line string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "Heute ·") {
			line = l
		}
	}
	if !strings.Contains(line, "Inbox ·") {
		t.Fatalf("expected side-by-side titles on one line:\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "Heute ·") && strings.Contains(l, "Inbox ·") {
			t.Fatalf("expected stacked layout at width 60")
		}
		if w := ansi.StringWidth(l); w > 60 {
			t.Fatalf("line wider than terminal (%d): %q", w, l)
		}
	}
}

func TestTinyWindowDoesNotPanic(t *testing.T) {
	m, _ := setup(t, "# 2026-11-08\n\n- [ ] T\n", testInbox)
	m.Update(key("a"))
	for w := 0; w < 30; w++ {
		for h := 0; h < 10; h++ {
			m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m.View()
		}
	}
}

// Bubble Tea renders once before the first load and before the window size
// is known; that must not panic.
func TestViewBeforeLoad(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, filepath.Join(dir, "Inbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.View()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.View()
}

func TestInboxIsLeftOfToday(t *testing.T) {
	m, _ := setup(t, "# 2026-11-08\n\n- [ ] T\n", testInbox)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if i, h := strings.Index(l, "Inbox ·"), strings.Index(l, "Heute ·"); i >= 0 && h >= 0 {
			if i > h {
				t.Fatalf("Inbox should be left of Heute: %q", l)
			}
			return
		}
	}
	t.Fatal("titles not found on one line")
}
