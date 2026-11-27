package plugin

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// say is a command that writes s to stdout and exits 0: s is data at 1 KiB,
// its iovec at 0.
func say(s string) mod {
	m := wasiMod(1, code(write(1, 1<<10, int32(len(s)), 0), exit(0)))
	m.data = []segment{{offset: 1 << 10, data: []byte(s)}}
	return m
}

// scripted is a Codec whose answer is read from what the module wrote: an
// answer that is a key of refusals is refused with its error, and any other
// is the empty answer. It counts the requests it wrote: a module runs only on
// a request, so the count is the most calls that can have reached a module.
type scripted struct {
	refusals map[string]error
	written  atomic.Int64
}

var _ Codec = (*scripted)(nil)

func (c *scripted) EncodeRequest(r plugin.Request) ([]byte, error) {
	c.written.Add(1)
	return []byte(r.Op), nil
}

func (c *scripted) DecodePlan(data []byte, _ int, _ []string) (plugin.PlanResponse, error) {
	return plugin.PlanResponse{}, c.refusals[string(data)]
}

func (c *scripted) DecodeEntries(data []byte) (plugin.EntriesResponse, error) {
	return plugin.EntriesResponse{}, c.refusals[string(data)]
}

func (c *scripted) DecodeFindings(data []byte) (plugin.FindingsResponse, error) {
	return plugin.FindingsResponse{}, c.refusals[string(data)]
}

func (c *scripted) DecodeRank(data []byte, _ int) (plugin.RankResponse, error) {
	return plugin.RankResponse{}, c.refusals[string(data)]
}

func (c *scripted) DecodePreset(data []byte, _ string) (plugin.PresetResponse, error) {
	return plugin.PresetResponse{}, c.refusals[string(data)]
}

// approvedBut is a Trust that approves every digest but no. The host only
// asks: any other method of Trust is a nil call, and panics.
type approvedBut struct {
	port.Trust
	no model.Digest
}

func (a approvedBut) Approved(_ context.Context, d model.Digest) (bool, error) {
	return d != a.no, nil
}

// TestTheHostKeepsThePluginsContract: the Host holds port.Plugins and
// port.Plugin as the contracttest suite reads them, on modules
// the assembler makes. Each refusal of Open is a bundle planted at the root;
// each failure of a call is a module that ends in it, or an answer the codec
// refuses, since the host passes the codec's refusals on as they are:
// ErrStepNotReady is the step runner's, planted here through the
// codec. Ran counts the requests the codec wrote, which no call that runs no
// code reaches.
func TestTheHostKeepsThePluginsContract(t *testing.T) {
	t.Parallel()
	contracttest.Plugins(t, pluginsFixture)
}

// pluginsFixture is a Host over the suite's plugins, one fresh per case.
func pluginsFixture(t *testing.T) contracttest.PluginsFixture {
	t.Helper()
	const at, empty = "/project", "/empty"
	ref := func(name, version string) model.PluginRef { return model.PluginRef{Name: name, Version: version} }
	bundles := &porttest.Bundles{}
	put := func(r model.PluginRef, m mod, replace ...string) []byte {
		module := m.bytes()
		named := append([]string{"name: standin", "name: " + r.Name, "version: 1.0.0", "version: " + r.Version}, replace...)
		bundles.Put(at, r, manifestOf(t, module, named...), module)
		return module
	}
	installed := []model.PluginRef{ref("standin", "1.10.0"), ref("standin", "1.9.0"), ref("tally", "1.0.0")}
	for _, r := range installed {
		put(r, say("ok"))
	}
	refused := map[error]model.PluginRef{
		port.ErrManifest:    ref("r-manifest", "1.0.0"),
		port.ErrNotApproved: ref("r-approval", "1.0.0"),
		port.ErrDigest:      ref("r-digest", "1.0.0"),
		port.ErrContract:    ref("r-contract", "1.0.0"),
		port.ErrFeature:     ref("r-feature", "1.0.0"),
		port.ErrModule:      ref("r-module", "1.0.0"),
	}
	put(refused[port.ErrManifest], say("ok"), "warning: false", "warning: false\nlater: true")
	put(refused[port.ErrNotApproved], say("ok"))
	put(refused[port.ErrContract], say("ok"), "contract: 1", "contract: 2")
	put(refused[port.ErrFeature], say("ok"), "needs: []", "needs: [streams]")
	noStart := say("ok")
	noStart.exports = noStart.exports[1:]
	put(refused[port.ErrModule], noStart)
	digested := put(refused[port.ErrDigest], say("ok"))
	if err := bundles.Change(at, refused[port.ErrDigest], digested[:len(digested)-1]); err != nil {
		t.Fatal(err)
	}
	unapproved, err := bundles.Read(t.Context(), at, refused[port.ErrNotApproved])
	if err != nil {
		t.Fatal(err)
	}
	codec := &scripted{refusals: map[string]error{
		"grant":     fmt.Errorf("planted: %w; %w", port.ErrGrant, &plugin.AnswerError{Path: "/steps/0/program", Why: "not granted"}),
		"not ready": fmt.Errorf("planted: %w", port.ErrStepNotReady),
		"own":       &plugin.CallError{Code: plugin.CodeRefused, Message: "planted"},
		"bad":       &plugin.AnswerError{Path: "/entries", Why: "planted"},
	}}
	plan := func(ctx context.Context, p port.Plugin) error {
		_, err := p.Plan(ctx, plugin.PlanRequest{})
		return err
	}
	var failures []contracttest.PluginFailure
	for _, f := range []struct {
		name string
		m    mod
		is   error
		as   any
	}{
		{name: "a program not granted", m: say("grant"), is: port.ErrGrant, as: new(*plugin.AnswerError)},
		{name: "a step not ready", m: say("not ready"), is: port.ErrStepNotReady},
		{name: "a crash", m: wasiMod(1, op(opUnreachable)), is: port.ErrTrap},
		{name: "the plugin's own error", m: say("own"), as: new(*plugin.CallError)},
		{name: "over memory", m: wasiMod(pastPages, nil), as: new(*plugin.LimitError)},
		{name: "a bad answer", m: say("bad"), as: new(*plugin.AnswerError)},
	} {
		r := ref("f-"+fmt.Sprint(len(failures)), "1.0.0")
		put(r, f.m)
		failures = append(failures, contracttest.PluginFailure{Name: f.name, Ref: r, Call: plan, Is: f.is, As: f.as})
	}
	h, err := New(Config{
		Bundles:  bundles,
		Trust:    approvedBut{no: unapproved.Locked},
		Judge:    func(plugin.Manifest) error { return nil },
		Codec:    codec,
		Features: []string{},
		Reserved: reservedOfTheRows(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close(context.WithoutCancel(t.Context())) })
	return contracttest.PluginsFixture{
		Plugins:   h,
		Root:      at,
		Empty:     empty,
		Installed: installed,
		Missing:   ref("standin", "9.9.9"),
		Refused:   refused,
		Call:      plan,
		Failures:  failures,
		Ran:       func(model.PluginRef) int { return int(codec.written.Load()) },
	}
}
