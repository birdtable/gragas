package ordered_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/ordered"
)

// laughs is a billion-laughs document: levels anchors, each a list of ten
// aliases of the one before, so the last expands to 10^levels scalars.
func laughs(levels int) string {
	var b strings.Builder
	b.WriteString("l0: &l0 lol\n")
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&b, "l%d: &l%d [", i, i)
		for j := range 10 {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "*l%d", i-1)
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// The hang guard is no oracle of cost: a loaded host under -race takes many
// times the quiet figure, so the tests hold the work a parse did, and the
// guard only stops a runaway from hanging the run. It is what the test's own
// deadline leaves less a margin to report the hang, so it grows with -race,
// with load and with the tier's -timeout without the test knowing why it is
// slow.
const (
	guardMargin  = 30 * time.Second
	guardNoLimit = 300 * time.Second
	guardFloor   = 60 * time.Second
)

// hangGuard is how long a parse may run before t calls it a hang: the time
// left to t's deadline less guardMargin, guardNoLimit with no deadline
// (-timeout 0), and guardFloor at the least, but never more than half the
// time left, so the guard fires before the binary's own timeout panic and the
// hang is reported by name.
func hangGuard(t *testing.T) time.Duration {
	t.Helper()
	dl, ok := t.Deadline()
	if !ok {
		return guardNoLimit
	}
	left := time.Until(dl)
	return max(left-guardMargin, min(guardFloor, left/2))
}

// within runs parse, failing the test once it runs past hangGuard.
func within[T any](t *testing.T, doc string, parse func([]byte) T) T {
	t.Helper()
	guard := hangGuard(t)
	done := make(chan T, 1)
	go func() { done <- parse([]byte(doc)) }()
	select {
	case r := <-done:
		return r
	case <-time.After(guard):
		t.Fatalf("a parse of %d bytes ran past %s, a hang", len(doc), guard)
		var zero T
		return zero
	}
}

// parsed is what ParseYAML returns.
type parsed struct {
	v   any
	err error
}

// parseWithin is ParseYAML of doc, under the hang guard.
func parseWithin(t *testing.T, doc string) (any, error) {
	t.Helper()
	r := within(t, doc, func(b []byte) parsed {
		v, err := ordered.ParseYAML(b)
		return parsed{v, err}
	})
	return r.v, r.err
}

// spent is what ParseYAMLSpent returns.
type spent struct {
	spent, budget int
	err           error
}

// spentWithin is ParseYAMLSpent of doc, under the hang guard.
func spentWithin(t *testing.T, doc string) spent {
	t.Helper()
	return within(t, doc, func(b []byte) spent {
		_, s, budget, err := ordered.ParseYAMLSpent(b)
		return spent{s, budget, err}
	})
}

// longThrough is a document whose anchor holds a plain scalar of n bytes,
// written by item, read through m aliases: each alias would resolve the
// whole scalar again, n bytes of work for a node of the document.
func longThrough(item func(n int) string, n, m int) string {
	return "a: &a " + item(n) + "\nb: [" + strings.Repeat("*a,", m-1) + "*a]\n"
}

// longFloat is a plain float of n bytes, which every pattern of resolve
// reads to its end.
func longFloat(n int) string { return "1." + strings.Repeat("1", n-2) }

// longKeyMap is a map of plain keys of 1000 bytes each, n bytes of keys in
// all: yaml.v3 refuses a plain key past 1024. Each key is digits that end
// in a letter, so the int and float patterns of resolve read it to its end
// and it is still a string.
func longKeyMap(n int) string {
	keys := make([]string, 0, n/1000)
	for i := range n / 1000 {
		id := strconv.Itoa(i)
		keys = append(keys, id+strings.Repeat("1", 999-len(id))+"x: 1")
	}
	return "{" + strings.Join(keys, ", ") + "}"
}

