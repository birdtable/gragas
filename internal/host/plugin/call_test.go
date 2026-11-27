package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// compiled is m compiled in a runtime of the host's, held to check C5 as Open
// holds a module, with the least size of its memory.
func compiled(t *testing.T, m mod) (wazero.Runtime, wazero.CompiledModule, int) {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	rt, wasi, err := (&Host{}).runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	cm, least, err := compile(ctx, rt, wasi, m.bytes())
	if err != nil {
		t.Fatal(err)
	}
	return rt, cm, least
}

// callOf is one call of m on stdin, under limit, with the memory the call was
// held in, or nil if limit was never asked.
func callOf(t *testing.T, limit limited, m mod, stdin []byte) ([]byte, *memory, error) {
	t.Helper()
	rt, cm, least := compiled(t, m)
	var mem *memory
	seen := func(ctx context.Context) (context.Context, context.CancelFunc, *memory) {
		ctx, cancel, got := limit(ctx)
		mem = got
		return ctx, cancel, got
	}
	out, err := run(t.Context(), seen, rt, cm, least, stdin)
	return out, mem, err
}

// limitOf is the limit err is, or "" for an error that is none.
func limitOf(err error) plugin.Limit {
	if le, ok := errors.AsType[*plugin.LimitError](err); ok {
		return le.What
	}
	return ""
}

// TestACallAnswersWhatTheModuleWrote: the request is the module's stdin and
// its stdout is the answer, when it exits with code 0. A request of several
// reads comes back whole.
func TestACallAnswersWhatTheModuleWrote(t *testing.T) {
	t.Parallel()
	req := bytes.Repeat([]byte("request "), 40<<10)
	out, _, err := callOf(t, callContext, echo(), req)
	if err != nil || !bytes.Equal(out, req) {
		t.Fatalf("the echo of %d bytes = %d bytes, %v; want the request", len(req), len(out), err)
	}
}

// TestACallThatDoesNotExitZeroIsACrash: a trap or an exit with a code other
// than 0 is port.ErrTrap and no limit, and what the module wrote before it is
// no answer. An exit with code 0, or a _start that returns, is the answer.
func TestACallThatDoesNotExitZeroIsACrash(t *testing.T) {
	t.Parallel()
	partial := code(write(1, 1024, 7, 0))
	for _, row := range []struct {
		name  string
		body  []byte
		crash bool
	}{
		{"an exit of 3", exit(3), true},
		{"a trap", op(opUnreachable), true},
		{"an answer, then an exit of 1", code(partial, exit(1)), true},
		{"an answer, then a trap", code(partial, op(opUnreachable)), true},
		{"an answer, then an exit of 0", code(partial, exit(0)), false},
		{"an answer, then a return", partial, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			m := wasiMod(1, row.body)
			m.data = []segment{{offset: 1024, data: []byte("partial")}}
			out, _, err := callOf(t, callContext, m, nil)
			switch {
			case row.crash && (!errors.Is(err, port.ErrTrap) || limitOf(err) != "" || out != nil):
				t.Errorf("the call = %q, %v; want no answer and port.ErrTrap", out, err)
			case !row.crash && (err != nil || string(out) != "partial"):
				t.Errorf("the call = %q, %v; want the answer", out, err)
			}
		})
	}
}

// stateful is a command that writes its memory's byte at 100 and its global
// to stdout, and then adds one to each: a call that saw a call before it
// writes another answer.
func stateful() mod {
	m := wasiMod(1, code(
		i32c(101), op(opGlobalGet, 0), memArg(opI32Store8, 0, 0),
		write(1, 100, 2, 0),
		i32c(100), i32c(100), memArg(opI32Load8, 0, 0), i32c(1), op(opI32Add), memArg(opI32Store8, 0, 0),
		op(opGlobalGet, 0), i32c(1), op(opI32Add), op(opGlobalSet, 0),
	))
	m.globals = []int32{'x'}
	m.data = []segment{{offset: 100, data: []byte("a")}}
	return m
}

// fresh is nil when two answers are each "ax", the answer of a module that saw
// nothing before it.
func fresh(first, second []byte) error {
	if string(first) != "ax" || string(second) != "ax" {
		return fmt.Errorf("two calls answered %q and %q; want \"ax\" each", first, second)
	}
	return nil
}

