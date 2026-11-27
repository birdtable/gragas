// Package plugin is the one package that imports wazero: the host of
// gragas's plugins. It fills
// port.Plugins: it opens an installed plugin, refusing it before any of its
// code runs, and serves each operation as one call into a fresh instance of
// its WebAssembly module, held to the limits below.
package plugin

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// The limits of one call. They are gragas's, and nothing a plugin sends moves them. The request's
// 32 MiB is the codec's, which refuses a request over it before a call starts.
const (
	// TimeLimit is how long one call may run.
	TimeLimit = 5 * time.Second
	// MemoryLimit is the most linear memory one instance may hold, in bytes.
	MemoryLimit = 256 << 20
	// ResponseLimit is the most bytes a module may write to stdout in one call.
	ResponseLimit = 16 << 20
	// LogLimit is the most bytes of stderr kept of one call; the rest is cut.
	LogLimit = 64 << 10
	// TableLimit is the most entries a module's tables may declare, all of
	// them together, as least size or as most. A table is host memory that
	// MemoryLimit does not hold, 8 bytes an entry, so the limit is held at
	// Open (check C5), and no table grows in a call (RuntimeConfig). The
	// guests it was set from declare at most 6947, standin.wasm's.
	TableLimit = 1 << 14
	// LocalsLimit is the most locals one function of a module may declare,
	// and ModuleLocalsLimit the most all its functions may declare together.
	// Locals are host memory that MemoryLimit does not hold: wazero v1.12.0
	// makes a slice of every function's locals as it compiles
	// (internal/wasm/binary/code.go), and its interpreter keeps about 64 bytes
	// a local, so 2^20 locals in one function of a 171-byte module are 407 MiB
	// allocated at compile, and 100,000 functions of 256 locals, a module of
	// 700 KB, keep 1.8 GiB. Both are held at Open, before the module
	// compiles. The guests they were set from declare at most 17 locals in a
	// function and 13,363 in all, standin.wasm's (go1.27.4).
	LocalsLimit       = 1 << 10
	ModuleLocalsLimit = 1 << 18
	// FrameLimit is the most slots of the interpreter's value stack one
	// function's frame may take: its parameters, its locals and the most its
	// operands take at once, 8 bytes a slot and a v128 two. wazero v1.12.0
	// keeps every frame of a call on one Go slice it grows by append, and
	// bounds only the frames, at callFrames, so StackLimit is the most a
	// call's value stack can hold, 32 MiB; append's growth holds up to a
	// quarter more, about 40 MiB live at its deepest, and allocates about 6
	// times it on the way, which TestTheValueStackIsHeldToStackLimit holds.
	// It is host memory that MemoryLimit does not hold. Open finds each
	// function's frame by its own pass over the code before the module
	// compiles (height.go): wazero's validator allows 2^27 values a function.
	// The guests it was set from take at most 27 slots in a frame,
	// standin.wasm's (go1.27.4).
	FrameLimit = 1 << 11
	StackLimit = callFrames * slotSize * FrameLimit
	// BlockLimit is the most blocks a function's code may be nested in at
	// once. wazero's validator and compiler keep a frame for each, and so
	// does Open's pass, so a module of 32 MiB, all blocks, would take
	// hundreds of MiB to read without it. The guests it was set from nest at
	// most 549, standin.wasm's (go1.27.4), one for each place a function
	// resumes.
	BlockLimit = 1 << 14
)

// callFrames is the most frames wazero v1.12.0's interpreter keeps on its call
// stack (callStackCeiling, internal/engine/interpreter/interpreter.go), and
// slotSize the bytes of one slot of its value stack.
const (
	callFrames = 2000
	slotSize   = 8
)

// pageSize is the size of one page of WebAssembly memory, and wasm32Pages the
// most pages a 32-bit memory can address.
const (
	pageSize    = 64 << 10
	wasm32Pages = 1 << 16
)

// RuntimeConfig is the config every runtime of gragas is made from: wazero's
// interpreter and nothing else. The compiler is never chosen, since it maps
// executable memory, which the hardened release build is not given, and no
// compilation cache is set anywhere, so nothing is written to disk. What mode
// a runtime runs in is read from one return line.
//
// Its features are WebAssembly 2.0's without reference types, so a module has
// at most one table and no instruction that grows or fills one: wazero v1.12.0
// grows a table without a declared most by appending to a Go slice
// (internal/wasm/table.go), so one table.grow could ask for gigabytes of the
// host's own memory and end the program, not the call. A table then holds
// the entries it declares, which Open holds to TableLimit. Standard Go's and
// TinyGo's guests use none of it.
//
// A call's context ending closes the module, so a loop that never yields ends
// at TimeLimit. The page limit is the whole of a 32-bit memory, written out: the
// memory limit is the allocator's (callContext), because wazero refuses a grow
// past its own page limit before it asks the allocator (internal/wasm/memory.go
// in v1.12.0), and a grow refused there looks to the host like any crash. Every
// grow reaches the allocator, which refuses one past MemoryLimit before it
// allocates and records it, so the call ends on memory.
func RuntimeConfig() wazero.RuntimeConfig {
	return wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 &^ api.CoreFeatureReferenceTypes).
		WithMemoryLimitPages(wasm32Pages).
		WithCloseOnContextDone(true)
}

// callContext is the one place a call's context is made: ctx with TimeLimit,
// and with the allocator of one instance's memory, through wazero's
// experimental.WithMemoryAllocator, which a module of the call is instantiated
// under. The allocator answers what it refused, so the call's end is read as
// over memory.
func callContext(ctx context.Context) (context.Context, context.CancelFunc, *memory) {
	ctx, cancel := context.WithTimeout(ctx, TimeLimit)
	mem := &memory{}
	ctx = experimental.WithMemoryAllocator(ctx, experimental.MemoryAllocatorFunc(func(_, _ uint64) experimental.LinearMemory { return mem }))
	return ctx, cancel, mem
}

// memory is one instance's linear memory: it grows as the module asks, up to
// MemoryLimit, and records the first size it refused and the most it held. It
// refuses before it allocates, so an instance never holds more than
// MemoryLimit.
type memory struct {
	buf     []byte
	refused int
	peak    int
}

// Reallocate answers the memory grown to n bytes, or nil past MemoryLimit.
func (m *memory) Reallocate(n uint64) []byte {
	if n > MemoryLimit {
		if m.refused == 0 {
			m.refused = MemoryLimit + 1
			if n <= wasm32Pages*pageSize {
				m.refused = int(n)
			}
		}
		return nil
	}
	if n <= uint64(cap(m.buf)) {
		m.buf = m.buf[:n]
		return m.buf
	}
	grown := make([]byte, n, min(max(n, 2*uint64(cap(m.buf))), MemoryLimit))
	copy(grown, m.buf)
	m.buf = grown
	m.peak = max(m.peak, cap(grown))
	return m.buf
}

// Free lets the memory go.
func (m *memory) Free() { m.buf = nil }
