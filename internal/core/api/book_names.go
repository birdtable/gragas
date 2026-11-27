package coreapi

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// NamesFound is the findings the names check made, in their order among
// Findings. They are told by the mark the check set and never by their
// words: a list that holds a word of them masks those words, and a caller
// that matched them would find none and call the rulebook clean.
func (r BookReport) NamesFound() []model.Finding {
	out := make([]model.Finding, 0, len(r.namesAt))
	for _, i := range r.namesAt {
		out = append(out, r.Findings[i])
	}
	return out
}

// checkerNames is the names check over what the load read of checkers/: the
// path of every file the listing holds, and every line of each declaration
// it read, since a declaration's words reach the report as the rulebook's do.
// No list finds nothing.
func checkerNames(names *format.Names, l format.Loaded, code link.CheckerCode) []format.Finding {
	if names == nil {
		return nil
	}
	data := map[string][]byte{}
	for _, f := range code.Files {
		data[f.Path] = f.Data
	}
	literals := format.Literals(l.Book.Terms)
	var out []format.Finding
	for _, rel := range code.Listing {
		out = append(out, format.NamesIn(names, literals, path.Join(l.Dir, rel), data[rel])...)
	}
	return out
}

// masked hides each name of the list in every finding and notice of r, so
// that no line the report says holds one while a list is held, whatever check
// made it: a path can hold a name, and a schema's or a contract's words can
// quote a value that does. It masks as the repository audit
// does. Dir is left as it is: a caller reads files by it,
// and a name in it is a path finding on every file below it.
func masked(names *format.Names, r *BookReport) {
	if names == nil {
		return
	}
	for i, f := range r.Findings {
		r.Findings[i].Entity, r.Findings[i].File = names.Mask(f.Entity), names.Mask(f.File)
		if slices.Contains(r.namesAt, i) {
			continue // a names finding's words are the check's own and hold no name
		}
		r.Findings[i].Msg, r.Findings[i].Fix = names.Mask(f.Msg), names.Mask(f.Fix)
	}
	for i, n := range r.Notices {
		r.Notices[i].Entity, r.Notices[i].File, r.Notices[i].Msg = names.Mask(n.Entity), names.Mask(n.File), names.Mask(n.Msg)
	}
}

// add puts f in r's findings, and its index among the names check's when that
// check made it.
func (r *BookReport) add(f format.Finding) {
	if f.Names {
		r.namesAt = append(r.namesAt, len(r.Findings))
	}
	file, line := placeOf(f)
	r.Findings = append(r.Findings, model.Finding{Entity: entityOf(f), Msg: f.Msg, Fix: f.Fix, File: file, Line: line})
}

// maskedError is an error a load ended on while a list was held, said with
// each name of the list hidden: a format error can quote a value or name a
// path that holds one, and a load's ending reaches the screen as its findings
// do. errors.Is and errors.As still see what it wraps.
type maskedError struct {
	err  error
	text string
}

func (e *maskedError) Error() string { return e.text }
func (e *maskedError) Unwrap() error { return e.err }

// maskErr is err with the names of the list masked, or err with no list.
func maskErr(names *format.Names, err error) error {
	if names == nil || err == nil {
		return err
	}
	return &maskedError{err: err, text: names.Mask(err.Error())}
}

// The three ways the names check is off, told apart by readBookNames's
// answer: no list named is no notice and no list (format says it); a list
// that is not there is namesMissing; a list with no name in it is
// namesEmpty. A caller ends on whichever it must.
const (
	namesMissing = "the names check did not run: the names file that GRAGAS_NAMES_FILE or the names-file key names does not exist"
	namesEmpty   = "the names check did not run: the names file that GRAGAS_NAMES_FILE or the names-file key names holds no name"
)

// readBookNames reads the names list through the Owned port: the path is
// named by the settings and lies outside every root, and the list is read only
// when this account or root owns it and no other account can write it, since
// an account that could write it could take names out and the scan would pass
// on what it no longer looks for. A list refused so ends the run not ready.
// The notice says why the check will not run when there is no list, or no name
// in it. Neither the notice nor the refusal names the path: both reach --json
// and SARIF, which leave the machine, and the path says where the private list
// lives.
func readBookNames(owned port.Owned, file string) (*format.Names, string, error) {
	if file == "" {
		return nil, "", nil
	}
	data, err := owned.ReadOwned(file)
	if err == nil {
		n, perr := format.ParseNames(data)
		if bad, ok := errors.AsType[*format.BadNames](perr); ok {
			return nil, "", fmt.Errorf("%w: %w", ErrNotReady, bad)
		}
		if perr == nil && n.Len() == 0 {
			return nil, namesEmpty, nil
		}
		return n, "", perr
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, namesMissing, nil
	}
	if errors.Is(err, port.ErrNotOwned) {
		return nil, "", fmt.Errorf("%w: %w", ErrNotReady, refusedFor(err))
	}
	return nil, "", namesFailure(err)
}
