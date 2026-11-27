package coreapi_test

import (
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// The caps of a readers row's counted, each held at its boundary:
// at the cap the row is read, one over it the file is refused naming the row.

// countedRow is a readers file of one row whose counted is the text given.
func countedRow(counted string) string {
	return "readers:\n  - tool: a\n    files: [a.yml]\n    kind: entries\n    counted:\n" + counted
}

// atAndOver reads the row at the cap and one over it: the first must be
// read, the second refused with want in its reason.
func atAndOver(t *testing.T, at, over, want string) coreapi.Readers {
	t.Helper()
	ok := readersFrom(t, countedRow(at))
	if ok.Refused != "" || len(ok.Rows) != 1 {
		t.Fatalf("at the cap: refused %q, want the row read", ok.Refused)
	}
	no := readersFrom(t, countedRow(over))
	if len(no.Rows) != 0 || !strings.Contains(no.Refused, "row 1: ") || !strings.Contains(no.Refused, want) {
		t.Errorf("one over the cap: %+v, want the file refused saying %q", no, want)
	}
	return ok
}

// list is n items of the form item(i), as a YAML flow list.
func list(n int, item func(int) string) string {
	items := make([]string, n)
	for i := range items {
		items[i] = item(i)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func key(i int) string { return fmt.Sprintf("k%d", i) }

// TestACountedHoldsSixtyFourExclusionPaths: 64 paths are read, 65 refused.
// boundary: maxExclusions
func TestACountedHoldsSixtyFourExclusionPaths(t *testing.T) {
	t.Parallel()
	path := func(i int) string { return "[" + key(i) + "]" }
	ok := atAndOver(t, "      exclusions: "+list(64, path)+"\n", "      exclusions: "+list(65, path)+"\n", "holds 65 paths, and holds one to 64")
	if got := len(ok.Rows[0].Exclusions); got != 64 {
		t.Errorf("read %d paths, want 64", got)
	}
}

// TestAKeyPathHoldsSixteenKeys: an exclusion path, and a loosening's at, of
// 16 keys are read; of 17, refused.
// boundary: maxSegments
func TestAKeyPathHoldsSixteenKeys(t *testing.T) {
	t.Parallel()
	ok := atAndOver(t, "      exclusions: ["+list(16, key)+"]\n", "      exclusions: ["+list(17, key)+"]\n", "is not a list of one to 16 keys")
	if got := len(ok.Rows[0].Exclusions[0]); got != 16 {
		t.Errorf("read a path of %d keys, want 16", got)
	}
	atAndOver(t, "      loosening: [{at: "+list(16, key)+", values: [x]}]\n", "      loosening: [{at: "+list(17, key)+", values: [x]}]\n", "counted.loosening[0].at is not a list of one to 16 keys")
}

// TestAKeyHoldsOneHundredTwentyEightBytes: a key of 128 bytes is read, of
// 129 refused, in an exclusion path and in a loosening's at.
// boundary: maxSegment
func TestAKeyHoldsOneHundredTwentyEightBytes(t *testing.T) {
	t.Parallel()
	at, over := strings.Repeat("k", 128), strings.Repeat("k", 129)
	ok := atAndOver(t, "      exclusions: [["+at+"]]\n", "      exclusions: [["+over+"]]\n", "of at most 128 bytes")
	if got := ok.Rows[0].Exclusions[0][0]; got != at {
		t.Errorf("read the key %q, want the 128 bytes", got)
	}
	atAndOver(t, "      loosening: [{at: ["+at+"], values: [x]}]\n", "      loosening: [{at: ["+over+"], values: [x]}]\n", "of at most 128 bytes")
}

// TestAWordHoldsThirtyTwoBytes: a directive and a loosening value of 32
// bytes are read, of 33 refused.
// boundary: maxDirective
func TestAWordHoldsThirtyTwoBytes(t *testing.T) {
	t.Parallel()
	at, over := strings.Repeat("w", 32), strings.Repeat("w", 33)
	ok := atAndOver(t, "      directive: "+at+"\n", "      directive: "+over+"\n", "counted.directive is not a word")
	if got := ok.Rows[0].Directive; got != at {
		t.Errorf("read the directive %q, want the 32 bytes", got)
	}
	atAndOver(t, "      loosening: [{at: [a], values: ["+at+"]}]\n", "      loosening: [{at: [a], values: ["+over+"]}]\n", "a value that is not a word given once")
}

// TestACountedHoldsSixteenLoosenings: 16 scalars are read, 17 refused.
// boundary: maxLoosening
func TestACountedHoldsSixteenLoosenings(t *testing.T) {
	t.Parallel()
	scalar := func(i int) string { return "{at: [" + key(i) + "], values: [x]}" }
	ok := atAndOver(t, "      loosening: "+list(16, scalar)+"\n", "      loosening: "+list(17, scalar)+"\n", "holds 17 scalars, and holds one to 16")
	if got := len(ok.Rows[0].Loosening); got != 16 {
		t.Errorf("read %d scalars, want 16", got)
	}
}

// TestALooseningHoldsSixteenValues: 16 values are read, 17 refused.
// boundary: maxValues
func TestALooseningHoldsSixteenValues(t *testing.T) {
	t.Parallel()
	word := func(i int) string { return fmt.Sprintf("w%d", i) }
	ok := atAndOver(t, "      loosening: [{at: [a], values: "+list(16, word)+"}]\n", "      loosening: [{at: [a], values: "+list(17, word)+"}]\n", "values holds 17 words, and holds one to 16")
	if got := len(ok.Rows[0].Loosening[0].Values); got != 16 {
		t.Errorf("read %d values, want 16", got)
	}
}
