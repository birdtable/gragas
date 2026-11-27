package tool_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/gragas/internal/host/tool"
)

// TestEachAnswerOfGoProcIsTheAdaptersOwnOutcomeOnEverySystem: what go-proc's
// Run answers, planted, comes out as this adapter's outcome, the same on
// every system, and every run is recorded. That a tree is ended at its cap,
// its deadline or its bound, that a process left in it is seen, that one
// outside it holding the output is named, and that a file that cannot be
// exec'd fails its start, is go-proc's to hold, and its own tests hold it
// (the pull request names each); what gragas makes of each answer is held
// here, with no process started.
// Risk: a mapping that holds on the one system an end-to-end test ran on and
// says another thing, or counts a run it should not, on the others.
func TestEachAnswerOfGoProcIsTheAdaptersOwnOutcomeOnEverySystem(t *testing.T) {
	t.Parallel()
	const wait = 3 * time.Second
	escapedSays := ", and a process outside its tree still holds its output"
	read := func(mib int64) proc.Result {
		return proc.Result{Code: 0, Peak: mib << 20, PeakRead: true, PeakKind: proc.PeakTreeCharged}
	}
	notExec := &fs.PathError{Op: "fork/exec", Path: "/tools/lint-a", Err: fs.ErrPermission}
	for name, c := range map[string]struct {
		mib  int
		res  proc.Result
		err  error
		code int
		cut  bool
		// why is the *tool.Ended's Why, "" when the run is counted or fails
		// another way; says is what its Says or the error must hold.
		why, says string
		is        error
		recEnded  string
		recPeak   int
	}{
		"an exit of its own is counted with its code": {res: proc.Result{Code: 3}, code: 3, recPeak: -1},
		"the cap ends it": {res: proc.Result{Code: -1}, err: proc.Ended{Why: proc.WhyCap}, code: -1, cut: true,
			why: tool.EndedCap, says: "printed more than its cap of 64 bytes", recEnded: tool.EndedCap, recPeak: -1},
		"the deadline ends it": {res: proc.Result{Code: -1}, err: proc.Ended{Why: proc.WhyDeadline}, code: -1,
			why: tool.EndedDeadline, says: "ran past its deadline of 3s", recEnded: tool.EndedDeadline, recPeak: -1},
		"the cap, with a process outside the tree holding the output": {res: proc.Result{Code: -1},
			err: errors.Join(proc.Ended{Why: proc.WhyCap}, proc.Ended{Why: proc.WhyEscaped}), code: -1, cut: true,
			why: tool.EndedCap, says: "printed more than its cap of 64 bytes" + escapedSays, recEnded: tool.EndedCap, recPeak: -1},
		"a process outside the tree holding the output, alone": {res: proc.Result{Code: 0}, err: proc.Ended{Why: proc.WhyEscaped},
			says: "lint-a ended" + escapedSays, is: proc.Ended{Why: proc.WhyEscaped}, recPeak: -1},
		"the bound its tree was held to ends it": {mib: 512, res: proc.Result{Code: -1}, err: proc.Ended{Why: proc.WhyLimit}, code: -1,
			why: tool.EndedMemory, says: "passed the bound of ", recEnded: tool.EndedMemory, recPeak: -1},
		"a peak over the limit is not counted": {mib: 512, res: read(600),
			why: tool.EndedMemory, says: "held 600 MiB at its peak (tree-charged), over the memory limit of 512 MiB", recEnded: tool.EndedMemory, recPeak: 600},
		"a peak under the limit is counted": {mib: 512, res: read(100), recPeak: 100},
		"a process left in the tree, held to a limit, is not counted": {mib: 512, res: func() proc.Result { r := read(100); r.Left = true; return r }(),
			why: tool.EndedMemory, says: tool.LeftUnread("lint-a", 512).Says, recEnded: tool.EndedMemory, recPeak: 100},
		"an unkillable tree whose peak was never read says so, not that a process was left unread": {mib: 512, res: proc.Result{Code: -1, Left: true}, code: -1,
			why: tool.EndedMemory, says: "gave no reading of its peak memory against ", recEnded: tool.EndedMemory, recPeak: -1},
		"a process left in the tree, held to none, is counted": {res: proc.Result{Code: 0, Left: true}, recPeak: -1},
		"a file that cannot be exec'd is a start that failed, no memory verdict": {mib: 512, res: proc.Result{Code: -1}, err: notExec,
			says: "fork/exec /tools/lint-a", is: fs.ErrPermission, recPeak: -1},
	} {
		// No subtest: t.Run reads as the adapter's Run, which signals.
		tl := tool.New(tool.Config{})
		tl.Bound(wait, 64)
		out, err := tool.Outcome(tl, t.Context(), c.mib, c.res, c.err)
		var e *tool.Ended
		ended := errors.As(err, &e)
		switch {
		case c.why != "" && (!ended || e.Why != c.why || !strings.HasPrefix(e.Says, c.says) || e.Tool != "lint-a"):
			t.Errorf("%s: %v, want ended by %s saying %q", name, err, c.why, c.says)
		case c.why == "" && ended:
			t.Errorf("%s: %v, want no *tool.Ended", name, err)
		case c.why == "" && c.says == "" && err != nil:
			t.Errorf("%s: %v, want it counted", name, err)
		case c.why == "" && c.says != "" && (err == nil || !strings.Contains(err.Error(), c.says) || !errors.Is(err, c.is)):
			t.Errorf("%s: %v, want an error holding %q and %v", name, err, c.says, c.is)
		}
		if (c.why == "" && err == nil || ended) && (out.Code != c.code || out.Cut != c.cut) {
			t.Errorf("%s: code %d, cut %v; want %d, %v", name, out.Code, out.Cut, c.code, c.cut)
		}
		ran := tl.Ran()
		if len(ran) != 1 {
			t.Errorf("%s: %d records, want one", name, len(ran))
			continue
		}
		if r := ran[0]; r.Tool != "lint-a" || r.Shape != "step" || r.Code != c.res.Code || r.Ended != c.recEnded || r.PeakMiB != c.recPeak || r.Left != c.res.Left {
			t.Errorf("%s: recorded %+v, want code %d, ended %q, peak %d, left %v", name, r, c.res.Code, c.recEnded, c.recPeak, c.res.Left)
		}
	}
}