func TestParseYAMLRefusesAnAliasBombBeforeItGrows(t *testing.T) {
	t.Parallel()
	bombs := map[string]string{
		"5 levels of laughs":                             laughs(5),
		"9 levels of laughs":                             laughs(9),
		"a 1000-byte plain float through 66 aliases":     longThrough(longFloat, 1000, 66),
		"a map of 1000 bytes of keys through 66 aliases": longThrough(longKeyMap, 1000, 66),
	}
	for name, doc := range bombs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseWithin(t, doc)
			var se *ordered.SyntaxError
			if !errors.As(err, &se) || !strings.Contains(se.Detail, "aliases expand to more than") || !strings.HasPrefix(se.Detail, "line ") {
				t.Fatalf("a %d-byte document: err %v, want a refusal that names its line", len(doc), err)
			}
			// The work: the converter spent no more than the budget before it
			// refused, and the refusal is the budget's, naming its figure.
			s := spentWithin(t, doc)
			if s.err == nil || s.err.Error() != err.Error() {
				t.Fatalf("the counted parse says %v, ParseYAML %v", s.err, err)
			}
			if want := max(64<<10, 6*len(doc)); s.budget != want || !strings.Contains(se.Detail, fmt.Sprintf("more than %d nodes", want)) {
				t.Errorf("a %d-byte document had a budget of %d, want %d named in the refusal: %s", len(doc), s.budget, want, se.Detail)
			}
			if s.spent > s.budget {
				t.Errorf("a %d-byte document spent %d inside its aliases, past its budget of %d", len(doc), s.spent, s.budget)
			}
		})
	}
}

// honest is a document whose anchored map, block written by block, is read
// back through aliases aliases.
func honest(block string, aliases int) string {
	var b strings.Builder
	b.WriteString("base: &b\n" + block)
	for i := range aliases {
		fmt.Fprintf(&b, "alias%d: *b\n", i)
	}
	return b.String()
}

// ordinary is the body of a block map of n entries, each a key and a short
// sentence, about 40 bytes an entry.
func ordinary(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "  key%d: a fairly ordinary value %d\n", i, i)
	}
	return b.String()
}

func TestParseYAMLReadsAFewHonestAliases(t *testing.T) {
	t.Parallel()
	v, err := parseWithin(t, "base: &b {x: 1, y: [a, b]}\none: *b\ntwo: *b\n")
	if err != nil {
		t.Fatalf("honest aliases refused: %v", err)
	}
	m := v.(*ordered.Map)
	for _, k := range []string{"one", "two"} {
		got, err := json.Marshal(ordered.Plain(m.Val(k)))
		if err != nil || string(got) != `{"x":1,"y":["a","b"]}` {
			t.Errorf("%s = %s, %v; want the anchored map", k, got, err)
		}
	}
}

// TestParseYAMLReadsABlockThatIsMostOfItsDocumentAgain: an anchored block
// read again a few times costs a few times its size, and a small document
// read through many aliases costs less than the floor.
func TestParseYAMLReadsABlockThatIsMostOfItsDocumentAgain(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		doc     string
		aliases int
	}{
		"a flow map through 3 aliases":        {"base: &b {x: 1, y: [a, b], name: a fairly ordinary value}\nalias0: *b\nalias1: *b\nalias2: *b\n", 3},
		"a small document through 10 aliases": {honest(ordinary(20), 10), 10},
		"a 40 KB block through 5 aliases":     {honest(ordinary(1000), 5), 5},
		"a 400 KB block through 5 aliases":    {honest(ordinary(10000), 5), 5},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := c.doc
			v, err := parseWithin(t, doc)
			if err != nil {
				t.Fatalf("a %d-byte document: %v", len(doc), err)
			}
			m := v.(*ordered.Map)
			want, _ := json.Marshal(ordered.Plain(m.Val("base")))
			for i := range c.aliases {
				k := "alias" + strconv.Itoa(i)
				if got, err := json.Marshal(ordered.Plain(m.Val(k))); err != nil || string(got) != string(want) {
					t.Fatalf("%s is not the anchored block: %v", k, err)
				}
			}
		})
	}
}

