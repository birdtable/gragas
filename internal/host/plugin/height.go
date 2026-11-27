package plugin

import (
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/port"
)

// The value stack of wazero's interpreter is one Go slice of 8-byte slots for
// the whole call, every frame on it: a function's parameters, its locals and
// the operands it pushes, a v128 two slots and every other value one. wazero
// grows it by append and bounds only the frames, at 2000, so a module sizes
// the host memory it takes by what its functions push. This file finds, before
// wazero compiles a module, how many slots each function's frame can take.

// shape is what the code section's functions need from the rest of the
// module: the function types, each its parameters' and results' value types
// as the type section spells them; the type index of every function, the
// imported ones first, and how many are imported; and the value type of every
// global, the imported ones first.
type shape struct {
	types    []funcType
	funcs    []int
	imported int
	globals  []byte
}

// funcType is one function type: its parameters' and its results' value
// types, one byte each.
type funcType struct{ params, results []byte }

// slots is the slots the value types vts take on wazero's value stack.
func slots(vts []byte) int {
	n := 0
	for _, vt := range vts {
		n += width(vt)
	}
	return n
}

// width is the slots one value of type vt takes: two for a v128, one for
// every other.
func width(vt byte) int {
	const v128 = 0x7b
	if vt == v128 {
		return 2
	}
	return 1
}

// withinFrameLimit is nil for a module none of whose functions can take more
// than limit slots of the value stack in one frame: its parameters, its locals
// and the most its operands can take at once; the refusal otherwise matches
// port.ErrModule and names the function by its place in the code section. It
// answers too the most any of its functions takes, which a row reads. Like
// withinLocalsLimit, it reads wasm before wazero does: a module whose sections
// cannot be read, or whose code is not code this reading knows, is refused, as
// wazero would refuse it, and the pass stops at the first slot past limit, so
// it holds itself to limit too.
func withinFrameLimit(wasm []byte, limit int) (int, error) {
	const typeSection, importSection, functionSection, globalSection, codeSection = 1, 2, 3, 6, 10
	var s shape
	most := 0
	err := eachSection(wasm, func(id byte, body []byte) error {
		r := &reader{b: body}
		switch id {
		case typeSection:
			s.readTypes(r)
		case importSection:
			s.readImports(r)
		case functionSection:
			r.each(func() {
				s.funcs = append(s.funcs, r.index())
			})
		case globalSection:
			r.each(func() {
				s.globals = append(s.globals, r.byte())
				r.byte()
				r.constExpr()
			})
		case codeSection:
			return s.eachFrame(r, limit, &most)
		}
		return r.err
	})
	if err != nil && !errors.Is(err, port.ErrModule) {
		err = fmt.Errorf("%w: its sections cannot be read: %w", port.ErrModule, err)
	}
	return most, err
}

// readTypes reads a type section's body into s.types.
func (s *shape) readTypes(r *reader) {
	const funcForm = 0x60
	r.each(func() {
		if r.byte() != funcForm && r.err == nil {
			r.fail("a type is not a function type")
		}
		params := r.bytes(r.count())
		s.types = append(s.types, funcType{params: params, results: r.bytes(r.count())})
	})
}

// readImports reads an import section's body: each function's type index into
// s.funcs and each global's value type into s.globals.
func (s *shape) readImports(r *reader) {
	const fn, table, memory, global = 0, 1, 2, 3
	r.each(func() {
		r.bytes(r.count())
		r.bytes(r.count())
		switch kind := r.byte(); kind {
		case fn:
			s.funcs = append(s.funcs, r.index())
			s.imported++
		case table:
			r.byte()
			r.limits()
		case memory:
			r.limits()
		case global:
			s.globals = append(s.globals, r.byte())
			r.byte()
		default:
			r.fail("an import is of no kind this reading knows")
		}
	})
}

// eachFrame reads a code section's body, each function's frame in turn, and
// keeps in most the most slots any takes; it stops at the first past limit.
func (s *shape) eachFrame(r *reader, limit int, most *int) error {
	defined := r.count()
	if r.err == nil && defined != len(s.funcs)-s.imported {
		return errors.New("the code section has not the function section's count of functions")
	}
	var f frame
	for i := range defined {
		size := r.index()
		body := r.bytes(size)
		if r.err != nil {
			return fmt.Errorf("function %d of the code section runs past it", i)
		}
		t := s.funcs[s.imported+i]
		if t >= len(s.types) {
			return fmt.Errorf("function %d of the code section has no type", i)
		}
		n, err := f.of(s, s.types[t], body, limit)
		switch {
		case errors.Is(err, errPastLimit):
			return fmt.Errorf("%w: function %d of its code section can take more than the %d slots of the value stack a frame may", port.ErrModule, i, limit)
		case errors.Is(err, errPastBlocks):
			return fmt.Errorf("%w: function %d of its code section nests more than the %d blocks a function may", port.ErrModule, i, BlockLimit)
		case err != nil:
			return fmt.Errorf("function %d of the code section: %w", i, err)
		}
		*most = max(*most, n)
	}
	return nil
}

