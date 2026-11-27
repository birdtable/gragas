package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// compile compiles wasm in rt and holds it to what a plugin's module is (check
// C5): it imports functions of WASI preview 1 alone, each one wasi exports and
// of its type, and nothing else, no memory, table or global; and it is a
// command, whose _start takes and answers nothing. Compiling runs no code of
// the module. A refusal matches port.ErrModule; a ctx that ended is its own
// error. Its tables are held to TableLimit, all of them together, as least
// size and as most, since a table is host memory MemoryLimit does not hold.
// It answers, beside the compiled module, the least size of its memory in
// bytes, which a call holds to MemoryLimit before it instantiates. Its
// functions' locals are held to LocalsLimit and ModuleLocalsLimit before it
// compiles, since compiling allocates them in the host's memory, and then
// each function's frame to FrameLimit and its blocks to BlockLimit, since a
// call's value stack and compiling are host memory too; and each count and
// length it declares to the bytes of its section, since wazero makes a slice
// of each before it reads what it counts.
func compile(ctx context.Context, rt wazero.Runtime, wasi map[string]api.FunctionDefinition, wasm []byte) (wazero.CompiledModule, int, error) {
	if err := withinLocalsLimit(wasm); err != nil {
		return nil, 0, err
	}
	if _, err := withinFrameLimit(wasm, FrameLimit); err != nil {
		return nil, 0, err
	}
	if err := withinItsBytes(wasm); err != nil {
		return nil, 0, err
	}
	cm, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, 0, ctxErr
		}
		return nil, 0, fmt.Errorf("%w: it does not compile: %s", port.ErrModule, plugin.Shown(err.Error()))
	}
	n, minPages, tables, err := declared(wasm)
	if err != nil {
		err = fmt.Errorf("%w: its sections cannot be read: %w", port.ErrModule, err)
	} else if err = commandOfWASI(cm, wasi, n); err == nil {
		err = withinTableLimit(tables)
	}
	if err != nil {
		return nil, 0, errors.Join(err, cm.Close(ctx))
	}
	return cm, minPages * pageSize, nil
}

// commandOfWASI is nil for a compiled module of n imports that imports WASI
// preview 1's functions alone and is a command, and the refusal otherwise.
func commandOfWASI(cm wazero.CompiledModule, wasi map[string]api.FunctionDefinition, n int) error {
	fns := cm.ImportedFunctions()
	if n != len(fns) {
		return fmt.Errorf("%w: it imports %d things, of which %d are functions: a memory, table or global is never WASI's", port.ErrModule, n, len(fns))
	}
	for _, fn := range fns {
		mod, name, _ := fn.Import()
		if mod != wasi_snapshot_preview1.ModuleName {
			return fmt.Errorf("%w: it imports %s from %s, which is not WASI preview 1", port.ErrModule, plugin.Shown(name), plugin.Shown(mod))
		}
		def, ok := wasi[name]
		if !ok {
			return fmt.Errorf("%w: it imports %s, which WASI preview 1 has not", port.ErrModule, plugin.Shown(name))
		}
		if !slices.Equal(fn.ParamTypes(), def.ParamTypes()) || !slices.Equal(fn.ResultTypes(), def.ResultTypes()) {
			return fmt.Errorf("%w: it imports %s as another type than WASI preview 1's", port.ErrModule, plugin.Shown(name))
		}
	}
	start, ok := cm.ExportedFunctions()["_start"]
	if !ok || len(start.ParamTypes()) != 0 || len(start.ResultTypes()) != 0 {
		return fmt.Errorf("%w: it is no command: it exports no _start that takes and answers nothing", port.ErrModule)
	}
	return nil
}

// declared is what wasm declares that the compiled module does not show:
// the number of its imports, of every kind, from its import section (the
// compiled module names its functions and memories, and no other kind); the
// least size of its memory in pages, from its memory section, or 0 for a
// module with none; and its tables, from its table section. wasm is a module
// wazero has compiled, so its sections are whole and in order, and it declares
// at most one memory; each read is still held to the bytes there are.
func declared(wasm []byte) (imports, minPages int, tables []table, err error) {
	err = eachSection(wasm, func(id byte, body []byte) error {
		const importSection, tableSection, memorySection = 2, 4, 5
		var err error
		switch id {
		case importSection:
			count, m := uleb(body)
			if m == 0 {
				return errors.New("the import section has no count")
			}
			imports = count
		case tableSection:
			tables, err = tablesOf(body)
		case memorySection:
			minPages, err = leastPages(body)
		}
		return err
	})
	if err != nil {
		return 0, 0, nil, err
	}
	return imports, minPages, tables, nil
}

// eachSection hands f the id and body of each section of wasm, in order, and
// answers f's first error, or an error for a module whose sections do not lie
// whole within it. Every read is held to the bytes there are, so wasm may be a
// module nothing has checked.
func eachSection(wasm []byte, f func(id byte, body []byte) error) error {
	const header = 8
	if len(wasm) < header {
		return errors.New("the module is shorter than its header")
	}
	rest := wasm[header:]
	for len(rest) > 0 {
		id := rest[0]
		size, n := uleb(rest[1:])
		if n == 0 || size > len(rest)-1-n {
			return fmt.Errorf("section %d runs past the module", id)
		}
		body := rest[1+n : 1+n+size]
		rest = rest[1+n+size:]
		if err := f(id, body); err != nil {
			return err
		}
	}
	return nil
}