// TestNothingSurvivesACall: two calls of one module each start from the
// module as it was compiled, its memory and its globals. Planted red: two
// calls into one instance carry the first's memory and global into the second.
func TestNothingSurvivesACall(t *testing.T) {
	t.Parallel()
	first, _, err := callOf(t, callContext, stateful(), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := callOf(t, callContext, stateful(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh(first, second); err != nil {
		t.Error(err)
	}
	t.Run("planted: one instance called twice", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		rt, cm, _ := compiled(t, stateful())
		var out bytes.Buffer
		inst, err := rt.InstantiateModule(ctx, cm, wazero.NewModuleConfig().WithName("").WithStdout(&out).WithStartFunctions())
		if err != nil {
			t.Fatal(err)
		}
		defer inst.Close(ctx)
		var answers [2][]byte
		for i := range answers {
			out.Reset()
			if _, err := inst.ExportedFunction("_start").Call(ctx); err != nil {
				t.Fatal(err)
			}
			answers[i] = bytes.Clone(out.Bytes())
		}
		if fresh(answers[0], answers[1]) == nil {
			t.Errorf("one instance called twice answered %q and %q, and fresh passed it", answers[0], answers[1])
		}
	})
}

// TestTheLogIsCutAt64KiB: what a module writes to stderr is a log, never the
// answer, and only its first LogLimit bytes are kept: a crash's error shows
// the end of what was kept, the 400 bytes of c before the cut, and none of the
// b past it. A log past the limit does not fail a call that exits with code 0.
func TestTheLogIsCutAt64KiB(t *testing.T) {
	t.Parallel()
	log := slices.Concat(bytes.Repeat([]byte("a"), LogLimit-tailSize), bytes.Repeat([]byte("c"), tailSize), bytes.Repeat([]byte("b"), 1024))
	logged := func(end []byte) mod {
		m := wasiMod(2, code(write(2, 0, int32(len(log)), int32(len(log))+16), end))
		m.data = []segment{{offset: 0, data: log}}
		return m
	}
	_, _, err := callOf(t, callContext, logged(op(opUnreachable)), nil)
	if !errors.Is(err, port.ErrTrap) || !strings.HasSuffix(err.Error(), "its log ends "+strings.Repeat("c", tailSize)) {
		t.Errorf("a crash after a log of %d bytes = %v; want port.ErrTrap ending on the c before the cut", len(log), err)
	}
	out, _, err := callOf(t, callContext, logged(exit(0)), nil)
	if err != nil || len(out) != 0 {
		t.Errorf("an exit of 0 after a log of %d bytes = %q, %v; want an empty answer", len(log), out, err)
	}
}

// TestAnAnswerPastItsLimitIsOverTheLimit: an answer of ResponseLimit bytes is
// the answer, and one byte more is over the response limit, whatever the
// module does after its write is refused.
func TestAnAnswerPastItsLimitIsOverTheLimit(t *testing.T) {
	t.Parallel()
	const pages = ResponseLimit/pageSize + 1
	answering := func(n int32) mod { return wasiMod(pages+1, write(1, 0, n, pages*pageSize)) }
	out, _, err := callOf(t, callContext, answering(ResponseLimit), nil)
	if err != nil || len(out) != ResponseLimit {
		t.Errorf("an answer of %d bytes = %d bytes, %v; want the answer", ResponseLimit, len(out), err)
	}
	_, _, err = callOf(t, callContext, answering(ResponseLimit+1), nil)
	le, ok := errors.AsType[*plugin.LimitError](err)
	if !ok || le.What != plugin.LimitResponse || le.Size != ResponseLimit+1 || le.Cap != ResponseLimit {
		t.Errorf("an answer of %d bytes = %v; want over the response limit, %d over %d", ResponseLimit+1, err, ResponseLimit+1, ResponseLimit)
	}
}

// pastPages is the pages of one page past MemoryLimit.
const pastPages = MemoryLimit/pageSize + 1

// growingStep is a command that grows its memory one page at a time and exits
// with code 0 when a grow is refused or when it holds pastPages.
func growingStep() mod {
	return wasiMod(1, code(
		op(opLoop, blockEmpty),
		i32c(1), op(opMemoryGrow, 0), op(opLocalTee, 0),
		i32c(-1), op(opI32Eq),
		op(opIf, blockEmpty), exit(0), op(opEnd),
		op(opLocalGet, 0), i32c(1), op(opI32Add), i32c(pastPages), op(opI32Eq),
		op(opIf, blockEmpty), exit(0), op(opEnd),
		op(opBr, 0),
		op(opEnd),
	))
}

// growingOnce is a command that grows its memory of one page to pastPages in
// one grow, and exits with code 0 whatever the grow answers.
func growingOnce() mod {
	return wasiMod(1, code(i32c(pastPages-1), op(opMemoryGrow, 0), op(opDrop), exit(0)))
}

// overMemory is nil when a call ended with err over the memory limit, at the
// first page past it, and the instance's memory, mem, never held more than
// MemoryLimit.
func overMemory(err error, mem *memory) error {
	le, ok := errors.AsType[*plugin.LimitError](err)
	if !ok || le.What != plugin.LimitMemory || le.Size != pastPages*pageSize || le.Cap != MemoryLimit {
		return fmt.Errorf("the call = %w; want over the memory limit, %d over %d", err, pastPages*pageSize, MemoryLimit)
	}
	if mem == nil || mem.peak > MemoryLimit {
		return fmt.Errorf("the instance's memory held %v bytes; want at most %d", mem, MemoryLimit)
	}
	return nil
}

// bare is a call's context with the time limit and no allocator, a runtime of
// the kind the memory rows are planted against.
func bare(ctx context.Context) (context.Context, context.CancelFunc, *memory) {
	ctx, cancel := context.WithTimeout(ctx, TimeLimit)
	return ctx, cancel, &memory{}
}

// TestAGrowPastTheMemoryLimitIsOverMemory: a module that grows its memory past
// MemoryLimit, a page at a time or in one grow, ends over the memory limit
// although it exits with code 0, and its memory never held more than the
// limit. Planted red: a call without the
// allocator lets the module hold the page past the limit and answer.
func TestAGrowPastTheMemoryLimitIsOverMemory(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		m    mod
	}{
		{"a page at a time", growingStep()},
		{"in one grow", growingOnce()},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			_, mem, err := callOf(t, callContext, row.m, nil)
			if err := overMemory(err, mem); err != nil {
				t.Error(err)
			}
			_, mem, err = callOf(t, bare, row.m, nil)
			if overMemory(err, mem) == nil {
				t.Errorf("planted: a call without the allocator = %v, and overMemory passed it", err)
			}
		})
	}
}

