package link_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// TestLoosensRanksAValueByThePluginsRankAndTakesWhatItCannotRankAsLoosening:
// off is below every value of the order, a higher value is tighter, a value in
// neither list is unknown, and a lower severity loosens with the value held.
// Risk: a rebind that turns an entry off, or to a value nobody can compare,
// with no decision (S70).
func TestLoosensRanksAValueByThePluginsRankAndTakesWhatItCannotRankAsLoosening(t *testing.T) {
	t.Parallel()
	sc := scale{off: []any{false, "off"}, order: []any{"warn", "strict"}}.rank()
	b := func(v any, s model.Severity) model.Binding { return model.Binding{Value: v, Severity: s} }
	for _, tc := range []struct {
		name      string
		old, next model.Binding
		want      bool
	}{
		{"same", b("warn", model.SeverityError), b("warn", model.SeverityError), false},
		{"higher", b("warn", model.SeverityError), b("strict", model.SeverityError), false},
		{"lower", b("strict", model.SeverityError), b("warn", model.SeverityError), true},
		{"turned off", b("warn", model.SeverityError), b(false, model.SeverityError), true},
		{"off spelled two ways", b(false, model.SeverityError), b("off", model.SeverityError), false},
		{"from off to on", b("off", model.SeverityError), b("warn", model.SeverityError), false},
		{"not on the scale", b("warn", model.SeverityError), b("elsewhere", model.SeverityError), true},
		{"lower severity", b("warn", model.SeverityError), b("warn", model.SeverityWarning), true},
		{"higher severity", b("warn", model.SeverityInfo), b("warn", model.SeverityError), false},
	} {
		if got := link.Loosens(sc, tc.old, tc.next); got != tc.want {
			t.Errorf("%s: Loosens = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !link.Loosens(nil, b("a", model.SeverityError), b("b", model.SeverityError)) {
		t.Error("a change with no rank at all is not taken as loosening")
	}
}
