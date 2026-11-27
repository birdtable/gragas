package wire

import (
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/readglob"
)

// TestGlobGrammarIsTheContracts holds that the codec asks the grammar of
// internal/readglob and keeps no copy: a glob the grammar's Parts refuses is
// refused here, and one it takes is taken, so the two cannot drift apart
// (gragas#2360). Risk: a second grammar that accepts or refuses what the first
// does not. A pattern the old copy of the grammar held is among the rows, and
// so are those only the contract's Parts judges (an empty glob).
func TestGlobGrammarIsTheContracts(t *testing.T) {
	t.Parallel()
	r := New(codec(t).rules.names).rules
	for _, g := range []string{
		"", "*", "**", "a/**/b", "*.go", ".*", "a?c", "src/**",
		"a[b]", "a{b,c}", "a**/b", "**a", "a/**b/c", "a//b", "/a", "a/",
	} {
		_, err := readglob.Parts(g)
		why := r.globFault(g)
		if contract := errors.Is(err, readglob.ErrGlob); contract && why == "" {
			t.Errorf("%q: the contract refuses it and the codec takes it", g)
		}
		if err == nil && why != "" && g != "a//b" && g != "/a" && g != "a/" && g != "" {
			t.Errorf("%q: the contract takes it and the codec refuses it: %s", g, why)
		}
	}
}
