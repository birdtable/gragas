package root

// The lock and the run record around a run that writes (cli.commands.3),
// as a sibling tool's recorder does it, through the ports cmd/gragas fills.

import (
	"errors"
	"strings"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// recorder is this process's run, once it is known to be one that writes.
type recorder struct {
	cfg   config.Config
	locks legacy.Locks
	runs  legacy.Runs
	argv  []string
	build string
	l     legacy.Lock
	h     legacy.Run
}

// claim takes the lock and opens the run's record, before the first write.
// The record opens here rather than at the first change, so a run that
// writes and then fails has an id to quote. root is the folder -C names, ""
// when the command has no -C, and the lock holds the state folder apart from
// it. It returns the ending of the run when it may not write,
// nil when it may.
func (r *recorder) claim(root string) *ending {
	if r.l != nil {
		return nil
	}
	if r.cfg.StateDir == "" {
		why, fix := noStateFolder()
		return &ending{word: model.NotReady, what: why, next: fix}
	}
	l, err := r.locks.Take(r.cfg.LockPath(), root)
	switch {
	case errors.Is(err, port.ErrCarried):
		// The state folder lies in a folder another person may have
		// written: nothing was taken or begun,
		// and the run is refused, not failed, as a lock another account
		// owns is. The refusal names the folder and the fix.
		return &ending{word: model.Refused, what: err.Error(), next: carriedFix + joinArgv(r.argv)}
	case errors.Is(err, legacy.ErrHeld):
		return &ending{word: model.Busy, what: "another gragas run is writing (" + err.Error() + ")", next: "let it finish, then run this again: " + joinArgv(r.argv)}
	case errors.Is(err, legacy.ErrNotOwner):
		// The lock is taken by its path, in a folder another account owns
		//: nothing was taken, and nothing is recorded, as busy.
		return &ending{word: model.Refused, what: err.Error(), next: "run this again as the account that owns that folder: " + joinArgv(r.argv)}
	case err != nil:
		return &ending{word: model.Failed, what: "the lock " + r.cfg.LockPath() + " could not be taken: " + err.Error()}
	}
	r.l = l
	r.h = r.runs.Begin(r.cfg.RunLog(), r.argv, r.build)
	return nil
}

// change records a write that is really made.
func (r *recorder) change(line string) {
	if r.h != nil {
		r.h.Change(line)
	}
}

// id is the run's id, "" when nothing is recorded.
func (r *recorder) id() string {
	if r.h == nil {
		return ""
	}
	return r.h.ID()
}

// finish closes the record with the word the run ended on, and drops the lock.
// died is the last error said when no closing word was drawn. warn says what
// could not be recorded or released.
func (r *recorder) finish(code int, ended *ending, died string, warn func(format string, a ...any)) {
	if r.h != nil {
		word, what := "", ""
		switch {
		case ended != nil:
			word, what = string(ended.word), ended.what
		case code != exit.Done && died != "":
			word, what = string(model.Failed), died
		}
		r.h.Finish(code, word, what)
		if err := r.h.Err(); err != nil {
			warn("this run's record could not be written to %s: %v", r.cfg.RunLog(), err)
		}
	}
	if r.l != nil {
		if err := r.l.Release(); err != nil {
			warn("the lock %s could not be released: %v", r.cfg.LockPath(), err)
		}
	}
}

// carriedFix is the start of the line a run refused on port.ErrCarried ends
// on, the recorder's and cache list's alike; the run's own argv follows it.
const carriedFix = "point the state folder at a folder outside every project and outside gragas's cache, then run this again: "

// joinArgv is argv as a person types it again.
func joinArgv(argv []string) string { return strings.Join(argv, " ") }

// wayBack records how the run is put back, for a person to read in the
// record.
func (r *recorder) wayBack(u legacy.Undo) {
	if r.h != nil {
		r.h.WayBack(u)
	}
}
