package contracttest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// PluginFailure is one failure of a call the port's doc names, planted by the
// adapter: a plugin of Ref whose Call ends in it. Name labels the case. The
// error must match Is with errors.Is where Is is set, and As with errors.As
// where As is set (a pointer to *plugin.CallError, *plugin.LimitError or
// *plugin.AnswerError).
type PluginFailure struct {
	Name string
	Ref  model.PluginRef
	Call func(ctx context.Context, p port.Plugin) error
	Is   error
	As   any
}

// PluginsFixture is what the Plugins suite is given: an adapter and the plugins
// it holds. A promise that cannot be planted from outside an adapter is planted
// here, by the adapter, which is the one that knows how to make its module
// break the promise; the suite fails, naming it, for one the fixture plants
// none of.
type PluginsFixture struct {
	// Plugins is the port under test, and Root the folder it holds Installed,
	// Refused and Failures under.
	Plugins port.Plugins
	Root    string
	// Empty is a root nothing is installed under.
	Empty string
	// Installed are plugins that open and serve: at least two of one name
	// with versions that sort one way as bytes and another as numbers, and
	// one of a name that sorts after it, so List's order is told from the
	// order they were installed in.
	Installed []model.PluginRef
	// Missing is a ref no plugin is installed under at Root.
	Missing model.PluginRef
	// Refused is, for each sentinel Open answers with (ErrManifest,
	// ErrNotApproved, ErrDigest, ErrContract, ErrFeature, ErrModule: ErrDigest
	// is a module that does not match its lock), the ref of a plugin installed
	// at Root that Open refuses with it.
	Refused map[error]model.PluginRef
	// Call is a call that succeeds on a live ctx, on any plugin of Installed.
	Call func(ctx context.Context, p port.Plugin) error
	// Failures is a failure for each of port.ErrGrant, port.ErrStepNotReady,
	// port.ErrTrap and the plugin's own, the limit's and the answer's errors.
	Failures []PluginFailure
	// Ran is how many calls have reached the plugin of ref's own code, since
	// the fixture was built. The suite fails, naming it, where it is nil: a
	// refused plugin, a call on a dead ctx and a call after Close that run no
	// code are checked by it alone.
	Ran func(ref model.PluginRef) int
}

// openSentinels are the sentinels Open answers with, in the doc's order, and
// each a ref of PluginsFixture.Refused.
func openSentinels() []error {
	return []error{port.ErrManifest, port.ErrNotApproved, port.ErrDigest, port.ErrContract, port.ErrFeature, port.ErrModule}
}

// Plugins runs the Plugins suite against the adapter build answers: the
// promises of port.Plugins and port.Plugin that are not what a call answers
// (the cases of Plugin hold that): which plugins List names, the refusals of
// Open, the context's end of an Open or a call, the failures of a call, and
// the end of a plugin at Close. Each case builds its own fixture.
func Plugins(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	pluginsListing(t, build)
	t.Run("the fixture counts the calls that reach the plugin's code", func(t *testing.T) {
		if build(t).Ran == nil {
			t.Fatalf("the fixture counts no calls of the plugin's code, so a refused plugin running, a call on a dead ctx and a call after Close are unchecked")
		}
	})
	pluginsOpening(t, build)
	pluginsDeadCtx(t, build)
	pluginsCalls(t, build)
	pluginsClosing(t, build)
}

// pluginsListing holds the cases of List.
func pluginsListing(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	t.Run("a root with no plugin lists none, and no error", func(t *testing.T) {
		f := build(t)
		got, err := f.Plugins.List(t.Context(), f.Empty)
		if err != nil || got != nil {
			t.Errorf("List of a root with none = %#v, %v, want a nil list and no error", got, err)
		}
	})
	t.Run("the plugins list by name, then by version as bytes", func(t *testing.T) {
		f := build(t)
		if len(f.Installed) < 3 {
			t.Fatalf("the fixture installs %d plugins, want at least 3", len(f.Installed))
		}
		got, err := f.Plugins.List(t.Context(), f.Root)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if want := listedRefs(f); !slices.Equal(got, want) {
			t.Errorf("List = %v, want %v", got, want)
		}
	})
}

