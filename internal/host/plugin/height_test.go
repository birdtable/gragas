package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"

	"github.com/birdtable/gragas/internal/port"
)

// The opcodes the frame rows use beside the assembler's.
const (
	v128        = 0x7b
	opBlock     = 0x02
	opElse      = 0x05
	opSelect    = 0x1b
	opI64Eqz    = 0x50
	opMisc      = 0xfc
	opVec       = 0xfd
	miscFill    = 11
	opV128Const = 0x0c
	opShuffle   = 0x0d
)

// times is n copies of b, one after another.
func times(n int, b []byte) []byte {
	return []byte(strings.Repeat(string(b), n))
}

// v128c is a v128.const of zero.
func v128c() []byte { return append([]byte{opVec, opV128Const}, make([]byte, 16)...) }

// framed is a command that does nothing, beside which the module defines
// these functions, after _start, and declares these types after wasiMod's.
func framed(types []fnType, fns ...fn) mod {
	m := wasiMod(1, nil)
	m.types = append(m.types, types...)
	m.funcs = append(m.funcs, fns...)
	return m
}

// mostFrame is the most slots any function of m takes, as Open's pass finds
// it, failing the row if the pass refuses m or wazero does not compile it.
func mostFrame(t *testing.T, m mod) int {
	t.Helper()
	most, err := withinFrameLimit(m.bytes(), FrameLimit)
	if err != nil {
		t.Fatalf("withinFrameLimit = %v; want the module read", err)
	}
	if err := compiling(t, RuntimeConfig(), m); err != nil {
		t.Fatalf("the row's module = %v; want it compiled, so the row is WebAssembly", err)
	}
	return most
}

// TestAFrameIsItsParamsItsLocalsAndItsHighestStack: the pass finds each
// function's frame as wazero's interpreter lays it out, 8 bytes a slot and a
// v128 two, through the instructions that
// move the stack, and every row's module compiles, so each is WebAssembly
// wazero takes. _start's frame is its 4 locals, so each row is over 4.
func TestAFrameIsItsParamsItsLocalsAndItsHighestStack(t *testing.T) {
	t.Parallel()
	const results6, takesV128 = 3, 4
	types := []fnType{
		results6:  {results: []byte{i32, i32, i32, i32, i32, i32}},
		takesV128: {params: []byte{v128}},
	}[3:]
	shuffle := append([]byte{opVec, opShuffle}, make([]byte, 16)...)
	for _, row := range []struct {
		name string
		fns  []fn
		want int
	}{
		{"operands of one slot", []fn{{typ: 2, body: code(times(10, i32c(0)), times(10, op(opDrop)))}}, 10},
		{"a v128 takes two slots", []fn{{typ: 2, body: code(times(5, v128c()), times(5, op(opDrop)))}}, 10},
		{"locals and operands add", []fn{{typ: 2, locals: 3, body: code(times(4, i32c(0)), times(4, op(opDrop)))}}, 7},
		{"a v128 parameter and its value", []fn{{typ: takesV128, body: code(op(opLocalGet, 0), op(opLocalGet, 0), op(opDrop), op(opDrop))}}, 6},
		{"a call pushes its results", []fn{
			{typ: results6, body: times(6, i32c(0))},
			{typ: 2, body: code(i32c(0), op(opCall, startFn+1), times(7, op(opDrop)))},
		}, 7},
		{"a block's results are what is left at its end", []fn{{typ: 2, body: code(
			op(opBlock, i32), times(7, i32c(0)), times(6, op(opDrop)), op(opEnd),
			times(3, i32c(0)), times(4, op(opDrop)))}}, 7},
		{"the higher arm of an if", []fn{{typ: 2, body: code(
			i32c(0), op(opIf, blockEmpty), times(8, i32c(0)), times(8, op(opDrop)),
			op(opElse), times(3, i32c(0)), times(3, op(opDrop)), op(opEnd))}}, 8},
		{"a branch drops the stack to its block", []fn{{typ: 2, body: code(
			op(opBlock, blockEmpty), times(6, i32c(0)), op(opBr, 0),
			times(3, i32c(0)), times(3, op(opDrop)), op(opEnd))}}, 6},
		{"code after a branch is counted", []fn{{typ: 2, body: code(
			op(opBlock, blockEmpty), op(opBr, 0), times(9, i32c(0)), times(9, op(opDrop)), op(opEnd))}}, 9},
		{"code after unreachable is counted", []fn{{typ: 2, body: code(
			op(opUnreachable), times(9, i32c(0)), times(9, op(opDrop)))}}, 9},
		{"select of v128", []fn{{typ: 2, body: code(v128c(), v128c(), i32c(0), op(opSelect), op(opDrop))}}, 5},
		{"globals", []fn{{typ: 2, body: code(times(6, op(opGlobalGet, 0)), times(6, op(opDrop)))}}, 6},
		{"memory.fill pops three", []fn{{typ: 2, body: code(
			times(3, i32c(0)), times(3, i32c(0)), op(opMisc, miscFill, 0), times(3, op(opDrop)))}}, 6},
		{"a shuffle pops two v128 and pushes one", []fn{{typ: 2, body: code(
			times(3, v128c()), shuffle, op(opDrop), op(opDrop))}}, 6},
		// wazero reads a vector opcode as one byte, so i16x8.abs, 0x80 0x01
		// as the format writes it, is the opcode 0x80 and a nop.
		{"an opcode past 0x7f as wazero reads it", []fn{{typ: 2, body: code(
			times(3, v128c()), []byte{opVec, 0x80, 0x01}, times(3, op(opDrop)))}}, 6},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			m := framed(types, row.fns...)
			m.globals = []int32{0}
			if got := mostFrame(t, m); got != row.want {
				t.Errorf("the most a frame takes = %d slots; want %d", got, row.want)
			}
		})
	}
}

