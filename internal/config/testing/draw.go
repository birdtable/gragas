package configtest

import "math"

// Draw is the seeded draw a property test makes: splitmix64, so the same
// seed gives the same draws on every system and a failure can name its seed.
// It is not a source of secrets and stands in for no random source a secret
// is drawn from; it is here so no test takes math/rand (gosec G404, which no
// exclusion silences: supply.scans.3).
type Draw struct{ state uint64 }

// NewDraw is the draw that starts from seed.
func NewDraw(seed uint64) *Draw { return &Draw{state: seed} }

// Uint64 is the next draw of the sequence.
func (d *Draw) Uint64() uint64 {
	d.state += 0x9e3779b97f4a7c15
	z := d.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// IntN is a draw in [0, n) for n of at least 1, and 0 for any smaller n.
func (d *Draw) IntN(n int) int {
	if n < 1 {
		return 0
	}
	v := d.Uint64() % uint64(n)
	if v > math.MaxInt {
		return 0 // never: v is below n, an int; the bound is what gosec G115 reads
	}
	return int(v)
}

// Shuffle puts n things in a drawn order, swapping them by swap, as
// Fisher and Yates do.
func (d *Draw) Shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		swap(i, d.IntN(i+1))
	}
}
