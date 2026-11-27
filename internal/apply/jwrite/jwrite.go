// Package jwrite is the journalled write of one file: its old content kept,
// the copy and the step journalled, the write made, and a read back held to
// what was meant (architecture 5, step 8); and the roll back of a run from
// the copies it kept. It is written in plain types, a path, bytes and a
// record of strings, and names none of the plan, the canonical form or the
// ports, so the write can move to a library that owns files. internal/apply
// is its adapter: it binds the ports to the interfaces here, turns a Detail
// into canonical bytes and decodes them back.
package jwrite

import (
	"errors"
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/digest"
)

// The kinds of record this package writes.
const (
	KindCopy  = "copy"
	KindStep  = "step"
	KindClose = "close"
)

// A File is one write: the bytes put in the file at Path, or, with Append,
// the line added to its end, or, with Delete, the file taken away.
type File struct {
	Path   string
	Data   []byte
	Append bool
	Delete bool
}

// Seen is what the file at a path held when the check read it, under the
// lock: its bytes, whether it was there, and its digest.
type Seen struct {
	Data   []byte
	Exists bool
	Digest string
}

// A Run is the run a record belongs to.
type Run struct {
	ID, Command, Plan, Engine string
}

// A Record is one line of the journal: the run it belongs to, its kind, and
// a Detail of strings, bools and lists, which the journal encodes.
type Record struct {
	Run, Kind, Command, Plan, Engine string
	Detail                           map[string]any
}

// A Journal keeps the records and the copies of a run. Append is called
// before the effect its record announces.
type Journal interface {
	Append(r Record) error
	Keep(run, path string, old []byte) error
}

// Files is the tree the write is made in, each path relative to the root.
type Files interface {
	Replace(path string, data []byte) error
	AppendLine(path string, line []byte) error
	Remove(path string) error
	Read(path string) ([]byte, error)
}

// A Stage says where One stopped.
type Stage string

// The stages One reports with an error.
const (
	StageJournal Stage = "journal"
	StageWrite   Stage = "write"
	StageRead    Stage = "read"
	StageVerify  Stage = "verify"
)

// Written is a file One wrote, for the close record: its digest before and
// after.
type Written struct {
	Path, Before, After string
}

// One writes a file: its old content to the journal, the step, the write,
// and a read back. The copy and the step are on the journal before the file
// is touched, and a copy that cannot be kept stops the write. The stage is
// set when err is not nil.
func One(j Journal, fs Files, run Run, t File, old Seen) (Written, Stage, error) {
	if err := j.Keep(run.ID, t.Path, old.Data); err != nil {
		return Written{}, StageJournal, fmt.Errorf("the old %s could not be kept: %w", t.Path, err)
	}
	want := Expected(old.Data, t)
	copyRec := map[string]any{"path": t.Path, "existed": old.Exists, "before": old.Digest}
	if err := journal(j, run, KindCopy, copyRec); err != nil {
		return Written{}, StageJournal, err
	}
	after := digest.Prefixed(want)
	if t.Delete {
		after = "" // the file is then not there
	}
	if err := journal(j, run, KindStep, StepDetail(t.Path, Op(t), after)); err != nil {
		return Written{}, StageJournal, err
	}
	var err error
	switch {
	case t.Delete:
		err = fs.Remove(t.Path)
	case t.Append:
		err = fs.AppendLine(t.Path, t.Data)
	default:
		err = fs.Replace(t.Path, t.Data)
	}
	if err != nil {
		return Written{}, StageWrite, fmt.Errorf("%s could not be written: %w", t.Path, err)
	}
	got, err := fs.Read(t.Path)
	if t.Delete {
		if !errors.Is(err, ErrAbsent) {
			return Written{}, StageVerify, fmt.Errorf("%s is still there after it was removed", t.Path)
		}
		return Written{Path: t.Path, Before: old.Digest}, "", nil
	}
	if err != nil {
		return Written{}, StageRead, fmt.Errorf("%s could not be read back: %w", t.Path, err)
	}
	if digest.Prefixed(got) != digest.Prefixed(want) {
		return Written{}, StageVerify, fmt.Errorf("%s does not hold what was written", t.Path)
	}
	return Written{Path: t.Path, Before: old.Digest, After: digest.Prefixed(got)}, "", nil
}

func journal(j Journal, run Run, kind string, detail map[string]any) error {
	return j.Append(Record{Run: run.ID, Kind: kind, Command: run.Command, Plan: run.Plan, Engine: run.Engine, Detail: detail})
}

// StepDetail is the detail of a step record: the file, what is done to it
// and the digest it has after, empty when it is then not there.
func StepDetail(path, op, after string) map[string]any {
	return map[string]any{"path": path, "op": op, "after": after}
}

// CloseDetail is the detail of a close record: how the run ended, the file it
// refused on when it did, and each written file's digest before and after.
func CloseDetail(outcome, file string, files []Written) map[string]any {
	list := []any{}
	for _, w := range files {
		list = append(list, map[string]any{"path": w.Path, "before": w.Before, "after": w.After})
	}
	return map[string]any{"outcome": outcome, "file": file, "files": list}
}

// Op names what t does to its file.
func Op(t File) string {
	if t.Delete {
		return "delete"
	}
	if t.Append {
		return "append"
	}
	return "replace"
}

// Expected is what the file holds once t is written to old: the new bytes,
// or the old with a torn last line ended and the line added, or nothing for a
// file that is taken away.
func Expected(old []byte, t File) []byte {
	if t.Delete {
		return nil
	}
	if !t.Append {
		return t.Data
	}
	out := slices.Clone(old)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, t.Data...)
	return append(out, '\n')
}

// ErrAbsent is what the error of a read matches when the file is not there.
var ErrAbsent = errors.New("the file is not there")

// ErrNotOwner is what the error of a write matches when the root is another
// account's, so the write made nothing.
var ErrNotOwner = errors.New("another account owns the folder")
