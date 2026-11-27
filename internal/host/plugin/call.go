package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// opened is one plugin the host opened: its manifest, as judged, and its
// module, compiled once. Each call is a fresh instance of the module.
type opened struct {
	host     *Host
	rt       wazero.Runtime
	cm       wazero.CompiledModule
	least    int
	opened   context.Context
	manifest plugin.Manifest
	closed   atomic.Bool
}

var _ port.Plugin = (*opened)(nil)

// errPluginClosed is what a call answers once its plugin is closed.
var errPluginClosed = errors.New("the plugin is closed")

func (o *opened) Manifest() plugin.Manifest { return o.manifest }

func (o *opened) Plan(ctx context.Context, in plugin.PlanRequest) (plugin.PlanResponse, error) {
	return answer(ctx, o, plugin.OpPlan, in.ToCanon(), func(out []byte) (plugin.PlanResponse, error) {
		return o.host.cfg.Codec.DecodePlan(out, in.Round, grants(o.manifest))
	})
}

func (o *opened) Entries(ctx context.Context, in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
	return answer(ctx, o, plugin.OpEntries, in.ToCanon(), o.host.cfg.Codec.DecodeEntries)
}

func (o *opened) Findings(ctx context.Context, in plugin.FindingsRequest) (plugin.FindingsResponse, error) {
	return answer(ctx, o, plugin.OpFindings, in.ToCanon(), o.host.cfg.Codec.DecodeFindings)
}

func (o *opened) Rank(ctx context.Context, in plugin.RankRequest) (plugin.RankResponse, error) {
	return answer(ctx, o, plugin.OpRank, in.ToCanon(), func(out []byte) (plugin.RankResponse, error) {
		return o.host.cfg.Codec.DecodeRank(out, len(in.Pairs))
	})
}

func (o *opened) Preset(ctx context.Context, in plugin.PresetRequest) (plugin.PresetResponse, error) {
	mark := ""
	if in.Mark != nil {
		mark = *in.Mark
	}
	return answer(ctx, o, plugin.OpPreset, in.ToCanon(), func(out []byte) (plugin.PresetResponse, error) {
		return o.host.cfg.Codec.DecodePreset(out, mark)
	})
}

// answer is one call of op with body, its answer read by decode, the codec's;
// a refusal of the call or of the codec is passed on as it is.
func answer[T any](ctx context.Context, o *opened, op plugin.Op, body canon.Value, decode func([]byte) (T, error)) (T, error) {
	out, err := o.call(ctx, op, body)
	if err != nil {
		var none T
		return none, err
	}
	return decode(out)
}

// Close frees the compiled module, under the context it was opened in, with
// its end cut off. A call after it is an error.
func (o *opened) Close() error {
	if o.closed.Swap(true) {
		return nil
	}
	return o.cm.Close(o.opened)
}

// grants is the ids of m's programs, the grants a plan's step may name.
func grants(m plugin.Manifest) []string {
	ids := make([]string, 0, len(m.Programs))
	for _, p := range m.Programs {
		switch {
		case p.Found != nil:
			ids = append(ids, p.Found.ID)
		case p.At != nil:
			ids = append(ids, p.At.ID)
		case p.Script != nil:
			ids = append(ids, p.Script.ID)
		}
	}
	return ids
}

