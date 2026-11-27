package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// root is the folder the rows install their plugins under.
const root = "/project"

// standinRef is the ref the rows install a bundle under.
func standinRef() model.PluginRef { return model.PluginRef{Name: "standin", Version: "1.0.0"} }

// manifestOf is a manifest of standin 1.0.0 for module that the bundle judge
// passes, with the lines a row replaces to plant one fault.
func manifestOf(t *testing.T, module []byte, replace ...string) []byte {
	t.Helper()
	m := `contract: 1
needs: []
name: standin
version: 1.0.0
tool: standin
tool_versions: ">=1.0.0 <2.0.0"
targets: [source]
provides: [plan, entries]
extends: false
warning: false
programs:
  - id: standin
    path: bin/standin
    version_argv: [--version]
    versions: ">=1.0.0 <2.0.0"
wasm: ` + porttest.Wasm(module) + "\n"
	for i := 0; i+1 < len(replace); i += 2 {
		if !strings.Contains(m, replace[i]) {
			t.Fatalf("the plant %q is not in the manifest", replace[i])
		}
		m = strings.Replace(m, replace[i], replace[i+1], 1)
	}
	return []byte(m)
}

// approvals is a Trust that answers yes, or no, or err, and counts its asks.
// The host only asks: any other method of Trust is a nil call, and panics.
type approvals struct {
	port.Trust
	mu    sync.Mutex
	no    bool
	err   error
	asked int
}

func (a *approvals) Approved(ctx context.Context, _ model.Digest) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asked++
	return !a.no && a.err == nil, a.err
}

// codec is a Codec that writes a request as the body's op and answers what
// a row plants: refused, the refusal of every answer, and otherwise an empty
// answer; it keeps what it was handed and counts its requests.
type codec struct {
	mu       sync.Mutex
	refused  error
	requests int
	answers  [][]byte
}

var _ Codec = (*codec)(nil)

func (c *codec) EncodeRequest(r plugin.Request) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests++
	return []byte("request of " + string(r.Op)), nil
}

func (c *codec) decoded(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.answers = append(c.answers, bytes.Clone(data))
	return c.refused
}

func (c *codec) DecodePlan(data []byte, _ int, _ []string) (plugin.PlanResponse, error) {
	return plugin.PlanResponse{}, c.decoded(data)
}

func (c *codec) DecodeEntries(data []byte) (plugin.EntriesResponse, error) {
	return plugin.EntriesResponse{}, c.decoded(data)
}

func (c *codec) DecodeFindings(data []byte) (plugin.FindingsResponse, error) {
	return plugin.FindingsResponse{}, c.decoded(data)
}

func (c *codec) DecodeRank(data []byte, _ int) (plugin.RankResponse, error) {
	return plugin.RankResponse{}, c.decoded(data)
}

func (c *codec) DecodePreset(data []byte, _ string) (plugin.PresetResponse, error) {
	return plugin.PresetResponse{}, c.decoded(data)
}

// rig is one host of the rows with what it was made with.
type rig struct {
	host    *Host
	bundles *porttest.Bundles
	trust   *approvals
	codec   *codec
	judged  int
	judge   error
}

// newRig is a host over a bundle of module and its manifest, approved, whose
// engine has no feature past the base and passes every manifest.
func newRig(t *testing.T, manifest, module []byte) *rig {
	t.Helper()
	return newRigOf(t, manifest, module, []string{})
}

// reservedOfTheRows is the names the rows' hosts hold cache_vars to, one in
// a spelling of its own, so a row holds the host to comparing without regard
// to case; main gives tool.Reserved, which tool's own rows hold to the step's
// environment.
func reservedOfTheRows() []string { return []string{"PATH", "Xdg_Cache_Home"} }

