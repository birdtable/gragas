package plugin

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// timeSlack is what C24 lets an endless loop run past TimeLimit before its
// call ends: 100 ms.
const timeSlack = 100 * time.Millisecond

// hostileRef is the ref of the plugin that tries to break a limit, installed
// beside standin in one host.
func hostileRef() model.PluginRef { return model.PluginRef{Name: "hostile", Version: "1.0.0"} }

// spinning is a command that loops forever and never yields.
func spinning() mod {
	return wasiMod(1, code(op(opLoop, blockEmpty), op(opBr, 0), op(opEnd)))
}

// beside is one host over standin, which echoes, and hostile, whose module is
// m, both opened, so both are compiled in the host's one runtime.
func beside(t *testing.T, m mod) (echoing, hostile port.Plugin) {
	t.Helper()
	r := newRig(t, manifestOf(t, echo().bytes()), echo().bytes())
	r.bundles.Put(root, hostileRef(), manifestOf(t, m.bytes(), "name: standin", "name: hostile"), m.bytes())
	echoing, err := r.open(t)
	if err != nil {
		t.Fatal(err)
	}
	hostile, err = r.host.Open(t.Context(), root, hostileRef())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hostile.Close() })
	return echoing, hostile
}

// ending is a call of p's Entries under ctx, started at once: done is closed
// when it ends, with err set.
type ending struct {
	done chan struct{}
	err  error
}

func callAway(ctx context.Context, p port.Plugin) *ending {
	e := &ending{done: make(chan struct{})}
	go func() {
		defer close(e.done)
		_, e.err = p.Entries(ctx, plugin.EntriesRequest{})
	}()
	return e
}

// inTime is a call of p under a caller that ends it timeSlack past TimeLimit:
// a loop that ends within that answers over the time limit, and one that runs
// longer answers the caller's own end, which is no limit. No row reads the
// clock (arch.ports.1); the deadline is the measure.
func inTime(t *testing.T, p port.Plugin) *ending {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), TimeLimit+timeSlack)
	t.Cleanup(cancel)
	return callAway(ctx, p)
}

// wait is e once it ended, or a failure of the row when it has not ended by
// limit, so a call that never ends turns the row red instead of hanging it.
func (e *ending) wait(t *testing.T, limit time.Duration, what string) *ending {
	t.Helper()
	select {
	case <-e.done:
		return e
	case <-time.After(limit):
		t.Fatalf("%s had not ended after %s", what, limit)
		return nil
	}
}

// goesOn is the half of C24 that says gragas goes on while a call of hostile
// runs: a call of echoing made half the time limit into it, when the module
// surely runs (the host gives no signal of a module that started), answers as
// usual and ends while hostile's call has not. A host that ran one call at a
// time would answer it only after the loop's.
func goesOn(t *testing.T, echoing port.Plugin, hostileEnded *atomic.Bool) {
	t.Helper()
	<-time.After(TimeLimit / 2)
	meanwhile := callAway(t.Context(), echoing).wait(t, TimeLimit, "a call made at the same moment")
	if meanwhile.err != nil {
		t.Errorf("a call made while the hostile call ran = %v; want it to answer as usual", meanwhile.err)
	}
	if hostileEnded.Load() {
		t.Error("the call made at the same moment ended only after the hostile call; want it to answer while that call runs")
	}
}

// TestC24AnEndlessLoopEndsWithinTheTimeLimit (C24): a plugin that
// loops forever ends over the time limit, at most 100 ms past TimeLimit, and
// gragas goes on. A call of another plugin of the same host, made half the
// time limit into the loop's call, answers before the loop's call ends. Once it has ended, the next
// call of each plugin answers as usual: the other's answer, and the loop's own
// limit again. Planted red: a runtime that does not close a module when its
// context ends never ends the loop; a time limit 200 ms longer ends it late;
// a host that runs one call at a time answers the other call only after the
// loop's.
func TestC24AnEndlessLoopEndsWithinTheTimeLimit(t *testing.T) {
	t.Parallel()
	echoing, hostile := beside(t, spinning())
	var ended atomic.Bool
	loop := inTime(t, hostile)
	go func() {
		<-loop.done
		ended.Store(true)
	}()
	goesOn(t, echoing, &ended)
	loop.wait(t, TimeLimit+time.Second, "an endless loop")
	if limitOf(loop.err) != plugin.LimitTime {
		t.Errorf("an endless loop = %v; want over the time limit of %s, before a caller that waits %s more ends it", loop.err, TimeLimit, timeSlack)
	}
	if _, err := echoing.Entries(t.Context(), plugin.EntriesRequest{}); err != nil {
		t.Errorf("the call after an endless loop = %v; want it to answer as usual", err)
	}
	again := inTime(t, hostile).wait(t, TimeLimit+time.Second, "the loop's next call")
	if limitOf(again.err) != plugin.LimitTime {
		t.Errorf("the loop's next call = %v; want over the time limit again, within %s of it", again.err, timeSlack)
	}
}

// TestC24AnAllocationLoopEndsAtTheMemoryLimit (C24): a plugin
// that grows its memory a page at a time, without end, ends over the memory
// limit at the first page past MemoryLimit, and gragas goes on. A call of
// another plugin of the same host, made as the grow starts, answers as usual
// (the grow is over in a fraction of a second, so the endless loop's row is
// the one that holds two calls at once).
// Once it has ended, the next call of each plugin answers as usual: the
// other's answer, and the grow's own limit again. Planted red: a runtime with
// no allocator lets the grow hold the page past the limit and exit 0.
func TestC24AnAllocationLoopEndsAtTheMemoryLimit(t *testing.T) {
	t.Parallel()
	echoing, hostile := beside(t, growingStep())
	grow := callAway(t.Context(), hostile)
	meanwhile := callAway(t.Context(), echoing).wait(t, TimeLimit, "a call made at the same moment")
	if meanwhile.err != nil {
		t.Errorf("a call made while the grow ran = %v; want it to answer as usual", meanwhile.err)
	}
	grow.wait(t, TimeLimit+time.Second, "an allocation loop")
	over := func(err error) bool {
		le, ok := errors.AsType[*plugin.LimitError](err)
		return ok && le.What == plugin.LimitMemory && le.Size == pastPages*pageSize && le.Cap == MemoryLimit
	}
	if !over(grow.err) {
		t.Errorf("an allocation loop = %v; want over the memory limit, %d over %d", grow.err, pastPages*pageSize, MemoryLimit)
	}
	if _, err := echoing.Entries(t.Context(), plugin.EntriesRequest{}); err != nil {
		t.Errorf("the call after an allocation loop = %v; want it to answer as usual", err)
	}
	if _, err := hostile.Entries(t.Context(), plugin.EntriesRequest{}); !over(err) {
		t.Errorf("the grow's next call = %v; want over the memory limit again", err)
	}
}
