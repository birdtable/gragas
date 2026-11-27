package plugin

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/tetratelabs/wazero"
)

// The value types beside i32 and v128, which the frame rows name already.
const (
	i64 = 0x7e
	f32 = 0x7d
	f64 = 0x7c
)

// accepted is one instruction and one shape wazero compiles: the operands
// pushed before it, in order, and the function's one result or none after it.
// instr is the instruction as the pass reads it, its prefix, its opcode and
// its immediates.
type accepted struct {
	instr, args, result []byte
}

// The opcodes the differential reads past each prefix.
const (
	opSelectT  = 0x1c
	firstOther = 0x12 // the first opcode past the control instructions
	lastOther  = 0xfb // the last before the 0xfc, 0xfd, 0xfe and 0xff prefixes
	miscOps    = 32
	longestImm = 16 // v128.const's and i8x16.shuffle's
)

// valueTypes is the five value types the host's features take.
func valueTypes() []byte { return []byte{i32, i64, f32, f64, v128} }

// constOf is an instruction that pushes one zero of vt.
func constOf(vt byte) []byte {
	switch vt {
	case i64:
		return []byte{0x42, 0x00}
	case f32:
		return append([]byte{0x43}, make([]byte, 4)...)
	case f64:
		return append([]byte{0x44}, make([]byte, 8)...)
	case v128:
		return v128c()
	default:
		return i32c(0)
	}
}

// argLists is each list of operands a non-control instruction may take, the
// shortest first: none, each type, each pair, each pair of a type and an
// i32 (select's and the stores' order), and three v128 (bitselect's).
func argLists() [][]byte {
	vts := valueTypes()
	lists := [][]byte{{}}
	for _, a := range vts {
		lists = append(lists, []byte{a})
	}
	for _, a := range vts {
		for _, b := range vts {
			lists = append(lists, []byte{a, b})
		}
	}
	for _, a := range vts {
		lists = append(lists, []byte{a, a, i32})
	}
	return append(lists, []byte{v128, v128, v128})
}

// resultLists is none, then each value type alone.
func resultLists() [][]byte {
	out := [][]byte{{}}
	for _, vt := range valueTypes() {
		out = append(out, []byte{vt})
	}
	return out
}

// probeModule is a module of one function, of type [] -> result, with one i32
// local, beside an i32 global, a memory, a funcref table, a passive element
// segment and a passive data segment, so every instruction of the host's
// features has what it names. The body is a const of each of args, instr,
// and the end.
func probeModule(args, instr, result []byte) []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	out = append(out, section(1, 1, append([]byte{0x60, 0x00}, vec(result)...))...)
	out = append(out, section(3, 1, []byte{0x00})...)
	out = append(out, section(4, 1, []byte{funcref, 0x00, 0x01})...)
	out = append(out, section(5, 1, []byte{0x00, 0x01})...)
	out = append(out, section(6, 1, code([]byte{i32, 0x01}, i32c(0), op(opEnd)))...)
	out = append(out, section(9, 1, []byte{0x01, 0x00, 0x01, 0x00})...)
	out = append(out, section(12, 1, nil)...)
	body := []byte{0x01, 0x01, i32}
	for _, a := range args {
		body = append(body, constOf(a)...)
	}
	body = append(append(body, instr...), opEnd)
	out = append(out, section(10, 1, append(uleb128(uint64(len(body))), body...))...)
	return append(out, section(11, 1, []byte{0x01, 0x00})...)
}

// oracle compiles probe modules in one runtime of the host's features.
type oracle struct {
	t   *testing.T
	ctx context.Context
	rt  wazero.Runtime
}

// newOracle is an oracle whose runtime closes with t.
func newOracle(t *testing.T) *oracle {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	rt := wazero.NewRuntimeWithConfig(ctx, RuntimeConfig())
	t.Cleanup(func() { _ = rt.Close(ctx) })
	return &oracle{t: t, ctx: ctx, rt: rt}
}

// compiles says whether wazero compiles the probe module of args, instr and
// result.
func (o *oracle) compiles(args, instr, result []byte) bool {
	cm, err := o.rt.CompileModule(o.ctx, probeModule(args, instr, result))
	if err != nil {
		return false
	}
	_ = cm.Close(o.ctx)
	return true
}

// shapes is every shape of instr that wazero compiles.
func (o *oracle) shapes(instr []byte) []accepted {
	var out []accepted
	for _, args := range argLists() {
		for _, result := range resultLists() {
			if o.compiles(args, instr, result) {
				out = append(out, accepted{instr: instr, args: args, result: result})
			}
		}
	}
	return out
}

