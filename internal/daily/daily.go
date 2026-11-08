// Package daily reads and edits Obsidian daily notes (Tagebuch/YYYY-MM-DD.md).
package daily

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

var (
	taskRe = regexp.MustCompile(`^(\s*[-*+] \[)([ xX])(\] ?)(.*)$`)
	nameRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.md$`)
)

// Task is a single checkbox line in a note.
type Task struct {
	Line   int    // 0-based line index in the file
	Raw    string // the full original line, used to detect concurrent edits
	Indent int    // leading whitespace width
	Done   bool
	Text   string
}

// Note is a parsed daily note.
type Note struct {
	Path  string
	Date  string // YYYY-MM-DD
	Tasks []Task
}

// Dir manages the daily-notes directory.
type Dir struct {
	Path string
}

func (d Dir) pathFor(date string) string { return filepath.Join(d.Path, date+".md") }

// Load returns the note for date. If it does not exist, it falls back to the
// newest note dated before date; fallback reports whether that happened.
// If no note exists at all, it returns (nil, false, nil).
func (d Dir) Load(date string) (note *Note, fallback bool, err error) {
	n, err := Parse(d.pathFor(date), date)
	if err == nil {
		return n, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	prev, err := d.newestBefore(date)
	if err != nil || prev == "" {
		return nil, false, err
	}
	n, err = Parse(d.pathFor(prev), prev)
	return n, err == nil, err
}

func (d Dir) newestBefore(date string) (string, error) {
	entries, err := os.ReadDir(d.Path)
	if err != nil {
		return "", err
	}
	var dates []string
	for _, e := range entries {
		if e.IsDir() || !nameRe.MatchString(e.Name()) {
			continue
		}
		if dt := strings.TrimSuffix(e.Name(), ".md"); dt < date {
			dates = append(dates, dt)
		}
	}
	if len(dates) == 0 {
		return "", nil
	}
	sort.Strings(dates)
	return dates[len(dates)-1], nil
}

// Parse reads a note file and extracts its tasks.
func Parse(path, date string) (*Note, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n := &Note{Path: path, Date: date}
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		m := taskRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent := len(m[1]) - len(strings.TrimLeft(m[1], " \t"))
		n.Tasks = append(n.Tasks, Task{
			Line:   i,
			Raw:    line,
			Indent: indent,
			Done:   m[2] != " ",
			Text:   strings.TrimSpace(m[4]),
		})
	}
	return n, nil
}

// ErrChanged is returned when the file changed underneath and the task could not be located.
var ErrChanged = errors.New("note changed on disk, reloaded")

// Toggle flips the checkbox of t in the file at path.
func Toggle(path string, t Task) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	idx := -1
	if t.Line < len(lines) && strings.TrimRight(lines[t.Line], "\r") == t.Raw {
		idx = t.Line
	} else {
		for i, l := range lines {
			if strings.TrimRight(l, "\r") == t.Raw {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return ErrChanged
	}
	m := taskRe.FindStringSubmatch(strings.TrimRight(lines[idx], "\r"))
	if m == nil {
		return ErrChanged
	}
	mark := "x"
	if m[2] != " " {
		mark = " "
	}
	cr := ""
	if strings.HasSuffix(lines[idx], "\r") {
		cr = "\r"
	}
	lines[idx] = m[1] + mark + m[3] + m[4] + cr
	return writeFile(path, strings.Join(lines, "\n"))
}

// AddTask appends an open task to the note for date, creating the note if needed.
func (d Dir) AddTask(date, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	path := d.pathFor(date)
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return writeFile(path, fmt.Sprintf("# %s\n\n- [ ] %s\n", date, text))
	case err != nil:
		return err
	}
	s := string(b)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return writeFile(path, s+"- [ ] "+text+"\n")
}

func writeFile(path, content string) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return os.WriteFile(path, []byte(content), mode)
}

// Today returns the current local date as YYYY-MM-DD.
func Today() string { return time.Now().Format(DateLayout) }
