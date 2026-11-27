package fs

// The adapter of legacy.Runs: the record of every run that writes, one JSON
// line in the run log, through go-run's runlog. This is the one file that
// imports runlog. Since go-run v0.5.0 a line holds no version and no way back
// of runlog's: the build and the way back are gragas's own fields, under the
// line's extra. Nothing in gragas reads the log back (only runs_test.go), and
// a gragas at go-run v0.1.4 refuses these lines, which carry no version 3.

import (
	"github.com/birdtable/go-run/runlog"

	"github.com/birdtable/gragas/internal/port/legacy"
)

// Runs fills legacy.Runs.
type Runs struct{}

// Begin opens a run's record in the log at path, the build under the
// record's own field build. A record that could not be opened says so at Err.
func (Runs) Begin(path string, argv []string, build string) legacy.Run {
	h, err := runlog.Log{Path: path}.Begin(argv, map[string]any{"build": build})
	return &record{h: h, err: err}
}

// record is one run's runlog handle, filling legacy.Run. A nil handle, from a
// Begin that failed, records nothing.
type record struct {
	h    *runlog.Handle
	err  error
	undo []undo
}

// undo is one way back, gragas's own field undo of the record, in the shape
// runlog's Undo had at v0.1.4: how it is taken (always printed, lines for a
// person to read and run), its lines, and what a person should know first.
type undo struct {
	How   string   `json:"how"`
	Lines []string `json:"lines,omitempty"`
	Why   string   `json:"why,omitempty"`
}

// ID is the run's id, "" when it has no record.
func (r *record) ID() string { return r.h.ID() }

// Change records one change the run made.
func (r *record) Change(line string) { r.h.Change(line) }

// WayBack records how the run is put back: lines a person types. Each way
// back is kept, in order, as runlog's AddUndo kept them: the field undo is
// set again with all of them.
func (r *record) WayBack(u legacy.Undo) {
	r.undo = append(r.undo, undo{How: "printed", Lines: u.Lines, Why: u.Why})
	r.h.Set("undo", r.undo)
}

// Finish writes the closing line; what it could not write is Err.
func (r *record) Finish(code int, word, what string) {
	if err := r.h.Finish(code, word, what); err != nil && r.err == nil {
		r.err = err
	}
}

// Err is the first failure to open or write the record.
func (r *record) Err() error {
	if r.err != nil {
		return r.err
	}
	return r.h.Err()
}
