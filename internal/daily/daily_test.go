package daily

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParse(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "2026-11-08.md", "# 2026-11-08\n\n- [ ] Open\n- [x] Done\nText\n  - [X] Nested\n* [ ] Star\n")
	n, err := Parse(filepath.Join(dir, "2026-11-08.md"), "2026-11-08")
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Tasks) != 4 {
		t.Fatalf("want 4 tasks, got %d", len(n.Tasks))
	}
	if n.Tasks[0].Done || !n.Tasks[1].Done || !n.Tasks[2].Done || n.Tasks[2].Indent != 2 || n.Tasks[3].Text != "Star" {
		t.Fatalf("unexpected tasks: %+v", n.Tasks)
	}
}

func TestLoadFallback(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "2026-11-05.md", "- [ ] a\n")
	write(t, dir, "2026-11-06.md", "- [ ] b\n")
	write(t, dir, "2026-12-01.md", "- [ ] future\n")
	write(t, dir, "Notes.md", "- [ ] other\n")
	n, fb, err := Dir{dir}.Load("2026-11-08")
	if err != nil || !fb || n.Date != "2026-11-06" {
		t.Fatalf("got %+v fb=%v err=%v", n, fb, err)
	}
	n, fb, _ = Dir{dir}.Load("2026-11-05")
	if fb || n.Date != "2026-11-05" {
		t.Fatalf("expected exact match, got %+v fb=%v", n, fb)
	}
	n, _, err = Dir{t.TempDir()}.Load("2026-11-08")
	if n != nil || err != nil {
		t.Fatalf("expected nil note, got %+v %v", n, err)
	}
}

func TestToggle(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.md", "# x\n- [ ] A\n- [x] B\n")
	p := filepath.Join(dir, "d.md")
	n, _ := Parse(p, "d")
	if err := Toggle(p, n.Tasks[0]); err != nil {
		t.Fatal(err)
	}
	if err := Toggle(p, n.Tasks[1]); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "d.md"); got != "# x\n- [x] A\n- [ ] B\n" {
		t.Fatalf("got %q", got)
	}
}

func TestToggleAfterLineShift(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "d.md")
	write(t, dir, "d.md", "- [ ] A\n")
	n, _ := Parse(p, "d")
	write(t, dir, "d.md", "inserted\n- [ ] A\n")
	if err := Toggle(p, n.Tasks[0]); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "d.md"); got != "inserted\n- [x] A\n" {
		t.Fatalf("got %q", got)
	}
	write(t, dir, "d.md", "gone\n")
	if err := Toggle(p, n.Tasks[0]); err != ErrChanged {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}

func TestAddTask(t *testing.T) {
	dir := t.TempDir()
	d := Dir{dir}
	if err := d.AddTask("2026-11-08", "New"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] New\n" {
		t.Fatalf("got %q", got)
	}
	write(t, dir, "2026-11-09.md", "# 2026-11-09\n- [ ] A")
	_ = d.AddTask("2026-11-09", "B")
	if got := read(t, dir, "2026-11-09.md"); got != "# 2026-11-09\n- [ ] A\n- [ ] B\n" {
		t.Fatalf("got %q", got)
	}
}
