package plugin

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/port"
)

// The sections withinItsBytes reads.
const (
	customID  = 0
	exportID  = 7
	elementID = 9
	dataID    = 11
)

// rawSection is a section of id whose body is written as it is, its count
// among it.
func rawSection(id byte, body []byte) []byte {
	return slices.Concat([]byte{id}, uleb128(uint64(len(body))), body)
}

// subsection is a subsection of the name section.
func subsection(id byte, body []byte) []byte { return rawSection(id, body) }

// offset is a constant expression of v, an active segment's offset.
func offset(v int32) []byte { return append(i32c(v), opEnd) }

// theNames is the name section's body declares writes, in its pieces: its
// name, then its module's name, its functions' names and its locals' names.
func theNames() [][]byte {
	return [][]byte{
		name("name"),
		subsection(0, name("standin")),
		subsection(1, slices.Concat(uleb128(1), uleb128(startFn), name("_start"))),
		subsection(2, slices.Concat(uleb128(1), uleb128(startFn), uleb128(1), uleb128(0), name("fd"))),
	}
}

// declares is a command each of whose sections withinItsBytes reads holds
// entries of every form wazero reads: its exports, element segments of all
// eight forms, data segments of all three, a name section of the module's
// name, the functions' and the locals', and another custom section.
func declares() mod {
	const fd = 0
	one := slices.Concat(uleb128(1), uleb128(fd))
	refFunc := []byte{0xd2, fd, opEnd}
	refNull := []byte{0xd0, funcref, opEnd}
	m := wasiMod(1, nil)
	m.tables = []limits{{least: 4, most: -1}}
	m.elements = [][]byte{
		slices.Concat([]byte{0x00}, offset(0), one),
		slices.Concat([]byte{0x01, 0x00}, one),
		slices.Concat([]byte{0x02, 0x00}, offset(1), []byte{0x00}, one),
		slices.Concat([]byte{0x03, 0x00}, one),
		slices.Concat([]byte{0x04}, offset(2), uleb128(1), refFunc),
		slices.Concat([]byte{0x05, funcref}, uleb128(1), refNull),
		slices.Concat([]byte{0x06, 0x00}, offset(3), []byte{funcref}, uleb128(1), refFunc),
		slices.Concat([]byte{0x07, funcref}, uleb128(1), refFunc),
	}
	m.rawData = [][]byte{
		slices.Concat([]byte{0x00}, offset(0), name("a")),
		slices.Concat([]byte{0x01}, name("b")),
		slices.Concat([]byte{0x02, 0x00}, offset(1), name("c")),
	}
	m.custom = [][]byte{slices.Concat(theNames()...), slices.Concat(name("producers"), []byte{0x01, 0x02})}
	return m
}

// TestWhatAModuleDeclaresIsReadWhole: a module of every form of export,
// element segment, data segment and name the pass reads compiles, so the pass
// reads each form as wazero does; and every cut of its export, element and
// data sections, and of its name section short of a subsection's end, is
// refused by the pass, so it reads every byte of each.
func TestWhatAModuleDeclaresIsReadWhole(t *testing.T) {
	t.Parallel()
	whole := declares().bytes()
	if err := compilingBytes(t, RuntimeConfig(), whole); err != nil {
		t.Fatalf("a module of every form = %v; want it compiled", err)
	}
	head := whole[:8]
	bodies := map[byte][]byte{}
	if err := eachSection(whole, func(id byte, body []byte) error {
		if id != customID {
			bodies[id] = body
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []byte{exportID, elementID, dataID} {
		for cut := range len(bodies[id]) {
			if err := withinItsBytes(slices.Concat(head, rawSection(id, bodies[id][:cut]))); !errors.Is(err, port.ErrModule) {
				t.Errorf("section %d cut to %d of its %d bytes = %v; want port.ErrModule", id, cut, len(bodies[id]), err)
			}
		}
	}
	names := slices.Concat(theNames()...)
	ends := map[int]bool{}
	for i, n := 0, 0; i < len(theNames()); i++ {
		n += len(theNames()[i])
		ends[n] = true
	}
	for cut := range len(names) {
		err := withinItsBytes(slices.Concat(head, rawSection(customID, names[:cut])))
		if ends[cut] && err != nil {
			t.Errorf("the name section cut at a subsection's end, %d bytes = %v; want it read", cut, err)
		}
		if !ends[cut] && !errors.Is(err, port.ErrModule) {
			t.Errorf("the name section cut to %d of its %d bytes = %v; want port.ErrModule", cut, len(names), err)
		}
	}
}

// TestACountPastItsBytesIsRefusedBeforeTheModuleCompiles: a count or a
// length past the bytes of its section is
// refused by the pass, before wazero makes a slice of it: each row's count is
// 2^20, which costs wazero v1.12.0 56 to 112 MB to refuse, and a count reaches
// 2^32-1. Planted red: with the pass dropped from compile, each row is
// wazero's refusal, after its slice, not the pass's.
func TestACountPastItsBytesIsRefusedBeforeTheModuleCompiles(t *testing.T) {
	t.Parallel()
	head := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	past := uleb128(1 << 20)
	names := name("name")
	for _, row := range []struct {
		name string
		id   byte
		body []byte
	}{
		{"exports", exportID, past},
		{"exports, a count of 32 bits", exportID, uleb128(1<<32 - 1)},
		{"an export's name", exportID, slices.Concat(uleb128(1), past)},
		{"element segments", elementID, past},
		{"a segment's function indexes", elementID, slices.Concat(uleb128(1), []byte{0x01, 0x00}, past)},
		{"a segment's expressions", elementID, slices.Concat(uleb128(1), []byte{0x05, funcref}, past)},
		{"an element segment of no form", elementID, slices.Concat(uleb128(1), []byte{0x08})},
		{"data segments", dataID, past},
		{"a data segment's bytes", dataID, slices.Concat(uleb128(1), []byte{0x01}, past)},
		{"a data segment of no form", dataID, slices.Concat(uleb128(1), []byte{0x03})},
		{"a custom section's name", customID, past},
		{"the module's name", customID, slices.Concat(names, subsection(0, past))},
		{"the functions' names", customID, slices.Concat(names, subsection(1, past))},
		{"a function's name", customID, slices.Concat(names, subsection(1, slices.Concat(uleb128(1), uleb128(0), past)))},
		{"a function's locals' names", customID, slices.Concat(names, subsection(2, slices.Concat(uleb128(1), uleb128(0), past)))},
		{"a subsection", customID, slices.Concat(names, []byte{0x01}, uleb128(64))},
	} {
		err := compilingBytes(t, RuntimeConfig(), slices.Concat(head, rawSection(row.id, row.body)))
		if !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), "its sections cannot be read") {
			t.Errorf("%s past the bytes of section %d = %v; want port.ErrModule, refused before it compiles", row.name, row.id, err)
		}
	}
}
