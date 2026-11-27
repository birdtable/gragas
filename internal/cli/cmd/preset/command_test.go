package preset_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/preset"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// session keeps whether the run was handed --check, and plans nothing.
type session struct {
	kit.Session // every other method is the run's, which these tests never reach
	ran, check  bool
}

func (s *session) Preset(_ context.Context, _ kit.Cmd, check bool) int {
	s.ran, s.check = true, check
	return 0
}

func (s *session) Exit(int) error { return nil }

func exec(t *testing.T, s *session, args ...string) error {
	t.Helper()
	c := preset.New(s)
	c.SetArgs(args) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	return c.ExecuteContext(context.Background())
}

// TestPresetHandsTheRunItsCheckFlag: --check reaches the run, and a bare
// preset hands it false. Risk: a flag wired to nothing, so --check writes.
func TestPresetHandsTheRunItsCheckFlag(t *testing.T) {
	t.Parallel()
	s := &session{}
	if err := exec(t, s, "--check"); err != nil || !s.ran || !s.check {
		t.Errorf("--check: err %v, ran %v, check %v", err, s.ran, s.check)
	}
	bare := &session{}
	if err := exec(t, bare); err != nil || !bare.ran || bare.check {
		t.Errorf("bare: err %v, ran %v, check %v", err, bare.ran, bare.check)
	}
	if err := exec(t, &session{}, "extra"); err == nil {
		t.Error("preset with an argument ran")
	}
}

// TestPresetWritesOnlyWithApply: the command carries the writes annotation and
// takes --apply (cli.commands.1). Risk: a writer that is not gated.
func TestPresetWritesOnlyWithApply(t *testing.T) {
	t.Parallel()
	c := preset.New(&session{})
	if got := c.Annotations["writes"]; got != "with --apply" {
		t.Errorf("writes annotation %q, want %q", got, "with --apply")
	}
	if c.Flags().Lookup("apply") == nil {
		t.Error("preset takes no --apply")
	}
}

// TestTheRowNamesEveryExit: preset can end findings under --check, declined,
// refused and not ready, so the row lists them. Planted failure: the row drops one.
func TestTheRowNamesEveryExit(t *testing.T) {
	t.Parallel()
	row := preset.Row()
	for _, w := range []model.Word{model.Done, model.Findings, model.Declined, model.Refused, model.NotReady} {
		if !slices.Contains(row.Exits, w) {
			t.Errorf("the row's closing words %v leave out %s", row.Exits, w)
		}
	}
}
