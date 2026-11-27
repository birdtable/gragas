package plugin

import "slices"

// This file is the rows' assembler: a module of WebAssembly written out byte
// by byte, so that each row's module is read in the row and no binary file is
// held for it. It writes what the rows need and no more: functions, imports of
// functions, one memory, mutable globals, data, exports, and element segments,
// data segments and custom sections written as they are.

// The value types and the opcodes the rows use.
const (
	i32     = 0x7f
	funcref = 0x70

	opUnreachable = 0x00
	opLoop        = 0x03
	opIf          = 0x04
	opEnd         = 0x0b
	opBr          = 0x0c
	opCall        = 0x10
	opDrop        = 0x1a
	opLocalGet    = 0x20
	opLocalTee    = 0x22
	opGlobalGet   = 0x23
	opGlobalSet   = 0x24
	opI32Load     = 0x28
	opI32Load8    = 0x2d
	opI32Store    = 0x36
	opI32Store8   = 0x3a
	opMemoryGrow  = 0x40
	opI32Const    = 0x41
	opI32Eqz      = 0x45
	opI32Eq       = 0x46
	opI32Add      = 0x6a
	blockEmpty    = 0x40
)

// fnType is a function's type: its params and its results.
type fnType struct{ params, results []byte }

// wasiImport is one function imported from a module, of a type by index.
type wasiImport struct {
	module, name string
	typ          int
}

// fn is one function the module defines: its type, its locals of i32 and its
// body, without the closing end, which the assembler writes.
type fn struct {
	typ    int
	locals int
	body   []byte
}

// limits is a memory's least size in pages, and its most, or -1 for none.
type limits struct{ least, most int }

// segment is data written at offset of memory when the module is instantiated.
type segment struct {
	offset int
	data   []byte
}

// export is a name a module exports: a function's index or the memory.
type export struct {
	name  string
	kind  byte
	index int
}

const (
	exportFunc   = 0x00
	exportMemory = 0x02
)

// mod is a module to assemble.
type mod struct {
	types   []fnType
	imports []wasiImport
	// rawImports are imports written as they are, after imports: a memory,
	// table or global, which wasiImport cannot name.
	rawImports [][]byte
	funcs      []fn
	tables     []limits
	memory     *limits
	// start is the index of the function the module runs when instantiated,
	// or nil for none.
	start   *int
	globals []int32
	exports []export
	// elements are element segments written as they are, in any form.
	elements [][]byte
	data     []segment
	// rawData are data segments written as they are, after data, in any
	// form; custom are the bodies of custom sections, its name first, written
	// after every other section.
	rawData [][]byte
	custom  [][]byte
}

// bytes is m in the binary format.
func (m mod) bytes() []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	if len(m.types) > 0 {
		var b []byte
		for _, t := range m.types {
			b = append(b, 0x60)
			b = append(b, vec(t.params)...)
			b = append(b, vec(t.results)...)
		}
		out = append(out, section(1, len(m.types), b)...)
	}
	if n := len(m.imports) + len(m.rawImports); n > 0 {
		var b []byte
		for _, im := range m.imports {
			b = append(b, name(im.module)...)
			b = append(b, name(im.name)...)
			b = append(b, 0x00)
			b = append(b, uleb128(uint64(im.typ))...)
		}
		for _, raw := range m.rawImports {
			b = append(b, raw...)
		}
		out = append(out, section(2, n, b)...)
	}
	if len(m.funcs) > 0 {
		var b []byte
		for _, f := range m.funcs {
			b = append(b, uleb128(uint64(f.typ))...)
		}
		out = append(out, section(3, len(m.funcs), b)...)
	}
	if len(m.tables) > 0 {
		var b []byte
		for _, l := range m.tables {
			b = append(b, funcref)
			b = append(b, memLimits(l)...)
		}
		out = append(out, section(4, len(m.tables), b)...)
	}
	if m.memory != nil {
		out = append(out, section(5, 1, memLimits(*m.memory))...)
	}
	if len(m.globals) > 0 {
		var b []byte
		for _, g := range m.globals {
			b = append(b, i32, 0x01)
			b = append(b, i32c(g)...)
			b = append(b, opEnd)
		}
		out = append(out, section(6, len(m.globals), b)...)
	}
	if len(m.exports) > 0 {
		var b []byte
		for _, e := range m.exports {
			b = append(b, name(e.name)...)
			b = append(b, e.kind)
			b = append(b, uleb128(uint64(e.index))...)
		}
		out = append(out, section(7, len(m.exports), b)...)
	}
	if m.start != nil {
		out = append(out, 8)
		out = append(out, uleb128(uint64(len(uleb128(uint64(*m.start)))))...)
		out = append(out, uleb128(uint64(*m.start))...)
	}
	if len(m.elements) > 0 {
		out = append(out, section(9, len(m.elements), slices.Concat(m.elements...))...)
	}
	if len(m.funcs) > 0 {
		var b []byte
		for _, f := range m.funcs {
			var body []byte
			if f.locals > 0 {
				body = append(body, 0x01)
				body = append(body, uleb128(uint64(f.locals))...)
				body = append(body, i32)
			} else {
				body = append(body, 0x00)
			}
			body = append(body, f.body...)
			body = append(body, opEnd)
			b = append(b, uleb128(uint64(len(body)))...)
			b = append(b, body...)
		}
		out = append(out, section(10, len(m.funcs), b)...)
	}
	if n := len(m.data) + len(m.rawData); n > 0 {
		var b []byte
		for _, d := range m.data {
			b = append(b, 0x00)
			b = append(b, i32c(int32(d.offset))...)
			b = append(b, opEnd)
			b = append(b, uleb128(uint64(len(d.data)))...)
			b = append(b, d.data...)
		}
		b = append(b, slices.Concat(m.rawData...)...)
		out = append(out, section(11, n, b)...)
	}
	for _, c := range m.custom {
		out = append(out, 0)
		out = append(out, uleb128(uint64(len(c)))...)
		out = append(out, c...)
	}
	return out
}