// errPastLimit is a frame's error once it takes a slot past its limit, and
// errPastBlocks once its code is nested in more than BlockLimit blocks.
var (
	errPastLimit  = errors.New("past the limit")
	errPastBlocks = errors.New("past the block limit")
)

// frame is the state of the pass over one function's body, kept from one
// function to the next so its buffers are reused: the type of each local, its
// parameters first; the slots each operand on the stack takes; and the blocks
// the code is in, the function's own the first.
type frame struct {
	locals []byte
	stack  []int
	blocks []block
	height int
}

// block is one block the code is in: the operands on the stack below it, its
// parameters not counted; its type (blockType); and whether it is an if whose
// else has not come.
type block struct {
	base int
	typ  int64
	isIf bool
}

// of is the most slots the frame of a function of type t and body takes, or
// errPastLimit once that is past limit. The height counts unreachable code
// too, where a value of no known type counts one slot: wazero's interpreter
// never runs it, but its validator holds a value for each push there all the
// same.
func (f *frame) of(s *shape, t funcType, body []byte, limit int) (int, error) {
	r := &reader{b: body}
	f.locals = append(f.locals[:0], t.params...)
	r.each(func() {
		n := r.index()
		vt := r.byte()
		if len(f.locals)+n > len(t.params)+LocalsLimit {
			r.fail("its locals are past the limit")
		}
		for i := 0; i < n && r.err == nil; i++ {
			f.locals = append(f.locals, vt)
		}
	})
	if r.err != nil {
		return 0, r.err
	}
	fixed := slots(f.locals)
	if fixed > limit {
		return 0, errPastLimit
	}
	f.stack, f.blocks, f.height = f.stack[:0], f.blocks[:0], 0
	f.blocks = append(f.blocks, block{typ: blockOfFunc})
	most := fixed
	for len(f.blocks) > 0 {
		if r.err != nil {
			return 0, r.err
		}
		if len(r.b) == 0 {
			return 0, errors.New("its body ends before its last end")
		}
		if err := f.step(s, t, r); err != nil {
			return 0, err
		}
		if most = max(most, fixed+f.height); most > limit {
			return 0, errPastLimit
		}
	}
	if r.err == nil && len(r.b) != 0 {
		return 0, errors.New("its body runs on past its last end")
	}
	return most, r.err
}

// blockOfFunc is the type of a function's own block, whose results are its
// function type's.
const blockOfFunc = -1 << 40

// step reads one instruction of the body of a function of type fn and moves
// the stack as it does.
func (f *frame) step(s *shape, fn funcType, r *reader) error {
	op := r.byte()
	switch {
	case op <= 0x11:
		return f.control(s, fn, op, r)
	case op >= 0x1a && op <= 0x26:
		return f.variable(s, op, r)
	case op >= 0x28 && op <= 0x35: // loads
		r.memarg()
		f.pop(1)
		f.pushSlots(1)
	case op >= 0x36 && op <= 0x3e: // stores
		r.memarg()
		f.pop(2)
	case op == 0x3f: // memory.size
		r.byte()
		f.pushSlots(1)
	case op == 0x40: // memory.grow
		r.byte()
		f.pop(1)
		f.pushSlots(1)
	case op >= 0x41 && op <= 0x44: // the constants
		r.constant(op)
		f.pushSlots(1)
	case op >= 0x45 && op <= 0xc4: // the numeric instructions
		f.pop(numericArity(op))
		f.pushSlots(1)
	case op == 0xd0: // ref.null
		if ht := r.byte(); ht != 0x70 && ht != 0x6f && ht != 0x69 && r.err == nil {
			return errors.New("it has a ref.null of a type this reading does not know")
		}
		f.pushSlots(1)
	case op == 0xd1: // ref.is_null
		f.pop(1)
		f.pushSlots(1)
	case op == 0xd2: // ref.func
		r.index()
		f.pushSlots(1)
	case op == 0xfc:
		return f.misc(r)
	case op == 0xfd:
		return f.vector(r)
	default:
		return fmt.Errorf("it has an instruction this reading does not know, %#x", op)
	}
	return nil
}