// newRigOf is newRig with the engine's features.
func newRigOf(t *testing.T, manifest, module []byte, features []string) *rig {
	t.Helper()
	r := &rig{bundles: &porttest.Bundles{}, trust: &approvals{}, codec: &codec{}}
	r.bundles.Put(root, standinRef(), manifest, module)
	h, err := New(Config{
		Bundles: r.bundles,
		Trust:   r.trust,
		Judge: func(plugin.Manifest) error {
			r.judged++
			return r.judge
		},
		Codec:    r.codec,
		Features: features,
		Reserved: reservedOfTheRows(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close(context.WithoutCancel(t.Context())) })
	r.host = h
	return r
}

// open is the host's Open of standin.
func (r *rig) open(t *testing.T) (port.Plugin, error) {
	t.Helper()
	p, err := r.host.Open(t.Context(), root, standinRef())
	if err == nil {
		t.Cleanup(func() { _ = p.Close() })
	}
	return p, err
}

// TestOpenRefusesBeforeAnyCodeRuns: each fault planted in a bundle that opens
// is refused by Open with its sentinel, and the clean bundle
// opens. A refusal of the bundle or its manifest comes before this machine's
// approval is asked; none of them runs the module.
func TestOpenRefusesBeforeAnyCodeRuns(t *testing.T) {
	t.Parallel()
	good := echo().bytes()
	type row = struct {
		name    string
		module  []byte
		named   []byte
		replace []string
		plant   func(r *rig) error
		want    error
		trusted bool
	}
	rows := []row{
		{name: "planted: the clean bundle", module: good},
		{name: "the module changed after its lock", module: good, plant: func(r *rig) error { return r.bundles.Change(root, standinRef(), good[:len(good)-1]) }, want: port.ErrDigest},
		{name: "the module is not the manifest's wasm", module: good, named: []byte("another module"), want: port.ErrDigest},
		{name: "another contract", module: good, replace: []string{"contract: 1", "contract: 2"}, want: port.ErrContract},
		{name: "an unknown key", module: good, replace: []string{"warning: false", "warning: false\nlater: true"}, want: port.ErrManifest},
		{name: "a feature the engine lacks", module: good, replace: []string{"needs: []", "needs: [streams]"}, want: port.ErrFeature},
		{name: "the engine's judgement refuses it", module: good, plant: func(r *rig) error { r.judge = errors.New("planted"); return nil }, want: port.ErrManifest},
		{name: "not approved", module: good, plant: func(r *rig) error { r.trust.no = true; return nil }, want: port.ErrNotApproved, trusted: true},
		{name: "the approvals cannot be read", module: good, plant: func(r *rig) error { r.trust.err = errors.New("planted"); return nil }, want: port.ErrNotApproved, trusted: true},
	}
	for _, f := range moduleFaults() {
		rows = append(rows, row{name: f.name, module: f.module, want: port.ErrModule, trusted: true})
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			named := row.module
			if row.named != nil {
				named = row.named
			}
			r := newRig(t, manifestOf(t, named, row.replace...), row.module)
			if row.plant != nil {
				if err := row.plant(r); err != nil {
					t.Fatal(err)
				}
			}
			p, err := r.open(t)
			switch {
			case row.want == nil && (err != nil || p == nil):
				t.Fatalf("the clean bundle = %v; want it opened", err)
			case row.want != nil && (!errors.Is(err, row.want) || p != nil):
				t.Fatalf("Open = %v, %v; want %v", p, err, row.want)
			case row.want != nil && !row.trusted && r.trust.asked != 0:
				t.Errorf("Open asked the approvals %d times before it refused the bundle; want none", r.trust.asked)
			case r.codec.requests != 0:
				t.Errorf("Open wrote %d requests; want none", r.codec.requests)
			}
		})
	}
}

// TestOpenHoldsCacheVarsToTheFeatureAndTheStepsOwnNames: a
// manifest whose grant gives cache_vars opens on an engine that has the
// feature cache-vars, and names none of the step's own variables. An engine
// without the feature ends ErrFeature and names it; a name on the reserved
// list, in any case, is ErrManifest at the field, before the approvals are
// asked. Planted failure: reserved compared with ==, or not asked at all.
// Risk: a plugin points XDG_CACHE_HOME, which go-proc sets fresh with the
// step's Home or, for a grant with cache_vars, at the grant's own kept folder,
// at a folder another step reads.
func TestOpenHoldsCacheVarsToTheFeatureAndTheStepsOwnNames(t *testing.T) {
	t.Parallel()
	good := echo().bytes()
	grant := `    versions: ">=1.0.0 <2.0.0"` + "\n"
	cached := func(names string) []string {
		return []string{"needs: []", "needs: [cache-vars]", grant, grant + "    cache_vars: [" + names + "]\n"}
	}
	for _, row := range []struct {
		name     string
		features []string
		replace  []string
		want     error
		says     string
	}{
		{name: "planted: a cache name, the feature had", features: []string{plugin.FeatureCacheVars}, replace: cached("GOCACHE, GOLANGCI_LINT_CACHE")},
		{name: "an engine without cache-vars", features: []string{}, replace: cached("GOCACHE"), want: port.ErrFeature, says: "it needs cache-vars"},
		{name: "a name the step sets itself, by another case", features: []string{plugin.FeatureCacheVars}, replace: cached("GOCACHE, XDG_CACHE_HOME"), want: port.ErrManifest,
			says: "/programs/0/cache_vars/1: XDG_CACHE_HOME is a variable the step's environment sets itself (Xdg_Cache_Home)"},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			r := newRigOf(t, manifestOf(t, good, row.replace...), good, row.features)
			p, err := r.open(t)
			switch {
			case row.want == nil && (err != nil || p == nil):
				t.Fatalf("Open = %v; want it opened", err)
			case row.want == nil:
				return
			case !errors.Is(err, row.want) || p != nil || !strings.Contains(err.Error(), row.says):
				t.Fatalf("Open = %v, %v; want %v saying %q", p, err, row.want, row.says)
			case r.trust.asked != 0:
				t.Errorf("Open asked the approvals %d times before it refused the manifest; want none", r.trust.asked)
			}
		})
	}
}

