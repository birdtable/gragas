package plugin

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// TestTryRefusesEachModuleOpenRefuses: a module
// under test is held to what a plugin's
// module is as an installed one is: each module fault Open refuses, Try
// refuses with port.ErrModule, and the clean module tries. Try asks no
// approval and no judgement, even of a host that approves nothing. Risk: a
// module plugin test runs without approval is held to less than one that is
// installed.
func TestTryRefusesEachModuleOpenRefuses(t *testing.T) {
	t.Parallel()
	rows := append([]fault{{"planted: the clean module", echo().bytes()}}, moduleFaults()...)
	for i, f := range rows {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, manifestOf(t, f.module), f.module)
			r.trust.no = true
			tr, err := r.host.Try(t.Context(), plugin.Manifest{}, f.module)
			if err == nil {
				t.Cleanup(func() { _ = tr.Close() })
			}
			switch {
			case i == 0 && (err != nil || tr == nil):
				t.Fatalf("the clean module = %v; want it tried", err)
			case i > 0 && (!errors.Is(err, port.ErrModule) || tr != nil):
				t.Fatalf("Try = %v, %v; want %v, as Open's", tr, err, port.ErrModule)
			case r.trust.asked != 0 || r.judged != 0:
				t.Errorf("Try asked the approvals %d times and the judge %d; want neither", r.trust.asked, r.judged)
			}
		})
	}
}

// TestATrialIsHeldToTheLimitsOfACall: a trial's raw call runs
// with the limits a typed call of an opened plugin has, callContext's: a
// module that grows its memory past MemoryLimit, in one grow or by a start
// that needs it, ends over the memory limit; a trap is ErrTrap; the echo
// answers the request; and a closed trial runs nothing. Risk: a raw call,
// which no codec holds, runs a module with no memory or crash limit.
func TestATrialIsHeldToTheLimitsOfACall(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		module mod
		check  func(out []byte, err error) bool
	}{
		{"planted: the echo answers the request", echo(), func(out []byte, err error) bool { return err == nil && bytes.Equal(out, []byte("request")) }},
		{"a grow past the memory limit", growingOnce(), func(_ []byte, err error) bool { return limitOf(err) == plugin.LimitMemory }},
		{"a start past the memory limit", wasiMod(pastPages, nil), func(_ []byte, err error) bool { return limitOf(err) == plugin.LimitMemory }},
		{"a crash", wasiMod(1, op(opUnreachable)), func(_ []byte, err error) bool { return errors.Is(err, port.ErrTrap) }},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			module := row.module.bytes()
			r := newRig(t, manifestOf(t, module), module)
			tr, err := r.host.Try(t.Context(), plugin.Manifest{}, module)
			if err != nil {
				t.Fatalf("Try = %v; want the module tried", err)
			}
			t.Cleanup(func() { _ = tr.Close() })
			out, err := tr.Raw(t.Context(), []byte("request"))
			if !row.check(out, err) {
				t.Errorf("Raw = %q, %v; want what a typed call of an opened plugin ends on", out, err)
			}
		})
	}
	module := echo().bytes()
	r := newRig(t, manifestOf(t, module), module)
	tr, err := r.host.Try(t.Context(), plugin.Manifest{}, module)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := tr.Raw(t.Context(), []byte("request")); err == nil || out != nil {
		t.Errorf("Raw on a closed trial = %q, %v; want it refused", out, err)
	}
}

// TestTryRefusesAModulePastTheBundlesBoundBeforeItCompiles: Open
// never compiles a module past pluginbundle.DefaultBounds().Module, since its
// bytes come from a read held
// to that bound; Try is handed its bytes, so it holds them to the same bound
// itself. The clean module, padded by a custom section to the bound, tries;
// one byte more is refused with port.ErrModule, naming the bound, and never
// reaches the compiler. Risk: plugin test hands the compiler a module of any
// size, which costs a large multiple of it in memory.
func TestTryRefusesAModulePastTheBundlesBoundBeforeItCompiles(t *testing.T) {
	t.Parallel()
	most := int(pluginbundle.DefaultBounds().Module)
	for _, row := range []struct {
		name    string
		size    int
		refused bool
	}{
		{"planted: a module of the bound", most, false},
		{"a module one byte past it", most + 1, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			module := paddedTo(t, echo().bytes(), row.size)
			r := newRig(t, manifestOf(t, module), module)
			tr, err := r.host.Try(t.Context(), plugin.Manifest{}, module)
			if err == nil {
				t.Cleanup(func() { _ = tr.Close() })
			}
			switch {
			case row.refused && (!errors.Is(err, port.ErrModule) || tr != nil || !strings.Contains(err.Error(), "past the")):
				t.Errorf("Try of %d bytes = %v, %v; want %v, past the bound", row.size, tr, err, port.ErrModule)
			case !row.refused && err != nil:
				t.Errorf("Try of %d bytes = %v; want the module tried", row.size, err)
			}
		})
	}
}

// paddedTo is module with a custom section, which no runtime runs, that
// brings it to size bytes in all.
func paddedTo(t *testing.T, module []byte, size int) []byte {
	t.Helper()
	label := name("pad")
	for k := 1; k <= 5; k++ {
		p := size - len(module) - 1 - k - len(label)
		if p >= 0 && len(uleb128(uint64(len(label)+p))) == k {
			out := append(slices.Clip(module), 0x00)
			out = append(out, uleb128(uint64(len(label)+p))...)
			out = append(out, label...)
			return append(out, make([]byte, p)...)
		}
	}
	t.Fatalf("no custom section brings %d bytes to %d", len(module), size)
	return nil
}
