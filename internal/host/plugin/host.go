package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// Codec is what the host needs of the engine's codec of the plugin contract:
// the request written in canonical form, and each answer read and held to
// the contract's rules. Main hands it wire.New's codec; an adapter never imports the
// core (arch.layers.7), so the host declares the methods it calls and does no
// JSON of its own. A refusal of the codec's is passed on as it is.
type Codec interface {
	EncodeRequest(r plugin.Request) ([]byte, error)
	DecodePlan(data []byte, round int, grants []string) (plugin.PlanResponse, error)
	DecodeEntries(data []byte) (plugin.EntriesResponse, error)
	DecodeFindings(data []byte) (plugin.FindingsResponse, error)
	DecodeRank(data []byte, pairs int) (plugin.RankResponse, error)
	DecodePreset(data []byte, mark string) (plugin.PresetResponse, error)
}

// Config is what a Host is made with, every field of it needed.
type Config struct {
	// Bundles is the installed bundles as they lie on disk; the host makes
	// every refusal on what it reads.
	Bundles port.Bundles
	// Trust is this machine's approvals, asked of the lock's digest.
	Trust port.Trust
	// Judge is the engine's judgement of a manifest (plugins.Judge).
	Judge port.ManifestJudge
	// Codec is the engine's codec (wire.New).
	Codec Codec
	// Features is the features the engine has (wire.Features); a request
	// carries them, and a manifest that needs one past them is refused.
	Features []string
	// Reserved is the names a step's environment sets itself (tool.Reserved),
	// the one list a grant's cache_vars is held to: a
	// manifest that names one, by any case, is refused at open.
	Reserved []string
}

// Host is the adapter of port.Plugins: one runtime, made at the first Open,
// that every plugin it opens is compiled in.
type Host struct {
	cfg Config

	once sync.Once
	rt   wazero.Runtime
	wasi map[string]api.FunctionDefinition
	err  error
}

var _ port.Plugins = (*Host)(nil)

// New is a host made with c, or an error naming the first field c lacks: a
// host with no judge or no codec would open what it cannot hold. Bundles and
// Trust are ports, which main always fills (arch.ports.5).
func New(c Config) (*Host, error) {
	switch {
	case c.Judge == nil:
		return nil, errors.New("the plugin host needs the engine's judgement of a manifest")
	case c.Codec == nil:
		return nil, errors.New("the plugin host needs the engine's codec")
	case len(c.Reserved) == 0:
		// With no list, a cache_vars that names a variable of the step's own
		// would pass the opener: the host fails closed.
		return nil, errors.New("the plugin host needs the names a step's environment sets itself")
	}
	c.Features = slices.Clone(c.Features)
	c.Reserved = slices.Clone(c.Reserved)
	return &Host{cfg: c}, nil
}

// List is the plugins the lock of root names, as Bundles reads them.
func (h *Host) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	return h.cfg.Bundles.List(ctx, root)
}

// Open reads the bundle the lock of root names under ref and holds it, in
// order, to: the bundle judge (pluginbundle.Judge: its lock digest, its files,
// its manifest's form, contract, name, version and module digest); the
// features the engine has; the engine's judgement of the manifest; this
// machine's approval of the lock's digest; and its module, which must compile,
// import nothing but the functions of WASI preview 1 and be a command. No code
// of the module runs before a call: compiling runs none. Every refusal matches
// its sentinel of port; a ctx that ends is its own error.
func (h *Host) Open(ctx context.Context, root string, ref model.PluginRef) (port.Plugin, error) {
	b, err := h.cfg.Bundles.Read(ctx, root, ref)
	if err != nil {
		return nil, err
	}
	m, err := pluginbundle.Judge(ref, b)
	if err != nil {
		return nil, err
	}
	if err := h.judge(m); err != nil {
		return nil, err
	}
	if err := h.approved(ctx, b.Locked); err != nil {
		return nil, err
	}
	o, err := h.compiled(ctx, m, b.Module)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// judge holds m to the engine's features, then to its judgement, then each
// grant's cache_vars to the names a step's environment sets itself.
func (h *Host) judge(m plugin.Manifest) error {
	for _, need := range m.Needs {
		if !slices.Contains(h.cfg.Features, need) {
			return fmt.Errorf("%w: it needs %s", port.ErrFeature, plugin.Shown(need))
		}
	}
	if err := h.cfg.Judge(m); err != nil {
		return fmt.Errorf("%w: %w", port.ErrManifest, err)
	}
	return h.reserved(m)
}

// reserved refuses a grant's cache_vars that names a variable the step's
// environment sets itself, compared without regard to
// case, as Windows reads a name: the engine points a cache name at a folder
// of its own, and one of the step's own would point the Home's, or the
// search path, there.
func (h *Host) reserved(m plugin.Manifest) error {
	for i, p := range m.Programs {
		for j, name := range p.CacheVars() {
			if k := slices.IndexFunc(h.cfg.Reserved, func(r string) bool { return strings.EqualFold(r, name) }); k >= 0 {
				return fmt.Errorf("%w: the manifest is refused at /programs/%d/cache_vars/%d: %s is a variable the step's environment sets itself (%s), and a grant names only a cache of its program's own",
					port.ErrManifest, i, j, plugin.Shown(name), plugin.Shown(h.cfg.Reserved[k]))
			}
		}
	}
	return nil
}

// approved asks this machine's approvals of d, failing closed: an error is a
// no. A ctx that ended is its own error.
func (h *Host) approved(ctx context.Context, d model.Digest) error {
	ok, err := h.cfg.Trust.Approved(ctx, d)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %w", port.ErrNotApproved, d, err)
	}
	if !ok {
		return fmt.Errorf("%w: %s", port.ErrNotApproved, d)
	}
	return nil
}

// runtime is the host's one runtime, with WASI preview 1 instantiated in it,
// and the functions WASI exports by name, made once under the context of the
// first Open with its end cut off, since the runtime outlives that Open.
func (h *Host) runtime(ctx context.Context) (wazero.Runtime, map[string]api.FunctionDefinition, error) {
	h.once.Do(func() {
		ctx := context.WithoutCancel(ctx)
		rt := wazero.NewRuntimeWithConfig(ctx, RuntimeConfig())
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
			h.err = errors.Join(fmt.Errorf("the plugin runtime cannot start: %w", err), rt.Close(ctx))
			return
		}
		h.rt = rt
		h.wasi = rt.Module(wasi_snapshot_preview1.ModuleName).ExportedFunctionDefinitions()
	})
	return h.rt, h.wasi, h.err
}

// errClosed is what Open answers once the host is closed.
var errClosed = errors.New("the plugin host is closed")

// Close frees the runtime and every module compiled in it. A host never
// opened makes no runtime after it, so Open answers errClosed; one that was
// opened answers the closed runtime's refusal to compile. Neither the host nor
// a plugin it opened is called at once with Close or after it.
func (h *Host) Close(ctx context.Context) error {
	h.once.Do(func() { h.err = errClosed })
	if h.rt == nil {
		return nil
	}
	return h.rt.Close(ctx)
}
