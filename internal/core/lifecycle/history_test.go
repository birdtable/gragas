package lifecycle_test

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// spec is one line to write: the number, the second of its time, the entity it
// is about, and the digest it names as the entry before it ("" for none).
type spec struct {
	number   int
	second   int
	entity   string
	previous string
}

// plan is a distinct plan digest for each second, so two lines of one second
// of one number differ only where a test makes them.
func plan(second int) string {
	return fmt.Sprintf("sha256:%064x", second)
}

func value(s spec) canon.Object {
	o := canon.Object{
		"format": canon.Int(3), "number": canon.Int(s.number), "kind": canon.String("promote"),
		"entity": canon.String(s.entity), "after": canon.Object{"status": canon.String("active")},
		"who": canon.String("ada"), "when": canon.String(fmt.Sprintf("2026-01-01T00:00:%02dZ", s.second)),
		"plan": canon.String(plan(s.second)),
	}
	if s.previous != "" {
		o["previous"] = canon.String(s.previous)
	}
	return o
}

// write is the canonical line of s and the digest the next line names it by.
func write(t testing.TB, s spec) (line []byte, digest string) {
	t.Helper()
	v := value(s)
	b, err := canon.Encode(v, canon.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	d, err := canon.DigestOf(v)
	if err != nil {
		t.Fatal(err)
	}
	return b, d.String()
}

// chain writes lines for specs in order, each naming the digest of the one
// before it, as the engine appends them, and answers them with the digests.
func chain(t testing.TB, n int) (lines [][]byte, digests []string) {
	t.Helper()
	prev := ""
	for i := 1; i <= n; i++ {
		l, d := write(t, spec{number: i, second: i, entity: "rule:a.b.1", previous: prev})
		lines, digests, prev = append(lines, l), append(digests, d), d
	}
	return lines, digests
}

func numbers(c lifecycle.Chain) []int {
	var out []int
	for _, e := range c.Entries {
		out = append(out, e.Number)
	}
	return out
}

func TestReadHistoryOrdersByNumberThenWhenThenPlan(t *testing.T) {
	t.Parallel()
	// Written in an order that is none of the three, and with a shared number.
	first, d1 := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	lateTwo, _ := write(t, spec{number: 2, second: 9, entity: "rule:a.b.2", previous: d1})
	earlyTwo, _ := write(t, spec{number: 2, second: 5, entity: "rule:a.b.3", previous: d1})
	three, _ := write(t, spec{number: 3, second: 2, entity: "rule:a.b.1", previous: d1})
	// The same number and the same second: the plan digest breaks the tie.
	a := value(spec{number: 3, second: 2, entity: "rule:a.b.4", previous: d1})
	a["plan"] = canon.String("sha256:" + strings.Repeat("0", 63) + "1")
	tied, err := canon.Encode(a, canon.Canonical)
	if err != nil {
		t.Fatal(err)
	}

	c := lifecycle.ReadHistory([][]byte{three, lateTwo, tied, first, earlyTwo})

	var got []string
	for _, e := range c.Entries {
		got = append(got, fmt.Sprintf("%d %s %s", e.Number, e.When, e.Entity))
	}
	want := []string{
		"1 2026-01-01T00:00:01Z rule:a.b.1",
		"2 2026-01-01T00:00:05Z rule:a.b.3",
		"2 2026-01-01T00:00:09Z rule:a.b.2",
		"3 2026-01-01T00:00:02Z rule:a.b.4", // plan ...0001 sorts before plan ...0002
		"3 2026-01-01T00:00:02Z rule:a.b.1",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("order\n got %q\nwant %q", got, want)
	}
}

func TestTwoEntriesThatShareANumberAndNameTheSameParentAreBothValid(t *testing.T) {
	t.Parallel()
	first, d1 := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	left, _ := write(t, spec{number: 2, second: 2, entity: "rule:a.b.1", previous: d1})
	right, _ := write(t, spec{number: 2, second: 3, entity: "rule:a.b.2", previous: d1})

	c := lifecycle.ReadHistory([][]byte{first, left, right})

	if got := numbers(c); !slices.Equal(got, []int{1, 2, 2}) {
		t.Fatalf("numbers %v, want [1 2 2]", got)
	}
	if len(c.Findings) != 0 || len(c.Skipped) != 0 {
		t.Fatalf("a shared number is normal (A38): findings %+v skipped %+v", c.Findings, c.Skipped)
	}
}

func TestTheNextNumberAfterAMergeIsTheHighestPlusOne(t *testing.T) {
	t.Parallel()
	first, d1 := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	left, _ := write(t, spec{number: 2, second: 2, entity: "rule:a.b.1", previous: d1})
	right, _ := write(t, spec{number: 2, second: 3, entity: "rule:a.b.2", previous: d1})
	for name, tc := range map[string]struct {
		lines [][]byte
		want  int
	}{
		"empty":         {nil, 1},
		"one":           {[][]byte{first}, 2},
		"after a merge": {[][]byte{first, left, right}, 3},
		"torn last":     {[][]byte{first, left, []byte(`{"number": 7, "ki`)}, 3},
	} {
		if got := lifecycle.ReadHistory(tc.lines).Next(); got != tc.want {
			t.Errorf("%s: next %d, want %d", name, got, tc.want)
		}
	}
}

func TestATornLineIsSkippedCountedAndListed(t *testing.T) {
	t.Parallel()
	lines, _ := chain(t, 3)
	torn := lines[1][:len(lines[1])/2]
	in := [][]byte{lines[0], torn, lines[2], []byte(`{"number": 4}`), []byte("not json")}

	c := lifecycle.ReadHistory(in)

	if got := numbers(c); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("read numbers %v, want [1 3]", got)
	}
	var at []int
	for _, s := range c.Skipped {
		at = append(at, s.Line)
		if s.Why == "" {
			t.Errorf("line %d is skipped with no reason", s.Line)
		}
	}
	if !slices.Equal(at, []int{2, 4, 5}) {
		t.Fatalf("skipped lines %v, want [2 4 5] (numbered from 1)", at)
	}
}

