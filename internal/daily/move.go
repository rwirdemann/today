package daily

import (
	"errors"
	"os"
	"strings"
)

// taskLine renders t as a top-level checkbox line, keeping its done state.
func taskLine(t Task) string {
	mark := " "
	if t.Done {
		mark = "x"
	}
	return "- [" + mark + "] " + t.Text
}

// Delete removes the line of t from the file at path.
func Delete(path string, t Task) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	i := locate(lines, t)
	if i < 0 {
		return ErrChanged
	}
	lines = append(lines[:i], lines[i+1:]...)
	return writeFile(path, strings.Join(lines, "\n"))
}

// InsertInSection inserts line at the end of the "## section" block in the
// file at path (after its last non-blank line). An empty section appends to
// the end of the file. Missing sections and files are created.
func InsertInSection(path, section, line string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := string(b)
	if section == "" {
		return writeFile(path, appendLine(text, line))
	}

	lines := strings.Split(text, "\n")
	h := -1
	for i, l := range lines {
		if m := headingRe.FindStringSubmatch(strings.TrimRight(l, "\r")); m != nil && m[1] == section {
			h = i
			break
		}
	}
	if h < 0 {
		trimmed := strings.TrimRight(text, " \t\r\n")
		if trimmed != "" {
			trimmed += "\n\n"
		}
		return writeFile(path, trimmed+"## "+section+"\n\n"+line+"\n")
	}

	end := len(lines)
	for i := h + 1; i < len(lines); i++ {
		if headingRe.MatchString(strings.TrimRight(lines[i], "\r")) || strings.HasPrefix(lines[i], "# ") {
			end = i
			break
		}
	}
	last := -1
	for i := h + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			last = i
		}
	}
	var ins []string
	at := last + 1
	if last < 0 {
		// Empty section: keep the blank line after the heading.
		at = h + 1
		ins = []string{"", line}
		if at < len(lines) && strings.TrimSpace(lines[at]) == "" && at < end {
			at, ins = h+2, []string{line}
		}
	} else {
		ins = []string{line}
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, ins...)
	out = append(out, lines[at:]...)
	return writeFile(path, strings.Join(out, "\n"))
}

// appendLine appends line to text using the carry-over separator rules.
func appendLine(text, line string) string {
	trimmed := strings.TrimRight(text, " \t\r\n")
	if trimmed == "" {
		return line + "\n"
	}
	sep := "\n\n"
	if anyTodoRe.MatchString(trimmed[strings.LastIndex(trimmed, "\n")+1:]) {
		sep = "\n"
	}
	return trimmed + sep + line + "\n"
}

// checkPresent verifies t is still in the file at path.
func checkPresent(path string, t Task) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if locate(strings.Split(string(b), "\n"), t) < 0 {
		return ErrChanged
	}
	return nil
}

// MoveToDay moves t from the file at src to the end of the note for date,
// creating that note if needed. The destination is written first so nothing
// is lost if removing from the source fails.
func (d Dir) MoveToDay(src string, t Task, date string) error {
	if err := checkPresent(src, t); err != nil {
		return err
	}
	path := d.pathFor(date)
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		b = []byte("# " + date + "\n")
	case err != nil:
		return err
	}
	if err := writeFile(path, appendLine(string(b), taskLine(t))); err != nil {
		return err
	}
	return Delete(src, t)
}

// MoveToSection moves t from the file at src into section of the file at dst.
func MoveToSection(src string, t Task, dst, section string) error {
	if err := checkPresent(src, t); err != nil {
		return err
	}
	if err := InsertInSection(dst, section, taskLine(t)); err != nil {
		return err
	}
	return Delete(src, t)
}
