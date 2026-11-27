package config_test

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/config"
)

// TestEveryCacheKindIsAKeptFile: a kind's files are each held by a row of the
// table of kept files, in the kind's place. Risk: a listing of files the table
// does not say gragas keeps.
func TestEveryCacheKindIsAKeptFile(t *testing.T) {
	t.Parallel()
	table := config.Persisted()
	for _, k := range config.CacheKinds() {
		for _, n := range k.Names {
			held := false
			for _, row := range table {
				held = held || (row.Place == k.Place && slices.Contains(row.Names, n))
			}
			if !held {
				t.Errorf("%s: %s is no row of the table of kept files in %s", k.Name, n, k.Place)
			}
		}
	}
	// the control: a kind with a file of no row is seen
	if _, ok := config.Holds(table, config.InState, "stray.copies"); ok {
		t.Error("the table holds a file it does not list")
	}
}

func TestTheKindsAreNamedOnceAndTheJournalIsBuilt(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, k := range config.CacheKinds() {
		if seen[k.Name] || k.Holds == "" || len(k.Names) == 0 {
			t.Errorf("kind %+v: declared twice or with a column empty", k)
		}
		seen[k.Name] = true
	}
	if k := config.CacheKinds()[0]; k.Name != "journal" || !k.Built || !k.Journal {
		t.Errorf("the first kind is %+v, want the journal, built", k)
	}
}

func TestKeepIsSeconds(t *testing.T) {
	t.Parallel()
	for keep, want := range map[string]int64{"30d": 30 * 86400, "12h": 12 * 3600, "1d": 86400} {
		got, err := config.Settings{Keep: keep}.KeepSeconds()
		if err != nil || got != want {
			t.Errorf("%s: %d, %v, want %d", keep, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "0d", "30", "3x", "-1d"} {
		if _, err := (config.Settings{Keep: bad}).KeepSeconds(); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}
