package undo_test

import (
	"context"
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/undo"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain registers -update, which this package keeps no goldens for, so
// go test -update over internal/cli runs here too, and starts from the
// shared test setup.
func TestMain(m *testing.M) {
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "write the goldens: this package keeps none")
	}
	os.Exit(configtest.Main(m))
}

// session counts the calls of undo and plans nothing.
type session struct {
	kit.Session // every other method is the run's, which these tests never reach
	calls       int
}

func (s *session) Undo(context.Context, kit.Cmd) int { s.calls++; return 0 }
func (s *session) Exit(int) error                    { return nil }

func exec(t *testing.T, s *session, args ...string) error {
	t.Helper()
	c := undo.New(s)
	c.SetArgs(append([]string{}, args...)) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	return c.ExecuteContext(context.Background())
}

// TestUndoTakesNoArgumentAndRunsOnce: the word takes no argument, and a bare
// run reaches the session once.
// Risk: an argument taken and dropped, or a word that never reaches the run.
func TestUndoTakesNoArgumentAndRunsOnce(t *testing.T) {
	t.Parallel()
	s := &session{}
	if err := exec(t, s); err != nil || s.calls != 1 {
		t.Fatalf("bare undo: err %v, %d calls, want one", err, s.calls)
	}
	if err := exec(t, s, "go.errors.1"); err == nil {
		t.Error("undo took an argument")
	}
}

// TestUndoWritesOnlyWithApply: the word carries the writes annotation and
// takes --apply (cli.commands.1).
// Risk: a word that writes without --apply being the gate.
func TestUndoWritesOnlyWithApply(t *testing.T) {
	t.Parallel()
	c := undo.New(&session{})
	if _, ok := c.Annotations[kit.Writes]; !ok {
		t.Error("undo has no writes annotation")
	}
	if c.Flags().Lookup("apply") == nil {
		t.Error("undo takes no --apply")
	}
}

// TestTheRowNamesEveryExit: undo can end done, declined, refused and not ready.
// Planted failure: the row drops refused or not ready.
func TestTheRowNamesEveryExit(t *testing.T) {
	t.Parallel()
	row := undo.Row()
	for _, w := range []model.Word{model.Done, model.Declined, model.Refused, model.NotReady} {
		if !slices.Contains(row.Exits, w) {
			t.Errorf("the row's closing words %v leave out %s", row.Exits, w)
		}
	}
}
