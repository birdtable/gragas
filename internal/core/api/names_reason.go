package coreapi

import (
	"errors"
	"io/fs"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/port"
)

// namesReason is why a names list was not used, from a closed set chosen by
// what err is and never by what it says: an error's words can hold the list's
// path, which may hold a login that is on the list, or the pattern of a bad
// line, which is the list itself.
func namesReason(err error) string {
	if bad, ok := errors.AsType[*format.BadNames](err); ok {
		return bad.Error()
	}
	switch {
	case errors.Is(err, port.ErrNotOwned):
		return "the names file is not owned by this account, or another account can write it"
	case errors.Is(err, fs.ErrNotExist):
		return "the names file that the names-file key names is not there"
	}
	return "the names file could not be read"
}

// namesUnread is a names list that could not be read for a reason the owned
// read did not refuse it for. Its line is namesReason's, never the cause's
// words, which name the path; the cause stays for errors.Is.
type namesUnread struct{ err error }

func (e *namesUnread) Error() string { return namesReason(e.err) }
func (e *namesUnread) Unwrap() error { return e.err }

// namesFailure is err, a names list that could not be read or parsed, in
// words that hold neither its path nor its pattern: a bad line says its
// number alone, a refusal of the owned read keeps its own words, which hold no
// path, and anything else is namesUnread.
func namesFailure(err error) error {
	if _, ok := errors.AsType[*format.BadNames](err); ok {
		return err
	}
	if _, ok := errors.AsType[*namesRefused](err); ok {
		return err
	}
	return &namesUnread{err: err}
}

// namesWords is the line of a names failure, or "" when err holds none.
func namesWords(err error) string {
	if bad, ok := errors.AsType[*format.BadNames](err); ok {
		return bad.Error()
	}
	if refused, ok := errors.AsType[*namesRefused](err); ok {
		return refused.Error()
	}
	if unread, ok := errors.AsType[*namesUnread](err); ok {
		return unread.Error()
	}
	return ""
}

// namesRefused is the names file the owned read refused, said from the
// closed set: it matches port.ErrNotOwned and the port's error of its reason,
// and wraps nothing else. The refusal's own words are never read: they name
// the path, and may name a folder above it or a mount GRAGAS_MOUNTS names, any
// of which can hold a login that is on the list. Its why and fix are its
// reason's, told by the port's error alone.
type namesRefused struct{ refusal }

// refusal is one reason the owned read refuses the names file: the port's
// error it matches, nil for none, and the closed words of its why and fix,
// which hold no part of the path.
type refusal struct {
	is       error
	why, fix string
}

// refusedFor is the names file's refusal err, which matches
// port.ErrNotOwned, as a namesRefused of the reason err matches.
func refusedFor(err error) *namesRefused {
	for _, r := range refusals() {
		if r.is == nil || errors.Is(err, r.is) {
			return &namesRefused{r}
		}
	}
	return nil // refusals ends on the one that matches every error
}

func (e *namesRefused) Error() string {
	return "the names check cannot run: the names file that GRAGAS_NAMES_FILE or the names-file key names " + e.why + "; fix: " + e.fix
}

func (e *namesRefused) Unwrap() []error {
	if e.is == nil {
		return []error{port.ErrNotOwned}
	}
	return []error{port.ErrNotOwned, e.is}
}

// refusals are the refusal's reasons, each with the fix that mends it
// in the order they are asked, the narrower first, though the adapter gives a
// refusal one. The last matches every error, a refusal of none of the others,
// and says each reason and each fix, as the closed words do, since which it
// was is not known. The 1 MiB is host/trust's
// MaxOwned, which the port's ErrOwnedTooLong names.
func refusals() []refusal {
	return []refusal{
		{port.ErrOwnedMountEntry, "is read through a GRAGAS_MOUNTS entry that is refused", "let each GRAGAS_MOUNTS entry name the folder a disk is mounted on itself, as root made it, with no link on the way and no folder above it another account can write"},
		{port.ErrOwnedMount, "is on a disk whose owners are not judged", "move the names file to a disk whose owners are judged, or, where root mounted that disk for this machine, set GRAGAS_MOUNTS to the folder it is mounted on"},
		{port.ErrOwnedByOther, "is owned by another account, not by this account or root", "give the names file to this account"},
		{port.ErrOwnedWritable, "can be written by another account", "let no other account write the names file"},
		{port.ErrOwnedLink, "is a link, and none is followed", "put the names file itself at the path, not a link to it"},
		{port.ErrOwnedNotRegular, "is a folder, a pipe or a device, not a regular file", "put the names file, a regular file, at the path"},
		{port.ErrOwnedTooLong, "holds more than 1 MiB, the most it may hold", "keep the names file to 1 MiB or less"},
		{nil, "is owned by another account, or another account can write it or a folder above it, or a mount it is read through is refused, or it is a link, a folder or no regular file, or it holds more than 1 MiB, or it was replaced while it was read",
			"give the names file to this account, let no other account write it or a folder above it, let each GRAGAS_MOUNTS entry name a mount, put the file itself at the path, not a link to it, keep it to 1 MiB or less, and run it again once nothing is replacing it"},
	}
}