// control reads one control instruction, 0x00 to 0x11.
func (f *frame) control(s *shape, fn funcType, op byte, r *reader) error {
	switch op {
	case 0x00, 0x0f: // unreachable, return
		f.unreachable()
	case 0x01: // nop
	case 0x02, 0x03, 0x04: // block, loop, if
		return f.open(s, op, r)
	case 0x05: // else
		b := &f.blocks[len(f.blocks)-1]
		if !b.isIf {
			return errors.New("it has an else outside an if")
		}
		params, _ := s.sig(b.typ, fn)
		f.truncate(b.base)
		f.push(params...)
		b.isIf = false
	case 0x0b: // end
		b := f.blocks[len(f.blocks)-1]
		_, results := s.sig(b.typ, fn)
		f.truncate(b.base)
		f.blocks = f.blocks[:len(f.blocks)-1]
		f.push(results...)
	case 0x0c: // br
		r.index()
		f.unreachable()
	case 0x0d: // br_if
		r.index()
		f.pop(1)
	case 0x0e: // br_table
		r.each(func() { r.index() })
		r.index()
		f.pop(1)
		f.unreachable()
	case 0x10, 0x11: // call, call_indirect
		return f.call(s, op, r)
	default:
		return fmt.Errorf("it has an instruction this reading does not know, %#x", op)
	}
	return nil
}

// open reads a block, a loop or an if, and opens it: its parameters move from
// below it to above its base.
func (f *frame) open(s *shape, op byte, r *reader) error {
	const ifOp = 0x04
	typ := r.blockType()
	if typ >= int64(len(s.types)) {
		return errors.New("it has a block of a type there is not")
	}
	if op == ifOp {
		f.pop(1)
	}
	params, _ := s.sig(typ, funcType{})
	f.pop(len(params))
	base := len(f.stack)
	f.push(params...)
	if len(f.blocks) > BlockLimit {
		return errPastBlocks
	}
	f.blocks = append(f.blocks, block{base: base, typ: typ, isIf: op == ifOp})
	return nil
}

// call reads a call or a call_indirect, which pops the callee's parameters, and
// the table index for call_indirect, and pushes its results.
func (f *frame) call(s *shape, op byte, r *reader) error {
	const callOp = 0x10
	i := r.index()
	if op == callOp {
		if r.err != nil || i >= len(s.funcs) || s.funcs[i] >= len(s.types) {
			return errors.New("it calls a function there is not")
		}
		i = s.funcs[i]
	} else {
		r.index()
		if r.err != nil || i >= len(s.types) {
			return errors.New("it calls indirectly by a type there is not")
		}
		f.pop(1)
	}
	f.pop(len(s.types[i].params))
	f.push(s.types[i].results...)
	return nil
}

// variable reads one parametric, variable or table instruction, 0x1a to 0x26.
func (f *frame) variable(s *shape, op byte, r *reader) error {
	switch op {
	case 0x1a: // drop
		f.pop(1)
	case 0x1b: // select
		f.pop(1)
		w := max(f.pop(1), f.pop(1), 1)
		f.pushSlots(w)
	case 0x1c: // select t*
		if r.byte() != 1 && r.err == nil {
			return errors.New("it has a typed select of other than one type")
		}
		vt := r.byte()
		if vt == 0x63 || vt == 0x64 {
			r.leb(5)
		}
		f.pop(3)
		f.push(vt)
	case 0x20: // local.get
		return f.local(r, 0)
	case 0x21, 0x24: // local.set, global.set
		r.index()
		f.pop(1)
	case 0x22: // local.tee
		return f.local(r, 1)
	case 0x23: // global.get
		i := r.index()
		if r.err != nil || i >= len(s.globals) {
			return errors.New("it gets a global there is not")
		}
		f.push(s.globals[i])
	case 0x25: // table.get
		r.index()
		f.pop(1)
		f.pushSlots(1)
	case 0x26: // table.set
		r.index()
		f.pop(2)
	default:
		return fmt.Errorf("it has an instruction this reading does not know, %#x", op)
	}
	return nil
}