// immediates is how many zero bytes of immediates follow head, as wazero
// reads it, or -1 when wazero compiles head with none of them. Zeros past the
// immediates are unreachable instructions, after which any shape compiles, so
// it is found in two steps. With longestImm zeros, the shortest operands that
// compile are the ones head pops, all of them; then the fewest zeros with
// which those operands compile, to none or one result, are its immediates:
// fewer are read past the body's end, which wazero refuses.
func (o *oracle) immediates(head []byte) int {
	pops := o.pops(head)
	if pops == nil {
		return -1
	}
	for n := range longestImm + 1 {
		instr := append(slices.Clone(head), make([]byte, n)...)
		for _, result := range resultLists() {
			if o.compiles(pops, instr, result) {
				return n
			}
		}
	}
	return -1
}

// pops is the shortest operands head compiles with, given longestImm zeros
// after it, or nil for none.
func (o *oracle) pops(head []byte) []byte {
	instr := append(slices.Clone(head), make([]byte, longestImm)...)
	for _, results := range [][][]byte{{{}}, resultLists()} {
		for _, args := range argLists() {
			for _, result := range results {
				if o.compiles(args, instr, result) {
					return args
				}
			}
		}
	}
	return nil
}

// heads is each opcode the pass and wazero may read past the control
// instructions, with its prefix, and the immediates each takes apart from
// zeros: a typed select of each value type.
func heads() (zeros, fixed [][]byte) {
	for op := firstOther; op <= lastOther; op++ {
		if op != opSelectT {
			zeros = append(zeros, []byte{byte(op)})
		}
	}
	for op := range miscOps {
		zeros = append(zeros, append([]byte{opMisc}, uleb128(uint64(op))...))
	}
	for op := range 256 {
		zeros = append(zeros, []byte{opVec, byte(op)})
	}
	for _, vt := range valueTypes() {
		fixed = append(fixed, []byte{opSelectT, 1, vt})
	}
	return zeros, fixed
}

// acceptedByWazero is every instruction and shape wazero compiles under the
// host's features, and the count of instructions they are of. It fails t if
// an instruction compiles with every shape, which would mean zeros past its
// immediates were read as unreachable.
func acceptedByWazero(t *testing.T) ([]accepted, int) {
	t.Helper()
	o := newOracle(t)
	zeros, fixed := heads()
	var instrs [][]byte
	for _, head := range zeros {
		if n := o.immediates(head); n >= 0 {
			instrs = append(instrs, append(slices.Clone(head), make([]byte, n)...))
		}
	}
	var out []accepted
	all := len(argLists()) * len(resultLists())
	for _, instr := range append(instrs, fixed...) {
		got := o.shapes(instr)
		if len(got) == all {
			t.Fatalf("% x compiles with every shape; want its immediates read, not unreachable", instr)
		}
		out = append(out, got...)
	}
	return out, len(instrs) + len(fixed)
}

// stepper is one step of the pass: frame.step, or a plant of it.
type stepper func(f *frame, r *reader) error

// The padding below each shape: a sentinel's slots are unlike any operand's,
// and four are more than any instruction pops.
const (
	padding  = 4
	sentinel = 100
)

// padded is a frame of padding sentinels, in the function's block, and in a
// block of their own above them when inBlock, as after unreachable code.
func padded(inBlock bool) *frame {
	f := &frame{locals: []byte{i32}, blocks: []block{{base: 0, typ: blockOfFunc}}}
	for range padding {
		f.pushSlots(sentinel)
	}
	if inBlock {
		f.blocks = append(f.blocks, block{base: padding, typ: -64})
	}
	return f
}

// kept says whether f keeps its padding as it was.
func kept(f *frame) bool {
	return len(f.stack) >= padding && !slices.ContainsFunc(f.stack[:padding], func(w int) bool { return w != sentinel })
}

// differences is each case where step parts from wazero. Reached, from the
// padding in the function's block: it reads all of the instruction, keeps the
// padding, pops no more than it should and leaves the slots of the result.
// Unreached, in a block above the padding with nothing on it: it reads all of
// the instruction and pops nothing below the block's base.
func differences(cases []accepted, step stepper) []string {
	var out []string
	for _, c := range cases {
		of := fmt.Sprintf("% x of [% x] to [% x]", c.instr, c.args, c.result)
		f := padded(false)
		f.push(c.args...)
		r := &reader{b: slices.Clone(c.instr)}
		err := step(f, r)
		left := 0
		if len(f.stack) >= padding {
			left = sumOf(f.stack[padding:])
		}
		switch {
		case err != nil || r.err != nil || len(r.b) != 0:
			out = append(out, fmt.Sprintf("%s: read with %v, %v and %d bytes left", of, err, r.err, len(r.b)))
		case !kept(f):
			out = append(out, fmt.Sprintf("%s: popped below its operands, to %v", of, f.stack))
		case left != slots(c.result) || f.height != sumOf(f.stack):
			out = append(out, fmt.Sprintf("%s: left %d slots, height %d; want %d", of, left, f.height-padding*sentinel, slots(c.result)))
		}
		f = padded(true)
		r = &reader{b: slices.Clone(c.instr)}
		if err := step(f, r); err != nil || r.err != nil || len(r.b) != 0 || !kept(f) {
			out = append(out, fmt.Sprintf("%s, unreached: %v, %v, %d bytes left, stack %v", of, err, r.err, len(r.b), f.stack))
		}
	}
	return out
}

