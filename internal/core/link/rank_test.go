package link_test

import (
	"reflect"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// scale is how the tests' plugin ranks a tool's values: the off
// values the same as each other and below every other value, then order,
// lowest to highest; keys gives a key a scale of its own.
type scale struct {
	off, order []any
	keys       map[string]scale
}

// rank is s as a plugin's rank op: unknown for a value on neither list.
func (s scale) rank() link.Rank {
	return func(key string, preset, found canon.Value) plugin.Word {
		sc := s
		if k, ok := s.keys[key]; ok {
			sc = k
		}
		p, pok := sc.place(preset)
		f, fok := sc.place(found)
		switch {
		case !pok || !fok:
			return plugin.WordUnknown
		case f == p:
			return plugin.WordSame
		case f > p:
			return plugin.WordTighter
		}
		return plugin.WordWeaker
	}
}

// every is s as the Rank of each tool.
func (s scale) every() func(string) link.Rank {
	return func(string) link.Rank { return s.rank() }
}

// place is where v stands in s: 0 for off, else one past its place in order.
func (s scale) place(v canon.Value) (int, bool) {
	for _, o := range s.off {
		if sameCanon(o, v) {
			return 0, true
		}
	}
	for i, o := range s.order {
		if sameCanon(o, v) {
			return i + 1, true
		}
	}
	return 0, false
}

func sameCanon(a any, v canon.Value) bool {
	c, err := link.Canon(a)
	return err == nil && reflect.DeepEqual(c, v)
}

// TestAValueTheSameAsTextIsNeverRankedAndOneThatCannotBeIsNeverTheSame: two
// values with the same text are the same with no call to the plugin; with no
// rank, two that differ are unknown, so neither conflicts nor tightens away;
// a rank that answers same for a pair is taken at its word. Planted: a nil
// rank taken as same (the no-rank rows red).
func TestAValueTheSameAsTextIsNeverRankedAndOneThatCannotBeIsNeverTheSame(t *testing.T) {
	t.Parallel()
	calls := 0
	counted := func(w plugin.Word) link.Rank {
		return func(string, canon.Value, canon.Value) plugin.Word { calls++; return w }
	}
	b := func(v any) model.Binding {
		return model.Binding{Tool: "lint", Target: "code", Key: "k", Value: v, Severity: model.SeverityError, Rung: 1}
	}
	if same, a := link.Tighten(counted(plugin.WordWeaker), b(1), b(1.0)); !same || a != nil || calls != 0 {
		t.Errorf("1 and 1.0: same %v, against %+v after %d calls; want the same with no call", same, a, calls)
	}
	if same, a := link.Tighten(nil, b("a"), b("b")); same || a == nil || a.Field != link.FieldValue {
		t.Errorf("no rank: same %v, against %+v; want the value refused", same, a)
	}
	if !link.Loosens(nil, b("a"), b("b")) {
		t.Error("no rank: a changed value is not taken as loosening")
	}
	if same, a := link.Tighten(counted(plugin.WordSame), b(false), b("off")); !same || a != nil || calls != 1 {
		t.Errorf("a rank of same: same %v, against %+v after %d calls; want the same after one", same, a, calls)
	}
}
