package configtest

import (
	"testing"

	"github.com/birdtable/go-console/uitest/golden"
)

// Golden compares got, byte for byte, with testdata/<name>, the whole name
// with its extension, or writes it there under -update, which a person passes
// and reads the diff of: no gate does. It is the one golden compare of the
// packages that keep goldens, and importing it declares the one -update flag in
// every package's test binary, since each links configtest, so go test -update
// over a folder of packages stops at none of them. The frames of
// a screen go through uitest.Golden instead, which trims the line ends a frame
// does not keep.
func Golden(t testing.TB, name string, got []byte) {
	t.Helper()
	golden.File(t, name, got)
}