// sumOf is the slots of the operands ws.
func sumOf(ws []int) int {
	n := 0
	for _, w := range ws {
		n += w
	}
	return n
}

// opOf is the opcode of instr past its prefix, and the prefix, or 0 for none.
func opOf(instr []byte) (prefix, op byte) {
	if len(instr) > 1 && (instr[0] == opMisc || instr[0] == opVec) {
		return instr[0], instr[1]
	}
	return 0, instr[0]
}

// after is a plant: step, then plant when the instruction it read is one of
// which.
func after(step stepper, which func(prefix, op byte) bool, plant func(f *frame)) stepper {
	return func(f *frame, r *reader) error {
		prefix, op := opOf(r.b)
		err := step(f, r)
		if which(prefix, op) {
			plant(f)
		}
		return err
	}
}

// popOneMore is a plant: the result goes back on after one more operand goes.
func popOneMore(f *frame) {
	w := f.pop(1)
	f.pop(1)
	f.pushSlots(w)
}

// TestThePassMovesTheStackAsWazeroDoes: for each instruction past the control
// ones that wazero v1.12.0 compiles under the host's features, each scalar opcode, each
// 0xfc and each 0xfd, and for each shape of none to three operands and none or
// one result it compiles with, the pass reads the instruction's every byte
// and moves the stack as wazero does: from padding in its block it leaves
// exactly the result's slots and keeps the padding, and in a block of nothing
// it pops nothing below the base. The control instructions have their rows in
// height_test.go. Planted red: each of the five undercounts that held the
// frame rows green turns this row red, a binary op that pops three, a vector
// unary op that pops two, a select that pushes one slot, a pop below the
// block's base, and a load that pushes nothing.
func TestThePassMovesTheStackAsWazeroDoes(t *testing.T) {
	t.Parallel()
	cases, instrs := acceptedByWazero(t)
	t.Logf("wazero compiles %d instructions in %d shapes", instrs, len(cases))
	if instrs < 400 {
		t.Fatalf("wazero compiles %d instructions; want the host's features' more than 400", instrs)
	}
	s := &shape{globals: []byte{i32}}
	pass := func(f *frame, r *reader) error { return f.step(s, funcType{}, r) }
	if diff := differences(cases, pass); len(diff) > 0 {
		t.Errorf("the pass parts from wazero in %d cases; the first:\n%s", len(diff), diff[0])
	}
	const i32Load, i64Load32U = 0x28, 0x35
	scalar := func(op byte) func(byte, byte) bool {
		return func(prefix, got byte) bool { return prefix == 0 && got == op }
	}
	for _, plant := range []struct {
		name string
		step stepper
	}{
		{"a binary op pops three", after(pass, func(prefix, op byte) bool {
			return prefix == 0 && op >= opI32Eqz && op <= 0xc4 && numericArity(op) == 2
		}, popOneMore)},
		{"a vector unary op pops two", after(pass, func(prefix, op byte) bool {
			return prefix == opVec && vectorKind(op) == vecUnary
		}, popOneMore)},
		{"a select pushes one slot", after(pass, scalar(opSelect), func(f *frame) { f.pop(1); f.pushSlots(1) })},
		{"a pop goes below the block's base", func(f *frame, r *reader) error {
			top := len(f.blocks) - 1
			base := f.blocks[top].base
			f.blocks[top].base = 0
			err := pass(f, r)
			f.blocks[top].base = base
			return err
		}},
		{"a load pushes nothing", after(pass, func(prefix, op byte) bool {
			return prefix == 0 && op >= i32Load && op <= i64Load32U
		}, func(f *frame) { f.pop(1) })},
	} {
		if diff := differences(cases, plant.step); len(diff) == 0 {
			t.Errorf("planted: %s = no case parts from wazero; want the row red", plant.name)
		}
	}
}