// listedRefs is the refs List must name for f, in the order it must name them.
func listedRefs(f PluginsFixture) []model.PluginRef {
	want := slices.Clone(f.Installed)
	for _, r := range f.Refused {
		want = append(want, r)
	}
	for _, r := range f.Failures {
		want = append(want, r.Ref)
	}
	slices.SortFunc(want, func(a, b model.PluginRef) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Version, b.Version)
	})
	return slices.Compact(want)
}

// pluginsOpening holds the cases of Open's refusals.
func pluginsOpening(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	t.Run("a ref no plugin is installed under is ErrNotInstalled", func(t *testing.T) {
		f := build(t)
		p, err := f.Plugins.Open(t.Context(), f.Root, f.Missing)
		if p != nil || !errors.Is(err, port.ErrNotInstalled) {
			t.Errorf("Open of a missing ref = %v, %v, want no plugin and ErrNotInstalled", p, err)
		}
		if err != nil && refusedBy(err, port.ErrNotInstalled) {
			t.Errorf("Open of a missing ref matches another sentinel too: %v", err)
		}
	})
	for _, want := range openSentinels() {
		t.Run("Open refuses with "+sentinelName(want)+", opens nothing and runs no code", func(t *testing.T) {
			openRefused(t, build(t), want)
		})
	}
}

// openRefused is the case of one sentinel Open refuses with.
func openRefused(t *testing.T, f PluginsFixture, want error) {
	t.Helper()
	ref, ok := f.Refused[want]
	if !ok {
		t.Fatalf("the fixture plants no plugin that Open refuses with %v", want)
	}
	before := pluginRan(f, ref)
	p, err := f.Plugins.Open(t.Context(), f.Root, ref)
	if p != nil {
		closeLogged(t, p)
		t.Errorf("Open of a refused plugin answered one")
	}
	if !errors.Is(err, want) {
		t.Errorf("Open = %v, want %v", err, want)
	}
	if err != nil && refusedBy(err, want) {
		t.Errorf("Open = %v, matches a sentinel besides %v", err, want)
	}
	if after := pluginRan(f, ref); after != before {
		t.Errorf("a refused plugin ran %d calls of its code", after-before)
	}
}

// pluginsDeadCtx holds the cases of a ctx that has ended.
func pluginsDeadCtx(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	for _, name := range []string{"Open", "Plan", "Entries", "Findings", "Rank", "Preset"} {
		for _, dead := range []string{"expired", "cancelled"} {
			t.Run(dead+" ctx ends "+name+" with the context's own error", func(t *testing.T) {
				f := build(t)
				ctx, cancel := deadCtx(t, dead)
				defer cancel()
				err := callDead(ctx, t, f, name)
				want := context.DeadlineExceeded
				if dead == "cancelled" {
					want = context.Canceled
				}
				if !errors.Is(err, want) {
					t.Errorf("%s = %v, want an error matching %v", name, err, want)
				}
				if err != nil && refusedBy(err, nil) {
					t.Errorf("%s = %v, matches a sentinel of the port, which the context's error must not", name, err)
				}
			})
		}
	}
}

// callDead is the operation name of f, on the dead ctx, and its error.
func callDead(ctx context.Context, t *testing.T, f PluginsFixture, name string) error {
	t.Helper()
	if name == "Open" {
		before := pluginRan(f, f.Installed[0])
		p, err := f.Plugins.Open(ctx, f.Root, f.Installed[0])
		if p != nil {
			closeLogged(t, p)
			t.Errorf("Open on a dead ctx answered a plugin")
		}
		if after := pluginRan(f, f.Installed[0]); after != before {
			t.Errorf("Open on a dead ctx ran the plugin's code")
		}
		return err
	}
	p := openPluginCtx(context.WithoutCancel(ctx), t, f, f.Installed[0])
	before := pluginRan(f, f.Installed[0])
	err := callOp(ctx, p, name)
	if after := pluginRan(f, f.Installed[0]); after != before {
		t.Errorf("%s on a dead ctx ran the plugin's code", name)
	}
	return err
}