// numericArity is how many operands a numeric instruction, 0x45 to 0xc4, pops:
// each pushes one value of one slot.
func numericArity(op byte) int {
	switch {
	case op == 0x45 || op == 0x50: // i32.eqz, i64.eqz
		return 1
	case op <= 0x66: // the comparisons
		return 2
	case op <= 0x69, op >= 0x79 && op <= 0x7b, op >= 0x8b && op <= 0x91, op >= 0x99 && op <= 0x9f, op >= 0xa7:
		return 1 // clz, ctz, popcnt, the float unary ops, the conversions and the sign extensions
	default:
		return 2
	}
}

// local reads a local's index and pops n operands and pushes the local's
// value: local.get pops none and local.tee one.
func (f *frame) local(r *reader, n int) error {
	i := r.index()
	if r.err != nil || i >= len(f.locals) {
		return errors.New("it names a local there is not")
	}
	f.pop(n)
	f.push(f.locals[i])
	return nil
}

// misc reads one instruction past the 0xfc prefix, whose opcode is an
// unsigned LEB128 number, as wazero reads it.
func (f *frame) misc(r *reader) error {
	op := r.index()
	switch {
	case op <= 7: // the saturating truncations
		f.pop(1)
		f.pushSlots(1)
	case op == 8: // memory.init
		r.index()
		r.byte()
		f.pop(3)
	case op == 9 || op == 13: // data.drop, elem.drop
		r.index()
	case op == 10: // memory.copy
		r.byte()
		r.byte()
		f.pop(3)
	case op == 11: // memory.fill
		r.byte()
		f.pop(3)
	case op == 12 || op == 14: // table.init, table.copy
		r.index()
		r.index()
		f.pop(3)
	case op == 15: // table.grow
		r.index()
		f.pop(2)
		f.pushSlots(1)
	case op == 16: // table.size
		r.index()
		f.pushSlots(1)
	case op == 17: // table.fill
		r.index()
		f.pop(3)
	default:
		if r.err == nil {
			return fmt.Errorf("it has a 0xfc instruction this reading does not know, %d", op)
		}
	}
	return nil
}

// vector reads one instruction past the 0xfd prefix. wazero v1.12.0 reads its
// opcode as one byte, not as the LEB128 number the format says: an opcode past
// 0x7f is two bytes there, and wazero reads its second, 0x01, as a nop. This
// reads it as wazero does, so the two never part on where an instruction ends.
func (f *frame) vector(r *reader) error {
	op := r.byte()
	switch vec := vectorKind(op); vec {
	case vecLoad:
		r.memarg()
		f.pop(1)
		f.pushSlots(2)
	case vecStore:
		r.memarg()
		f.pop(2)
	case vecConst:
		r.bytes(16)
		f.pushSlots(2)
	case vecShuffle:
		r.bytes(16)
		f.pop(2)
		f.pushSlots(2)
	case vecExtract:
		r.byte()
		f.pop(1)
		f.pushSlots(1)
	case vecReplace:
		r.byte()
		f.pop(2)
		f.pushSlots(2)
	case vecLoadLane:
		r.memarg()
		r.byte()
		f.pop(2)
		f.pushSlots(2)
	case vecStoreLane:
		r.memarg()
		r.byte()
		f.pop(2)
	case vecUnary, vecSplat:
		f.pop(1)
		f.pushSlots(2)
	case vecBinary, vecShift:
		f.pop(2)
		f.pushSlots(2)
	case vecTernary:
		f.pop(3)
		f.pushSlots(2)
	case vecTest:
		f.pop(1)
		f.pushSlots(1)
	default:
		if r.err == nil {
			return fmt.Errorf("it has a 0xfd instruction this reading does not know, %#x", op)
		}
	}
	return nil
}

// vecKind is what a vector instruction reads and how it moves the stack.
type vecKind int

// The kinds of vector instruction, from wazero v1.12.0's validator.
const (
	vecNone      vecKind = iota
	vecLoad              // a memarg; i32 to v128
	vecStore             // a memarg; i32 and v128 to none
	vecConst             // 16 bytes; none to v128
	vecShuffle           // 16 lanes; two v128 to v128
	vecExtract           // a lane; v128 to one slot
	vecReplace           // a lane; v128 and a scalar to v128
	vecLoadLane          // a memarg and a lane; i32 and v128 to v128
	vecStoreLane         // a memarg and a lane; i32 and v128 to none
	vecSplat             // a scalar to v128
	vecUnary             // v128 to v128
	vecBinary            // two v128 to v128
	vecShift             // v128 and i32 to v128
	vecTernary           // three v128 to v128
	vecTest              // v128 to i32
)