func TestABrokenPreviousDigestIsListed(t *testing.T) {
	t.Parallel()
	lines, _ := chain(t, 3)
	// Line 2 is rewritten: its bytes change, so the digest line 3 names is no
	// entry's digest any more.
	v := value(spec{number: 2, second: 2, entity: "rule:a.b.1", previous: "sha256:" + strings.Repeat("a", 64)})
	v["who"] = canon.String("mallory")
	rewritten, err := canon.Encode(v, canon.Canonical)
	if err != nil {
		t.Fatal(err)
	}

	c := lifecycle.ReadHistory([][]byte{lines[0], rewritten, lines[2]})

	var got []string
	for _, f := range c.Findings {
		got = append(got, fmt.Sprintf("%s number %d line %d", f.Kind, f.Number, f.Line))
	}
	want := []string{"broken-previous number 2 line 2", "broken-previous number 3 line 3"}
	if !slices.Equal(got, want) {
		t.Fatalf("findings %q, want %q", got, want)
	}
	if len(c.Entries) != 3 {
		t.Fatalf("a broken link keeps the entry: %d entries, want 3", len(c.Entries))
	}
}

func TestAnEntryAfterTheFirstWithNoPreviousIsListed(t *testing.T) {
	t.Parallel()
	first, _ := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	second, _ := write(t, spec{number: 2, second: 2, entity: "rule:a.b.1"})

	c := lifecycle.ReadHistory([][]byte{first, second})

	if len(c.Findings) != 1 || c.Findings[0].Kind != lifecycle.MissingPrevious || c.Findings[0].Number != 2 {
		t.Fatalf("findings %+v, want one missing-previous at number 2", c.Findings)
	}
}

