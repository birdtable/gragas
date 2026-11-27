package journeys

import (
	"cmp"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/go-files/appdirs"
)

// stateRowMasked is s with the help's Environment row for the state folder
// printed as the goldens hold it: the help names the variable the system the
// binary is built for reads (go-files' appdirs, as gragas reads it), and the goldens hold the one
// testdata/help-state-var.txt names, at the help's column. Where the two are
// the same word, s is returned as it is.
func stateRowMasked(tb testing.TB, s string) string {
	tb.Helper()
	b, err := os.ReadFile("testdata/help-state-var.txt")
	if err != nil {
		tb.Fatalf("the mask of the help's state-variable row has no testdata: %v", err) // never off unseen
	}
	var base string
	for line := range strings.SplitSeq(string(b), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			base = line
			break
		}
	}
	own := "the home"
	if src, err := appdirs.Sources(appdirs.XDG); err == nil {
		own = cmp.Or(src.State, appdirs.HomeVar)
	}
	if base == "" || own == base {
		return s
	}
	const words = "where gen"
	return strings.ReplaceAll(s, helpColumn(own)+words, helpColumn(base)+words)
}

// helpColumn is a variable name padded to the help's column of words.
func helpColumn(v string) string {
	const col = 19
	return v + strings.Repeat(" ", max(1, col-len(v)))
}
