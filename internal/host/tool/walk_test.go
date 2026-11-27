package tool_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TestATrustWalkAnswerIsTheAdaptersOwnWithItsFix: what the trust walk
// answers, planted, is the adapter's: a name it does not trust is refused
// with the walk's own words and fix, or the line that points the setting at
// the program where the walk has none; a name it cannot read refuses the
// tool with the line that points it at a folder this account can read; a
// name that is not there is the walk's error, no refusal, so the next place
// is tried. Which names the walk trusts, a folder its group can write or
// one any account can add a name to among them, is go-files' to hold
// (trust.Path), and its own tests hold it on each system.
// Risk: a refusal of the walk reaches a person with no fix, or a missing
// tool is refused and the next place in the locate order is never tried.
func TestATrustWalkAnswerIsTheAdaptersOwnWithItsFix(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		err       error
		says, fix string
	}{
		"a name it does not trust, with its fix": {&hosttrust.Unsafe{Observed: "the tool at /p/lint-a can be written by its group", Fix: "chmod go-w /p/lint-a"},
			"the tool at /p/lint-a can be written by its group", "chmod go-w /p/lint-a"},
		"a name it does not trust, with none": {&hosttrust.Unsafe{Observed: "the tool at /work/lint-a is right in /work"},
			"the tool at /work/lint-a is right in /work", "point the setting at the program itself"},
		"a name it cannot read": {&hosttrust.Failed{What: "the path of the tool could not be read", Err: fs.ErrPermission},
			"the path of the tool could not be read: ", "point the setting at a program in a folder this account can read"},
	} {
		err := tool.WalkRefusal(fmt.Errorf("walk: %w", c.err))
		var r *tool.Refused
		if !errors.As(err, &r) || !errors.Is(err, port.ErrRefused) || !strings.HasPrefix(r.Observed(), c.says) || r.Hint() != c.fix {
			t.Errorf("%s: %v, want refused saying %q with the fix %q", name, err, c.says, c.fix)
		}
	}
	if err := tool.WalkRefusal(fmt.Errorf("walk: %w", fs.ErrNotExist)); !errors.Is(err, fs.ErrNotExist) || errors.As(err, new(*tool.Refused)) {
		t.Errorf("a name that is not there: %v, want the walk's own error, no refusal", err)
	}
	// The walk itself, on a folder of this account's: a tool there passes and
	// answers its own path, and one that is not there is no refusal.
	d := filepath.FromSlash(private(t))
	at := program(t, filepath.Join(d, "bin", "lint-a"), echo)
	// The walk takes the system's own form and answers the slash form.
	if got, err := tool.Trusted(filepath.ToSlash(at)); err != nil || got != filepath.ToSlash(at) {
		t.Errorf("a private tool: %q, %v; want %q trusted", got, err, filepath.ToSlash(at))
	}
	if _, err := tool.Trusted(filepath.Join(d, "bin", "missing")); !errors.Is(err, fs.ErrNotExist) || errors.As(err, new(*tool.Refused)) {
		t.Errorf("a missing tool: %v, want fs.ErrNotExist and no refusal", err)
	}
}

// TestAFolderTheWalkCannotCompareSaysWhy: a folder whose way
// the walk stops on, while it compares it with the root, is refused in words
// that say which: a name it does not trust, or one whose writers it cannot
// judge, as on a 9p share or a volume that ignores ownership; the walk's own
// fix comes first, and each has a line of its own where it gives none.
// Risk: a person on a share the walk cannot judge is told a folder there is
// not trusted, and looks for an account that can write it.
func TestAFolderTheWalkCannotCompareSaysWhy(t *testing.T) {
	t.Parallel()
	const what, root = "the folder '/s/tmp' a version run is made in", model.Root("/p")
	for name, c := range map[string]struct {
		u         hosttrust.Unsafe
		says, fix string
	}{
		"a name it does not trust": {hosttrust.Unsafe{Observed: "/s can be written by its group"},
			"as a folder on its way is not trusted: /s can be written by its group", "use folders that root or this account alone can write, with no link on their way"},
		"a name it cannot judge": {hosttrust.Unsafe{Observed: "/s is on a file system of type 0x1021997", Unjudged: true},
			"as the walk cannot judge who may write a folder on its way: /s is on a file system of type 0x1021997", "use folders on a disk whose owners this system keeps and reads"},
		"one with the walk's fix": {hosttrust.Unsafe{Observed: "/s can be written by its group", Fix: "chmod g-w /s"},
			"as a folder on its way is not trusted", "chmod g-w /s"},
	} {
		err := tool.WayRefused(what, root, &c.u)
		r, ok := errors.AsType[*tool.Refused](err)
		if !ok || !errors.Is(err, port.ErrRefused) || !strings.HasPrefix(r.Observed(), what+" cannot be compared with the root '/p', ") || !strings.Contains(r.Observed(), c.says) || r.Hint() != c.fix {
			t.Errorf("%s: %v; want refused saying %q with the fix %q", name, err, c.says, c.fix)
		}
	}
}

// TestAHomeIsRefusedOverItsOwnFolders: a Home with a control character, one
// that is relative or not clean here, the root of its volume, one beside a
// project folder or a call folder that cannot be read, or one inside the
// project, is refused in words that say which; a private folder is
// accepted. A folder of root's any account can write, a link on the way,
// and the project by another spelling are the trust walk's, kept apart from
// the project by go-files' Who.Apart (TestAHomeIsATrustedFolderOutside
// TheProject, the tests of a Home in another spelling, and go-files' own
// tests).
// Risk: a Home the caller did not make, or the root of a drive, holds the
// caches a tool reads, and another account plants what it reads there.
func TestAHomeIsRefusedOverItsOwnFolders(t *testing.T) {
	t.Parallel()
	d := filepath.FromSlash(private(t))
	h, p, gone := filepath.Join(d, "h"), filepath.Join(d, "p"), filepath.Join(d, "gone")
	for _, dir := range []string{h, p, filepath.Join(p, "h")} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	top := d
	for up := filepath.Dir(top); up != top; up = filepath.Dir(top) {
		top = up
	}
	for name, c := range map[string][4]string{
		"a control character":               {h + "\n", p, p, "control character"},
		"a relative folder":                 {"h", p, p, "absolute, clean folder"},
		"a folder that is not clean":        {h + "/.", p, p, "absolute, clean folder"},
		"the root of its volume":            {top, p, p, "absolute, clean folder"},
		"a root that cannot be read":        {h, gone, p, "the project folder " + hosttrust.Quote(gone) + " cannot be checked"},
		"a call folder that cannot be read": {h, p, gone, "the project folder " + hosttrust.Quote(gone) + " cannot be checked"},
		"inside the project":                {filepath.Join(p, "h"), p, p, "is, or is inside, the project folder " + hosttrust.Quote(p)},
	} {
		err := tool.HomeOK(c[0], filepath.ToSlash(c[1]), filepath.ToSlash(c[2]))
		if r, ok := errors.AsType[*tool.Refused](err); !ok || !strings.Contains(r.Observed(), c[3]) {
			t.Errorf("%s: %v; want a refusal saying %q", name, err, c[3])
		}
	}
	if err := tool.HomeOK(h, filepath.ToSlash(p), filepath.ToSlash(p)); err != nil {
		t.Errorf("a private folder: %v; want it accepted", err)
	}
}