// TestNewRefusesAHostWithNoReservedNames: a host made with no
// list of the step's own names would let every cache_vars through, so New
// fails closed. Planted failure: the check of Reserved taken out of New.
func TestNewRefusesAHostWithNoReservedNames(t *testing.T) {
	t.Parallel()
	h, err := New(Config{Bundles: &porttest.Bundles{}, Trust: &approvals{}, Judge: func(plugin.Manifest) error { return nil }, Codec: &codec{}, Features: []string{}})
	if err == nil || h != nil || !strings.Contains(err.Error(), "the names a step's environment sets itself") {
		t.Fatalf("New = %v, %v; want the refusal of a host with no reserved names", h, err)
	}
}

// fault is a module the host refuses as no plugin's module, port.ErrModule,
// at Open and at Try alike.
type fault struct {
	name   string
	module []byte
}

// moduleFaults is each module the host holds to what a plugin's module is,
// and refuses: one that does not compile, an import that is not WASI preview
// 1's or not of its type, a memory imported, and a _start missing or with a
// param.
func moduleFaults() []fault {
	nonWASI := wasiMod(1, nil)
	nonWASI.imports = []wasiImport{{module: "env", name: "fd_read", typ: 0}, wasiImports()[1], wasiImports()[2]}
	unknownFn := wasiMod(1, nil)
	unknownFn.imports = []wasiImport{{module: "wasi_snapshot_preview1", name: "sock_open", typ: 0}, wasiImports()[1], wasiImports()[2]}
	wrongType := wasiMod(1, nil)
	wrongType.imports = []wasiImport{wasiImports()[0], wasiImports()[1], {module: "wasi_snapshot_preview1", name: "proc_exit", typ: 0}}
	memImport := wasiMod(1, nil)
	memImport.memory = nil
	memImport.exports = memImport.exports[:1]
	memImport.rawImports = [][]byte{code(name("wasi_snapshot_preview1"), name("memory"), []byte{0x02}, memLimits(limits{least: 2, most: -1}))}
	noStart := wasiMod(1, nil)
	noStart.exports = []export{{name: "main", kind: exportFunc, index: startFn}, {name: "memory", kind: exportMemory}}
	startWithParams := wasiMod(1, nil)
	startWithParams.types = append(wasiTypes(), fnType{params: []byte{i32}})
	startWithParams.funcs[0].typ = 3
	return []fault{
		{"a module that does not compile", []byte("\x00asm\x01\x00\x00\x00\xff")},
		{"an import from another module", nonWASI.bytes()},
		{"an import WASI has not", unknownFn.bytes()},
		{"a WASI import of another type", wrongType.bytes()},
		{"a memory imported", memImport.bytes()},
		{"no _start", noStart.bytes()},
		{"a _start that takes a param", startWithParams.bytes()},
	}
}

// TestOpenRunsNoCode: a module whose start section traps opens, since
// compiling runs none of its code, and its call is the crash.
func TestOpenRunsNoCode(t *testing.T) {
	t.Parallel()
	m := wasiMod(1, op(opUnreachable))
	at := startFn
	m.start = &at
	r := newRig(t, manifestOf(t, m.bytes()), m.bytes())
	p, err := r.open(t)
	if err != nil {
		t.Fatalf("a module whose start traps = %v; want it opened", err)
	}
	if _, err := p.Entries(t.Context(), plugin.EntriesRequest{}); !errors.Is(err, port.ErrTrap) {
		t.Errorf("its call = %v; want port.ErrTrap", err)
	}
}

