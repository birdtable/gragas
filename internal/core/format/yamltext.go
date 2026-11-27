package format

import (
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// ErrMoved is a delta whose before value is not what the file holds now: the
// tree moved since the plan read it. Its message names the file and line.
var ErrMoved = errors.New("the tree moved")

// ErrUnsupported is a layout the engine will not edit by position, such as a
// value written with an anchor; the message says how to edit it by hand.
var ErrUnsupported = errors.New("the engine does not edit this layout")

// posError is a refusal at a line of a file; kind is ErrMoved or ErrUnsupported.
type posError struct {
	kind error
	file string
	line int
	msg  string
}

func (e *posError) Error() string { return fmt.Sprintf("%v: %s:%d: %s", e.kind, e.file, e.line, e.msg) }

func (e *posError) Unwrap() error { return e.kind }

// shift moves the line of a refusal by by, for text that starts below the first
// line of its file.
func shift(err error, by int) error {
	if pe, ok := errors.AsType[*posError](err); ok {
		return &posError{pe.kind, pe.file, pe.line + by, pe.msg}
	}
	return err
}

func moved(file string, line int, format string, args ...any) error {
	return &posError{ErrMoved, file, line, fmt.Sprintf(format, args...)}
}

func unsupported(file string, line int, why string) error {
	return &posError{ErrUnsupported, file, line, why}
}

// text is a YAML file as lines, for editing in place. Joining the lines gives
// the bytes back exactly; an edit touches only the lines it names.
type text struct {
	file  string
	lines []string // split at "\n"; a final "" when the file ends in a newline
	cr    string   // "\r" when every line ends CRLF
	// pin is the line of the rule an id names where the file holds the id
	// twice: the rule at that line is the one meant, whatever its id says now.
	pin map[string]int
}

// newText splits data into lines. A file that mixes line endings is refused,
// since an edit could not keep each line's own.
func newText(file string, data []byte) (*text, error) {
	t := &text{file: file, lines: strings.Split(string(data), "\n")}
	crs := 0
	for i, s := range t.lines {
		if strings.HasSuffix(s, "\r") && i < len(t.lines)-1 {
			crs++
		}
	}
	switch {
	case crs == 0:
	case crs == len(t.lines)-1:
		t.cr = "\r"
		for i := range t.lines[:len(t.lines)-1] {
			t.lines[i] = strings.TrimSuffix(t.lines[i], "\r")
		}
	default:
		return nil, unsupported(file, 1, "the file mixes LF and CRLF line endings; make them one and run again")
	}
	return t, nil
}

func (t *text) bytes() []byte {
	if t.cr == "" {
		return []byte(strings.Join(t.lines, "\n"))
	}
	var b strings.Builder
	for i, s := range t.lines {
		b.WriteString(s)
		if i < len(t.lines)-1 {
			b.WriteString("\r\n")
		}
	}
	return []byte(b.String())
}

// splice replaces lines[lo:hi] with repl.
func (t *text) splice(lo, hi int, repl []string) {
	out := append([]string{}, t.lines[:lo]...)
	out = append(out, repl...)
	t.lines = append(out, t.lines[hi:]...)
}

// doc parses the lines as they are now. Positions are read fresh before each
// edit, so an edit never relies on a line number an earlier edit moved.
func (t *text) doc() (*ordered.Map, error) {
	v, err := ordered.ParseYAML(t.bytes())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.file, err)
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		return nil, unsupported(t.file, 1, "the file is not a mapping")
	}
	return m, nil
}

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func comment(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "#") }

// valueEnd is the line after the last line that belongs to the value of the
// key at line i, which is written at column ind: every following line that is
// indented deeper. A blank line belongs only when a deeper line follows it.
func (t *text) valueEnd(i, ind int) int {
	last := i
	for j := i + 1; j < len(t.lines); j++ {
		s := t.lines[j]
		if blank(s) {
			continue
		}
		if indentOf(s) <= ind {
			break
		}
		last = j
	}
	return last + 1
}

// listEnd is valueEnd for a block list, which may stand at the key's own column.
func (t *text) listEnd(i, ind int) (end, dash int) {
	last, dash := i, ind+2
	first := true
	for j := i + 1; j < len(t.lines); j++ {
		s := t.lines[j]
		if blank(s) {
			continue
		}
		in := indentOf(s)
		isDash := strings.HasPrefix(strings.TrimSpace(s), "-")
		if in < ind || (in == ind && !isDash) {
			break
		}
		if first && isDash {
			dash, first = in, false
		}
		last = j
	}
	return last + 1, dash
}

// splitComment cuts a value from its trailing comment, quotes respected. The
// comment keeps the spaces before its #.
func splitComment(s string) (value, tail string) {
	var quote rune
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote && (quote == '\'' || s[i-1] != '\\') {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#' && (i == 0 || s[i-1] == ' '):
			return strings.TrimRight(s[:i], " "), s[len(strings.TrimRight(s[:i], " ")):]
		}
	}
	return s, ""
}

// scalarText is the text a scalar value of a parsed document is compared as.
func scalarText(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case string:
		return x, true
	case ordered.Timestamp:
		return string(x), true
	case bool, int64, float64:
		return ordered.JSON(x, -1), true
	}
	return "", false
}

// yamlScalar is s as a YAML scalar that reads back as the same string: plain
// when it does, double-quoted otherwise.
func yamlScalar(s string) string {
	if s != "" && !strings.ContainsAny(s, "\n\t\"'#:[]{}&*!|>%@`,") && strings.TrimSpace(s) == s {
		if v, err := ordered.ParseYAML([]byte("k: " + s)); err == nil {
			if m, ok := v.(*ordered.Map); ok && m.Len() == 1 {
				if got, ok := m.Val("k").(string); ok && got == s {
					return s
				}
			}
		}
	}
	return ordered.JSON(s, -1)
}

// valueLines is the lines a value takes after "key:": the first line holds
// the text that follows the colon, the rest are indented under the key at ind.
func valueLines(s string, ind int) []string {
	if !strings.Contains(s, "\n") {
		return []string{yamlScalar(s)}
	}
	head := "|-"
	if cut, ok := strings.CutSuffix(s, "\n"); ok {
		head, s = "|", cut
	}
	out := []string{head}
	for l := range strings.SplitSeq(s, "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Repeat(" ", ind+2)+l)
	}
	return out
}