// memLimits is l in the binary format, a memory's or a table's.
func memLimits(l limits) []byte {
	if l.most < 0 {
		return append([]byte{0x00}, uleb128(uint64(l.least))...)
	}
	b := append([]byte{0x01}, uleb128(uint64(l.least))...)
	return append(b, uleb128(uint64(l.most))...)
}

// section is a section of id holding n items of body.
func section(id byte, n int, body []byte) []byte {
	content := append(uleb128(uint64(n)), body...)
	return append(append([]byte{id}, uleb128(uint64(len(content)))...), content...)
}

// vec is a vector of value types.
func vec(types []byte) []byte { return append(uleb128(uint64(len(types))), types...) }

// name is a name in the binary format.
func name(s string) []byte { return append(uleb128(uint64(len(s))), s...) }

// uleb128 is v as unsigned LEB128.
func uleb128(v uint64) []byte {
	var b []byte
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b = append(b, c|0x80)
			continue
		}
		return append(b, c)
	}
}

// i32c is i32.const v.
func i32c(v int32) []byte {
	b := []byte{opI32Const}
	x := int64(v)
	for {
		c := byte(x & 0x7f)
		x >>= 7
		if (x == 0 && c&0x40 == 0) || (x == -1 && c&0x40 != 0) {
			return append(b, c)
		}
		b = append(b, c|0x80)
	}
}

// code joins pieces of a body.
func code(pieces ...[]byte) []byte {
	var b []byte
	for _, p := range pieces {
		b = append(b, p...)
	}
	return b
}

// op is one instruction with its immediates.
func op(o byte, immediates ...uint64) []byte {
	b := []byte{o}
	for _, v := range immediates {
		b = append(b, uleb128(v)...)
	}
	return b
}

// memArg is a load or store of o at offset, aligned to align (log 2).
func memArg(o byte, align, offset uint64) []byte { return op(o, align, offset) }

// wasiTypes is the types a module made by wasiMod declares first: fd_read
// and fd_write are type 0, proc_exit type 1, and _start type 2.
func wasiTypes() []fnType {
	return []fnType{
		{params: []byte{i32, i32, i32, i32}, results: []byte{i32}},
		{params: []byte{i32}},
		{},
	}
}

// wasiImports is the WASI functions a module made by wasiMod imports.
func wasiImports() []wasiImport {
	return []wasiImport{
		{module: "wasi_snapshot_preview1", name: "fd_read", typ: 0},
		{module: "wasi_snapshot_preview1", name: "fd_write", typ: 0},
		{module: "wasi_snapshot_preview1", name: "proc_exit", typ: 1},
	}
}

// The indexes of the imported functions, and of _start, the first defined.
const (
	fdRead   = 0
	fdWrite  = 1
	procExit = 2
	startFn  = 3
)

// wasiMod is a command that imports fd_read, fd_write and proc_exit, has a
// memory of least pages, and runs body as _start.
func wasiMod(least int, body []byte) mod {
	return mod{
		types:   wasiTypes(),
		imports: wasiImports(),
		funcs:   []fn{{typ: 2, locals: 4, body: body}},
		memory:  &limits{least: least, most: -1},
		exports: []export{{name: "_start", kind: exportFunc, index: startFn}, {name: "memory", kind: exportMemory}},
	}
}

// write is fd_write of the len bytes at buf to fd, with its iovec at iov and
// its count at iov+8; an errno other than 0 is a trap.
func write(fd int32, buf, length, iov int32) []byte {
	return code(
		i32c(iov), i32c(buf), memArg(opI32Store, 2, 0),
		i32c(iov), i32c(length), memArg(opI32Store, 2, 4),
		i32c(fd), i32c(iov), i32c(1), i32c(iov+8), op(opCall, fdWrite),
		op(opIf, blockEmpty), op(opUnreachable), op(opEnd),
	)
}

// tableGrow is table.grow of table 0 by n entries of null, its answer
// dropped: an instruction of reference types.
func tableGrow(n int32) []byte {
	return code([]byte{0xd0, funcref}, i32c(n), []byte{0xfc, 0x0f, 0x00}, op(opDrop))
}

// exit is proc_exit(c).
func exit(c int32) []byte { return code(i32c(c), op(opCall, procExit)) }

// echo is a command that copies stdin to stdout, 64 KiB at a time, and exits
// 0 at the end of stdin: its iovec is at 0, its count at 16, its buffer at
// 64 KiB.
func echo() mod {
	const buf, chunk = 1 << 16, 1 << 16
	return wasiMod(2, code(
		op(opLoop, blockEmpty),
		i32c(0), i32c(buf), memArg(opI32Store, 2, 0),
		i32c(0), i32c(chunk), memArg(opI32Store, 2, 4),
		i32c(0), i32c(0), i32c(1), i32c(16), op(opCall, fdRead),
		op(opIf, blockEmpty), op(opUnreachable), op(opEnd),
		i32c(16), memArg(opI32Load, 2, 0), op(opLocalTee, 0),
		op(opI32Eqz),
		op(opIf, blockEmpty), exit(0), op(opEnd),
		i32c(32), i32c(buf), memArg(opI32Store, 2, 0),
		i32c(32), op(opLocalGet, 0), memArg(opI32Store, 2, 4),
		i32c(1), i32c(32), i32c(1), i32c(40), op(opCall, fdWrite),
		op(opIf, blockEmpty), op(opUnreachable), op(opEnd),
		op(opBr, 0),
		op(opEnd),
	))
}
