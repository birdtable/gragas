package legacy_test

import (
	"os"
	"reflect"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/port/legacy"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestTheFakeFillsEveryPort: the fake fills every port, required or not, so
// the core's tests run with no adapter (arch.ports.17).
func TestTheFakeFillsEveryPort(t *testing.T) {
	t.Parallel()
	if missing := porttest.Missing((&porttest.Fake{}).Ports(), false); len(missing) > 0 {
		t.Errorf("the fake leaves %v empty", missing)
	}
}

// TestAnEmptyPortIsSeen is the guard's negative test: a port left nil is
// named.
func TestAnEmptyPortIsSeen(t *testing.T) {
	t.Parallel()
	p := (&porttest.Fake{}).Ports()
	p.Locks = nil
	if got := porttest.Missing(p, true); len(got) != 1 || got[0] != "Locks" {
		t.Errorf("Missing = %v, want [Locks]", got)
	}
}

// TestEveryPortIsStatedRequiredOrNot: each field of Ports says, in its tag,
// whether a platform must fill it.
func TestEveryPortIsStatedRequiredOrNot(t *testing.T) {
	t.Parallel()
	ty := reflect.TypeFor[legacy.Ports]()
	for f := range ty.Fields() {
		if v, ok := f.Tag.Lookup("port"); !ok || (v != "required" && v != "optional") {
			t.Errorf("Ports.%s: tag port is %q, want required or optional", f.Name, v)
		}
	}
}