// vectorKind is the kind of the vector instruction op, or vecNone for an
// opcode wazero v1.12.0 does not know. The groups are its validator's.
func vectorKind(op byte) vecKind {
	switch op {
	case 0x0b:
		return vecStore
	case 0x0c:
		return vecConst
	case 0x0d:
		return vecShuffle
	case 0x52:
		return vecTernary
	case 0x53, 0x63, 0x64, 0x83, 0x84, 0xa3, 0xa4, 0xc3, 0xc4:
		return vecTest
	case 0x15, 0x16, 0x18, 0x19, 0x1b, 0x1d, 0x1f, 0x21:
		return vecExtract
	case 0x17, 0x1a, 0x1c, 0x1e, 0x20, 0x22:
		return vecReplace
	case 0x6b, 0x6c, 0x6d, 0x8b, 0x8c, 0x8d, 0xab, 0xac, 0xad, 0xcb, 0xcc, 0xcd:
		return vecShift
	case 0x4d, 0x5e, 0x5f, 0x60, 0x61, 0x62, 0x67, 0x68, 0x69, 0x6a, 0x74, 0x75, 0x7a,
		0x7c, 0x7d, 0x7e, 0x7f, 0x80, 0x81, 0x87, 0x88, 0x89, 0x8a, 0x94, 0xa0, 0xa1,
		0xa7, 0xa8, 0xa9, 0xaa, 0xc0, 0xc1, 0xc7, 0xc8, 0xc9, 0xca, 0xe0, 0xe1, 0xe3,
		0xec, 0xed, 0xef, 0xf8, 0xf9, 0xfa, 0xfb, 0xfc, 0xfd, 0xfe, 0xff:
		return vecUnary
	case 0x0e, 0x4e, 0x4f, 0x50, 0x51, 0x65, 0x66, 0x6e, 0x6f, 0x70, 0x71, 0x72, 0x73,
		0x76, 0x77, 0x78, 0x79, 0x7b, 0x82, 0x85, 0x86, 0x8e, 0x8f, 0x90, 0x91, 0x92,
		0x93, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9b, 0x9c, 0x9d, 0x9e, 0x9f, 0xae, 0xb1,
		0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xbc, 0xbd, 0xbe, 0xbf, 0xce, 0xd1, 0xd5,
		0xdc, 0xdd, 0xde, 0xdf, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xeb, 0xf0,
		0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7:
		return vecBinary
	}
	switch {
	case op <= 0x0a, op == 0x5c, op == 0x5d:
		return vecLoad
	case op >= 0x0f && op <= 0x14:
		return vecSplat
	case op >= 0x23 && op <= 0x4c, op >= 0xd6 && op <= 0xdb:
		return vecBinary
	case op >= 0x54 && op <= 0x57:
		return vecLoadLane
	case op >= 0x58 && op <= 0x5b:
		return vecStoreLane
	}
	return vecNone
}

// sig is the parameters and results of a block of type typ in a function of
// type fn, as blockType reads it; open has refused a type index there is not.
func (s *shape) sig(typ int64, fn funcType) (params, results []byte) {
	switch {
	case typ == blockOfFunc:
		return nil, fn.results
	case typ >= 0 && typ < int64(len(s.types)):
		return s.types[typ].params, s.types[typ].results
	case typ < 0 && typ != -64: // one value, its type the byte; -64, 0x40, is none
		return nil, []byte{byte(typ & 0x7f)}
	default:
		return nil, nil
	}
}

// unreachable is the code after a branch: the stack drops to the block's base.
func (f *frame) unreachable() {
	f.truncate(f.blocks[len(f.blocks)-1].base)
}

// truncate drops the stack to n operands.
func (f *frame) truncate(n int) {
	for len(f.stack) > n {
		f.height -= f.stack[len(f.stack)-1]
		f.stack = f.stack[:len(f.stack)-1]
	}
}

// pop drops n operands and answers the slots the last of them took, or 0 when
// there was none above the block's base. Past the base, the code cannot be
// reached (after a branch the stack's values are of any type) or is an
// underflow wazero refuses, so the pass keeps the stack at the base, never
// below it: the height it finds can only be the higher for it.
func (f *frame) pop(n int) int {
	base := f.blocks[len(f.blocks)-1].base
	w := 0
	for range n {
		if len(f.stack) <= base {
			return 0
		}
		w = f.stack[len(f.stack)-1]
		f.height -= w
		f.stack = f.stack[:len(f.stack)-1]
	}
	return w
}

