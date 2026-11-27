package coreapi_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// zebraList is a planted names list whose third line does not compile. Its
// made-up word and the folder it sits in are what no line may say.
const (
	zebraList = "\\bacme\\b\n# a comment\n(ZEBRAMARKER\n"
	zebraFile = "/work/zebra-secret/list.txt"
)

// saysNothingOfTheList fails when line holds the bad line's pattern or any
// part of the list's path.
func saysNothingOfTheList(t *testing.T, what, line string) {
	t.Helper()
	for _, secret := range []string{"ZEBRAMARKER", "zebra-secret", "list.txt", "/work/", "regexp"} {
		if strings.Contains(line, secret) {
			t.Errorf("%s %q says %q", what, line, secret)
		}
	}
}

// TestABadLineOfTheNamesListEndsNotReadyWithNeitherItsPatternNorItsPath: a
// list whose third line does not compile ends the run not ready, through the
// engine's loader and the old engine's alike, and the line that reaches the
// ending, and --json with it, says the line's number and the closed reason,
// never the pattern regexp's error quotes and never the path. The
// error still is the typed BadNames at line 3. The control is the list with
// the line mended, which loads.
func TestABadLineOfTheNamesListEndsNotReadyWithNeitherItsPatternNorItsPath(t *testing.T) {
	t.Parallel()
	const want = "line 3 of the names file is not a pattern that compiles"
	loads := map[string]struct {
		load func(t *testing.T, list string) error
		is   error
	}{
		"the engine's loader": {func(t *testing.T, list string) error {
			t.Helper()
			w, root := namesBook(t)
			w.SeedOutside(zebraFile, list)
			_, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, zebraFile)
			return err
		}, coreapi.ErrNotReady},
		"the old engine's loader": {func(t *testing.T, list string) error {
			t.Helper()
			_, err := coreapi.Load(t.Context(), world(t, list).Ports(), ".", zebraFile)
			return err
		}, coreapi.ErrNames},
	}
	for name, l := range loads {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := l.load(t, strings.Replace(zebraList, "(ZEBRAMARKER", "ZEBRAMARKER", 1)); err != nil {
				t.Fatalf("the control: the mended list = %v", err)
			}
			err := l.load(t, zebraList)
			if !errors.Is(err, l.is) {
				t.Fatalf("err = %v; want %v", err, l.is)
			}
			if bad, ok := errors.AsType[*format.BadNames](err); !ok || bad.Line != 3 {
				t.Errorf("err = %#v; want BadNames at line 3", err)
			}
			if got := coreapi.Reason(err); got != want {
				t.Errorf("Reason = %q, want %q", got, want)
			}
			saysNothingOfTheList(t, "Reason", coreapi.Reason(err))
			saysNothingOfTheList(t, "the error", err.Error())
		})
	}
}

// TestARefusedNamesReadSaysWhyWithoutThePath: a read of the list that is
// refused, with an error whose words carry the path, ends on a line that says
// why from the closed set and holds no part of the path, through Reason and
// through the error itself, which a command that does not ask Reason prints.
// The cases are the owned read's refusal, a read the Tree refuses for another
// reason, and the old engine's open that fails. The owned read's refusal is
// said from the closed set whatever its own words hold: they carry the list's
// path, and a folder above it or a mount GRAGAS_MOUNTS names, each made up
// here under zebra-secret. A list that is a link is said by the link's own
// closed words and fix. Planted: namesRefused saying the refusal's why with
// the path replaced (the mount row red); the link's reason not asked (the link
// row red).
func TestARefusedNamesReadSaysWhyWithoutThePath(t *testing.T) {
	t.Parallel()
	const refusedWhy = "is owned by another account, or another account can write it or a folder above it, or a mount it is read through is refused, or it is a link, a folder or no regular file"
	tests := map[string]struct {
		load func(t *testing.T) error
		is   error
		says string
	}{
		"another account can write the list": {func(t *testing.T) error {
			t.Helper()
			w, root := namesBook(t)
			w.SeedRefused(zebraFile, porttest.Refused{Why: "is writable by its group", Fix: "chmod g-w " + zebraFile})
			_, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, zebraFile)
			return err
		}, port.ErrNotOwned, refusedWhy},
		"a mount the list is read through is refused, in words that name it": {func(t *testing.T) error {
			t.Helper()
			w, root := namesBook(t)
			w.SeedRefused(zebraFile, porttest.Refused{
				Why: "is read with the mounts GRAGAS_MOUNTS names, and the mount /work/zebra-secret/home names nothing",
				Fix: "take /work/zebra-secret/home out of GRAGAS_MOUNTS",
			})
			_, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, zebraFile)
			return err
		}, port.ErrNotOwned, refusedWhy},
		"the old engine's open is refused, in words that name a folder above the list": {func(t *testing.T) error {
			t.Helper()
			f := world(t, "")
			f.NamesErr = refusing{why: "has a folder on its way, /work/zebra-secret, that its group can write", fix: "chmod g-w /work/zebra-secret"}
			_, err := coreapi.Load(t.Context(), f.Ports(), ".", zebraFile)
			return err
		}, port.ErrNotOwned, refusedWhy},
		"the list is a link, in words that name where it leads": {func(t *testing.T) error {
			t.Helper()
			w, root := namesBook(t)
			w.SeedRefused(zebraFile, porttest.Refused{
				Why:    "is a link to /work/zebra-secret/real/list.txt, and none is followed",
				Fix:    "replace the link with the file itself",
				Reason: port.ErrOwnedLink,
			})
			_, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, zebraFile)
			return err
		}, port.ErrOwnedLink, "is a link, and none is followed; fix: put the names file itself at the path, not a link to it"},
		"the Tree refuses the path and quotes it": {func(t *testing.T) error {
			t.Helper()
			w, root := namesBook(t)
			_, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "work/zebra-secret/list.txt")
			return err
		}, nil, "the names file could not be read"},
		"the old engine cannot open the list": {func(t *testing.T) error {
			t.Helper()
			f := world(t, "")
			f.NamesErr = &fs.PathError{Op: "open", Path: zebraFile, Err: fs.ErrPermission}
			_, err := coreapi.Load(t.Context(), f.Ports(), ".", zebraFile)
			return err
		}, fs.ErrPermission, "the names file could not be read"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := tc.load(t)
			if err == nil || (tc.is != nil && !errors.Is(err, tc.is)) {
				t.Fatalf("err = %v; want one that is %v", err, tc.is)
			}
			if got := coreapi.Reason(err); !strings.Contains(got, tc.says) {
				t.Errorf("Reason = %q; want it to say %q", got, tc.says)
			}
			saysNothingOfTheList(t, "Reason", coreapi.Reason(err))
			saysNothingOfTheList(t, "the error", err.Error())
		})
	}
}

// refusing is a refusal of the owned read, as the host adapter's is: it
// matches port.ErrNotOwned, and its words and its Refusal name the list's
// folder.
type refusing struct{ why, fix string }

func (r refusing) Error() string              { return zebraFile + " " + r.why }
func (r refusing) Is(target error) bool       { return target == port.ErrNotOwned }
func (r refusing) Refusal() (why, fix string) { return r.why, r.fix }
