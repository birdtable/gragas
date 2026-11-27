package clock_test

import (
	"errors"
	"math"
	"testing"

	"github.com/birdtable/gragas/internal/host/clock"

	"github.com/birdtable/go-proc/load"
)

// TestTheLoadIsReadOrSaidToBeNone: on every system Busy gives a load that is
// finite and not below zero and a children's time of at least zero, or an
// error that matches load.ErrNoAverage or errors.ErrUnsupported, so
// the tier budget is judged on the wall time alone. Both arms are held on
// every system, with no GOOS between them: a system that keeps an average
// and one that keeps none each pass by the arm that is theirs.
func TestTheLoadIsReadOrSaidToBeNone(t *testing.T) {
	t.Parallel()
	got, cpu, err := clock.Busy()
	t.Logf("this host: load %.2f, the children's time %s, %v", got, cpu, err)
	if err != nil {
		if !errors.Is(err, load.ErrNoAverage) && !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("Busy failed with %v, which is neither load.ErrNoAverage nor errors.ErrUnsupported", err)
		}
		return
	}
	if math.IsNaN(got) || math.IsInf(got, 0) || got < 0 || cpu < 0 {
		t.Errorf("Busy = %v, %v; want a finite load and a time of at least 0", got, cpu)
	}
}
