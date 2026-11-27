package scan_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/scan"
)

// TestANameOfMaxRunWordsIsHashedAndOneWordMoreIsRefused (boundary: MaxRun): the
// longest name the scan hashes is MaxRun words, and a name of one word more is
// refused, naming how many it had, so the refusal is the cap's.
func TestANameOfMaxRunWordsIsHashedAndOneWordMoreIsRefused(t *testing.T) {
	t.Parallel()
	words := func(n int) string {
		w := make([]string, n)
		for i := range w {
			w[i] = "w" + strings.Repeat("a", i)
		}
		return strings.Join(w, " ")
	}
	if got, err := scan.NameHashes(words(scan.MaxRun)); err != nil || len(got) != 2 {
		t.Errorf("a name of %d words (MaxRun) = %q, %v; want its two lines", scan.MaxRun, got, err)
	}
	_, err := scan.NameHashes(words(scan.MaxRun + 1))
	if err == nil || !strings.Contains(err.Error(), "a name of "+strconv.Itoa(scan.MaxRun+1)+" words") {
		t.Errorf("a name of %d words (MaxRun+1) = %v; want it refused by its count", scan.MaxRun+1, err)
	}
}