// TestAFramePastTheLimitIsRefusedAtOpen: a module one of whose functions can
// take more than FrameLimit slots of the value stack is refused by check C5 before it
// compiles, the refusal naming the function; one at the limit compiles. Each
// way a frame grows is held: operands, v128s at two slots, locals, and code
// that cannot be reached, which wazero's validator holds values for all the
// same.
func TestAFramePastTheLimitIsRefusedAtOpen(t *testing.T) {
	t.Parallel()
	consts := func(n int) []byte { return code(times(n, i32c(0)), times(n, op(opDrop))) }
	vectors := func(n int) []byte { return code(times(n, v128c()), times(n, op(opDrop))) }
	for _, row := range []struct {
		name string
		f    fn
		over bool
	}{
		{"operands one past the limit", fn{typ: 2, body: consts(FrameLimit + 1)}, true},
		{"planted: operands at the limit", fn{typ: 2, body: consts(FrameLimit)}, false},
		{"v128s one past the limit", fn{typ: 2, body: vectors(FrameLimit/2 + 1)}, true},
		{"planted: v128s at the limit", fn{typ: 2, body: vectors(FrameLimit / 2)}, false},
		{"locals and operands past the limit", fn{typ: 2, locals: 1000, body: consts(FrameLimit - 999)}, true},
		{"planted: locals and operands at the limit", fn{typ: 2, locals: 1000, body: consts(FrameLimit - 1000)}, false},
		{"unreachable code past the limit", fn{typ: 2, body: code(op(opUnreachable), consts(FrameLimit+1))}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			err := compiling(t, RuntimeConfig(), framed(nil, row.f))
			want := fmt.Sprintf("function 1 of its code section can take more than the %d slots", FrameLimit)
			switch {
			case !row.over && err != nil:
				t.Errorf("a frame at the limit = %v; want it compiled", err)
			case row.over && (!errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), want)):
				t.Errorf("a frame past the limit = %v; want port.ErrModule naming %q", err, want)
			}
		})
	}
}

// TestTheFrameIsReadBeforeTheModuleCompiles: a module past the frame limit
// that would not compile anyway is refused on its frame, so the pass runs
// before wazero's validator allocates for it. Planted red: the same module
// within the limit is refused as not compiling.
func TestTheFrameIsReadBeforeTheModuleCompiles(t *testing.T) {
	t.Parallel()
	broken := func(n int) mod {
		// i64.eqz of an i32 does not compile, though the pass reads it.
		return framed(nil, fn{typ: 2, body: code(times(n, i32c(0)), op(opI64Eqz), times(n, op(opDrop)))})
	}
	if err := compiling(t, RuntimeConfig(), broken(FrameLimit+1)); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), "slots") {
		t.Errorf("a module past the frame limit, of a body that does not check = %v; want port.ErrModule on its frame", err)
	}
	if err := compiling(t, RuntimeConfig(), broken(1)); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), "does not compile") {
		t.Errorf("planted: a module within the frame limit, of a body that does not check = %v; want port.ErrModule, not compiling", err)
	}
}

