package layers_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/layers"
)

const alsoLib = "{path: example.org/lib, owner: internal/host/x}"

func alsoTable(also string) string {
	return strings.Replace(small, alsoLib, "{path: example.org/lib, owner: internal/host/x, also: ["+also+"]}", 1)
}

// TestAnAlsoNamesTheFilesItCovers: an also with files is spared in the lint rule by those files alone,
// tests being the owner's test files; one with no files keeps its whole folder and is named in the
// report line of WideAlsos, so the width is seen (gragas#1765).
func TestAnAlsoNamesTheFilesItCovers(t *testing.T) {
	t.Parallel()
	tb := parse(t, alsoTable(`{owner: internal/core/b, why: "A1: tests only", files: [tests]}, {owner: internal/port/testing, why: "A1: two files and its tests", files: [one.go, two.go, tests]}, {owner: internal/port, until: M3-01}`))
	current := "        " + layers.BeginMark + "\n        " + layers.EndMark + "\n"
	lint, err := tb.Lint([]byte(current))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"!**/internal/core/b/*_test.go"`, `"!**/internal/port/testing/one.go"`, `"!**/internal/port/testing/two.go"`, `"!**/internal/port/testing/*_test.go"`, `"!**/internal/port/*.go"`} {
		if !strings.Contains(string(lint), want) {
			t.Errorf("the lint rule lacks %s", want)
		}
	}
	for _, not := range []string{`"!**/internal/core/b/*.go"`, `"!**/internal/port/testing/*.go"`} {
		if strings.Contains(string(lint), not) {
			t.Errorf("the lint rule still spares a whole folder: %s", not)
		}
	}
	if doc := tb.Doc(); !strings.Contains(doc, "and `internal/core/b` in `tests` (A1: tests only)") {
		t.Errorf("the document does not say the files of the row:\n%s", doc)
	}
	wide := tb.WideAlsos()
	if len(wide) != 1 || !strings.Contains(wide[0], "internal/port") || !strings.Contains(wide[0], "example.org/lib") {
		t.Errorf("WideAlsos = %q, want one line for internal/port", wide)
	}
}

// TestAnAlsoWithFilesOutOfFormIsRefused: files is a list of tests and file names of the owner's folder;
// an empty list, a scalar, a path, a pattern, a name that is no Go file and a name twice are each refused.
func TestAnAlsoWithFilesOutOfFormIsRefused(t *testing.T) {
	t.Parallel()
	for name, files := range map[string]string{
		"a scalar":        "tests",
		"an empty list":   "[]",
		"a path":          "[sub/main.go]",
		"a pattern":       "[\"*.go\"]",
		"an alternation":  "[\"{a,b}.go\"]",
		"a class":         "[\"[ab].go\"]",
		"no Go file":      "[main.txt]",
		"a name twice":    "[tests, tests]",
		"a number":        "[7]",
		"the bare suffix": "[.go]",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := layers.Parse([]byte(alsoTable(`{owner: internal/core/b, why: "A1: x", files: ` + files + `}`)))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), "the files of also internal/core/b") {
				t.Errorf("Parse = %v, want ErrTable naming the files of the also", err)
			}
		})
	}
}