func TestAPreviousThatNamesALaterEntryIsListed(t *testing.T) {
	t.Parallel()
	// Entry 2 names the digest of entry 3, which cannot be its parent.
	first, d1 := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	three, d3 := write(t, spec{number: 3, second: 3, entity: "rule:a.b.1", previous: d1})
	two, _ := write(t, spec{number: 2, second: 2, entity: "rule:a.b.1", previous: d3})

	c := lifecycle.ReadHistory([][]byte{first, two, three})

	if len(c.Findings) != 1 || c.Findings[0].Kind != lifecycle.BrokenPrevious || c.Findings[0].Number != 2 {
		t.Fatalf("findings %+v, want one broken-previous at number 2", c.Findings)
	}
}

func TestAGapInTheNumbersIsListed(t *testing.T) {
	t.Parallel()
	lines, _ := chain(t, 4)

	c := lifecycle.ReadHistory([][]byte{lines[0], lines[1], lines[3]})

	var got []string
	for _, f := range c.Findings {
		got = append(got, fmt.Sprintf("%s number %d", f.Kind, f.Number))
	}
	// 4 names the digest of 3, which is not there, so it is broken as well.
	want := []string{"gap number 3", "broken-previous number 4"}
	if !slices.Equal(got, want) {
		t.Fatalf("findings %q, want %q", got, want)
	}
}

func TestACleanChainOfTenHasNoFinding(t *testing.T) {
	t.Parallel()
	lines, _ := chain(t, 10)
	c := lifecycle.ReadHistory(lines)
	if len(c.Findings) != 0 || len(c.Skipped) != 0 || len(c.Entries) != 10 || c.Next() != 11 {
		t.Fatalf("%d entries, next %d, findings %+v, skipped %+v", len(c.Entries), c.Next(), c.Findings, c.Skipped)
	}
}

func TestAnEntryCarriesItsFieldsAndItsLifecycle(t *testing.T) {
	t.Parallel()
	line := []byte(`{"after":{"owner":"ada","review_by":"2027-01-01","status":"active"},"before":{"status":"trial"},` +
		`"decision":"keep-it","entity":"rule:a.b.1","format":3,"kind":"promote","number":1,` +
		`"plan":"sha256:` + strings.Repeat("c", 64) + `","when":"2026-02-03T04:05:06Z","who":"ada"}`)

	c := lifecycle.ReadHistory([][]byte{line})

	if len(c.Entries) != 1 {
		t.Fatalf("%+v", c)
	}
	e := c.Entries[0].HistoryEntry
	want := model.HistoryEntry{
		Format: 3, Number: 1, Kind: "promote", Entity: "rule:a.b.1", Who: "ada", When: "2026-02-03T04:05:06Z",
		Plan: model.Digest("sha256:" + strings.Repeat("c", 64)), Decision: "keep-it",
		Before: &model.Lifecycle{Status: "trial"},
		After:  model.Lifecycle{Status: "active", Owner: "ada", ReviewBy: "2027-01-01"},
	}
	if e.Before == nil || *e.Before != *want.Before {
		t.Fatalf("before %+v, want %+v", e.Before, want.Before)
	}
	e.Before, want.Before = nil, nil
	if !reflect.DeepEqual(e, want) {
		t.Fatalf("entry\n got %+v\nwant %+v", e, want)
	}
}

func TestLinesSplitsOnNewlinesAndDropsBlankOnes(t *testing.T) {
	t.Parallel()
	got := lifecycle.Lines([]byte("a\n\n  \nb\r\nc"))
	var s []string
	for _, l := range got {
		s = append(s, string(l))
	}
	if !slices.Equal(s, []string{"a", "b", "c"}) {
		t.Fatalf("lines %q, want [a b c]", s)
	}
}

