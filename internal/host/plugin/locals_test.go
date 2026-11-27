package plugin

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/port"
)

// localed is a command that does nothing, beside which the module defines a
// function of each count of locals, after _start.
func localed(counts ...int) mod {
	m := wasiMod(1, nil)
	for _, n := range counts {
		m.funcs = append(m.funcs, fn{typ: 2, locals: n})
	}
	return m
}

// TestAFunctionPastTheLocalsLimitIsRefusedAtOpen: a module one of whose
// functions declares more than LocalsLimit locals is refused by check C5,
// before it compiles, the refusal naming the function and its count; one at
// the limit compiles.
func TestAFunctionPastTheLocalsLimitIsRefusedAtOpen(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		counts []int
		named  string
	}{
		{"one past the limit", []int{LocalsLimit + 1}, "function 1 of its code section declares more than the 1024 locals"},
		{"far past the limit, in a later function", []int{1, 1, 1 << 24}, "function 3 of its code section declares more than the 1024 locals"},
		{"planted: at the limit", []int{LocalsLimit}, ""},
		{"planted: none", []int{0}, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			err := compiling(t, RuntimeConfig(), localed(row.counts...))
			switch {
			case row.named == "" && err != nil:
				t.Errorf("functions of %v locals = %v; want them compiled", row.counts, err)
			case row.named != "" && (!errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), row.named)):
				t.Errorf("functions of %v locals = %v; want port.ErrModule naming %q", row.counts, err, row.named)
			}
		})
	}
}

// TestLocalsAreCountedAcrossTheModule: the locals of all a module's functions
// are held to ModuleLocalsLimit together, so many functions within
// LocalsLimit each cannot add up past it; a module at the limit compiles.
func TestLocalsAreCountedAcrossTheModule(t *testing.T) {
	t.Parallel()
	const each = ModuleLocalsLimit / LocalsLimit
	full := make([]int, each)
	for i := range full {
		full[i] = LocalsLimit
	}
	// _start declares 4 locals of its own, so the module at the limit leaves
	// them out of its last function.
	full[each-1] -= 4
	if err := compiling(t, RuntimeConfig(), localed(full...)); err != nil {
		t.Errorf("planted: %d functions of locals up to the module's limit = %v; want them compiled", each, err)
	}
	over := append(slices.Clone(full), 1)
	want := "more than the 262144 locals a module may, all together, by function 257"
	if err := compiling(t, RuntimeConfig(), localed(over...)); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), want) {
		t.Errorf("functions of locals one past the module's limit = %v; want port.ErrModule naming %q", err, want)
	}
}

// TestTheLocalsAreReadBeforeTheModuleCompiles: a module past the locals limit
// that would not compile anyway is refused on its locals, so the count runs
// before wazero allocates any of them. Planted red: the same module within
// the limit is refused as not compiling.
func TestTheLocalsAreReadBeforeTheModuleCompiles(t *testing.T) {
	t.Parallel()
	// i64.eqz of an i32 does not compile, though every pass of Open's reads it.
	const opI64Eqz = 0x50
	broken := func(locals int) mod {
		m := wasiMod(1, nil)
		m.funcs = append(m.funcs, fn{typ: 2, locals: locals, body: code(i32c(0), op(opI64Eqz), op(opDrop))})
		return m
	}
	if err := compiling(t, RuntimeConfig(), broken(LocalsLimit+1)); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), "locals") {
		t.Errorf("a module past the locals limit, of a body that does not check = %v; want port.ErrModule on its locals", err)
	}
	if err := compiling(t, RuntimeConfig(), broken(1)); !errors.Is(err, port.ErrModule) || !strings.Contains(err.Error(), "does not compile") {
		t.Errorf("planted: a module within the locals limit, of a body that does not check = %v; want port.ErrModule, not compiling", err)
	}
}

// TestLocalsOfAFunctionAddEveryDeclaration: a function's locals are the sum
// of its declarations, each a count and a type, so several within the limit
// add up past it, a count past 31 bits is past it, and a body whose
// declarations run past its bytes is refused, not read beyond.
func TestLocalsOfAFunctionAddEveryDeclaration(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		body   []byte
		locals int
		fails  bool
	}{
		{"two declarations", append(append([]byte{0x02}, append(uleb128(3), i32)...), append(uleb128(5), i32)...), 8, false},
		{"two declarations past the limit together", append(append([]byte{0x02}, append(uleb128(600), i32)...), append(uleb128(600), i32)...), LocalsLimit + 1, false},
		{"a count past 31 bits", []byte{0x01, 0xff, 0xff, 0xff, 0xff, 0x7f, i32}, LocalsLimit + 1, false},
		{"none", []byte{0x00}, 0, false},
		{"no count", nil, 0, true},
		{"more declarations than bytes", []byte{0x7f}, 0, true},
		{"a declaration with no type", append([]byte{0x01}, uleb128(3)...), 0, true},
	} {
		locals, err := localsOf(row.body)
		if (err != nil) != row.fails || locals != row.locals {
			t.Errorf("%s: localsOf = %d, %v; want %d, failing %v", row.name, locals, err, row.locals, row.fails)
		}
	}
}

// TestACodeSectionThatCannotBeReadIsRefused: a code section cut short, or a
// function's body running past it, is refused as a module, before wazero
// reads it.
func TestACodeSectionThatCannotBeReadIsRefused(t *testing.T) {
	t.Parallel()
	head := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, row := range []struct {
		name string
		code []byte
	}{
		{"no count", nil},
		{"a body past the section", []byte{0x01, 0x10, 0x00}},
		{"a body with no locals", []byte{0x01, 0x00}},
	} {
		wasm := append(append(append([]byte(nil), head...), 10), uleb128(uint64(len(row.code)))...)
		wasm = append(wasm, row.code...)
		if err := withinLocalsLimit(wasm); !errors.Is(err, port.ErrModule) {
			t.Errorf("%s: withinLocalsLimit = %v; want port.ErrModule", row.name, err)
		}
	}
}

// TestTheGuestsLocalsAreWithinTheLimit: the locals standin.wasm declares, read
// from its code section, compile: at most 17 in a function, 13,363 in all, of
// 2851 functions (go1.27.4).
func TestTheGuestsLocalsAreWithinTheLimit(t *testing.T) {
	t.Parallel()
	const fns, largest, all = 2851, 17, 13363
	counts := make([]int, fns-1)
	rest := all - 4 - largest
	counts[0] = largest
	for i := 1; i < len(counts); i++ {
		counts[i] = min(rest, largest)
		rest -= counts[i]
	}
	if err := compiling(t, RuntimeConfig(), localed(counts...)); err != nil {
		t.Errorf("the locals of standin.wasm = %v; want them compiled", err)
	}
}
