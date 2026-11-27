package porttest_test

import (
	"context"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestTheInMemoryStandinAnswersTheSuiteOfCases: the fake plugin every core test
// uses answers the cases of internal/port/contracttest as the wasm standin and
// the real host do, so a core test that passes on it passes on
// them.
func TestTheInMemoryStandinAnswersTheSuiteOfCases(t *testing.T) {
	t.Parallel()
	contracttest.Plugin(t, func(t *testing.T) port.Plugin {
		t.Helper()
		var s porttest.Standin
		r := model.PluginRef{Name: "standin", Version: "1.0.0"}
		s.Install("/plugins", r, porttest.NewStandinPlugin())
		p, err := s.Open(context.Background(), "/plugins", r)
		if err != nil {
			t.Fatal(err)
		}
		return p
	})
}
