package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFallbackToggleAndAdd(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "2026-11-06.md")
	os.WriteFile(old, []byte("# 2026-11-06\n\n- [ ] Fix pipelines\n- [x] Checklist\n"), 0o644)

	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.today = "2026-11-08"
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
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.today = "2026-11-08"
	m.Update(m.load()())

	// Hide done, then move C up: it swaps with A (the visible neighbour).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	m.Update(cmd())
	if b, _ := os.ReadFile(p); string(b) != "# 2026-11-08\n\n- [ ] C\n- [x] B\n- [ ] A\n" {
		t.Fatalf("got %q", b)
	}
	if m.cursor != 0 || m.visible()[0].Text != "C" {
		t.Fatalf("cursor should follow C, cursor=%d", m.cursor)
	}
	// Moving up at the top is a no-op.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlUp}); cmd != nil {
		t.Fatal("expected no command at top")
	}
}