// TestACallIsOneRequestThroughTheCodec: a call writes its request through the
// codec once, the module reads it on stdin, and the codec is handed what the
// module wrote: here, the request, echoed.
func TestACallIsOneRequestThroughTheCodec(t *testing.T) {
	t.Parallel()
	r := newRig(t, manifestOf(t, echo().bytes()), echo().bytes())
	p, err := r.open(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Entries(t.Context(), plugin.EntriesRequest{}); err != nil {
		t.Fatal(err)
	}
	if r.codec.requests != 1 || len(r.codec.answers) != 1 || string(r.codec.answers[0]) != "request of entries" {
		t.Errorf("one call wrote %d requests and handed the codec %q; want one, and the request echoed", r.codec.requests, r.codec.answers)
	}
}

// TestTheHostsSixErrorsFailClosed: each of the host's six errors is an error
// with its sentinel or type, and never an empty answer: a
// row's answer is the zero value with it. No call is retried: each wrote one
// request. The codec's refusals, a program the plugin is not granted among
// them, are passed on as they are.
func TestTheHostsSixErrorsFailClosed(t *testing.T) {
	t.Parallel()
	grant := fmt.Errorf("planted grant refusal: %w; %w", port.ErrGrant, &plugin.AnswerError{Path: "/steps/0/program", Why: "not granted"})
	bad := &plugin.AnswerError{Path: "/entries/0/id", Why: "planted"}
	t.Run("not approved, and the digest", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, manifestOf(t, echo().bytes()), echo().bytes())
		r.trust.no = true
		if _, err := r.open(t); !errors.Is(err, port.ErrNotApproved) {
			t.Errorf("not approved = %v", err)
		}
		r = newRig(t, manifestOf(t, []byte("another")), echo().bytes())
		if _, err := r.open(t); !errors.Is(err, port.ErrDigest) {
			t.Errorf("the wrong digest = %v", err)
		}
	})
	t.Run("the contract, and a feature", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, manifestOf(t, echo().bytes(), "contract: 1", "contract: 2"), echo().bytes())
		if _, err := r.open(t); !errors.Is(err, port.ErrContract) {
			t.Errorf("another contract = %v", err)
		}
		r = newRig(t, manifestOf(t, echo().bytes(), "needs: []", "needs: [streams]"), echo().bytes())
		if _, err := r.open(t); !errors.Is(err, port.ErrFeature) {
			t.Errorf("a missing feature = %v", err)
		}
	})
	for _, row := range []struct {
		name    string
		module  mod
		refused error
		check   func(error) bool
	}{
		{"a program not granted", echo(), grant, func(err error) bool {
			ae, ok := errors.AsType[*plugin.AnswerError](err)
			return errors.Is(err, grant) && err.Error() == grant.Error() && errors.Is(err, port.ErrGrant) && ok && ae.Path == "/steps/0/program"
		}},
		{"over a limit", wasiMod(pastPages, nil), nil, func(err error) bool { return limitOf(err) == plugin.LimitMemory }},
		{"a crash", wasiMod(1, op(opUnreachable)), nil, func(err error) bool { return errors.Is(err, port.ErrTrap) }},
		{"a bad answer", echo(), bad, func(err error) bool {
			ae, ok := errors.AsType[*plugin.AnswerError](err)
			return errors.Is(err, bad) && err.Error() == bad.Error() && ok && ae.Path == "/entries/0/id"
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, manifestOf(t, row.module.bytes()), row.module.bytes())
			r.codec.refused = row.refused
			p, err := r.open(t)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := p.Plan(t.Context(), plugin.PlanRequest{})
			if !row.check(err) || plan.Steps != nil || r.codec.requests != 1 {
				t.Errorf("plan = %+v, %v after %d requests; want the zero answer and the error, after one", plan, err, r.codec.requests)
			}
		})
	}
}

// TestACallAfterCloseIsAnError: a closed plugin answers every call with an
// error and writes no request, and a second Close is nil.
func TestACallAfterCloseIsAnError(t *testing.T) {
	t.Parallel()
	r := newRig(t, manifestOf(t, echo().bytes()), echo().bytes())
	p, err := r.host.Open(t.Context(), root, standinRef())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Entries(t.Context(), plugin.EntriesRequest{}); !errors.Is(err, errPluginClosed) || r.codec.requests != 0 {
		t.Errorf("a call after Close = %v after %d requests; want errPluginClosed and none", err, r.codec.requests)
	}
	if err := p.Close(); err != nil {
		t.Errorf("a second Close = %v", err)
	}
}
