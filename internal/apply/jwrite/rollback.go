package jwrite

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
)

// A Line is one record of a run as the journal holds it, for a roll back to
// read: its kind and its detail as the journal encoded it.
type Line struct {
	Kind   string
	Detail []byte
}

// A Store is the journal as a roll back reads it as well as writes it: the
// detail of a line decoded, and the copy a run kept of a file, by its bytes
// and by where it lies, for a person to copy back.
type Store interface {
	Journal
	// Decode is a line's detail as the journal decodes it: a string, a bool,
	// a []any, a map[string]any or nil, and any other value as a value of
	// none of those types.
	Decode(detail []byte) (any, error)
	Kept(run, path string) ([]byte, error)
	KeptAt(run, path string) (string, error)
}

// A Back is one roll back: the run it undoes, the run that does it, which
// its close names as "by", and Unsafe, which says why a path is not one a
// plan may name, empty when it is.
type Back struct {
	Run    Run
	By     string
	Unsafe func(path string) string
}

// A copied file is one copy record of a run: the file it names, whether it
// was there, and its digest before.
type copied struct {
	path    string
	existed bool
	before  string
}

// RollBack puts back every file the run kept a copy of, newest first, from
// the journal's copies, and closes the run's record "rolled back" with each
// file's digest before and after the roll-back (cli.commands.7,
// cli.commands.25). The copies decide, never a run's memory, so a run rolled
// back by itself and one rolled back by the next command take one path.
// Every copy record is read and checked before the first file is
// touched, and so is every file: it must hold what the run could have left
// there, its digest before or the digest after of one of the run's own steps
// on it, so a file a person changed since is never overwritten or removed. A
// step is journalled before each file is put back (store.consistency.4). A
// file that already holds its copy is left as it is, so a roll-back run
// again writes only what is still out of place.
func RollBack(j Store, fs Files, b Back, lines []Line) error {
	var copies []copied
	left := map[string][]string{}
	for _, l := range lines {
		switch l.Kind {
		case KindCopy:
			c, err := b.copyOf(j, l)
			if err != nil {
				return err
			}
			copies = append(copies, c)
			left[c.path] = append(left[c.path], c.before)
		case KindStep:
			p, after, err := b.stepOf(j, l)
			if err != nil {
				return err
			}
			left[p] = append(left[p], after)
		}
	}
	var changed []changedFile
	for _, c := range copies {
		now, err := digestNow(fs, c.path)
		if err != nil {
			return err
		}
		if !slices.Contains(left[c.path], now) {
			changed = append(changed, changedFile{c, now, left[c.path]})
		}
	}
	if len(changed) > 0 {
		return b.changedSince(j, changed)
	}
	var back []Written
	for _, c := range slices.Backward(copies) {
		w, err := b.restore(j, fs, c)
		if err != nil {
			return err
		}
		back = append(back, w)
	}
	detail := CloseDetail("rolled back", "", back)
	detail["by"] = b.By
	return journal(j, b.Run, KindClose, detail)
}

// A changedFile is a copied file that holds neither its before nor the
// after of a step of the run: now is its digest, and left what the run
// could have left there.
type changedFile struct {
	copied
	now  string
	left []string
}

// changedSince is the error of a roll-back that found files a person
// changed since the run: each file and its digests, and the steps that keep
// the person's version and let the roll-back finish. A version the person
// made goes out of the root; a file the run replaced gets back the copy the
// run kept, named by where it lies; then the command runs again, and the
// person's version comes back. A file the run made needs nothing in its
// place, since the roll-back removes nothing that is not there.
func (b Back) changedSince(j Store, changed []changedFile) error {
	var said, moved, made, put []string
	for _, c := range changed {
		said = append(said, fmt.Sprintf("%s changed since run %s: it is %s, and the run left it %s",
			c.path, b.Run.ID, shown(c.now), strings.Join(shownAll(c.left), " or ")))
		if c.now != "" {
			moved = append(moved, c.path)
		}
		if !c.existed {
			made = append(made, c.path)
			continue
		}
		// gragas journal restore <run> prints these lines: once it is
		// built, the message names it too.
		at, err := j.KeptAt(b.Run.ID, c.path)
		if err != nil {
			return fmt.Errorf("%s, so it is not overwritten, and where the run kept the copy of %s is not known: %w", strings.Join(said, "; "), c.path, err)
		}
		put = append(put, "copy "+at+" to "+c.path+" in the root")
	}
	versions, none := "version", "it is not"
	if len(moved) > 1 {
		versions = "versions"
	}
	if len(changed) > 1 {
		none = "none is"
	}
	var steps []string
	keep := "To finish the roll-back: "
	if len(moved) > 0 {
		keep = "To keep your " + versions + " and finish the roll-back: "
		steps = append(steps, "move your "+versions+" of "+and(moved)+" out of the root")
	}
	if len(made) > 0 {
		steps = append(steps, "the run made "+and(made)+", so nothing goes back in "+their(made)+" place")
	}
	steps = append(steps, put...)
	steps = append(steps, "run the command again")
	if len(moved) > 0 {
		steps = append(steps, "then bring your "+versions+" back")
	}
	return fmt.Errorf("%s, so %s overwritten. %s%s", strings.Join(said, "; "), none, keep, strings.Join(steps, "; "))
}

