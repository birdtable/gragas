package configtest

import (
	"slices"
	"testing"
)

// TestADrawIsTheSameForASeedAndStaysInItsRange: two draws from one seed
// give the same sequence, another seed another one, IntN stays in [0, n)
// and answers 0 below 1, and Shuffle gives a permutation. Risk: a property
// test whose failure cannot be replayed from the seed it names.
func TestADrawIsTheSameForASeedAndStaysInItsRange(t *testing.T) {
	t.Parallel()
	a, b, c := NewDraw(178), NewDraw(178), NewDraw(179)
	same, other := true, true
	for range 64 {
		x, y, z := a.Uint64(), b.Uint64(), c.Uint64()
		same = same && x == y
		other = other && x == z
	}
	if !same {
		t.Error("one seed gave two sequences")
	}
	if other {
		t.Error("two seeds gave one sequence")
	}
	d := NewDraw(7)
	for n := 1; n < 40; n++ {
		if v := d.IntN(n); v < 0 || v >= n {
			t.Errorf("IntN(%d) = %d", n, v)
		}
	}
	for _, n := range []int{0, -3} {
		if v := d.IntN(n); v != 0 {
			t.Errorf("IntN(%d) = %d, want 0", n, v)
		}
	}
	xs := []int{0, 1, 2, 3, 4, 5, 6, 7}
	d.Shuffle(len(xs), func(i, j int) { xs[i], xs[j] = xs[j], xs[i] })
	if sorted := slices.Sorted(slices.Values(xs)); !slices.Equal(sorted, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("Shuffle gave %v, not a permutation", xs)
	}
}