// push pushes one operand of each value type in vts.
func (f *frame) push(vts ...byte) {
	for _, vt := range vts {
		f.pushSlots(width(vt))
	}
}

// pushSlots pushes one operand of w slots.
func (f *frame) pushSlots(w int) {
	f.stack = append(f.stack, w)
	f.height += w
}

// reader reads a body that nothing has checked: each read is held to the
// bytes there are, and the first that runs past them sets err, after which
// every read answers zero.
type reader struct {
	b   []byte
	err error
}

// fail sets err, if it is not set yet.
func (r *reader) fail(why string) {
	if r.err == nil {
		r.err = errors.New(why)
	}
	r.b = nil
}

// byte reads one byte.
func (r *reader) byte() byte {
	if len(r.b) == 0 {
		r.fail("it runs past its end")
		return 0
	}
	c := r.b[0]
	r.b = r.b[1:]
	return c
}

// bytes reads n bytes.
func (r *reader) bytes(n int) []byte {
	if n < 0 || n > len(r.b) {
		r.fail("it runs past its end")
		return nil
	}
	b := r.b[:n]
	r.b = r.b[n:]
	return b
}

// index reads an unsigned LEB128 number of at most 31 bits: an index, a
// count or a size. One past 31 bits names nothing a module has.
func (r *reader) index() int {
	v, n := uleb(r.b)
	if n == 0 {
		r.fail("it has a number that is not whole, or past 31 bits")
		return 0
	}
	r.b = r.b[n:]
	return v
}

// count reads a vector's count, as index does.
func (r *reader) count() int { return r.index() }

// each reads a vector's count and calls f that many times, stopping at the
// first read that fails, so a count past the bytes there are costs nothing.
func (r *reader) each(f func()) {
	for n, i := r.count(), 0; i < n && r.err == nil; i++ {
		f()
	}
}

// leb skips one LEB128 number of at most n bytes, whatever its value.
func (r *reader) leb(n int) {
	for i := 0; i < n && i < len(r.b); i++ {
		if r.b[i]&0x80 == 0 {
			r.b = r.b[i+1:]
			return
		}
	}
	r.fail("it has a number that is not whole")
}

// memarg skips a memory instruction's alignment and offset.
func (r *reader) memarg() {
	r.leb(5)
	r.leb(5)
}

// limits skips a table's or a memory's limits: a flag, the least size and,
// for a flag of 1 or 3, the most.
func (r *reader) limits() {
	flag := r.byte()
	if flag > 3 && r.err == nil {
		r.fail("it has limits of no flag this reading knows")
	}
	r.leb(5)
	if flag&1 == 1 {
		r.leb(5)
	}
}

// blockType reads a block type, a signed LEB128 number of 33 bits: a type
// index, or a negative one-byte value type, or -64 for none. A reference type
// with a heap type after it (0x63, 0x64) reads the heap type too.
func (r *reader) blockType() int64 {
	var v int64
	for i := range 5 {
		c := r.byte()
		v |= int64(c&0x7f) << (7 * i)
		if c&0x80 == 0 {
			if c&0x40 != 0 {
				v -= 1 << (7 * (i + 1))
			}
			if v == -29 || v == -28 { // 0x63, 0x64
				r.leb(5)
			}
			return v
		}
	}
	r.fail("it has a block type that is not whole")
	return 0
}

// constant skips the immediate of a constant instruction, 0x41 to 0x44.
func (r *reader) constant(op byte) {
	switch op {
	case 0x41:
		r.leb(5)
	case 0x42:
		r.leb(10)
	case 0x43:
		r.bytes(4)
	default:
		r.bytes(8)
	}
}

// constExpr skips a constant expression, to its end, as wazero's decoder reads
// one: the constants, global.get, ref.null, ref.func and v128.const.
func (r *reader) constExpr() {
	for r.err == nil {
		switch op := r.byte(); op {
		case 0x0b:
			return
		case 0x41, 0x42, 0x43, 0x44:
			r.constant(op)
		case 0x23, 0xd2:
			r.leb(5)
		case 0xd0:
			if ht := r.byte(); ht != 0x70 && ht != 0x6f && ht != 0x69 {
				r.fail("it has a ref.null of a type this reading does not know")
			}
		case 0xfd:
			if r.byte() != 0x0c {
				r.fail("it has a constant expression this reading does not know")
			}
			r.bytes(16)
		default:
			r.fail("it has a constant expression this reading does not know")
		}
	}
}
