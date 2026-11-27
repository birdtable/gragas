package root

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// listFails is a Tree whose List fails: a folder that cannot be read, as a
// double and not a permission bit.
type listFails struct {
	port.Tree
	err error
}

func (l listFails) List(model.Root, string, model.Scope) ([]model.Entry, error) { return nil, l.err }

// carriedJournal is a Journal whose reader refuses its state folder, as
// host/fs's does for one a clone carries.
type carriedJournal struct{ port.Journal }

func (carriedJournal) RecordsOf(context.Context, []byte) ([]model.Record, int, error) {
	return nil, 0, fmt.Errorf("journal records: the state folder lies in a project: %w", port.ErrCarried)
}

// TestCacheListOverACarriedStateFolderEndsRefused: cache list
// whose Journal refuses a state folder a clone carries ends on the refused
// word, not failed, with the recorder's fix and the line to type again, and
// lists nothing. Planted: kitSession.CacheList passing ErrCarried up raw (the
// run then ends failed, red here).
func TestCacheListOverACarriedStateFolderEndsRefused(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "cache", "list")
	p.Config = config.Config{StateDir: fakeState(), CacheDir: fakeCache(), Settings: config.Defaults()}
	p, sc := attach(t, p)
	sc.out.Instant(time.Unix(1_800_000_000, 0))
	sc.out.Seed(fakeState(), map[string]string{})
	sc.out.Seed(fakeCache(), map[string]string{})
	p.World.Journal = carriedJournal{p.World.Journal}
	code := Execute(t.Context(), p)
	if code != exit.Refused || sc.closing() != model.Refused {
		t.Fatalf("exit %d, closing %q, want %d and refused", code, sc.closing(), exit.Refused)
	}
	if out := sc.output(); !strings.Contains(out, carriedFix+"gragas cache list") || strings.Contains(out, "Oldest closed") {
		t.Errorf("the run said:\n%s\nwant the fix and the line, and no table", out)
	}
}

// TestAnUnreadableCacheEndsFailedOnExit1: cache list over a folder that
// cannot be read ends on the failed word with exit 1 and says what failed;
// it does not list the folder as empty. Risk: a size of 0 trusted over a disk
// that said no.
func TestAnUnreadableCacheEndsFailedOnExit1(t *testing.T) {
	t.Parallel()
	cause := errors.New("the disk said no")
	p, _ := process(t, world(t), "cache", "list")
	p.Config = config.Config{StateDir: fakeState(), CacheDir: fakeCache(), Settings: config.Defaults()}
	p, sc := attach(t, p)
	sc.out.Instant(time.Unix(1_800_000_000, 0))
	sc.out.Seed(fakeState(), map[string]string{})
	sc.out.Seed(fakeCache(), map[string]string{})
	p.World.Tree = listFails{p.World.Tree, cause}
	code := Execute(t.Context(), p)
	if code != exit.Failed || sc.closing() != model.Failed {
		t.Fatalf("exit %d, closing %q, want %d and failed", code, sc.closing(), exit.Failed)
	}
	if out := sc.output(); !strings.Contains(out, "the disk said no") || strings.Contains(out, "journal ") {
		t.Errorf("the run said:\n%s\nwant the cause and no table", out)
	}
}
