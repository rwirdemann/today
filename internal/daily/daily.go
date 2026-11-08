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
	"unicode"
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
	idx := locate(lines, t)
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

// locate finds t in lines: at its recorded line if unchanged, otherwise the
// first line with identical content. Returns -1 if not found.
func locate(lines []string, t Task) int {
	if t.Line < len(lines) && strings.TrimRight(lines[t.Line], "\r") == t.Raw {
		return t.Line
	}
	for i, l := range lines {
		if strings.TrimRight(l, "\r") == t.Raw {
			return i
		}
	}
	return -1
}

// Swap exchanges the lines of tasks a and b in the file at path.
func Swap(path string, a, b Task) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	i, j := locate(lines, a), locate(lines, b)
	if i < 0 || j < 0 {
		return ErrChanged
	}
	lines[i], lines[j] = lines[j], lines[i]
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

// openTodoRe matches an open todo exactly like the Obsidian todo-carry-over plugin.
var openTodoRe = regexp.MustCompile(`^(\s*[-*+] )\[ \] (.*)$`)

func openTodo(line string) (string, bool) {
	m := openTodoRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil || strings.TrimSpace(m[2]) == "" {
		return "", false
	}
	return strings.TrimSpace(m[2]), true
}

func extractOpenTodos(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if t, ok := openTodo(l); ok {
			out = append(out, t)
		}
	}
	return out
}

func removeOpenTodos(text string) string {
	var keep []string
	for _, l := range strings.Split(text, "\n") {
		if _, ok := openTodo(l); !ok {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

var anyTodoRe = regexp.MustCompile(`^\s*[-*+] \[.\] `)

func appendTodos(text string, todos []string) string {
	block := "- [ ] " + strings.Join(todos, "\n- [ ] ")
	trimmed := strings.TrimRightFunc(text, unicode.IsSpace)
	if trimmed == "" {
		return block + "\n"
	}
	sep := "\n\n"
	if anyTodoRe.MatchString(trimmed[strings.LastIndex(trimmed, "\n")+1:]) {
		sep = "\n"
	}
	return trimmed + sep + block + "\n"
}

// CarryOverResult describes what CarryOver did.
type CarryOverResult struct {
	Created bool // today's note was created
	Moved   int  // todos appended to today's note
	Sources int  // earlier notes that had open todos
}

// CarryOver ensures the note for date exists and moves all open todos from
// earlier daily notes into it (oldest first, skipping todos already open in
// the target). Carried todos are removed from their source notes. Semantics
// match the Obsidian todo-carry-over plugin.
func (d Dir) CarryOver(date string) (CarryOverResult, error) {
	var res CarryOverResult
	path := d.pathFor(date)
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		b = []byte("# " + date + "\n")
		if err := writeFile(path, string(b)); err != nil {
			return res, err
		}
		res.Created = true
	case err != nil:
		return res, err
	}
	target := string(b)

	entries, err := os.ReadDir(d.Path)
	if err != nil {
		return res, err
	}
	var prev []string
	for _, e := range entries {
		if e.IsDir() || !nameRe.MatchString(e.Name()) {
			continue
		}
		if dt := strings.TrimSuffix(e.Name(), ".md"); dt < date {
			prev = append(prev, dt)
		}
	}
	sort.Strings(prev)

	seen := map[string]bool{}
	for _, t := range extractOpenTodos(target) {
		seen[t] = true
	}
	var todos []string
	sources := map[string]string{} // path -> content
	for _, dt := range prev {
		p := d.pathFor(dt)
		c, err := os.ReadFile(p)
		if err != nil {
			return res, err
		}
		open := extractOpenTodos(string(c))
		if len(open) == 0 {
			continue
		}
		sources[p] = string(c)
		for _, t := range open {
			if !seen[t] {
				seen[t] = true
				todos = append(todos, t)
			}
		}
	}
	res.Sources = len(sources)
	if len(sources) == 0 {
		return res, nil
	}

	// Write the target first so nothing gets lost if a later write fails.
	if len(todos) > 0 {
		if err := writeFile(path, appendTodos(target, todos)); err != nil {
			return res, err
		}
		res.Moved = len(todos)
	}
	for p, c := range sources {
		if err := writeFile(p, removeOpenTodos(c)); err != nil {
			return res, err
		}
	}
	return res, nil
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