// call writes the request of op with body through the codec, which refuses
// one over its cap, and runs one instance of the module on it, answering what
// the module wrote to stdout. No call is retried. A call on a ctx that has
// ended is the ctx's own error, and writes no request and runs nothing.
func (o *opened) call(ctx context.Context, op plugin.Op, body canon.Value) ([]byte, error) {
	if o.closed.Load() {
		return nil, errPluginClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	obj, ok := body.(canon.Object)
	if !ok {
		return nil, fmt.Errorf("the %s request is no object", op)
	}
	req, err := o.host.cfg.Codec.EncodeRequest(plugin.Request{Features: o.host.cfg.Features, Op: op, Body: obj})
	if err != nil {
		return nil, err
	}
	return run(ctx, callContext, o.rt, o.cm, o.least, req)
}

// limited is what makes one call's context: callContext, the one place the
// limits are installed; run takes it as an argument so that a test can plant
// a call without them and see its rows go red.
type limited func(context.Context) (context.Context, context.CancelFunc, *memory)

// run is one call: a fresh instance of cm, with
// stdin the request, stdout the answer and stderr a log, and nothing else: no
// argument, no environment variable, no folder, no socket, wazero's fixed
// clocks in place of the system's and its fixed random source. The instance is
// closed when run returns, so nothing of it is seen by another call. It is
// held to the limits of limit and ResponseLimit, and the answer is the
// module's only when it exits with code 0.
//
// A module whose memory is least bytes past MemoryLimit is refused as over
// memory before it is instantiated: wazero v1.12.0 slices the allocator's
// answer to the least size unchecked (internal/wasm/memory.go), so a refusal
// there would end the program, not the call.
func run(ctx context.Context, limit limited, rt wazero.Runtime, cm wazero.CompiledModule, least int, stdin []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if least > MemoryLimit {
		return nil, &plugin.LimitError{What: plugin.LimitMemory, Size: least, Cap: MemoryLimit}
	}
	callCtx, cancel, mem := limit(ctx)
	defer cancel()
	out := &capped{limit: ResponseLimit}
	log := &capped{limit: LogLimit, cut: true}
	cfg := wazero.NewModuleConfig().
		WithName("").
		WithStdin(bytes.NewReader(stdin)).
		WithStdout(out).
		WithStderr(log).
		WithStartFunctions()
	mod, err := rt.InstantiateModule(callCtx, cm, cfg)
	if err == nil {
		_, err = mod.ExportedFunction("_start").Call(callCtx)
		err = errors.Join(err, mod.Close(context.WithoutCancel(ctx)))
	}
	if err := ended(ctx, callCtx, err, mem, out, log); err != nil {
		return nil, err
	}
	return out.buf.Bytes(), nil
}

// ended is the error of a call that ran with err, or nil for one that exited
// with code 0, read in this order: the caller's ctx ended, which is its own
// error; the response went past its limit; the allocator refused a grow, which
// is over memory whatever the module did after; the call's time ran out; and
// any other end, a trap or an exit with another code, which is port.ErrTrap
// with the end of the log, shown.
func ended(ctx, callCtx context.Context, err error, mem *memory, out, log *capped) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	switch {
	case out.size > out.limit:
		return &plugin.LimitError{What: plugin.LimitResponse, Size: out.size, Cap: out.limit}
	case mem.refused > 0:
		return &plugin.LimitError{What: plugin.LimitMemory, Size: mem.refused, Cap: MemoryLimit}
	case errors.Is(callCtx.Err(), context.DeadlineExceeded):
		return &plugin.LimitError{What: plugin.LimitTime}
	case err == nil:
		return nil
	}
	if exit, ok := errors.AsType[*sys.ExitError](err); ok && exit.ExitCode() == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s; its log ends %s", port.ErrTrap, plugin.Shown(err.Error()), plugin.Shown(string(log.tail())))
}

// capped is a writer that keeps at most limit bytes and counts every byte it
// was handed. Past its limit it refuses the write, or, when cut, keeps the
// first limit bytes and drops the rest.
type capped struct {
	buf   bytes.Buffer
	limit int
	cut   bool
	size  int
}

// errOverLimit is what a write past a stream's limit answers the module.
var errOverLimit = errors.New("past the stream's limit")

func (c *capped) Write(p []byte) (int, error) {
	c.size += len(p)
	room := c.limit - c.buf.Len()
	if len(p) <= room {
		return c.buf.Write(p)
	}
	if !c.cut {
		return 0, errOverLimit
	}
	c.buf.Write(p[:max(room, 0)])
	return len(p), nil
}

// tailSize is how much of the end of a log a crash's error shows.
const tailSize = 400

// tail is the last tailSize bytes kept.
func (c *capped) tail() []byte {
	b := c.buf.Bytes()
	if len(b) > tailSize {
		return b[len(b)-tailSize:]
	}
	return b
}