// and names ps as a list in words.
func and(ps []string) string {
	if len(ps) < 2 {
		return strings.Join(ps, "")
	}
	return strings.Join(ps[:len(ps)-1], ", ") + " and " + ps[len(ps)-1]
}

// their names ps as one file or several.
func their(ps []string) string {
	if len(ps) > 1 {
		return "their"
	}
	return "its"
}

// copyOf reads a copy record on its parsed form: an object of exactly a
// path a plan may name, whether the file existed and its digest before,
// which is empty exactly when it did not exist. Anything else is refused,
// naming the record, and nothing is put back.
func (b Back) copyOf(j Store, l Line) (copied, error) {
	bad := func(why string) (copied, error) {
		return copied{}, fmt.Errorf("a copy record of run %s cannot be rolled back from: %s", b.Run.ID, why)
	}
	v, err := j.Decode(l.Detail)
	if err != nil {
		return bad(err.Error())
	}
	o, ok := v.(map[string]any)
	if !ok || len(o) != 3 {
		return bad("it is not an object of a path, whether it existed and its digest before")
	}
	p, okP := o["path"].(string)
	existed, okE := o["existed"].(bool)
	before, okB := o["before"].(string)
	switch {
	case !okP || !okE || !okB:
		return bad("it is not an object of a path, whether it existed and its digest before")
	case b.Unsafe(p) != "":
		return bad(fmt.Sprintf("%s: %s", p, b.Unsafe(p)))
	case existed == (before == ""):
		return bad(fmt.Sprintf("%s: a file that existed has a digest before, and one that did not has none", p))
	case existed && !strings.HasPrefix(before, digest.Prefix):
		return bad(fmt.Sprintf("%s: its digest before is not a digest", p))
	}
	return copied{path: p, existed: existed, before: before}, nil
}

// stepOf reads a step record on its parsed form: an object of exactly a
// path a plan may name, what is done and the digest after, empty when the
// file is then not there. Anything else is refused, naming the record.
func (b Back) stepOf(j Store, l Line) (string, string, error) {
	bad := func(why string) (string, string, error) {
		return "", "", fmt.Errorf("a step record of run %s cannot be rolled back from: %s", b.Run.ID, why)
	}
	v, err := j.Decode(l.Detail)
	if err != nil {
		return bad(err.Error())
	}
	o, ok := v.(map[string]any)
	p, okP := o["path"].(string)
	_, okO := o["op"].(string)
	after, okA := o["after"].(string)
	switch {
	case !ok || len(o) != 3 || !okP || !okO || !okA:
		return bad("it is not an object of a path, what is done and its digest after")
	case b.Unsafe(p) != "":
		return bad(fmt.Sprintf("%s: %s", p, b.Unsafe(p)))
	case after != "" && !strings.HasPrefix(after, digest.Prefix):
		return bad(fmt.Sprintf("%s: its digest after is not a digest", p))
	}
	return p, after, nil
}

// shown is a digest as a message names it: "no file" for none.
func shown(d string) string {
	if d == "" {
		return "no file"
	}
	return d
}

// shownAll is each digest as shown names it, each once.
func shownAll(ds []string) []string {
	var out []string
	for _, d := range ds {
		if s := shown(d); !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// restore puts one file back as its copy says: the kept bytes when it
// existed, which must have the digest the copy record names, or no file
// when it did not. It reads the file back after.
func (b Back) restore(j Store, fs Files, c copied) (Written, error) {
	now, err := digestNow(fs, c.path)
	if err != nil {
		return Written{}, err
	}
	if now != c.before {
		if err := b.put(j, fs, c); err != nil {
			return Written{}, err
		}
	}
	after, err := digestNow(fs, c.path)
	if err != nil {
		return Written{}, err
	}
	if after != c.before {
		return Written{}, fmt.Errorf("%s does not hold its copy once it is put back", c.path)
	}
	return Written{Path: c.path, Before: now, After: after}, nil
}

// put journals the step and makes it: the copy written back whole, or the
// file the run made removed.
func (b Back) put(j Store, fs Files, c copied) error {
	if !c.existed {
		if err := journal(j, b.Run, KindStep, StepDetail(c.path, "remove", "")); err != nil {
			return err
		}
		if err := fs.Remove(c.path); err != nil {
			return fmt.Errorf("%s, which the run made, could not be removed: %w", c.path, err)
		}
		return nil
	}
	old, err := j.Kept(b.Run.ID, c.path)
	if err != nil {
		return fmt.Errorf("the copy of %s could not be read: %w", c.path, err)
	}
	if digest.Prefixed(old) != c.before {
		return fmt.Errorf("the copy of %s is not the file its copy record names, so it is not written back", c.path)
	}
	if err := journal(j, b.Run, KindStep, StepDetail(c.path, "restore", c.before)); err != nil {
		return err
	}
	if err := fs.Replace(c.path, old); err != nil {
		return fmt.Errorf("%s could not be put back: %w", c.path, err)
	}
	return nil
}

// digestNow is the digest of the file at p as the tree holds it now, empty
// when it is not there.
func digestNow(fs Files, p string) (string, error) {
	got, err := fs.Read(p)
	switch {
	case errors.Is(err, ErrAbsent):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("%s could not be read: %w", p, err)
	}
	return digest.Prefixed(got), nil
}