// TestSkippedLinesCountFileLinesNotNonBlankOnes: the reader's line numbers
// are the file's, so a person finds the torn line with an editor.
func TestSkippedLinesCountFileLinesNotNonBlankOnes(t *testing.T) {
	t.Parallel()
	lines, _ := chain(t, 2)
	data := string(lines[0]) + "\n\n\ngarbage\n" + string(lines[1]) + "\n"

	c := lifecycle.ReadFile([]byte(data))

	if len(c.Skipped) != 1 || c.Skipped[0].Line != 4 {
		t.Fatalf("skipped %+v, want the one at line 4", c.Skipped)
	}
}

// Property: two branches each append their own lines; git merges the file by
// concatenation, in either order, and the reader gives the same sequence
// (tests.generated.2, A38).
func TestConcatenatingTwoBranchesInEitherOrderReadsTheSameSequence(t *testing.T) {
	t.Parallel()
	for seed := range 200 {
		base, digests := chain(t, 1+seed%4)
		parent := digests[len(digests)-1]
		next := len(base) + 1
		branch := func(who string, count int) [][]byte {
			var out [][]byte
			prev := parent
			for i := range count {
				v := value(spec{number: next + i, second: 10 + (seed+i)%3, entity: "rule:" + who, previous: prev})
				v["who"] = canon.String(who)
				b, err := canon.Encode(v, canon.Canonical)
				if err != nil {
					t.Fatal(err)
				}
				d, _ := canon.DigestOf(v)
				out, prev = append(out, b), d.String()
			}
			return out
		}
		left, right := branch("left", 1+seed%3), branch("right", 1+(seed/3)%3)

		ab := lifecycle.ReadHistory(slices.Concat(base, left, right))
		ba := lifecycle.ReadHistory(slices.Concat(base, right, left))

		if len(ab.Findings) != 0 || len(ba.Findings) != 0 {
			t.Fatalf("seed %d: a merge is not a finding: %+v %+v", seed, ab.Findings, ba.Findings)
		}
		if !slices.EqualFunc(ab.Entries, ba.Entries, func(a, b lifecycle.Entry) bool {
			return a.Digest == b.Digest
		}) {
			t.Fatalf("seed %d: the two orders read two sequences\n%v\n%v", seed, numbers(ab), numbers(ba))
		}
		if ab.Next() != ba.Next() || ab.Next() != next+max(len(left), len(right)) {
			t.Fatalf("seed %d: next %d and %d", seed, ab.Next(), ba.Next())
		}
	}
}

// Property: whatever the input order, the entries come out sorted by the
// three keys and no line is lost: each is an entry or is skipped.
func TestShufflingTheLinesChangesNothingButTheSkippedLineNumbers(t *testing.T) {
	t.Parallel()
	lines, _ := chain(t, 12)
	want := lifecycle.ReadHistory(lines)
	for seed := range 50 {
		shuffled := slices.Clone(lines)
		for i := len(shuffled) - 1; i > 0; i-- {
			j := (seed*7919 + i*104729) % (i + 1)
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		}
		got := lifecycle.ReadHistory(shuffled)
		if !slices.EqualFunc(got.Entries, want.Entries, func(a, b lifecycle.Entry) bool { return a.Digest == b.Digest }) {
			t.Fatalf("seed %d: %v, want %v", seed, numbers(got), numbers(want))
		}
		if len(got.Findings) != 0 {
			t.Fatalf("seed %d: the chain check reads digests, not positions: %+v", seed, got.Findings)
		}
	}
}

