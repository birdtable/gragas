package exit_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestTheTableIsTheClosedSetWithItsCodes: gragas's table of endings is
// checked by go-run/exit's New with no error, failed and findings sharing 1
// (A39), and holds the closed set of words in order, each kind
// named by its word, with the code Code gives it. An ending the table builds
// carries that word and code, and a name it does not hold builds no done.
// main stops on NewTable's error before anything runs, so this is where a
// fault in the table is found, not in a run.
// Risk: a script reading a code that means another word, or a run that ends
// on a kind the kit was never given.
func TestTheTableIsTheClosedSetWithItsCodes(t *testing.T) {
	t.Parallel()
	table, err := exit.NewTable()
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	words, kinds := model.Words(), table.Kinds()
	if len(kinds) != len(words) {
		t.Fatalf("the table has %d kinds, the closed set %d words", len(kinds), len(words))
	}
	for i, k := range kinds {
		want, _ := exit.Code(words[i])
		if k.Name != string(words[i]) || k.Word != string(words[i]) || k.Code != want {
			t.Errorf("kind %d = %+v, want name and word %q, code %d", i, k, words[i], want)
		}
		e := table.End(k.Name, "x", "")
		if e.Word() != k.Word || e.Code() != want {
			t.Errorf("End(%q) = %q, %d; want %q, %d", k.Name, e.Word(), e.Code(), k.Word, want)
		}
	}
	if e := table.End("ok", "x", ""); e.Code() == exit.Done || e.Word() != "" {
		t.Errorf("End of a name outside the table = %q, %d; want no word and not done", e.Word(), e.Code())
	}
}
