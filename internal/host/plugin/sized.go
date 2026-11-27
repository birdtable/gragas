package plugin

import (
	"fmt"

	"github.com/birdtable/gragas/internal/port"
)

// withinItsBytes is nil for a module each of whose exports, element segments,
// data segments and names lies whole within its section; the refusal
// otherwise matches port.ErrModule and names the section. wazero v1.12.0's
// decoder makes a slice of each count and length it reads before it reads
// what they count, and reads them from the whole module, not from their
// section (internal/wasm/binary: section.go, element.go, data.go, names.go,
// and decodeUTF8 in value.go): a 13-byte module that declares 2^26 exports
// took 4 GiB of the host's memory to refuse, and a count reaches 2^32-1.
// This pass reads each entry in turn,
// before wazero does, so a count past the bytes there are costs nothing, and
// once it passes, each slice wazero makes holds entries the module has. The
// type, import, function and global sections are read so by withinFrameLimit,
// and the code section by it and withinLocalsLimit; the table and memory
// sections hold at most one entry (RuntimeConfig). What entries a module has
// still cost is held by pluginbundle's 32 MiB cap on a module: wazero
// allocates up to 40 times the bytes of one-byte element indexes, 28 of
// empty data segments and 24 of exports, where 6.7 million empty functions
// take 67. standin.wasm (go1.27.4) has 2 exports, one element segment of
// 2851 entries, 48,679 data segments and 2851 functions' names.
func withinItsBytes(wasm []byte) error {
	const customSection, exportSection, elementSection, dataSection = 0, 7, 9, 11
	err := eachSection(wasm, func(id byte, body []byte) error {
		r := &reader{b: body}
		switch id {
		case customSection:
			r.custom()
		case exportSection:
			r.each(func() {
				r.name()
				r.byte()
				r.index()
			})
		case elementSection:
			r.each(r.element)
		case dataSection:
			r.each(r.data)
		}
		if r.err != nil {
			return fmt.Errorf("section %d: %w", id, r.err)
		}
		return nil
	})
	if err != nil {
		err = fmt.Errorf("%w: its sections cannot be read: %w", port.ErrModule, err)
	}
	return err
}

// name reads a name: its length, then that many bytes.
func (r *reader) name() { r.bytes(r.count()) }

// custom reads a custom section's body: its name and, for the name section,
// which wazero decodes, each subsection, held to its own bytes. Of the
// subsections, wazero reads the module's name, the functions' names and the
// locals' names, and skips the rest.
func (r *reader) custom() {
	if string(r.bytes(r.count())) != "name" {
		return
	}
	const moduleName, functionNames, localNames = 0, 1, 2
	for len(r.b) > 0 {
		id := r.byte()
		sub := &reader{b: r.bytes(r.index())}
		switch id {
		case moduleName:
			sub.name()
		case functionNames:
			sub.each(sub.indexedName)
		case localNames:
			sub.each(func() {
				sub.index()
				sub.each(sub.indexedName)
			})
		}
		if sub.err != nil {
			r.fail(fmt.Sprintf("its name subsection %d: %s", id, sub.err))
		}
	}
}

// indexedName reads an index and its name, one entry of a name map.
func (r *reader) indexedName() {
	r.index()
	r.name()
}

// element reads one element segment, of any of its eight forms. Bit 0 of its
// flag marks a segment with no offset; bit 1 a table index before the offset,
// or, with bit 0, a declared segment; and bit 2 constant expressions in place
// of function indexes. A flag with bit 0 or 1 set has an element kind, or a
// reference type, before its entries.
func (r *reader) element() {
	const noOffset, tableIndex, exprs = 1, 2, 4
	flag := r.index()
	if flag > noOffset|tableIndex|exprs {
		r.fail("it has an element segment of no form this reading knows")
		return
	}
	if flag&noOffset == 0 {
		if flag&tableIndex != 0 {
			r.index()
		}
		r.constExpr()
	}
	if flag&(noOffset|tableIndex) != 0 {
		r.byte()
	}
	if flag&exprs != 0 {
		r.each(r.constExpr)
		return
	}
	r.each(func() { r.index() })
}

// data reads one data segment: its flag; an offset for an active segment, after
// a memory index for a flag of 2; and its bytes.
func (r *reader) data() {
	const active, passive, memoryIndex = 0, 1, 2
	switch r.index() {
	case active:
		r.constExpr()
	case passive:
	case memoryIndex:
		r.index()
		r.constExpr()
	default:
		r.fail("it has a data segment of no form this reading knows")
	}
	r.name()
}