// values is how many values v holds, itself included, each scalar, list,
// map and map key, and the bytes of each string: a parse allocates a bounded
// number of bytes for each, on top of yaml.v3's own tree, which is linear
// in the document.
func values(v any) int {
	switch x := v.(type) {
	case []any:
		n := 1
		for _, e := range x {
			n += values(e)
		}
		return n
	case *ordered.Map:
		n := 1
		for _, k := range x.Keys() {
			n += 1 + len(k) + values(x.Val(k))
		}
		return n
	case string:
		return 1 + len(x)
	default:
		return 1
	}
}

// TestParseYAMLReturnsAFixedMultipleOfItsSize: what a parse builds, and so
// what it allocates past yaml.v3's own tree, is at most two values or
// string bytes for each byte of the document, a value written out taking at
// least two bytes and a string at least its own, and what its aliases
// expand to: at most six a byte, or 64 KiB for a small document, the
// numbers yaml.go's aliasBudget and aliasFloor hold. The count stands in for
// the bytes, which -race inflates past any fixed bound.
func TestParseYAMLReturnsAFixedMultipleOfItsSize(t *testing.T) {
	t.Parallel()
	const size = 128 << 10
	aliased := func(item string, k int) string {
		head := "a: &a [" + strings.Repeat(item+",", k-1) + item + "]\nb: ["
		return head + strings.Repeat("*a,", (size-len(head))/3-1) + "*a]\n"
	}
	for name, doc := range map[string]string{
		"flat scalars":              "[" + strings.Repeat("x,", size/2-1) + "x]",
		"flat maps":                 "[" + strings.Repeat("{},", size/3-1) + "{}]",
		"a block list":              strings.Repeat("- a\n", size/4),
		"aliases of one scalar":     aliased("x", 1),
		"aliases of one map":        aliased("{}", 1),
		"aliases of ten scalars":    aliased("x", 10),
		"aliases of ten maps":       aliased("{}", 10),
		"aliases of a long scalar":  longThrough(func(n int) string { return strings.Repeat("s", n) }, 4<<10, (size-(4<<10))/3),
		"aliases of a long-key map": longThrough(longKeyMap, 4000, 600),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v, err := parseWithin(t, doc)
			if err != nil {
				if !errors.Is(err, ordered.ErrSyntax) {
					t.Fatalf("err %v", err)
				}
				return
			}
			if n, most := values(v), 2*len(doc)+max(64<<10, 6*len(doc)); n > most {
				t.Errorf("a %d-byte document read as %d values, more than %d", len(doc), n, most)
			}
		})
	}
}

// TestTheAliasBudgetIsHeldAtItsBoundary (boundary: aliasBudget): the smallest
// documents that cross the budget, with their sizes asserted, are refused, and
// the one-smaller neighbour of each is read, so the refusal is the budget's
// and not a size's.
func TestTheAliasBudgetIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		over, at string
		overLen  int
		atLen    int
	}{
		"laughs":       {laughs(5), laughs(4), 262, 212},
		"a long float": {longThrough(longFloat, 1000, 66), longThrough(longFloat, 1000, 65), 1210, 1207},
		"long keys":    {longThrough(longKeyMap, 1000, 66), longThrough(longKeyMap, 1000, 65), 1215, 1212},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if len(c.over) != c.overLen || len(c.at) != c.atLen {
				t.Fatalf("the documents are %d and %d bytes, want %d and %d", len(c.over), len(c.at), c.overLen, c.atLen)
			}
			if _, err := parseWithin(t, c.at); err != nil {
				t.Errorf("the %d-byte neighbour under the budget: %v", c.atLen, err)
			}
			if _, err := parseWithin(t, c.over); !errors.Is(err, ordered.ErrSyntax) {
				t.Errorf("the %d-byte document over the budget: %v", c.overLen, err)
			}
		})
	}
}
