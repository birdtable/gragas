package fs

// The one judgement of where a state folder may lie: the
// approvals, the journal and both locks call it before they read, make or
// take anything in their folder, so a folder a clone carries is refused by
// each the same way, matching port.ErrCarried.

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/go-files/confine"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
)

// carried is the one comparison of a state folder with the folders a clone
// carries: dir is refused, matching port.ErrCarried, when it lies in any of
// roots, or is one. Both are compared as the Tree names a root, resolved by
// go-files' confine.Resolve with every link followed, then by name, the
// resolved dir a local path from the resolved root (filepath.Rel and
// IsLocal), and then by confine.Below, which compares folders by file
// identity, so a link on the way, a second spelling of a root, or another
// case on a disk that folds case does not pass. Below answers a refusal of
// its own resolve as ErrOutside too, so dir and each root are resolved here
// first, and a name that cannot be resolved is refused, never taken to be
// clear. It is called only through apartOf.
//
// Of dir, only the nearest folder that is there is resolved, and the names
// below it, which are not there and so are no links, are kept as written. A
// missing tail is the one name another run may be making while this resolves
// it, which confine.Resolve refuses as a changed link; nothing is made first,
// as a state folder a clone
// carries must not be made at all. A root is resolved the same way: a cache
// folder no run has made yet is held apart by the name it will have, so a
// state folder in its place is refused before it, or the cache, is made.
// Below compares only a root that is there: nothing that is
// there lies below one that is not. A root that is not there is compared by
// inMissingRoot, as its folder will compare the names once they are made.
func carried(dir string, roots []string) error {
	real, there, err := resolvedName(dir)
	if err != nil {
		return fmt.Errorf("the state folder %s could not be resolved: %w", dir, err)
	}
	for _, root := range roots {
		at, atThere, err := resolvedName(root)
		if err != nil {
			return fmt.Errorf("the folder %s a clone carries could not be resolved: %w", root, err)
		}
		if rel, err := filepath.Rel(at, real); err == nil && filepath.IsLocal(rel) {
			return carriedIn(dir, root)
		}
		if at != atThere {
			in, err := inMissingRoot(atThere, at, there, real)
			if err != nil {
				return fmt.Errorf("the state folder %s could not be held against %s: %w", dir, root, err)
			}
			if in {
				return carriedIn(dir, root)
			}
			continue
		}
		_, err = confine.Below(at, there)
		switch {
		case err == nil:
			return carriedIn(dir, root)
		case !errors.Is(err, confine.ErrOutside):
			return fmt.Errorf("the state folder %s could not be held against %s: %w", dir, root, err)
		}
	}
	return nil
}

// resolvedName is p as carried compares it: the nearest folder of p that is
// there, resolved by confine.Resolve through every link, with the names below
// it, which are not there, kept as written. there is that resolved folder, so
// real == there when p is there itself.
func resolvedName(p string) (real, there string, err error) {
	top, err := hosttrust.Nearest(p)
	if err != nil {
		return "", "", err
	}
	tail, err := filepath.Rel(top, p)
	if err != nil {
		return "", "", err
	}
	there, err = confine.Resolve(top)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(there, tail), there, nil
}

// inMissingRoot answers whether the state folder real, whose nearest folder
// that is there is there, lies in a root that is not there: at, whose nearest
// folder that is there is rootThere. The folders that are there are compared
// by file identity (confine.Below), so another spelling of them passes no
// state folder. Each name below rootThere, which no folder holds yet, is
// compared as rootThere will compare it once it is made, by sameName on
// go-files' confine.Folds of rootThere, which asks the system and writes
// nothing, so a read-only folder is judged as a writable one and two
// judgements at once never meet. A case or composition variant of the cache
// is so refused where the folder may fold (APFS as macOS formats it, NTFS)
// and taken where it does not (ext4), from the first run, before the cache is
// made. Folds that cannot ask answers FoldUnknown, which is compared as a
// folder that folds. The names below the first missing one are compared by
// rootThere's answer too: a folder made in it takes its rule.
func inMissingRoot(rootThere, at, there, real string) (bool, error) {
	rel, err := confine.Below(rootThere, there)
	switch {
	case errors.Is(err, confine.ErrOutside):
		return false, nil
	case err != nil:
		return false, err
	}
	tail, err := filepath.Rel(there, real)
	if err != nil {
		return false, err
	}
	rootTail, err := filepath.Rel(rootThere, at)
	if err != nil {
		return false, err
	}
	names, want := namesOf(filepath.Join(rel, tail)), namesOf(rootTail)
	if len(names) < len(want) {
		return false, nil
	}
	folding, err := confine.Folds(rootThere)
	if err != nil {
		// Folds answers FoldUnknown with its error: a folder it cannot ask
		// is held to fold, as FoldUnknown's bits say.
		folding = confine.FoldUnknown
	}
	for i, name := range want {
		same, err := sameName(folding, names[i], name)
		if err != nil || !same {
			return false, err
		}
	}
	return true, nil
}