// pluginsCalls holds the cases of a call on a live ctx.
func pluginsCalls(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	t.Run("a call that succeeds answers no error", func(t *testing.T) {
		f := build(t)
		p := openPlugin(t, f, f.Installed[0])
		if err := f.Call(t.Context(), p); err != nil {
			t.Errorf("the fixture's call = %v, want it to succeed", err)
		}
	})
	t.Run("the manifest is read at Open and is no call", func(t *testing.T) {
		f := build(t)
		p := openPlugin(t, f, f.Installed[0])
		before := pluginRan(f, f.Installed[0])
		m1, m2 := p.Manifest(), p.Manifest()
		if m1.Name != f.Installed[0].Name || m1.Version != f.Installed[0].Version {
			t.Errorf("Manifest names %s %s, want %v", m1.Name, m1.Version, f.Installed[0])
		}
		if m1.Name != m2.Name || m1.Version != m2.Version {
			t.Errorf("Manifest answered two manifests: %v and %v", m1, m2)
		}
		if after := pluginRan(f, f.Installed[0]); after != before {
			t.Errorf("Manifest ran the plugin's code %d times", after-before)
		}
	})
	pluginsTogether(t, build)
}

// pluginsTogether holds the cases of calls and plugins at once.
func pluginsTogether(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	t.Run("calls at once each succeed", func(t *testing.T) {
		f := build(t)
		p := openPlugin(t, f, f.Installed[0])
		var wg sync.WaitGroup
		errs := make([]error, 8)
		for i := range errs {
			// measured: calls of one plugin at once, 8 overlapping where a
			// serial loop would hide a shared-state fault, as the suite's
			// planted serial fault shows
			wg.Go(func() { errs[i] = f.Call(t.Context(), p) })
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Errorf("call %d of 8 at once = %v", i, err)
			}
		}
	})
	t.Run("a plugin opened twice is two plugins", func(t *testing.T) {
		f := build(t)
		a, b := openPlugin(t, f, f.Installed[0]), openPlugin(t, f, f.Installed[0])
		if err := a.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := f.Call(t.Context(), b); err != nil {
			t.Errorf("a call on a plugin opened beside one closed = %v, want it to succeed", err)
		}
	})
	pluginsFailures(t, build)
}

// pluginsFailures holds the cases of a call that fails.
func pluginsFailures(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	t.Run("a call failing is an error, and a plugin's, a limit's, an answer's and the host's are told apart", func(t *testing.T) {
		f := build(t)
		for _, want := range []error{port.ErrGrant, port.ErrStepNotReady, port.ErrTrap} {
			if !slices.ContainsFunc(f.Failures, func(c PluginFailure) bool { return errors.Is(c.Is, want) }) {
				t.Errorf("the fixture plants no call that fails with %v", want)
			}
		}
		var ce *plugin.CallError
		var le *plugin.LimitError
		var ae *plugin.AnswerError
		for _, kind := range []any{&ce, &le, &ae} {
			if !slices.ContainsFunc(f.Failures, func(c PluginFailure) bool { return sameType(c.As, kind) }) {
				t.Errorf("the fixture plants no call that fails with a %T", kind)
			}
		}
	})
	pluginsFailureMatches(t, build)
}

// pluginsFailureMatches holds the case that each planted failure matches what
// it names.
func pluginsFailureMatches(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	t.Run("every failure of a call is an error that matches what it names", func(t *testing.T) {
		f := build(t)
		for _, c := range f.Failures {
			t.Run(c.Name, func(t *testing.T) { failureMatches(t, f, c) })
		}
	})
}

// failureMatches is the case of one planted failure.
func failureMatches(t *testing.T, f PluginsFixture, c PluginFailure) {
	t.Helper()
	p := openPlugin(t, f, c.Ref)
	err := c.Call(t.Context(), p)
	if err == nil {
		t.Fatalf("the call answered no error, want one")
	}
	if c.Is != nil && !errors.Is(err, c.Is) {
		t.Errorf("error = %v, want one matching %v", err, c.Is)
	}
	if c.As != nil && !errors.As(err, c.As) {
		t.Errorf("error = %v, want one taken as %T", err, c.As)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, matches a context's end, which the ctx did not come to", err)
	}
	if refusedBy(err, c.Is) {
		t.Errorf("error = %v, matches a sentinel of the port besides what the failure names", err)
	}
}