func FuzzReadHistory(f *testing.F) {
	lines, _ := chain(f, 3)
	f.Add([]byte(strings.Join([]string{string(lines[0]), string(lines[1]), string(lines[2])}, "\n") + "\n"))
	f.Add([]byte(""))
	f.Add([]byte("\n\n"))
	f.Add([]byte(`{"number":1}` + "\n"))
	f.Add([]byte(`{"number":-1,"entity":"x"}` + "\n"))
	f.Add([]byte(string(lines[0][:len(lines[0])/2]) + "\n" + string(lines[1]) + "\n"))
	f.Add([]byte("[1,2,3]\nnull\n\"x\"\n123\n"))
	f.Add([]byte(`{"number":99999999999999999999}` + "\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		c := lifecycle.ReadFile(data)
		n := len(lifecycle.Lines(data))
		if len(c.Entries)+len(c.Skipped) != n {
			t.Fatalf("%d lines read as %d entries and %d skipped", n, len(c.Entries), len(c.Skipped))
		}
		for i := 1; i < len(c.Entries); i++ {
			if c.Entries[i-1].Number > c.Entries[i].Number {
				t.Fatalf("entries out of order at %d", i)
			}
		}
		if c.Next() < 1 {
			t.Fatalf("next %d", c.Next())
		}
	})
}

// BenchmarkReadTenThousandEntries is the one pass the spec asks for: the
// time and the allocations of reading ten thousand entries are recorded in
// docs/checks.md beside the budget they keep.
func BenchmarkReadTenThousandEntries(b *testing.B) {
	lines, _ := chain(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c := lifecycle.ReadHistory(lines)
		if len(c.Entries) != 10000 || len(c.Findings) != 0 {
			b.Fatalf("%d entries, %d findings", len(c.Entries), len(c.Findings))
		}
	}
}

func TestAPreviousThatNamesAnEntryOfTheSameNumberIsListed(t *testing.T) {
	t.Parallel()
	// Two entries that share a number are siblings: neither is the other's parent.
	first, d1 := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	left, dl := write(t, spec{number: 2, second: 2, entity: "rule:a.b.1", previous: d1})
	right, _ := write(t, spec{number: 2, second: 3, entity: "rule:a.b.2", previous: dl})

	c := lifecycle.ReadHistory([][]byte{first, left, right})

	if len(c.Findings) != 1 || c.Findings[0].Kind != lifecycle.BrokenPrevious || c.Findings[0].Line != 3 {
		t.Fatalf("findings %+v, want one broken-previous at line 3", c.Findings)
	}
}

// TestWhenOrdersBeforePlanAndPlanBeforeTheDigest: of two entries with one
// number, the earlier when comes first although its plan digest is the
// greater; of two with one when, the lesser plan digest comes first although
// the line's own digest is the greater. Each pair is found by trying plans
// until the keys disagree, so no key can stand in for another.
func TestWhenOrdersBeforePlanAndPlanBeforeTheDigest(t *testing.T) {
	t.Parallel()
	_, d1 := write(t, spec{number: 1, second: 1, entity: "rule:a.b.1"})
	at := func(second int, planNo int) ([]byte, string) {
		v := value(spec{number: 2, second: second, entity: "rule:a.b.1", previous: d1})
		v["plan"] = canon.String(plan(planNo))
		b, err := canon.Encode(v, canon.Canonical)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := canon.DigestOf(v)
		return b, d.String()
	}
	// when before plan: the earlier second carries the greater plan.
	early, _ := at(5, 90)
	late, _ := at(9, 10)
	c := lifecycle.ReadHistory([][]byte{late, early})
	if c.Entries[0].When != "2026-01-01T00:00:05Z" {
		t.Errorf("the later when came first: %s", c.Entries[0].When)
	}
	// plan before digest: find a pair whose digests sort against their plans.
	for k := 1; k < 200; k++ {
		a, da := at(5, k)
		b, db := at(5, k+1)
		if da < db {
			continue
		}
		c := lifecycle.ReadHistory([][]byte{b, a})
		if c.Entries[0].Plan != model.Digest(plan(k)) {
			t.Fatalf("plan %d (digest %s) should come before plan %d (digest %s)", k, da, k+1, db)
		}
		return
	}
	t.Fatal("no pair of plans whose digests sort against them in 200 tries")
}
