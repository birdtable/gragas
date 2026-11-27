package porttest_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// readsAnyName is the fake tree with ReadSoleWithin reading as Read does, a file's
// names uncounted: the adapter host/fs once was. A planted
// adapter.
type readsAnyName struct{ port.Tree }

func (r readsAnyName) ReadSoleWithin(root model.Root, p string, _ int64) ([]byte, error) {
	return r.Read(root, p)
}

// TestAnAdapterThatReadsAFileByAnyOfItsNamesFailsTheNamesSuite: the plant
// reads a file with a second name as one with a sole name. The control is the
// fake, which counts the names.
func TestAnAdapterThatReadsAFileByAnyOfItsNamesFailsTheNamesSuite(t *testing.T) {
	t.Parallel()
	build := func(plant bool) contracttest.NamesMaker {
		return func(t *testing.T, files, names map[string]string) (port.Tree, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/tree", files)
			for second, first := range names {
				w.Name(root, first, second)
			}
			if plant {
				return readsAnyName{w.Ports().Tree}, "/work/tree"
			}
			return w.Ports().Tree, "/work/tree"
		}
	}
	if passes(t, "reads any name", wants()["reads any name"], func(t *testing.T) {
		t.Helper()
		contracttest.Names(t, build(true))
	}) {
		t.Error("an adapter that reads a file by any of its names passed the Names suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Names(t, build(false))
	}) {
		t.Error("the control, the fake that counts a file's names, failed the Names suite")
	}
}

// placesALink is the fake tree with Within placing a path whose last part is
// a link at the link's own name: the adapter host/fs once was.
// A planted adapter.
type placesALink struct{ port.Tree }

func (p placesALink) Within(root model.Root, abs string) (string, error) {
	rel, err := p.Tree.Within(root, abs)
	if errors.Is(err, port.ErrLinked) {
		return strings.TrimPrefix(abs, string(root)+"/"), nil
	}
	return rel, err
}

// TestAnAdapterThatPlacesALinkAtItsOwnNameFailsTheLinksSuite: the plant
// answers a path whose last part is a link as a file at the link's name. The
// control is the fake, which answers ErrLinked.
func TestAnAdapterThatPlacesALinkAtItsOwnNameFailsTheLinksSuite(t *testing.T) {
	t.Parallel()
	build := func(plant bool) contracttest.LinksMaker {
		return func(t *testing.T, files, links map[string]string) (port.Tree, port.Writer, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/tree", files)
			for p, target := range links {
				w.Link(root, p, target)
			}
			p := w.Ports()
			if plant {
				return placesALink{p.Tree}, p.Writer, "/work/tree"
			}
			return p.Tree, p.Writer, "/work/tree"
		}
	}
	if passes(t, "places a link", wants()["places a link"], func(t *testing.T) {
		t.Helper()
		contracttest.Links(t, build(true))
	}) {
		t.Error("an adapter that places a link at its own name passed the Links suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Links(t, build(false))
	}) {
		t.Error("the control, the fake that answers ErrLinked, failed the Links suite")
	}
}

// readsPastItsBound is the fake tree with ReadSoleWithin reading with no
// bound, the names still counted: an adapter that reads a file whole before it looks
// at its size. A planted adapter.
type readsPastItsBound struct{ port.Tree }

func (r readsPastItsBound) ReadSoleWithin(root model.Root, p string, _ int64) ([]byte, error) {
	return r.Tree.ReadSoleWithin(root, p, math.MaxInt64-1)
}

// TestAnAdapterThatReadsPastItsBoundFailsTheNamesSuite: the plant answers a
// file of max+1 bytes whole. The control is the fake, which refuses it.
func TestAnAdapterThatReadsPastItsBoundFailsTheNamesSuite(t *testing.T) {
	t.Parallel()
	build := func(plant bool) contracttest.NamesMaker {
		return func(t *testing.T, files, names map[string]string) (port.Tree, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/tree", files)
			for second, first := range names {
				w.Name(root, first, second)
			}
			if plant {
				return readsPastItsBound{w.Ports().Tree}, "/work/tree"
			}
			return w.Ports().Tree, "/work/tree"
		}
	}
	if passes(t, "reads past its bound", wants()["reads past its bound"], func(t *testing.T) {
		t.Helper()
		contracttest.Names(t, build(true))
	}) {
		t.Error("an adapter that reads a file past its bound passed the Names suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Names(t, build(false))
	}) {
		t.Error("the control, the fake that holds the bound, failed the Names suite")
	}
}
