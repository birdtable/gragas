package journeys

import (
	"encoding/json"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The goldens hold no system's shape: no carriage return, and no Windows-shaped path,
// which is a drive letter followed by a colon and a backslash, or a backslash between two path segments.
// A hit is fixed by normalising the output in the code under test, never in the golden. A golden that is a
// JSON document is read by its decoded strings, because JSON writes a newline or a quote as a backslash.
// backslash is the character the plants spell by its code, so this file names no separator.
const backslash = 0x5c

var (
	winDrive = regexp.MustCompile(`(^|[^A-Za-z0-9])[A-Za-z]:\\`)
	winSegs  = regexp.MustCompile(`[A-Za-z0-9_.-]\\[A-Za-z0-9_.-]`)
)

// jsonStrings returns every string a JSON document holds, keys included; ok is false when text is not one.
func jsonStrings(text string) (out []string, ok bool) {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, false
	}
	var walk func(any)
	walk = func(x any) {
		switch x := x.(type) {
		case string:
			out = append(out, x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for k, e := range x {
				out = append(out, k)
				walk(e)
			}
		}
	}
	walk(v)
	return out, true
}

// goldenStrings is the text of a golden to read: it is sections ("--- stdout", "--- stderr"), and a section
// that is a JSON document gives its decoded strings, the rest its lines.
func goldenStrings(text string) []string {
	var out []string
	for _, sec := range splitSections(text) {
		if js, ok := jsonStrings(sec); ok {
			out = append(out, js...)
		} else {
			out = append(out, strings.Split(sec, "\n")...)
		}
	}
	return out
}

// splitSections cuts text at each line that starts "--- ".
func splitSections(text string) []string {
	var secs []string
	var cur []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "--- ") {
			secs = append(secs, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return append(secs, strings.Join(cur, "\n"))
}

// shapeProblems is what a golden's text holds of a system's shape.
func shapeProblems(text string) []string {
	var problems []string
	if strings.Contains(text, "\r") {
		problems = append(problems, "a carriage return")
	}
	for _, s := range goldenStrings(text) {
		for line := range strings.SplitSeq(s, "\n") {
			switch {
			case winDrive.MatchString(line):
				problems = append(problems, "a drive letter path: "+line)
			case winSegs.MatchString(line):
				problems = append(problems, "a backslash between path segments: "+line)
			}
		}
	}
	return problems
}

func TestTheGoldensHoldNoCarriageReturnAndNoWindowsPath(t *testing.T) {
	t.Parallel()
	bs := string(rune(backslash))
	for _, tc := range []struct{ name, text, want string }{
		{"clean", "a/b\nc: d\n", ""},
		{"json escapes are not paths", "{\n  \"about\": \"that\\nprojects\\u003c and \\\"x\\\"\"\n}\n", ""},
		{"a carriage return", "a\r\nb\n", "carriage return"},
		{"a drive", "see " + "C:" + bs + "Users\n", "drive letter"},
		{"segments", "dir" + bs + "file\n", "between path segments"},
		{"segments in json", "{\"p\": \"dir" + bs + bs + "file\"}\n", "between path segments"},
	} {
		got := strings.Join(shapeProblems(tc.text), "; ")
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("%s: want %q, got %q", tc.name, tc.want, got)
		}
	}
	fsys := os.DirFS("testdata")
	n := 0
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".golden") {
			return err
		}
		b, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return rerr
		}
		n++
		for _, pr := range shapeProblems(string(b)) {
			t.Errorf("testdata/%s holds %s: normalise the output in the code under test, not the golden", p, pr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("no golden found under testdata")
	}
}
