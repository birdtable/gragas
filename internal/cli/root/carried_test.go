package root

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// carriedLocks answers every Take with the refusal of a state folder a clone
// carries, and keeps the root each take was handed.
type carriedLocks struct{ roots *[]string }

func (k carriedLocks) Take(_, root string) (legacy.Lock, error) {
	*k.roots = append(*k.roots, root)
	return nil, fmt.Errorf("the state folder /made/up/project/.state lies in /made/up/project: %w", port.ErrCarried)
}

// TestAStateFolderAProjectCarriesEndsRefusedAndRecordsNothing: the recorder's lock refusing a state folder a clone carries ends the
// run refused, as a lock another account owns does, not failed, and nothing
// is written or recorded. Planted: claim answering ErrCarried as any other
// error (the run then ends failed, red here).
func TestAStateFolderAProjectCarriesEndsRefusedAndRecordsNothing(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "--apply")
	var roots []string
	p.Ports.Locks = carriedLocks{roots: &roots}
	code, sc := execute(t.Context(), t, p)
	said := sc.output()
	if code != exit.Refused || len(f.Wrote) != 0 || len(f.Runs) != 0 {
		t.Fatalf("exit %d, wrote %v, runs %d, want refused and nothing written or recorded; said:\n%s", code, f.Wrote, len(f.Runs), said)
	}
	if !strings.Contains(said, "the state folder /made/up/project/.state lies in /made/up/project") {
		t.Errorf("the run does not say the refusal:\n%s", said)
	}
}

// TestTheRecordersLockIsHandedTheFolderDashCNames ((c)): the
// lock the recorder takes before it begins the run's record is handed the
// folder -C names, its default included, so a state folder in a root opened
// elsewhere is judged before the record is begun there; a command with no -C
// hands none. Planted: claim handing the lock "" (the -C rows are then red).
func TestTheRecordersLockIsHandedTheFolderDashCNames(t *testing.T) {
	t.Parallel()
	elsewhere := filepath.Join(t.TempDir(), "project")
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"-C names a folder elsewhere", []string{"gen", "-C", elsewhere, "--apply"}, elsewhere},
		{"no -C is its default, the working folder", []string{"gen", "--apply"}, "."},
		{"a command with no -C hands none", []string{"cache", "clear", "--apply"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := world(t)
			p, _ := process(t, f, c.args...)
			var roots []string
			p.Ports.Locks = carriedLocks{roots: &roots}
			if code, sc := execute(t.Context(), t, p); code != exit.Refused {
				t.Fatalf("exit %d, want refused; said:\n%s", code, sc.output())
			}
			if len(roots) != 1 || roots[0] != c.want {
				t.Errorf("the lock was handed %q, want [%q]", roots, c.want)
			}
		})
	}
}
