package contract

import (
	"errors"
	"slices"
	"testing"
)

// TestNextLineEndsOnTheHelpWhenTheSessionGivesNoLine: nextLine hands the
// session's Line the whole argv, head then rest, in one call, and ends on the
// line it gives. When Line refuses a word, next is the command's help, which
// holds no value, and why is Line's reason; no line is quoted some other way
// (A5, A6;  X 11, X 205). Which words a line refuses,
// in each shell, is held where the line is made: the root's
// TestAValueNoLineCanCarryGivesNoLineAndNamesIt and
// TestEachDialectQuotesOrRefusesAsShellQuoteDoes. Risk: a contract line
// quoted for a shell other than the run's, or a line printed for a value the
// session refused.
func TestNextLineEndsOnTheHelpWhenTheSessionGivesNoLine(t *testing.T) {
	t.Parallel()
	head := []string{"gragas", "contract", "validate"}
	var handed [][]string
	refusing := func(argv []string) (string, error) {
		handed = append(handed, argv)
		return "", errors.New(`"a\tb.json" cannot be written in a line to paste, so none is printed`)
	}
	next, why := nextLine(refusing, head, "a\tb.json", "--as", "plan")
	if next != "gragas help contract validate" || why != `; "a\tb.json" cannot be written in a line to paste, so none is printed` {
		t.Errorf("a refused word: nextLine = %q, %q; want the command's help and the session's reason", next, why)
	}
	if len(handed) != 1 || !slices.Equal(handed[0], []string{"gragas", "contract", "validate", "a\tb.json", "--as", "plan"}) {
		t.Errorf("Line was handed %q; want the whole argv once", handed)
	}
	fixture := []string{"gragas", "contract", "fixture", "--out"}
	if next, _ := nextLine(refusing, fixture, "d", "--apply"); next != "gragas help contract fixture" {
		t.Errorf("a refused fixture line: next = %q; want the help with no flag", next)
	}
	saying := func([]string) (string, error) { return "the line", nil }
	if next, why := nextLine(saying, head, "a b.json", "--as", "plan"); next != "the line" || why != "" {
		t.Errorf("a line given: nextLine = %q, %q; want that line and no reason", next, why)
	}
}
