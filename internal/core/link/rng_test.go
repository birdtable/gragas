package link_test

// rng is a small deterministic generator, so a failing property names its seed.
type rng uint64

func (r *rng) next() uint64 {
	*r ^= *r << 13
	*r ^= *r >> 7
	*r ^= *r << 17
	return uint64(*r)
}

func (r *rng) n(n int) int { return int(r.next() % uint64(n)) }
