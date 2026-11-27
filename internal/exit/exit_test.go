package exit_test

import (
	"os"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func TestEachWordHasItsCode(t *testing.T) {
	t.Parallel()
	table := []struct {
		word model.Word
		code int
	}{
		{model.Done, 0},
		{model.Failed, 1},
		{model.Findings, 1},
		{model.Usage, 2},
		{model.Busy, 3},
		{model.NotReady, 4},
		{model.Refused, 5},
		{model.Declined, 6},
	}
	if len(table) != len(model.Words()) {
		t.Fatalf("the table has %d rows, the closed set %d words", len(table), len(model.Words()))
	}
	for _, c := range table {
		got, ok := exit.Code(c.word)
		if !ok || got != c.code {
			t.Errorf("Code(%q) = %d, %v; want %d, true", c.word, got, ok, c.code)
		}
	}
}

func TestFailedAndFindingsShareOneAndOnlyThose(t *testing.T) {
	t.Parallel()
	seen := map[int][]model.Word{}
	for _, w := range model.Words() {
		c, _ := exit.Code(w)
		seen[c] = append(seen[c], w)
	}
	if len(seen) != 7 {
		t.Errorf("%d distinct codes, want 7", len(seen))
	}
	if len(seen[1]) != 2 {
		t.Errorf("code 1 is held by %q, want failed and findings", seen[1])
	}
	for c, ws := range seen {
		if c != 1 && len(ws) != 1 {
			t.Errorf("code %d is shared by %q", c, ws)
		}
	}
}

func TestAWordOutsideTheSetIsNotACode(t *testing.T) {
	t.Parallel()
	for _, w := range []model.Word{"", "ok", "needs you", "Done"} {
		code, ok := exit.Code(w)
		if ok || code != exit.Failed {
			t.Errorf("Code(%q) = %d, %v; want %d, false", w, code, ok, exit.Failed)
		}
	}
}
