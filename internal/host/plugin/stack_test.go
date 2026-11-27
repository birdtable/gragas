package plugin

import (
	"context"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// stackRuns is how many calls the stack row makes, keeping the least any
// allocated. The row measures in a rerun child where it is the only test, so
// no other test's work is in a window; the least of the calls leaves out what
// the Go runtime itself does in one, and a call can only read more than its
// own, never less.
const stackRuns = 5

// deepestFrame is the word the stack row's child is given.
const deepestFrame = "deepest-frame"

// deepModule is a module whose exported rec calls itself until depth frames
// are on the call stack, each of locals locals and operands values held
// across its call, and at the deepest calls env.probe. Its frame, as the pass
// finds it, is locals+operands+2: the two the depth's test takes beside them.
func deepModule(locals, operands, depth int) mod {
	const unit, recFn, probeFn = 0, 1, 0
	return mod{
		types:   []fnType{unit: {}},
		imports: []wasiImport{{module: "env", name: "probe", typ: unit}},
		funcs: []fn{{typ: unit, locals: locals, body: code(
			times(operands, i32c(0)),
			op(opGlobalGet, 0), i32c(1), op(opI32Add), op(opGlobalSet, 0),
			op(opGlobalGet, 0), i32c(int32(depth)), op(opI32Eq),
			op(opIf, blockEmpty), op(opCall, probeFn), op(opElse), op(opCall, recFn), op(opEnd),
			times(operands, op(opDrop)),
		)}},
		globals: []int32{0},
		exports: []export{{name: "rec", kind: exportFunc, index: recFn}},
	}
}

// deepestAlloc is the least, over stackRuns calls of m's rec, that one call
// allocated, failing the row if a call does not reach its deepest frame.
func deepestAlloc(t *testing.T, m mod) uint64 {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	least := ^uint64(0)
	for range stackRuns {
		rt := wazero.NewRuntimeWithConfig(ctx, RuntimeConfig())
		probed := false
		if _, err := rt.NewHostModuleBuilder("env").NewFunctionBuilder().WithFunc(func(context.Context) {
			probed = true
		}).Export("probe").Instantiate(ctx); err != nil {
			t.Fatalf("instantiating env = %v", err)
		}
		inst, err := rt.Instantiate(ctx, m.bytes())
		if err != nil {
			t.Fatalf("instantiating the module = %v", err)
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err = inst.ExportedFunction("rec").Call(ctx)
		runtime.ReadMemStats(&after)
		if err != nil || !probed {
			t.Fatalf("rec = %v, probed %v; want the deepest frame reached", err, probed)
		}
		least = min(least, after.TotalAlloc-before.TotalAlloc)
		_ = rt.Close(ctx)
	}
	return least
}

// series is the most, in slots, that append allocates to grow a slice to n
// slots, over n: each array it outgrows is left behind, every one a quarter
// smaller than the next past 256 elements (runtime.nextslicecap), so they sum
// to 5 times the last, which holds up to 5/4 of n: 6.25, and 7 with room.
// frameRecord is the bytes wazero keeps beside the slots for each frame on
// the call stack, its callFrame and the pointer to it, with room to spare.
const (
	series      = 7
	frameRecord = 64
)

// TestTheValueStackIsHeldToStackLimit: wazero's interpreter keeps a call's
// frames on one Go slice of 8-byte slots, so what a call allocates to reach its deepest frame,
// all callFrames of them, is within series x callFrames x slotSize x the frame
// Open's pass finds, and a record a frame. The row's module recurses to the
// ceiling in frames of FrameLimit, the most a module may take, and allocates
// 6.14 times its slots (go1.27.4, wazero v1.12.0); at its deepest it holds
// 1.23 times them live, about 40 MiB. TotalAlloc is the whole process's, so
// the measure is made in a rerun of this test binary that runs
// TestTheHelperRunsOnlyWhenRerun alone, where no parallel test allocates in
// the window, however many run beside this row and however busy the machine
// (beside them it once read 15.09 x). This row is parallel and fails
// with the child's output; the bound is never raised to quiet it. Planted red:
// the call allocates at least the slots, so a row that read nothing fails.
func TestTheValueStackIsHeldToStackLimit(t *testing.T) {
	t.Parallel()
	code, out, err := configtest.Rerun(t.Context(), deepestFrame)
	if err != nil || code != 0 {
		t.Fatalf("the child that measures the deepest call ended %d, %v:\n%s", code, err, out)
	}
}

// measureDeepestFrame is the stack row's measure, made in its child: a call
// of the module that recurses to the ceiling in frames of FrameLimit allocates
// within the bound, and at least its slots.
func measureDeepestFrame(t *testing.T) {
	t.Helper()
	m := deepModule(LocalsLimit, FrameLimit-LocalsLimit-2, callFrames-2)
	frame, err := withinFrameLimit(m.bytes(), FrameLimit)
	if err != nil || frame != FrameLimit {
		t.Fatalf("withinFrameLimit = %d, %v; want the module read, a frame of FrameLimit", frame, err)
	}
	got := deepestAlloc(t, m)
	slots := uint64(callFrames * slotSize * frame)
	t.Logf("a frame of %d slots: %d bytes allocated to the deepest of %d frames, %.2f x %d x %d x the frame",
		frame, got, callFrames, float64(got)/float64(slots), callFrames, slotSize)
	if bound := slots*series + callFrames*frameRecord; got > bound {
		t.Errorf("a call to the deepest frame allocated %d bytes; want within %d, %d x %d x %d x %d slots and %d bytes a frame", got, bound, series, callFrames, slotSize, frame, frameRecord)
	}
	if got < slots {
		t.Errorf("planted: a call to the deepest frame allocated %d bytes; want at least the %d its slots take, or the row does not read the stack", got, slots)
	}
}