// TestADeclaredMemoryPastTheLimitIsRefusedBeforeItRuns: a module whose least
// memory is past MemoryLimit is over the memory limit, and is refused before
// a call's context is made, so it is never instantiated. Planted red: the
// least memory at the limit is instantiated and answers.
func TestADeclaredMemoryPastTheLimitIsRefusedBeforeItRuns(t *testing.T) {
	t.Parallel()
	asked := false
	never := func(ctx context.Context) (context.Context, context.CancelFunc, *memory) {
		asked = true
		return callContext(ctx)
	}
	out, _, err := callOf(t, never, wasiMod(pastPages, nil), nil)
	le, ok := errors.AsType[*plugin.LimitError](err)
	if !ok || le.What != plugin.LimitMemory || le.Size != pastPages*pageSize || le.Cap != MemoryLimit || asked || out != nil {
		t.Errorf("a least memory of %d pages = %q, %v, instantiated %t; want over the memory limit before it runs", pastPages, out, err, asked)
	}
	if _, _, err := callOf(t, callContext, wasiMod(pastPages-1, nil), nil); err != nil {
		t.Errorf("planted: a least memory of %d pages, the limit = %v; want it to run", pastPages-1, err)
	}
}

// TestASpinEndsOnTime: a module that never yields ends over the time limit,
// before a caller that waits a second more ends it. A caller's ctx that ends first is its own error, and
// no limit.
func TestASpinEndsOnTime(t *testing.T) {
	t.Parallel()
	spin := wasiMod(1, code(op(opLoop, blockEmpty), op(opBr, 0), op(opEnd)))
	t.Run("the time limit", func(t *testing.T) {
		t.Parallel()
		rt, cm, least := compiled(t, spin)
		ctx, cancel := context.WithTimeout(t.Context(), TimeLimit+time.Second)
		defer cancel()
		_, err := run(ctx, callContext, rt, cm, least, nil)
		if limitOf(err) != plugin.LimitTime {
			t.Errorf("a spin = %v; want over the time limit of %s, before a caller that waits a second more", err, TimeLimit)
		}
	})
	t.Run("the caller's end", func(t *testing.T) {
		t.Parallel()
		rt, cm, least := compiled(t, spin)
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		_, err := run(ctx, callContext, rt, cm, least, nil)
		if !errors.Is(err, context.DeadlineExceeded) || limitOf(err) != "" {
			t.Errorf("a spin whose caller ended = %v; want the caller's own error", err)
		}
	})
}

// TestCallsAtOnceGiveTheSameBytes: calls of one compiled module at once each
// answer what they would one after the other.
func TestCallsAtOnceGiveTheSameBytes(t *testing.T) {
	t.Parallel()
	rt, cm, least := compiled(t, echo())
	const n = 8
	reqs := make([][]byte, n)
	for i := range reqs {
		reqs[i] = bytes.Repeat([]byte{byte('a' + i)}, (i+1)*50<<10)
	}
	var wg sync.WaitGroup
	got := make([][]byte, n)
	errs := make([]error, n)
	for i := range reqs {
		wg.Go(func() { got[i], errs[i] = run(t.Context(), callContext, rt, cm, least, reqs[i]) })
	}
	wg.Wait()
	for i := range reqs {
		alone, err := run(t.Context(), callContext, rt, cm, least, reqs[i])
		if errs[i] != nil || err != nil || !bytes.Equal(got[i], alone) || !bytes.Equal(alone, reqs[i]) {
			t.Errorf("call %d at once = %d bytes, %v; alone %d bytes, %v; want the request each", i, len(got[i]), errs[i], len(alone), err)
		}
	}
}
