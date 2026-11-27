package root

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// notMine is the old port's refusal of a folder another account owns, said as
// the host says it, with made-up names.
func notMine(what, dir string) error {
	return fmt.Errorf("%s %s is owned by account 7, and this run is account 0: nothing is written: %w", what, dir, legacy.ErrNotOwner)
}

// refusingLocks answers every Take with the refusal of a state folder another
// account owns.
type refusingLocks struct{}

func (refusingLocks) Take(string, string) (legacy.Lock, error) {
	return nil, notMine("the state folder", "/made/up/state")
}

// TestALockInAnotherAccountsFolderIsRefusedAndWritesNothing: the old lock
// refuses a state folder another account owns, so gen --apply ends refused
// with the host's sentence and the line to type, takes no lock, writes
// nothing and records nothing, as a held lock does. Risk: root running
// gen --apply over another account's state folder, its refusal said as a
// failure or a busy lock.
func TestALockInAnotherAccountsFolderIsRefusedAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "--apply")
	p.Ports.Locks = refusingLocks{}
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Refused || len(f.Wrote) != 0 || len(f.Runs) != 0 {
		t.Fatalf("exit %d, wrote %v, runs %d, want refused and nothing written or recorded; said:\n%s", code, f.Wrote, len(f.Runs), said)
	}
	for _, says := range []string{"the state folder /made/up/state is owned by account 7", "as the account that owns that folder: gragas gen --apply"} {
		if !strings.Contains(said, says) {
			t.Errorf("the run does not say %q:\n%s", says, said)
		}
	}
}

// TestARepositoryAnotherAccountOwnsIsRefused: the old writer refuses a
// repository another account owns at the first write, so gen --apply ends
// refused with the host's sentence, the run's id and the line to type, and
// its record closes on refused. Risk: the refusal of another account's
// repository said as a failed write to retry.
func TestARepositoryAnotherAccountOwnsIsRefused(t *testing.T) {
	t.Parallel()
	f := world(t)
	f.WriteErr = notMine("the repository", "/made/up/repo")
	p, _ := process(t, f, "gen", "--apply")
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Refused || len(f.Wrote) != 0 {
		t.Fatalf("exit %d, wrote %v, want refused and nothing written; said:\n%s", code, f.Wrote, said)
	}
	for _, says := range []string{"the repository /made/up/repo is owned by account 7", "(run run-1)", "as the account that owns that folder: gragas gen --apply"} {
		if !strings.Contains(said, says) {
			t.Errorf("the run does not say %q:\n%s", says, said)
		}
	}
	if len(f.Runs) != 1 || f.Runs[0].Word != string(model.Refused) || f.Runs[0].Code != exit.Refused {
		t.Errorf("the run's record is %+v, want one closed on refused", f.Runs)
	}
}

// TestEveryCommandThatWritesEndsRefusedOverAnotherAccountsLock: the lock is
// taken for every command given --apply that writes, not for gen alone, so
// cache clear --apply ends refused too, and declares it. Risk: a writing
// command ending on a word its registry row does not declare.
func TestEveryCommandThatWritesEndsRefusedOverAnotherAccountsLock(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"gen", "--apply"}, {"cache", "clear", "--apply"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			f := world(t)
			p, _ := process(t, f, args...)
			p.Ports.Locks = refusingLocks{}
			code, sc := execute(t.Context(), t, p)
			said := sc.output()
			if code != exit.Refused || len(f.Runs) != 0 || !strings.Contains(said, "the state folder /made/up/state is owned by account 7") {
				t.Fatalf("exit %d, runs %d, want refused and nothing recorded; said:\n%s", code, len(f.Runs), said)
			}
			declared := false
			for _, c := range commands().Rows() {
				if c.Name == args[0] {
					declared = slices.Contains(c.Exits, model.Refused)
				}
			}
			if !declared {
				t.Errorf("%s ends refused, which its registry row does not declare", args[0])
			}
		})
	}
}
