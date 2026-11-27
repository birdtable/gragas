package plugin

import (
	"context"
	"fmt"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// Try is the module wasm of a bundle under test (gragas plugin test), held to
// what a plugin's module is by the same code that holds an installed one at
// Open: the host's one runtime, compile (C5) and, at every call, run with
// callContext's limits. It reads no lock and asks no digest and no approval:
// the bundle's checks judge it, and nothing a trial answers is run, since
// this package runs no step. m is the manifest the caller read, the grants a
// plan's step is held to. A module it refuses matches port.ErrModule and none
// of its code has run; a ctx that ended is its own error. A module past the
// bundle's module bound (pluginbundle.DefaultBounds) is refused before it is
// compiled, as Open's never reaches the compiler: Open's bytes come from a
// read held to that bound, and Try's from whoever calls it.
func (h *Host) Try(ctx context.Context, m plugin.Manifest, wasm []byte) (port.Trial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if most := pluginbundle.DefaultBounds().Module; int64(len(wasm)) > most {
		return nil, fmt.Errorf("%w: it is %d bytes, past the %d a module may hold", port.ErrModule, len(wasm), most)
	}
	o, err := h.compiled(ctx, m, wasm)
	if err != nil {
		return nil, err
	}
	return &trial{opened: o}, nil
}

// compiled is wasm compiled in the host's runtime and held by compile, as
// the plugin of manifest m: what Open answers once the bundle is judged and
// approved, and Try at once.
func (h *Host) compiled(ctx context.Context, m plugin.Manifest, wasm []byte) (*opened, error) {
	rt, wasi, err := h.runtime(ctx)
	if err != nil {
		return nil, err
	}
	cm, least, err := compile(ctx, rt, wasi, wasm)
	if err != nil {
		return nil, err
	}
	return &opened{host: h, rt: rt, cm: cm, least: least, opened: context.WithoutCancel(ctx), manifest: m}, nil
}

// trial is a module under test: an opened plugin, whose typed calls are
// Open's, with a raw call beside them. Only Try makes one, so a plugin Open
// answers has no raw call.
type trial struct {
	*opened
}

var _ port.Trial = (*trial)(nil)

// Raw runs one instance of the module on request as it is, by run with
// callContext's limits, as a typed call does after its codec: the request is
// the caller's, written by its own codec, which holds its size. A raw call on
// a closed trial, or on a ctx that ended, runs nothing.
func (t *trial) Raw(ctx context.Context, request []byte) ([]byte, error) {
	if t.closed.Load() {
		return nil, errPluginClosed
	}
	return run(ctx, callContext, t.rt, t.cm, t.least, request)
}
