package sandbox_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TestPrepareMakesNothingThroughANameItRefuses: every name of the cache folder
// that is there is judged
// before anything is made, so a refused Prepare leaves no folder wherever a
// name another account controls leads. Risk: a link in a folder others may
// write sends the making of the base into a folder of that account's
// choosing, leaving folders of this account's there. The probe is the cache
// folder open/c/gragas, open written by others and c a link to the victim,
// the cache folder itself a link, open/c2 to the second victim, and the cache
// folder open/new, not there yet, whose nearest folder that is there, open,
// is itself one others may write. The control is a cache folder that is not
// there yet, made and prepared. Planted: the base made before the walk (each
// victim then holds a folder, red here), and the walk started at the folder
// above top, s.walk(filepath.Dir(top)) (open then holds new, red in the
// third row: in the two link rows top is the link and the folder above it is
// open, refused anyway).
func TestPrepareMakesNothingThroughANameItRefuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		// dir is the cache folder, and empty the folder that must hold
		// nothing after the refusal.
		dir func(open, victim string) (dir, empty string)
	}{
		{"a cache folder below a link in a folder others may write", func(open, victim string) (string, string) {
			testenv.Link(t, victim, filepath.Join(open, "c"))
			return filepath.Join(open, "c", "gragas"), victim
		}},
		{"a cache folder that is a link in a folder others may write", func(open, victim string) (string, string) {
			testenv.Link(t, victim, filepath.Join(open, "c2"))
			return filepath.Join(open, "c2"), victim
		}},
		{"a cache folder not there yet whose nearest folder others may write", func(open, _ string) (string, string) {
			return filepath.Join(open, "new"), open
		}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			open, victim := filepath.Join(cache(t), "open"), cache(t)
			if err := os.Mkdir(open, 0o700); err != nil {
				t.Fatal(err)
			}
			dir, empty := tc.dir(open, victim)
			trusttest.Loosen(t, open, trusttest.OthersWrite)
			sc, err := sandbox.New(sandbox.Config{Dir: dir}).Prepare(t.Context(), model.Prepare{Level: model.Isolated})
			if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), dir+", the cache folder") || !strings.Contains(err.Error(), open) {
				t.Errorf("Prepare = %+v, %v; want refused by the cache folder's name and %s's", sc, err, open)
			}
			if left := entries(t, empty); len(left) != 0 {
				t.Errorf("%s holds %v after a refused Prepare, want nothing", empty, left)
			}
		})
	}
	t.Run("the control, a cache folder that is not there yet", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(cache(t), "new", "cache")
		s := sandbox.New(sandbox.Config{Dir: dir})
		sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
		if err != nil {
			t.Fatalf("Prepare in a cache folder not there yet = %v; want it made and prepared", err)
		}
		if err := s.Release(sc); err != nil {
			t.Errorf("Release = %v", err)
		}
	})
}

// TestSweepAndLeftAreRefusedInACacheFolderOthersMayWrite: Sweep and Left each walk the cache folder
// before they read the base, so one others may write refuses each by its
// name and the run folder stays. Risk: a later split of the base's walk from
// left counts, or removes, through a folder another account may rename. The
// control is Left before the folder is loosened, which counts the run
// folder. Planted: left reaching the base without the walk (each row
// counts the run folder, red here).
func TestSweepAndLeftAreRefusedInACacheFolderOthersMayWrite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		call func(*sandbox.Sandbox) (int, error)
	}{
		{"Sweep", (*sandbox.Sandbox).Sweep},
		{"Left", (*sandbox.Sandbox).Left},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			dir := cache(t)
			s := sandbox.New(sandbox.Config{Dir: dir})
			if _, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated}); err != nil {
				t.Fatal(err)
			}
			if tc.what == "Left" {
				if n, err := tc.call(s); err != nil || n != 1 {
					t.Fatalf("the control: Left = %d, %v; want the one run folder", n, err)
				}
			}
			trusttest.Loosen(t, dir, trusttest.OthersWrite)
			if n, err := tc.call(s); !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), dir+", the cache folder") {
				t.Errorf("%s in a cache folder others may write = %d, %v; want refused by its name", tc.what, n, err)
			}
			if left := runs(t, dir); len(left) != 1 {
				t.Errorf("the base holds %v after a refused %s, want the one run folder", left, tc.what)
			}
		})
	}
}

// entries is the names in dir, links not followed.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}
