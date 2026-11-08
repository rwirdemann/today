package daily

import (
	"path/filepath"
	"testing"
)

const inbox = "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n\n## Leer\n\n## Ende\n\n- [x] D\n"

func TestParseSections(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Inbox.md", inbox)
	n, err := Parse(filepath.Join(dir, "Inbox.md"), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Neo", "Privat", "Leer", "Ende"}
	if len(n.Sections) != len(want) {
		t.Fatalf("sections %v", n.Sections)
	}
	for i := range want {
		if n.Sections[i] != want[i] {
			t.Fatalf("sections %v", n.Sections)
		}
	}
	if n.Tasks[0].Section != "Neo" || n.Tasks[2].Section != "Privat" || n.Tasks[3].Section != "Ende" {
		t.Fatalf("tasks %+v", n.Tasks)
	}
}

func TestInsertInSection(t *testing.T) {
	cases := []struct{ section, want string }{
		{"Neo", "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n- [ ] N\n## Privat\n\n- [ ] C\n\n## Leer\n\n## Ende\n\n- [x] D\n"},
		{"Privat", "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n- [ ] N\n\n## Leer\n\n## Ende\n\n- [x] D\n"},
		{"Leer", "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n\n## Leer\n\n- [ ] N\n## Ende\n\n- [x] D\n"},
		{"Ende", "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n\n## Leer\n\n## Ende\n\n- [x] D\n- [ ] N\n"},
		{"Neu", "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n\n## Leer\n\n## Ende\n\n- [x] D\n\n## Neu\n\n- [ ] N\n"},
		{"", "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n\n## Leer\n\n## Ende\n\n- [x] D\n- [ ] N\n"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		write(t, dir, "Inbox.md", inbox)
		if err := InsertInSection(filepath.Join(dir, "Inbox.md"), c.section, "- [ ] N"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, dir, "Inbox.md"); got != c.want {
			t.Errorf("section %q:\n got %q\nwant %q", c.section, got, c.want)
		}
	}
	// Missing file.
	dir := t.TempDir()
	if err := InsertInSection(filepath.Join(dir, "Inbox.md"), "Neo", "- [ ] N"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "Inbox.md"); got != "## Neo\n\n- [ ] N\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Inbox.md")
	write(t, dir, "Inbox.md", inbox)
	n, _ := Parse(p, "")
	if err := Delete(p, n.Tasks[1]); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "Inbox.md"); got != "# Inbox\n\n## Neo\n\n- [ ] A\n## Privat\n\n- [ ] C\n\n## Leer\n\n## Ende\n\n- [x] D\n" {
		t.Fatalf("got %q", got)
	}
	if err := Delete(p, n.Tasks[1]); err != ErrChanged {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}

func TestMoveToDay(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Inbox.md")
	write(t, dir, "Inbox.md", inbox)
	n, _ := Parse(p, "")
	d := Dir{dir}
	if err := d.MoveToDay(p, n.Tasks[2], "2026-11-08"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] C\n" {
		t.Fatalf("today %q", got)
	}
	if got := read(t, dir, "Inbox.md"); got != "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n\n## Leer\n\n## Ende\n\n- [x] D\n" {
		t.Fatalf("inbox %q", got)
	}
	// Done state is kept; appends directly after the last task.
	if err := d.MoveToDay(p, n.Tasks[3], "2026-11-08"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] C\n- [x] D\n" {
		t.Fatalf("today %q", got)
	}
	// Stale task: nothing written.
	if err := d.MoveToDay(p, n.Tasks[2], "2026-11-08"); err != ErrChanged {
		t.Fatalf("want ErrChanged, got %v", err)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] C\n- [x] D\n" {
		t.Fatalf("today changed: %q", got)
	}
}

func TestMoveToSection(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Inbox.md", inbox)
	write(t, dir, "2026-11-08.md", "# 2026-11-08\n\n- [ ] T\n- [ ] U\n")
	today := filepath.Join(dir, "2026-11-08.md")
	n, _ := Parse(today, "2026-11-08")
	if err := MoveToSection(today, n.Tasks[0], filepath.Join(dir, "Inbox.md"), "Privat"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] U\n" {
		t.Fatalf("today %q", got)
	}
	if got := read(t, dir, "Inbox.md"); got != "# Inbox\n\n## Neo\n\n- [ ] A\n- [ ] B\n## Privat\n\n- [ ] C\n- [ ] T\n\n## Leer\n\n## Ende\n\n- [x] D\n" {
		t.Fatalf("inbox %q", got)
	}
}