// withinLocalsLimit is nil for a module none of whose functions declares more
// than LocalsLimit locals and whose functions declare at most
// ModuleLocalsLimit together; the refusal otherwise matches port.ErrModule
// and names the function, by its place in the code section. It reads wasm
// before wazero does, so a module whose code section cannot be read is
// refused too, as wazero would refuse it, and the count stops at the first
// function past either limit: a declared count is never added up past them.
func withinLocalsLimit(wasm []byte) error {
	const codeSection = 10
	total := 0
	err := eachSection(wasm, func(id byte, body []byte) error {
		if id != codeSection {
			return nil
		}
		count, m := uleb(body)
		if m == 0 {
			return errors.New("the code section has no count")
		}
		body = body[m:]
		for i := range count {
			size, k := uleb(body)
			if k == 0 || size > len(body)-k {
				return fmt.Errorf("function %d of the code section runs past it", i)
			}
			locals, err := localsOf(body[k : k+size])
			switch {
			case err != nil:
				return fmt.Errorf("function %d of the code section: %w", i, err)
			case locals > LocalsLimit:
				return fmt.Errorf("%w: function %d of its code section declares more than the %d locals a function may", port.ErrModule, i, LocalsLimit)
			}
			if total += locals; total > ModuleLocalsLimit {
				return fmt.Errorf("%w: its functions declare more than the %d locals a module may, all together, by function %d of its code section", port.ErrModule, ModuleLocalsLimit, i)
			}
			body = body[k+size:]
		}
		return nil
	})
	if err != nil && !errors.Is(err, port.ErrModule) {
		err = fmt.Errorf("%w: its sections cannot be read: %w", port.ErrModule, err)
	}
	return err
}

// localsOf is the number of locals a function's body declares, its parameters
// not counted, or LocalsLimit+1 once it is past LocalsLimit, so no sum of
// declared counts overflows. A count past 31 bits is past the limit too.
func localsOf(fn []byte) (int, error) {
	decls, m := uleb(fn)
	if m == 0 {
		return 0, errors.New("it has no count of its locals")
	}
	fn = fn[m:]
	locals := 0
	for range decls {
		if len(fn) == 0 {
			return 0, errors.New("its locals run past it")
		}
		n, k := uleb(fn)
		if k == 0 {
			return LocalsLimit + 1, nil
		}
		if locals += n; locals > LocalsLimit {
			return LocalsLimit + 1, nil
		}
		if len(fn) < k+1 {
			return 0, errors.New("its locals run past it")
		}
		fn = fn[k+1:]
	}
	return locals, nil
}

// table is the size a table declares, in entries: its least, and its most,
// or -1 for a table that declares none.
type table struct{ least, most int }

// String is the table's bounds as a refusal names them.
func (t table) String() string {
	if t.most < 0 {
		return fmt.Sprintf("least %d, no most", t.least)
	}
	return fmt.Sprintf("least %d, most %d", t.least, t.most)
}

// tablesOf is the tables a table section's body declares, each its type and
// its limits; a size past 31 bits is refused, which no table of TableLimit
// reaches.
func tablesOf(body []byte) ([]table, error) {
	count, m := uleb(body)
	if m == 0 {
		return nil, errors.New("the table section has no count")
	}
	body = body[m:]
	tables := make([]table, 0, min(count, len(body)/3))
	for i := range count {
		if len(body) < 3 {
			return nil, fmt.Errorf("table %d has no type and limits", i)
		}
		bounded := body[1]
		least, k := uleb(body[2:])
		if k == 0 || bounded > 1 {
			return nil, fmt.Errorf("table %d has no limits of 31 bits", i)
		}
		body = body[2+k:]
		t := table{least: least, most: -1}
		if bounded == 1 {
			if t.most, k = uleb(body); k == 0 {
				return nil, fmt.Errorf("table %d has no most of 31 bits", i)
			}
			body = body[k:]
		}
		tables = append(tables, t)
	}
	return tables, nil
}

// withinTableLimit is nil for tables whose least sizes, all together, are at
// most TableLimit, and none of which declares a most past it; the refusal
// otherwise matches port.ErrModule and names each table's index and bounds. A
// table with no most is held to its least: no table grows in a call
// (RuntimeConfig).
func withinTableLimit(tables []table) error {
	least := 0
	for _, t := range tables {
		least += t.least
	}
	over := least > TableLimit
	for _, t := range tables {
		over = over || t.most > TableLimit
	}
	if !over {
		return nil
	}
	named := make([]string, len(tables))
	for i, t := range tables {
		named[i] = fmt.Sprintf("table %d: %s", i, t)
	}
	return fmt.Errorf("%w: it declares tables past the %d entries a module may (%s)", port.ErrModule, TableLimit, strings.Join(named, "; "))
}

// leastPages is the least size in pages of the one memory a memory section's
// body declares, or 0 for none. A size past the pages of a 32-bit memory is
// refused: wazero compiled the module, so none is.
func leastPages(body []byte) (int, error) {
	count, m := uleb(body)
	if m == 0 || count > 1 {
		return 0, errors.New("the memory section is not one memory")
	}
	if count == 0 {
		return 0, nil
	}
	if len(body) < m+2 {
		return 0, errors.New("the memory has no limits")
	}
	least, k := uleb(body[m+1:])
	if k == 0 || least > wasm32Pages {
		return 0, errors.New("the memory has no least size of a 32-bit memory")
	}
	return least, nil
}

// uleb reads one unsigned LEB128 number of at most 31 bits from b: the number
// and the bytes it took, or 0 bytes for one that is not whole or is past 31
// bits, which no size in a bundle reaches.
func uleb(b []byte) (int, int) {
	var v uint32
	for i := 0; i < 5 && i < len(b); i++ {
		v |= uint32(b[i]&0x7f) << (7 * i)
		if b[i]&0x80 == 0 {
			if v > math.MaxInt32 || (i == 4 && b[i] > 0x0f) {
				return 0, 0
			}
			return int(v), i + 1
		}
	}
	return 0, 0
}
