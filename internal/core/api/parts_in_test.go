package coreapi_test

import (
	"slices"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// TestAPartsFileWithCRLFNamesTheSamePartsAsOneWithLF: a CR is a separator, so
// a file saved with Windows line ends never names a part `std\r`, which
// matches no rule. Risk: a checked-out CRLF parts file that quietly checks
// nothing. Planted failure: '\r' off the separators.
func TestAPartsFileWithCRLFNamesTheSamePartsAsOneWithLF(t *testing.T) {
	t.Parallel()
	want := []string{"std", "web"}
	for name, raw := range map[string]string{
		"LF":    "std\nweb\n",
		"CRLF":  "std\r\nweb\r\n",
		"comma": "std, web\r\n",
		"tabs":  "std\tweb",
	} {
		if got := coreapi.PartsIn(raw); !slices.Equal(got, want) {
			t.Errorf("%s: parts %q, want %q", name, got, want)
		}
	}
}
