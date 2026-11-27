package coreapi_test

import (
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// namesBook is a world whose rule holds the stand-in name "alex", so a list
// that is read finds it, and its root.
func namesBook(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	rules := strings.Replace(bookRules, "Wrap an error.", "Ask alex before you wrap an error.", 1)
	return w, w.Seed("repo", map[string]string{"rulebook.yaml": bookIdentity, "structure.yaml": bookStructure, "language/go/rules/errors.yaml": rules})
}

// TestANamesFileAnotherAccountCanWriteIsRefused: a list the owned read
// refuses ends the run not ready, never a notice and never a clean answer,
// and its line says why and the fix and names the settings key and the
// variable, never the path, since the line reaches --json. The owned read's
// fix names the path, as go-files' does, and a path that quoting changes in
// it too; the line's why and fix are the closed set's, so neither is left in
// it. The control is the same list read, which finds the name.
func TestANamesFileAnotherAccountCanWriteIsRefused(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"/work/private/list.txt", "/work/it's/list.txt"} {
		w, root := namesBook(t)
		w.SeedOutside(file, "\\balex\\b\n")
		r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, file)
		if err != nil || len(r.Findings) != 1 {
			t.Fatalf("the control: %q read finds %+v, %v; want the one name", file, r.Findings, err)
		}
		w.SeedRefused(file, porttest.Refused{Why: "is writable by its group", Fix: "chmod g-w '" + strings.ReplaceAll(file, "'", `'\''`) + "'"})
		r, err = coreapi.LoadBookNamed(t.Context(), w.Ports(), root, file)
		if !errors.Is(err, coreapi.ErrNotReady) || !errors.Is(err, port.ErrNotOwned) {
			t.Fatalf("%q refused: report %+v, %v; want ErrNotReady and port.ErrNotOwned", file, r, err)
		}
		if len(r.Findings) != 0 || len(r.Notices) != 0 {
			t.Errorf("%q refused: report %+v, want none beside the error", file, r)
		}
		line := err.Error()
		for _, want := range []string{"GRAGAS_NAMES_FILE", "names-file key", "another account can write it", "fix: "} {
			if !strings.Contains(line, want) {
				t.Errorf("%q refused: %q does not say %q", file, line, want)
			}
		}
		if strings.Contains(line, "/work/") || strings.Contains(line, "list.txt") {
			t.Errorf("%q refused: %q names the path", file, line)
		}
	}
}

// TestAnEmptyNamesFileIsSaid: a list with no name in it, empty or comments
// only, is a notice that the check did not run because the list holds no
// name, told apart from a list that is not there. The control is a list
// with one name, which is no notice.
func TestAnEmptyNamesFileIsSaid(t *testing.T) {
	t.Parallel()
	w, root := namesBook(t)
	w.SeedOutside("/work/private/empty.txt", "")
	w.SeedOutside("/work/private/comments.txt", "# a comment\n\n")
	w.SeedOutside("/work/private/one.txt", "\\balex\\b\n")
	for _, file := range []string{"/work/private/empty.txt", "/work/private/comments.txt"} {
		r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, file)
		if err != nil || len(r.Notices) != 1 || !strings.Contains(r.Notices[0].Msg, "names check did not run") || !strings.Contains(r.Notices[0].Msg, "holds no name") {
			t.Errorf("%q: notices = %+v, %v; want the one that says the list holds no name", file, r.Notices, err)
		}
	}
	r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "/work/private/missing.txt")
	if err != nil || len(r.Notices) != 1 || !strings.Contains(r.Notices[0].Msg, "does not exist") {
		t.Errorf("a list not there: notices = %+v, %v; want the one that says so", r.Notices, err)
	}
	if r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "/work/private/one.txt"); err != nil || len(r.Notices) != 0 {
		t.Errorf("the control: notices = %+v, %v; want none", r.Notices, err)
	}
}

// ownedRefusal is a refusal of the owned read as the legacy files adapter
// answers it: it matches port.ErrNotOwned and its message names the path.
type ownedRefusal struct{ path string }

func (e ownedRefusal) Error() string {
	return "open names file: " + e.path + " is writable by its group"
}
func (ownedRefusal) Is(target error) bool { return target == port.ErrNotOwned }
func (e ownedRefusal) Refusal() (why, fix string) {
	return "is writable by its group", "chmod g-w '" + e.path + "'"
}

// TestTheOldEnginesNamesFileAnotherAccountCanWriteIsRefused: the old
// engine's read of the list through legacy Files refuses a list another
// account can write as ErrNames, and its line names the variable and the
// key, never the path. The control is the same list read.
func TestTheOldEnginesNamesFileAnotherAccountCanWriteIsRefused(t *testing.T) {
	t.Parallel()
	const file = "/work/private/list.txt"
	if _, err := coreapi.Load(t.Context(), world(t, "\\balex\\b\n").Ports(), ".", file); err != nil {
		t.Fatalf("the control: Load = %v", err)
	}
	f := world(t, "\\balex\\b\n")
	f.NamesErr = ownedRefusal{path: file}
	_, err := coreapi.Load(t.Context(), f.Ports(), ".", file)
	if !errors.Is(err, coreapi.ErrNames) || !errors.Is(err, port.ErrNotOwned) {
		t.Fatalf("Load = %v; want ErrNames and port.ErrNotOwned", err)
	}
	if line := err.Error(); !strings.Contains(line, "GRAGAS_NAMES_FILE") || !strings.Contains(line, "another account can write it") || strings.Contains(line, "/work/") || strings.Contains(line, "list.txt") {
		t.Errorf("Load = %q; want why and the fix by the variable, and no path", line)
	}
}