// TestACancelledRunIsTheCancelWhateverItsLimit: a run whose caller's context
// ended answers that context's error, recorded, and never a memory verdict
// on a peak go-proc could not read, nor a deadline it did not reach.
// Risk: a run the caller cancelled is reported as one over its memory limit
// or its deadline, and the caller acts on a verdict no tool earned.
func TestACancelledRunIsTheCancelWhateverItsLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, mib := range []int{0, 512} {
		tl := tool.New(tool.Config{})
		_, err := tool.Outcome(tl, ctx, mib, proc.Result{Code: -1},
			errors.Join(proc.Ended{Why: proc.WhyCancelled}, context.Canceled))
		var e *tool.Ended
		if !errors.Is(err, context.Canceled) || errors.As(err, &e) {
			t.Errorf("held to %d MiB: %v, want the cancel and no *tool.Ended", mib, err)
		}
		if ran := tl.Ran(); len(ran) != 1 || ran[0].Code != -1 || ran[0].Ended != "" {
			t.Errorf("held to %d MiB: recorded %+v, want one record of code -1, not ended", mib, ran)
		}
	}
	// A refusal before the start is the adapter's own words, recorded too.
	tl := tool.New(tool.Config{})
	_, err := tool.Outcome(tl, t.Context(), 0, proc.Result{Code: -1}, proc.ErrBatch)
	var r *tool.Refused
	if !errors.As(err, &r) || len(tl.Ran()) != 1 {
		t.Errorf("a refused start: %v, %d records; want *tool.Refused and one record", err, len(tl.Ran()))
	}
}