// pluginsClosing holds the cases of a plugin after Close.
func pluginsClosing(t *testing.T, build func(t *testing.T) PluginsFixture) {
	t.Helper()
	for _, name := range []string{"Plan", "Entries", "Findings", "Rank", "Preset"} {
		t.Run("nothing is called after Close: "+name, func(t *testing.T) {
			f := build(t)
			p := openPlugin(t, f, f.Installed[0])
			if err := p.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			before := pluginRan(f, f.Installed[0])
			err := callOp(t.Context(), p, name)
			if err == nil {
				t.Errorf("%s after Close answered no error", name)
			}
			if after := pluginRan(f, f.Installed[0]); after != before {
				t.Errorf("%s after Close ran the plugin's code", name)
			}
		})
	}
	t.Run("the fixture's call after Close is an error", func(t *testing.T) {
		f := build(t)
		p := openPlugin(t, f, f.Installed[0])
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := f.Call(t.Context(), p); err == nil {
			t.Error("a call after Close answered no error")
		}
	})
}

// closeLogged closes p, and logs an error of it: the plugin was not to be
// opened, and its Close is not the case's.
func closeLogged(t *testing.T, p port.Plugin) {
	t.Helper()
	if err := p.Close(); err != nil {
		t.Logf("Close: %v", err)
	}
}

// openPlugin opens ref of f and closes it with the test.
func openPlugin(t *testing.T, f PluginsFixture, ref model.PluginRef) port.Plugin {
	t.Helper()
	return openPluginCtx(t.Context(), t, f, ref)
}

// openPluginCtx is openPlugin on a ctx the caller holds.
func openPluginCtx(ctx context.Context, t *testing.T, f PluginsFixture, ref model.PluginRef) port.Plugin {
	t.Helper()
	p, err := f.Plugins.Open(ctx, f.Root, ref)
	if err != nil {
		t.Fatalf("Open(%v): %v", ref, err)
	}
	t.Cleanup(func() { closeLogged(t, p) })
	return p
}

// pluginRan is Ran of f for ref, 0 where the fixture reads none.
func pluginRan(f PluginsFixture, ref model.PluginRef) int {
	if f.Ran == nil {
		return 0
	}
	return f.Ran(ref)
}

// deadCtx is a ctx whose deadline has passed ("expired") or that is cancelled.
func deadCtx(t *testing.T, how string) (context.Context, context.CancelFunc) {
	t.Helper()
	if how == "cancelled" {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx, cancel
	}
	return context.WithDeadline(t.Context(), time.Time{})
}

// callOp calls the operation named on p with an empty request.
func callOp(ctx context.Context, p port.Plugin, name string) error {
	var err error
	switch name {
	case "Plan":
		_, err = p.Plan(ctx, plugin.PlanRequest{})
	case "Entries":
		_, err = p.Entries(ctx, plugin.EntriesRequest{})
	case "Findings":
		_, err = p.Findings(ctx, plugin.FindingsRequest{})
	case "Rank":
		_, err = p.Rank(ctx, plugin.RankRequest{})
	case "Preset":
		_, err = p.Preset(ctx, plugin.PresetRequest{})
	}
	return err
}

// refusedBy is whether err matches a sentinel of the port besides except (nil
// for any): of the ones Open and a call name.
func refusedBy(err, except error) bool {
	for _, s := range append(openSentinels(), port.ErrNotInstalled, port.ErrGrant, port.ErrStepNotReady, port.ErrTrap) {
		if !errors.Is(s, except) && errors.Is(err, s) {
			return true
		}
	}
	return false
}

// sentinelName is the sentinel's text, the name of a case.
func sentinelName(err error) string {
	return err.Error()
}

// sameType is whether a and b are of one type, as %T writes it.
func sameType(a, b any) bool {
	return a != nil && b != nil && fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
}
