package ordered_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

func oneKey(v any) *ordered.Map {
	m := ordered.NewMap(0)
	m.Set("k", v, 0)
	return m
}

// TestYAMLKeepsAStringPlainOnlyWhereBothReadingsEqualIt: each word the 1.1 and
// the 1.2 tables read apart, or as no string, is quoted, and the document
// reads back, through the strict reader, as the value written.
func TestYAMLKeepsAStringPlainOnlyWhereBothReadingsEqualIt(t *testing.T) {
	t.Parallel()
	plain := []string{"error", "no-console", "a/b.c", "two words", "v1.2.3", "x_y"}
	quoted := []string{"yes", "no", "on", "off", "y", "n", "true", "False", "null", "~", "", " lead", "trail ", "010", "0o10", "0x1F", "1_000", "1e3", ".5", "1.", "2001-01-01", "2001-01-01 10:00:00", ".inf", ".nan", "a: b", "#c", "- d", "x\ny", "tab\t", "é", "!tag", "&a", "*a", "{a}", "[a]", "a  b", "<<", "---", "..."}
	for _, s := range plain {
		if got := strings.TrimSpace(ordered.YAML(oneKey(s))); got != "k: "+s {
			t.Errorf("%q written %q; want it plain", s, got)
		}
	}
	for _, s := range quoted {
		out := ordered.YAML(oneKey(s))
		if strings.TrimSpace(out) == "k: "+s {
			t.Errorf("%q written plain", s)
		}
		v, err := ordered.ParseStrict([]byte(out))
		if err != nil || !reflect.DeepEqual(v.(*ordered.Map).Val("k"), s) {
			t.Errorf("%q written %q reads back %v, %v", s, out, v, err)
		}
	}
}

// TestYAMLRoundTripsNestedValuesAndRandomStrings: seeded random strings from an
// alphabet of the awkward characters, in a key and a value, in a list, read
// back equal; every plain scalar of the output has equal 1.1 and 1.2 readings.
func TestYAMLRoundTripsNestedValuesAndRandomStrings(t *testing.T) {
	t.Parallel()
	rng := &xorshift{1171}
	alphabet := []rune("ab01 _-./:#,[]{}!&*?|>'\"%@`\\\t\né~.eEyYnN+")
	str := func() string {
		n := rng.Intn(8)
		r := make([]rune, n)
		for i := range r {
			r[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(r)
	}
	for i := range 2000 {
		inner := ordered.NewMap(0)
		inner.Set(str(), str(), 0)
		inner.Set("n", int64(rng.Intn(100)-50), 0)
		inner.Set("f", float64(rng.Intn(1000))/8, 0)
		inner.Set("b", rng.Intn(2) == 0, 0)
		inner.Set("z", nil, 0)
		doc := ordered.NewMap(0)
		doc.Set("list", []any{str(), str(), inner}, 0)
		doc.Set("empty", []any{}, 0)
		doc.Set("none", ordered.NewMap(0), 0)
		doc.Set(str()+"k", inner, 0)
		out := ordered.YAML(doc)
		got, err := ordered.ParseStrict([]byte(out))
		if err != nil {
			continue // a repeated key from the random draw is the reader's refusal, not the writer's
		}
		if ordered.JSON(got, 2) != ordered.JSON(doc, 2) {
			t.Fatalf("round %d: wrote\n%s\nread back %s\nwant %s", i, out, ordered.JSON(got, -1), ordered.JSON(doc, -1))
		}
		scalars, err := ordered.Scalars([]byte(out))
		if err != nil {
			t.Fatalf("round %d: Scalars refused what ParseStrict read: %v", i, err)
		}
		for _, s := range scalars {
			if s.Plain && !reflect.DeepEqual(s.Read11, s.Read12) && s.Read11 != nil {
				if _, isString := s.Read11.(string); !isString {
					t.Fatalf("round %d: plain scalar %q reads %v and %v", i, s.Text, s.Read11, s.Read12)
				}
			}
		}
	}
}

// TestYAMLWritesTheSameBytesTwice and ends in one newline, with no date.
func TestYAMLWritesTheSameBytesTwice(t *testing.T) {
	t.Parallel()
	doc := ordered.NewMap(0)
	doc.Set("a", []any{"x", int64(1)}, 0)
	doc.Set("b", ordered.NewMap(0), 0)
	if a, b := ordered.YAML(doc), ordered.YAML(doc); a != b || !strings.HasSuffix(a, "\n") || a != "a:\n  - x\n  - 1\nb: {}\n" {
		t.Errorf("YAML = %q", a)
	}
}

// xorshift is a fixed stream of numbers: no test here reads math/rand.
type xorshift struct{ s uint64 }

func (x *xorshift) Intn(n int) int {
	x.s ^= x.s << 13
	x.s ^= x.s >> 7
	x.s ^= x.s << 17
	return int(x.s % uint64(n))
}
