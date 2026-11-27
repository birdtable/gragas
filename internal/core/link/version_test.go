package link_test

import (
	"errors"
	"fmt"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
)

// TestParseRangeReadsTheClosedGrammarAndRefusesTheRest.
func TestParseRangeReadsTheClosedGrammarAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	b := func(v string, in bool) model.VersionBound { return model.VersionBound{Version: v, Included: in} }
	good := map[string]model.VersionRange{
		">=1.2.3":                            {Lower: b("1.2.3", true)},
		">1.2.3":                             {Lower: b("1.2.3", false)},
		"<=2.0.0":                            {Upper: b("2.0.0", true)},
		"<2.0.0":                             {Upper: b("2.0.0", false)},
		"=1.0.0":                             {Lower: b("1.0.0", true), Upper: b("1.0.0", true)},
		">=1.0.0 <2.0.0":                     {Lower: b("1.0.0", true), Upper: b("2.0.0", false)},
		"<2.0.0 >=1.0.0":                     {Lower: b("1.0.0", true), Upper: b("2.0.0", false)},
		">=1.0.0 <=1.0.0":                    {Lower: b("1.0.0", true), Upper: b("1.0.0", true)},
		">=0.0.0 <100000000000000000000.0.0": {Lower: b("0.0.0", true), Upper: b("100000000000000000000.0.0", false)},
	}
	for s, want := range good {
		if got, err := link.ParseRange(s); err != nil || got != want {
			t.Errorf("ParseRange(%q) = %+v, %v; want %+v", s, got, err, want)
		}
	}
	for _, s := range []string{"", " ", "1.2.3", "^1.2.3", "~1.2.3", "1.x", ">=1.2", ">=1.2.3.4", ">=01.2.3", ">=1.2.3-rc.1",
		">=1.0.0  <2.0.0", ">=1.0.0 <2.0.0 <3.0.0", ">=1.0.0 >=2.0.0", "<1.0.0 <2.0.0", "=1.0.0 <2.0.0", ">=1.0.0 || <0.5.0",
		">=2.0.0 <1.0.0", ">1.0.0 <1.0.0", ">=1.0.0 <1.0.0", ">1.0.0 <=1.0.0", "> 1.0.0", "==1.0.0", ">=v1.0.0"} {
		if got, err := link.ParseRange(s); !errors.Is(err, link.ErrRange) {
			t.Errorf("ParseRange(%q) = %+v, %v; want ErrRange", s, got, err)
		}
	}
	if _, err := link.InRange(model.VersionRange{}, "1.0"); !errors.Is(err, link.ErrRange) {
		t.Errorf("InRange of no version: %v", err)
	}
}

// TestARangeHoldsExactlyTheVersionsItsBoundsSay is the property test
// (tests.generated.2): over versions drawn at a fixed seed, Compare is an
// order, and a version is in ">=a <b" exactly when a <= v < b, and in "=a"
// exactly when it is a.
func TestARangeHoldsExactlyTheVersionsItsBoundsSay(t *testing.T) {
	t.Parallel()
	rng := configtest.NewDraw(178)
	v := func() string {
		n := func() string {
			if rng.IntN(8) == 0 {
				return fmt.Sprint(rng.Uint64())
			}
			return fmt.Sprint(rng.IntN(4))
		}
		return n() + "." + n() + "." + n()
	}
	for range 5000 {
		a, b, c := v(), v(), v()
		if link.Compare(a, b) != -link.Compare(b, a) || link.Compare(a, a) != 0 {
			t.Fatalf("Compare is not antisymmetric at %s, %s", a, b)
		}
		if link.Compare(a, b) <= 0 && link.Compare(b, c) <= 0 && link.Compare(a, c) > 0 {
			t.Fatalf("Compare is not transitive at %s, %s, %s", a, b, c)
		}
		vr, err := link.ParseRange(">=" + a + " <" + b)
		if link.Compare(a, b) >= 0 {
			if !errors.Is(err, link.ErrRange) {
				t.Fatalf(">=%s <%s holds no version and parsed: %+v", a, b, vr)
			}
		} else {
			in, err := link.InRange(vr, c)
			if want := link.Compare(a, c) <= 0 && link.Compare(c, b) < 0; err != nil || in != want {
				t.Fatalf("%s in >=%s <%s = %v, %v; want %v", c, a, b, in, err, want)
			}
		}
		exact, err := link.ParseRange("=" + a)
		if in, ierr := link.InRange(exact, c); err != nil || ierr != nil || in != (link.Compare(a, c) == 0) {
			t.Fatalf("%s in =%s = %v, %v, %v", c, a, in, err, ierr)
		}
	}
}
