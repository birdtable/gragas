package model_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestNumbersAreSaidAsAPersonReadsThem: thousands are separated, a plural
// follows the count, and the three irregular plurals hold (cli.look.3).
func TestNumbersAreSaidAsAPersonReadsThem(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 165440: "165,440", -1234567: "-1,234,567"} {
		if got := model.Count(n); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
	for _, c := range []struct {
		n    int
		noun string
		want string
	}{
		{1, "rule", "1 rule"}, {2, "rule", "2 rules"}, {1234, "entry", "1,234 entries"}, {3, "key", "3 keys"},
		{2, "person", "2 people"}, {2, "other person", "2 other people"}, {2, "y", "2 ys"}, {2, "bus", "2 buss"},
	} {
		if got := model.Plural(c.n, c.noun); got != c.want {
			t.Errorf("Plural(%d, %q) = %q, want %q", c.n, c.noun, got, c.want)
		}
	}
}
