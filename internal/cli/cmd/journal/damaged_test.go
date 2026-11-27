package journal

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestADamagedCopyNamesItsPathOnlyWhenThereIsOne: a copy nobody kept has no
// path, so the words drop "(copy …)" whole and keep every other word.
func TestADamagedCopyNamesItsPathOnlyWhenThereIsOne(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		d    model.Damaged
		want string
	}{
		"kept but unreadable": {model.Damaged{Path: "a.txt", Copy: "/s/c", Expected: "sha256:e"}, "the copy of a.txt that run r1 kept is damaged: it could not be read, and the record names sha256:e (copy /s/c)"},
		"kept, other bytes":   {model.Damaged{Path: "a.txt", Copy: "/s/c", Got: "sha256:g", Expected: "sha256:e"}, "the copy of a.txt that run r1 kept is damaged: it holds sha256:g, and the record names sha256:e (copy /s/c)"},
		"nobody kept one":     {model.Damaged{Path: "a.txt", Expected: "sha256:e"}, "the copy of a.txt that run r1 kept is damaged: it could not be read, and the record names sha256:e"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := damagedSaid("r1", tc.d); got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}