// TestBlocksPastTheLimitAreRefusedAtOpen: a function whose code is nested in
// more than BlockLimit blocks at once is refused before it compiles, since
// wazero and the pass keep a frame for each block; one at the limit compiles.
func TestBlocksPastTheLimitAreRefusedAtOpen(t *testing.T) {
	t.Parallel()
	nested := func(n int) mod {
		return framed(nil, fn{typ: 2, body: code(times(n, op(opBlock, blockEmpty)), times(n, op(opEnd)))})
	}
	want := fmt.Sprintf("function 1 of its code section nests more than the %d blocks", BlockLimit)
	if err := compiling(t, RuntimeConfig(), nested(BlockLimit+1)); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), want) {
		t.Errorf("blocks one past the limit = %v; want port.ErrModule naming %q", err, want)
	}
	if err := compiling(t, RuntimeConfig(), nested(BlockLimit)); err != nil {
		t.Errorf("planted: blocks at the limit = %v; want them compiled", err)
	}
}

// TestCodeThePassCannotReadIsCodeWazeroRefuses: the pass refuses what it does
// not know, an instruction or a body cut short, and wazero refuses each such
// module too, so the pass never turns away a module wazero would take for an
// instruction it misreads. Planted red: a body the pass reads compiles.
func TestCodeThePassCannotReadIsCodeWazeroRefuses(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		body []byte
	}{
		{"try, of exceptions", code(op(0x06, blockEmpty), op(opEnd))},
		{"return_call, of tail calls", op(0x12, startFn)},
		{"a reserved vector opcode", code(v128c(), []byte{opVec, 0x9a}, op(opDrop))},
		{"an unknown 0xfc opcode", code(i32c(0), op(opMisc, 18), op(opDrop))},
		{"a reserved opcode", []byte{0xc5}},
		{"an end too many", code(op(opEnd), op(opEnd))},
		{"a body cut inside a constant", []byte{opI32Const, 0x80}},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			wasm := framed(nil, fn{typ: 2, body: row.body}).bytes()
			if _, err := withinFrameLimit(wasm, FrameLimit); !errors.Is(err, port.ErrModule) {
				t.Errorf("withinFrameLimit = %v; want port.ErrModule", err)
			}
			ctx := context.WithoutCancel(t.Context())
			rt := wazero.NewRuntimeWithConfig(ctx, RuntimeConfig())
			t.Cleanup(func() { _ = rt.Close(ctx) })
			if _, err := rt.CompileModule(ctx, wasm); err == nil {
				t.Errorf("wazero compiled a module the pass refuses")
			}
		})
	}
	if _, err := withinFrameLimit(framed(nil, fn{typ: 2, body: code(i32c(0), op(opDrop))}).bytes(), FrameLimit); err != nil {
		t.Errorf("planted: a body the pass reads = %v; want it read", err)
	}
}

// TestThePassKnowsWazerosVectorOpcodes: the pass knows 236 vector opcodes,
// wazero v1.12.0's, and none of the 20 the format leaves free, so a vector
// instruction is read as wazero reads it or refused as wazero refuses it.
func TestThePassKnowsWazerosVectorOpcodes(t *testing.T) {
	t.Parallel()
	free := []byte{0x9a, 0xa2, 0xa5, 0xa6, 0xaf, 0xb0, 0xb2, 0xb3, 0xb4, 0xbb, 0xc2, 0xc5, 0xc6, 0xcf, 0xd0, 0xd2, 0xd3, 0xd4, 0xe2, 0xee}
	known := 0
	for op := range 256 {
		if vectorKind(byte(op)) != vecNone {
			known++
		}
	}
	if known != 236 {
		t.Errorf("the pass knows %d vector opcodes; want wazero's 236", known)
	}
	for _, op := range free {
		if vectorKind(op) != vecNone {
			t.Errorf("the pass knows the free vector opcode %#x", op)
		}
	}
}

// TestTheGuestsFramesAreWithinTheLimits: frames and blocks of standin.wasm's
// figures compile: at most 27 slots in a frame, and 549 blocks nested at
// once (go1.27.4).
func TestTheGuestsFramesAreWithinTheLimits(t *testing.T) {
	t.Parallel()
	const largest, deepest, locals = 27, 549, 5
	m := framed(nil,
		fn{typ: 1, locals: locals, body: code(times(largest-locals-1, i32c(0)), times(largest-locals-1, op(opDrop)))},
		fn{typ: 2, body: code(times(deepest, op(opBlock, blockEmpty)), times(deepest, op(opEnd)))},
	)
	if got := mostFrame(t, m); got != largest {
		t.Errorf("the frame of standin.wasm's figures = %d slots; want %d", got, largest)
	}
}
