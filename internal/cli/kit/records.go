package kit

import (
	"context"

	"github.com/birdtable/gragas/internal/model"
)

// Said is an error whose text is the sentence the run ends on: the session
// words a failure it met before the core was asked (the working folder would
// not open, a setting is wrong), and the command ends failed on it as it is.
type Said struct{ What string }

func (e Said) Error() string { return e.What }

// Refusing is an error the run ends refused on, not failed: the session found
// that the call may not go on, as a state folder a clone carries,
// and Next is the line to type once it is put right.
type Refusing struct{ What, Next string }

func (e Refusing) Error() string { return e.What }

// Records are the operations of the history, the cache and the journal, each
// one operation of the core, called once, with the folder and the settings
// the session holds. An error is a read or a write that failed; the ones the
// session worded itself are a Said.
type Records interface {
	// History lists the entries of the rulebook of the working folder.
	History(ctx context.Context, ask model.HistoryAsk) (model.HistoryListing, error)
	// CacheList lists what gragas keeps and how old it is.
	CacheList(ctx context.Context) (model.CacheListing, error)
	// CacheClearPlan counts, from the journal of the working folder, what a
	// clear would remove at the keep age. It writes nothing.
	CacheClearPlan(ctx context.Context) (model.ClearPlan, error)
	// CacheClear removes the closed runs past the keep age, under the
	// folder's lock and the run record of the run.
	CacheClear(ctx context.Context) (model.Cleared, error)
	// JournalList lists the runs that wrote on the working folder.
	JournalList(ctx context.Context) (model.JournalListing, error)
	// JournalRestore says how to put one run's files back.
	JournalRestore(ctx context.Context, run string) (model.Restoration, error)
	// JournalRecover rolls back every run that was left open, under the
	// folder's lock and the run record of the run.
	JournalRecover(ctx context.Context) (model.Recovery, error)
	// Writing is what a command that writes shares with the run.
	Writing
}

// Writing is the state of a run that can write: whether it was given --apply,
// the document it answers with under --json, and the lines it says it
// changed, which the run records.
type Writing interface {
	// ApplyFlag is the variable the command's --apply flag sets.
	ApplyFlag() *bool
	// Applying is whether the run was given --apply.
	Applying() bool
	// Data sets the document the run answers with, whatever word it ends on.
	Data(v any)
	// Change is the line of one change, "would " first in a dry run, which
	// the run keeps for its record.
	Change(verb, path string) string
	// WithRun names the run the sentence is about, when the run has a record.
	WithRun(what string) string
}
