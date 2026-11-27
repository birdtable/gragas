package coreapi_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/rulebook"
	"github.com/birdtable/gragas/internal/port/legacy"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// world is a repository holding only the parts table, with names as the
// names file. Inputs read: testdata/valid/rulebook/parts.yaml.
func world(t *testing.T, names string) *porttest.Fake {
	t.Helper()
	parts, err := os.ReadFile("testdata/valid/rulebook/parts.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return &porttest.Fake{Repo: fstest.MapFS{"rulebook/parts.yaml": &fstest.MapFile{Data: parts}}, Names: names}
}

func TestABadNamesFileIsErrNames(t *testing.T) {
	t.Parallel()
	_, err := coreapi.Load(t.Context(), world(t, "(unclosed\n").Ports(), ".", "names")
	if !errors.Is(err, coreapi.ErrNames) {
		t.Errorf("Load = %v, want ErrNames", err)
	}
}

func TestAFolderThatCannotBeOpenedIsNoRulebook(t *testing.T) {
	t.Parallel()
	f := world(t, "")
	f.RepoErr = errors.New("no such folder")
	if _, err := coreapi.Load(t.Context(), f.Ports(), "nowhere", ""); !errors.Is(err, rulebook.ErrNoParts) {
		t.Errorf("Load = %v, want ErrNoParts", err)
	}
}

func TestApplyWritesInOrderAndStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	outs := []coreapi.Output{{Path: "a"}, {Path: "b"}}
	f := world(t, "")
	var told []string
	if err := coreapi.Apply(t.Context(), f.Ports().Writer, ".", outs, func(p string) { told = append(told, p) }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.Wrote, []string{"a", "b"}) || !slices.Equal(told, f.Wrote) {
		t.Errorf("wrote %v, told %v", f.Wrote, told)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f = world(t, "")
	if err := coreapi.Apply(ctx, f.Ports().Writer, ".", outs, func(string) {}); !errors.Is(err, context.Canceled) || len(f.Wrote) != 0 {
		t.Errorf("a cancelled Apply: %v, wrote %v", err, f.Wrote)
	}
}

func TestCountsLeaveOutAnEmptyPart(t *testing.T) {
	t.Parallel()
	l, err := coreapi.Load(t.Context(), world(t, "acme\n").Ports(), ".", "names")
	if err != nil {
		t.Fatal(err)
	}
	if got := coreapi.Counts(l); len(got) != 0 {
		t.Errorf("Counts of a book with no rules = %v, want none", got)
	}
}

// counted is a source that counts its closes.
type counted struct {
	legacy.Source
	closed *int
}

func (c counted) Close() error {
	*c.closed++
	return nil
}

// closing is a Files whose every repository is a counted source.
type closing struct {
	legacy.Files
	closed *int
}

func (c closing) Repo(dir string) (legacy.Source, error) {
	src, err := c.Files.Repo(dir)
	if err != nil {
		return nil, err
	}
	return counted{src, c.closed}, nil
}

// TestALoadedHoldsTheRepositoryUntilItIsClosed: Load holds the repository it
// read open, and Close lets go of it once; a Load that ends on an error holds
// nothing, since it closes what it opened (Windows removes no folder held open). Planted
// failure: Load returning its error without closing the source.
func TestALoadedHoldsTheRepositoryUntilItIsClosed(t *testing.T) {
	t.Parallel()
	var closed int
	ports := world(t, "").Ports()
	ports.Files = closing{ports.Files, &closed}
	l, err := coreapi.Load(t.Context(), ports, ".", "")
	if err != nil {
		t.Fatal(err)
	}
	if closed != 0 {
		t.Fatalf("Load closed the repository it answers %d times; want it held", closed)
	}
	if err := l.Close(); err != nil || closed != 1 {
		t.Errorf("Close = %v, closed %d times; want nil, once", err, closed)
	}
	closed = 0
	f := world(t, "")
	f.Repo = fstest.MapFS{}
	ports = f.Ports()
	ports.Files = closing{ports.Files, &closed}
	if _, err := coreapi.Load(t.Context(), ports, ".", ""); !errors.Is(err, coreapi.ErrNoParts) || closed != 1 {
		t.Errorf("Load of no rulebook = %v, closed %d times; want ErrNoParts, closed once", err, closed)
	}
	if err := (coreapi.Loaded{}).Close(); err != nil {
		t.Errorf("Close of the zero Loaded = %v, want nil", err)
	}
}