// reasoned is a refusal of the owned read for one reason, as the host
// adapter's is: it matches port.ErrNotOwned and the port's error of its
// reason, and its words and its Refusal name the list's path.
type reasoned struct{ is error }

func (r reasoned) Error() string { return "open names file: /work/zebra-secret/list.txt is refused" }
func (r reasoned) Is(target error) bool {
	return target == port.ErrNotOwned || target == r.is
}
func (r reasoned) Refusal() (why, fix string) {
	return "is refused", "chmod 600 /work/zebra-secret/list.txt"
}

// TestEachNamesRefusalSaysItsOwnFix: the names file's refusal
// says the why and the fix of its own reason, told by the port's error
// alone, in the new engine and the old, and no other reason's fix: a list over
// 1 MiB, a folder or a pipe at the name is not told to be given to this
// account. No line holds any part of the path. Planted: refusedFor answering
// the closed words of every reason whatever the error (every row red here).
func TestEachNamesRefusalSaysItsOwnFix(t *testing.T) {
	t.Parallel()
	const give = "give the names file to this account"
	for _, c := range []struct {
		is       error
		why, fix string
	}{
		{port.ErrOwnedByOther, "is owned by another account, not by this account or root", give},
		{port.ErrOwnedWritable, "can be written by another account", "let no other account write the names file"},
		{port.ErrOwnedLink, "is a link, and none is followed", "put the names file itself at the path, not a link to it"},
		{port.ErrOwnedNotRegular, "is a folder, a pipe or a device, not a regular file", "put the names file, a regular file, at the path"},
		{port.ErrOwnedTooLong, "holds more than 1 MiB, the most it may hold", "keep the names file to 1 MiB or less"},
		{port.ErrOwnedMount, "is on a disk whose owners are not judged", "set GRAGAS_MOUNTS to the folder it is mounted on"},
		{port.ErrOwnedMountEntry, "is read through a GRAGAS_MOUNTS entry that is refused", "let each GRAGAS_MOUNTS entry name the folder a disk is mounted on itself"},
	} {
		w, root := namesBook(t)
		w.SeedRefused(zebraFile, porttest.Refused{Why: "is refused at " + zebraFile, Fix: "chmod 600 " + zebraFile, Reason: c.is})
		_, newErr := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, zebraFile)
		f := world(t, "")
		f.NamesErr = reasoned{is: c.is}
		_, oldErr := coreapi.Load(t.Context(), f.Ports(), ".", zebraFile)
		for _, got := range []struct {
			engine string
			err    error
		}{{"the new engine", newErr}, {"the old engine", oldErr}} {
			if !errors.Is(got.err, port.ErrNotOwned) || !errors.Is(got.err, c.is) {
				t.Errorf("%v, %s: %v; want port.ErrNotOwned and its reason", c.is, got.engine, got.err)
				continue
			}
			line := coreapi.Reason(got.err)
			if !strings.Contains(line, c.why) || !strings.Contains(line, "fix: ") || !strings.Contains(line, c.fix) {
				t.Errorf("%v, %s: %q; want why %q and fix %q", c.is, got.engine, line, c.why, c.fix)
			}
			if c.fix != give && strings.Contains(line, give) {
				t.Errorf("%v, %s: %q tells to give the file to this account, which fixes nothing here", c.is, got.engine, line)
			}
			saysNothingOfTheList(t, got.engine, line)
			saysNothingOfTheList(t, got.engine, got.err.Error())
		}
	}
}

// TestTheOldEngineSaysAnEmptyNamesList: the old engine reads a list with no name in it, empty or comments only, with the new
// engine's notice that the check did not run, so gragas check never passes
// it in silence. The controls are a list with one name and no list named,
// neither of which has the notice. Planted: Load leaving Notice empty (the
// two empty rows red).
func TestTheOldEngineSaysAnEmptyNamesList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, list, file string
		notice           bool
	}{
		{"an empty list", "", zebraFile, true},
		{"comments only", "# a comment\n\n", zebraFile, true},
		{"the control, one name", "\\balex\\b\n", zebraFile, false},
		{"the control, no list named", "", "", false},
	} {
		l, err := coreapi.Load(t.Context(), world(t, c.list).Ports(), ".", c.file)
		if err != nil {
			t.Fatalf("%s: Load = %v", c.name, err)
		}
		switch said := l.Notice; {
		case c.notice && (!strings.Contains(said, "names check did not run") || !strings.Contains(said, "holds no name")):
			t.Errorf("%s: Notice = %q; want the one that says the list holds no name", c.name, said)
		case !c.notice && said != "":
			t.Errorf("%s: Notice = %q; want none", c.name, said)
		}
		saysNothingOfTheList(t, c.name, l.Notice)
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	}
}
