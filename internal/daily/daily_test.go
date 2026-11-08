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

func TestCarryOverCreatesNote(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "2026-11-05.md", "# 2026-11-05\n\n- [ ] A\n- [x] Done\n  - [ ] Nested\n")
	write(t, dir, "2026-11-06.md", "# 2026-11-06\n\n- [ ] B\n- [ ] A\nNotiz\n")
	write(t, dir, "2026-11-09.md", "- [ ] future\n")
	write(t, dir, "Inbox.md", "- [ ] inbox\n")

	res, err := Dir{dir}.CarryOver("2026-11-08")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.Moved != 3 || res.Sources != 2 {
		t.Fatalf("unexpected result %+v", res)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] A\n- [ ] Nested\n- [ ] B\n" {
		t.Fatalf("today: %q", got)
	}
	if got := read(t, dir, "2026-11-05.md"); got != "# 2026-11-05\n\n- [x] Done\n" {
		t.Fatalf("source 05: %q", got)
	}
	if got := read(t, dir, "2026-11-06.md"); got != "# 2026-11-06\n\nNotiz\n" {
		t.Fatalf("source 06: %q", got)
	}
	if got := read(t, dir, "2026-11-09.md"); got != "- [ ] future\n" {
		t.Fatalf("future note touched: %q", got)
	}
	if got := read(t, dir, "Inbox.md"); got != "- [ ] inbox\n" {
		t.Fatalf("inbox touched: %q", got)
	}

	// Second run is a no-op.
	res, _ = Dir{dir}.CarryOver("2026-11-08")
	if res.Created || res.Moved != 0 || res.Sources != 0 {
		t.Fatalf("second run: %+v", res)
	}
}

func TestCarryOverIntoExistingNote(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "2026-11-06.md", "- [ ] A\n- [ ] B\n")
	write(t, dir, "2026-11-08.md", "# 2026-11-08\n\n- [ ] A\n")
	res, err := Dir{dir}.CarryOver("2026-11-08")
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || res.Moved != 1 || res.Sources != 1 {
		t.Fatalf("unexpected result %+v", res)
	}
	if got := read(t, dir, "2026-11-08.md"); got != "# 2026-11-08\n\n- [ ] A\n- [ ] B\n" {
		t.Fatalf("today: %q", got)
	}
	if got := read(t, dir, "2026-11-06.md"); got != "" {
		t.Fatalf("source: %q", got)
	}
}

func TestAppendTodosSeparator(t *testing.T) {
	if got := appendTodos("# x\nText\n\n", []string{"A"}); got != "# x\nText\n\n- [ ] A\n" {
		t.Fatalf("got %q", got)
	}
	if got := appendTodos("# x\n- [x] done", []string{"A", "B"}); got != "# x\n- [x] done\n- [ ] A\n- [ ] B\n" {
		t.Fatalf("got %q", got)
	}
	if got := appendTodos("  \n", []string{"A"}); got != "- [ ] A\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSwap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "d.md")
	write(t, dir, "d.md", "# x\n- [ ] A\nText\n- [x] B\n- [ ] C\n")
	n, _ := Parse(p, "d")
	if err := Swap(p, n.Tasks[0], n.Tasks[1]); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "d.md"); got != "# x\n- [x] B\nText\n- [ ] A\n- [ ] C\n" {
		t.Fatalf("got %q", got)
	}
	write(t, dir, "d.md", "gone\n")
	if err := Swap(p, n.Tasks[1], n.Tasks[2]); err != ErrChanged {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}
