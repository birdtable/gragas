package porttest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// Standin is the in-memory stand-in of port.Plugins, the one fake plugin every
// core test uses: no wasm, no tool, and no name of
// either. It holds the plugins a test installs, each with the answers the test
// scripts, and nothing it answers is one a person wrote: a call a test has not
// scripted answers the wire's own refusal, unsupported. The wasm stand-in of
// internal/host/plugin answers the same cases, and a test holds the two equal.
type Standin struct {
	mu        sync.Mutex
	installed map[string]map[model.PluginRef]*StandinPlugin

	// ListErr, when set, is every List's error.
	ListErr error
	// Trial is the plugin Try opens, whatever module it is handed; nil, Try
	// refuses every module as ErrModule, since the stand-in has none to run.
	// Its OpenErr, when set, is Try's error.
	Trial *StandinPlugin
	// Tried is the modules Try was handed, in order.
	Tried [][]byte
}

// Try answers s.Trial as the module wasm, with m as its manifest (the
// stand-in's Facts are not read), or its OpenErr; with no Trial set, a refusal
// matching port.ErrModule.
func (s *Standin) Try(ctx context.Context, m plugin.Manifest, wasm []byte) (port.Trial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.Tried = append(s.Tried, slices.Clone(wasm))
	p := s.Trial
	s.mu.Unlock()
	switch {
	case p == nil:
		return nil, fmt.Errorf("%w: the stand-in scripts no module", port.ErrModule)
	case p.OpenErr != nil:
		return nil, p.OpenErr
	}
	p.mu.Lock()
	p.opened++
	p.mu.Unlock()
	return &opened{p: p, manifest: &m}, nil
}

// Install puts p at root under ref, in place of any plugin already there.
func (s *Standin) Install(root string, ref model.PluginRef, p *StandinPlugin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installed == nil {
		s.installed = map[string]map[model.PluginRef]*StandinPlugin{}
	}
	if s.installed[root] == nil {
		s.installed[root] = map[model.PluginRef]*StandinPlugin{}
	}
	s.installed[root][ref] = p
}

// List answers the refs installed at root, by name and then version, as bytes.
func (s *Standin) List(_ context.Context, root string) ([]model.PluginRef, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []model.PluginRef
	for ref := range s.installed[root] {
		out = append(out, ref)
	}
	slices.SortFunc(out, func(a, b model.PluginRef) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Version, b.Version)
	})
	return out, nil
}

// Open answers the plugin installed at root under ref, or, where the plugin
// was scripted to fail its opening (StandinPlugin.OpenErr), that error, with
// the plugin not opened. A ref nothing is installed under is port.ErrNotInstalled.
func (s *Standin) Open(ctx context.Context, root string, ref model.PluginRef) (port.Plugin, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	p := s.installed[root][ref]
	s.mu.Unlock()
	switch {
	case p == nil:
		return nil, fmt.Errorf("%s %s: %w", ref.Name, ref.Version, port.ErrNotInstalled)
	case p.OpenErr != nil:
		return nil, p.OpenErr
	}
	p.mu.Lock()
	p.opened++
	p.mu.Unlock()
	return &opened{p: p}, nil
}

// StandinPlugin is one plugin a test installs: its facts, what each operation
// answers and what it cannot be opened for. An operation with no answer
// scripted answers unsupported.
type StandinPlugin struct {
	// Facts is the manifest Manifest answers with.
	Facts plugin.Manifest
	// OpenErr, when set, is the error Open answers with and the plugin is
	// not opened: ErrManifest, ErrNotApproved, ErrDigest, ErrContract,
	// ErrFeature or ErrModule.
	OpenErr error

	// The five operations, each the answer to a request: a value or an error
	// of the host's or the plugin's (port.PluginError, port.LimitError,
	// port.AnswerError, port.ErrTrap and the others).
	OnPlan     func(plugin.PlanRequest) (plugin.PlanResponse, error)
	OnEntries  func(plugin.EntriesRequest) (plugin.EntriesResponse, error)
	OnFindings func(plugin.FindingsRequest) (plugin.FindingsResponse, error)
	OnRank     func(plugin.RankRequest) (plugin.RankResponse, error)
	OnPreset   func(plugin.PresetRequest) (plugin.PresetResponse, error)
	// OnRaw, when set, is every raw call's answer (port.Trial.Raw): the bytes
	// of a request in, what the module wrote out. With none scripted, a raw
	// call is answered by the operation's own scripted answer (raw.go).
	OnRaw func(request []byte) ([]byte, error)

	mu     sync.Mutex
	calls  []plugin.Op
	opened int
	closed int
}

// Calls is the operations called, in the order they were called.
func (p *StandinPlugin) Calls() []plugin.Op {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

// Opened is how many times the plugin was opened, and Closed how many times an
// opening of it was closed.
func (p *StandinPlugin) Opened() (opened, closed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opened, p.closed
}

// ErrClosed is what a call on a closed plugin answers: the caller's own fault,
// never a plugin's.
var ErrClosed = errors.New("the plugin is closed")

// opened is one Open's plugin.
type opened struct {
	p *StandinPlugin
	// manifest is a trial's, the one Try was handed; nil for Open's, which
	// answers the plugin's Facts.
	manifest *plugin.Manifest
	mu       sync.Mutex
	closed   bool
}

func (o *opened) call(op plugin.Op) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrClosed
	}
	o.p.mu.Lock()
	o.p.calls = append(o.p.calls, op)
	o.p.mu.Unlock()
	return nil
}

// unsupported is a plugin's answer for an operation it serves no answer for.
func unsupported(op plugin.Op) error {
	return &plugin.CallError{Code: plugin.CodeUnsupported, Message: "the plugin serves no " + string(op)}
}

func (o *opened) Manifest() plugin.Manifest {
	if o.manifest != nil {
		return *o.manifest
	}
	return o.p.Facts
}

// serve is one call: ended with the ctx's own error where it is dead, recorded, refused if the plugin is closed, unsupported
// where the test scripted no answer, else the answer.
func serve[Q, A any](ctx context.Context, o *opened, op plugin.Op, on func(Q) (A, error), in Q) (A, error) {
	var none A
	if err := ctx.Err(); err != nil {
		return none, err
	}
	if err := o.call(op); err != nil {
		return none, err
	}
	if on == nil {
		return none, unsupported(op)
	}
	return on(in)
}

func (o *opened) Plan(ctx context.Context, in plugin.PlanRequest) (plugin.PlanResponse, error) {
	return serve(ctx, o, plugin.OpPlan, o.p.OnPlan, in)
}

func (o *opened) Entries(ctx context.Context, in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
	return serve(ctx, o, plugin.OpEntries, o.p.OnEntries, in)
}

func (o *opened) Findings(ctx context.Context, in plugin.FindingsRequest) (plugin.FindingsResponse, error) {
	return serve(ctx, o, plugin.OpFindings, o.p.OnFindings, in)
}

func (o *opened) Rank(ctx context.Context, in plugin.RankRequest) (plugin.RankResponse, error) {
	return serve(ctx, o, plugin.OpRank, o.p.OnRank, in)
}

func (o *opened) Preset(ctx context.Context, in plugin.PresetRequest) (plugin.PresetResponse, error) {
	return serve(ctx, o, plugin.OpPreset, o.p.OnPreset, in)
}

// Close ends this opening; a second Close is no error.
func (o *opened) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	o.p.mu.Lock()
	o.p.closed++
	o.p.mu.Unlock()
	return nil
}

var (
	_ port.Plugins = (*Standin)(nil)
	_ port.Trial   = (*opened)(nil)
)