// sameName is whether a folder whose rule is folding reads a and b as one
// name. Two names whose bytes are equal are one. On a folder that folds
// nothing, no other two are. On any other, FoldUnknown among them, two names
// are one when their go-files confine.Portable keys are equal. The key folds
// both case and composition, so on a folder that folds case alone, as NTFS
// does, a few spellings it keeps apart are held as one: the compare errs
// toward refusing. A name Portable refuses cannot be compared so, and is an
// error that names the reason, never taken to be another name.
func sameName(folding confine.Folding, a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	if folding == confine.FoldNone {
		return false, nil
	}
	ka, err := portableKey(a)
	if err != nil {
		return false, err
	}
	kb, err := portableKey(b)
	if err != nil {
		return false, err
	}
	return ka == kb, nil
}

// portableKey is name's confine.Portable key, or Portable's refusal with the
// name it refused.
func portableKey(name string) (string, error) {
	key, err := confine.Portable(name)
	if err != nil {
		return "", fmt.Errorf("the name %q cannot be compared as a folder that folds names reads it: %w", name, err)
	}
	return key, nil
}

// namesOf is p, a clean relative path, a name at a time, split in its slash
// form; "." holds none.
func namesOf(p string) []string {
	if p == "." {
		return nil
	}
	return strings.Split(filepath.ToSlash(p), "/")
}

// carriedIn is carried's refusal of dir, which lies in root: the folder named,
// and the fix. A root is one the run works in, with or without a .git, or the
// cache a fetched rulebook is kept in, so the text says that,
// not that a clone carries it. The variable that sets the state folder is
// config's to name, so the fix says what to do with it, not its
// name.
func carriedIn(dir, root string) error {
	return fmt.Errorf("the state folder %s lies in %s, a folder this run works in or fetches rulebooks into, whose files another person may have written, "+
		"so nothing is read, made or taken in it: %w: "+
		"clear the variable that moved the state folder, or point it at a folder outside every project and outside gragas's cache", dir, root, port.ErrCarried)
}

// apartOf is the one judgement of where a state folder may lie: dir must lie
// outside each folder of roots, which a clone carries, judged by carried on
// resolved names, never on strings. It answers the roots clean, for the make
// to hold apart (privateDirApart), so a link made at a missing name after
// this check leads nothing into a clone. Empty roots judge nothing.
func apartOf(dir string, roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	if err := carried(dir, roots); err != nil {
		return nil, err
	}
	apart := make([]string, 0, len(roots))
	for _, root := range roots {
		// A root that is not there, a cache no run has made yet, was judged
		// by its name above; go-files reads each folder kept apart and
		// refuses one that is not there, so it is not handed on.
		if top, err := hosttrust.Nearest(root); err != nil || filepath.Clean(top) != filepath.Clean(root) {
			continue
		}
		apart = append(apart, filepath.Clean(root))
	}
	return apart, nil
}

// runApart is apartOf over the folders roots answers and also, the root a
// call of the Journal or the Lock names: the folder the run reads and writes
// is one a clone carries too. nil roots judges nothing and answers no folder
// apart: the approvals' own lock over the state folder they judged first is
// the one such Lock the program makes (Approvals.change).
func runApart(dir string, roots func() ([]string, error), also ...string) ([]string, error) {
	if roots == nil {
		return nil, nil
	}
	got, err := roots()
	if err != nil {
		return nil, fmt.Errorf("the folders a clone carries could not be found: %w", err)
	}
	return apartOf(dir, append(slices.Clone(got), also...))
}
