package tool

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-proc/proc"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// makeHome makes each folder proc.HomeEnv points a variable at below the Home,
// which makes none: the run's home, config, cache, state and temp folders. A
// value that is no folder of the Home, as a drive name is, is left alone.
func makeHome(h string) (err error) {
	if h == "" {
		return nil
	}
	home := filepath.FromSlash(h)
	root, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	for _, kv := range proc.HomeEnv(home) {
		_, v, _ := strings.Cut(kv, "=")
		rel, err := filepath.Rel(home, v)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		// Each folder is made in the handle of the one above it, and a link at any of them is
		// refused, so no run's HOME is a folder a link leads to.
		made, err := confine.Mkdir(root, filepath.ToSlash(rel), 0o700)
		if err != nil {
			return err
		}
		if err := made.Close(); err != nil {
			return err
		}
	}
	return nil
}

// runHome makes the Home of one run, a step's or a version probe's, kept apart
// from the folders of apart (each in the model's slash form), and answers the
// call that ends it; what names the run in a report. The Homes an earlier run
// left, because a child of its program was still writing in one when it was
// removed, are swept first: those no live run can hold, older than twice the
// longest a step waits, which no probe outlives. The tool's answer stands over
// the Home's removal: one that cannot be removed is reported and left for the
// next sweep, and the call that ends the run answers nothing. A
// step and a probe make and remove their Homes here alone.
func (t *Tool) runHome(what string, apart ...string) (home string, end func(), err error) {
	kept := make([]string, len(apart))
	for i, a := range apart {
		kept[i] = filepath.FromSlash(a)
	}
	if _, serr := hosttrust.SweepHomes(kept, 2*max(t.wait, Deadline)); serr != nil {
		t.report(fmt.Sprintf("the Homes of earlier runs could not be swept: %v", serr))
	}
	home, remove, err := hosttrust.RunHome(kept)
	if err != nil {
		return "", nil, err
	}
	return home, func() {
		if rerr := remove(); rerr != nil {
			t.report(fmt.Sprintf("the %s's Home could not be removed, and is left for a later sweep: %v", what, rerr))
		}
	}, nil
}

// homeOK refuses a Home that a run may not be given, before the
// run starts: the run writes there, and the programs it starts read files
// below HOME that decide what they run.
//
// The root and the folder are held in the slash form of the model and the
// check reads the system's own (filepath.IsAbs, Clean, the separator), so
// all three are turned to it here.
func (t *Tool) homeOK(h, root, dir string) error {
	return home(filepath.FromSlash(h), []string{filepath.FromSlash(root), filepath.FromSlash(dir)}, t.cfg.Mounts)
}

// home is the check of the Home h against apart, the project root and the
// folder of the call, where a head can commit a file the tools read: each is
// an absolute, clean, slash-separated path, as the call and the find hold
// it, or "" for none. h is refused when it:
//   - holds a control character, or is not an absolute, clean folder other
//     than the root of its volume;
//   - fails the trust walk as a folder with no link anywhere on its way and
//     kept apart from the folders of apart (go-files trust.Path,
//     Who{NoLinks, Apart}): every name from the top of its volume down is
//     root's or this account's and no other account can write it, the home
//     included, so root's /tmp is no home; and none is a folder of apart by
//     its file ID, so a link, a case or a normalization the volume ignores
//     that names the project another way names it all the same.
//
// A name that cannot be read refuses the home, and so does a folder of apart
// that cannot be: nothing is run unchecked.
func home(h string, apart, mounts []string) error {
	switch {
	case strings.ContainsFunc(h, unicode.IsControl):
		return refused("the home holds a control character", "give the home a folder the caller made")
	case !filepath.IsAbs(h) || filepath.Clean(h) != h || filepath.Dir(h) == h:
		return refused("the home "+hosttrust.Quote(h)+" is not an absolute, clean folder below the root", "give the home a folder the caller made")
	}
	var folders []string
	for _, a := range apart {
		if a != "" {
			folders = append(folders, filepath.FromSlash(a))
		}
	}
	s := hosttrust.Subject{Name: "the home " + hosttrust.Quote(h), File: "the home", Last: hosttrust.Folder, Bare: true}
	_, err := hosttrust.Walk(s, h, hosttrust.Who{NoLinks: true, Apart: folders, Mounts: mounts})
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		if u.Inside != "" {
			return refused("the home "+hosttrust.Quote(h)+" is, or is inside, the project folder "+hosttrust.Quote(u.Inside), "give the home a folder outside the project")
		}
		fix := u.Fix
		if fix == "" {
			fix = "give the home a folder this account made, with no link on its way"
		}
		return refused("the home "+hosttrust.Quote(h)+": "+u.Observed, fix)
	}
	if pe, ok := errors.AsType[*fs.PathError](err); ok && slices.Contains(folders, pe.Path) {
		return refused("the project folder "+hosttrust.Quote(pe.Path)+" cannot be checked: "+pe.Error(), "run in a folder that exists")
	}
	if err != nil {
		return refused("the home "+hosttrust.Quote(h)+" cannot be checked: "+err.Error(), "make the home first, a folder of this account's")
	}
	return nil
}
