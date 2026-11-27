package readglob_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/readglob"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func match(t *testing.T, glob, path string) bool {
	t.Helper()
	parts, err := readglob.Parts(glob)
	if err != nil {
		t.Fatalf("Parts(%q) = %v", glob, err)
	}
	return readglob.Match(parts, strings.Split(path, "/"))
}

// TestAGlobHasStarQuestionAndDoubleStarOnlyMatchedCaseExact: the rows of the
// host's read test, asked of the matcher alone: * within a part, ? one
// character, ** any number of folders, none included, case exact. Planted:
// folding case (the case row red), ** taking one folder at least (the none
// row red), * crossing a slash (the part row red).
func TestAGlobHasStarQuestionAndDoubleStarOnlyMatchedCaseExact(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		glob, path string
		want       bool
	}{
		{"c.json", "C.JSON", false},
		{"*.json", "a.json", true},
		{"*.json", "ab.json", true},
		{"*.json", "x/a.json", false},
		{"a?.json", "ab.json", true},
		{"a?.json", "a.json", false},
		{"**/a.json", "a.json", true},
		{"**/a.json", "x/a.json", true},
		{"**/a.json", "x/y/a.json", true},
		{"x/*", "x/a.json", true},
		{"x/*", "x/y/a.json", false},
		{"z/**/*.json", "z/a.json", true},
		{"o/**/*.json", "o/x/a.json", true},
		{"w/**/*.json", "w/x/y/a.json", true},
		{"q/a?b.json", "q/a/b.json", false},
		{"t/*", "t/.x", true},
		{"*.toml", "a.json", false},
	} {
		if got := match(t, c.glob, c.path); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

// TestAGlobOutsideTheGrammarIsRefused: an empty glob, [, ], {, } or a
// backslash, and a ** beside other characters are refused, matching ErrGlob.
// Planted: Parts judging nothing (every row red).
func TestAGlobOutsideTheGrammarIsRefused(t *testing.T) {
	t.Parallel()
	for _, g := range []string{"", "[ab].json", "{a,b}.json", "x/a\\.json", "**.json", "x/a**", "q/a**b.json"} {
		if _, err := readglob.Parts(g); !errors.Is(err, readglob.ErrGlob) {
			t.Errorf("Parts(%q) = %v, want ErrGlob", g, err)
		}
	}
}

// TestAGlobOfManyStarsEndsAtOnce: many stars against a long name and many **
// against a deep path that matches nothing end at once, as the match goes
// back only to its last star; the same globs that end on the name match it.
// Planted: every split tried at each * or ** (the rows never end, red at the
// bound).
func TestAGlobOfManyStarsEndsAtOnce(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 200)
	deep := strings.Repeat("a/", 40) + "c"
	stars := strings.Repeat("*a", 16)
	doubles := strings.Repeat("**/a/", 12)
	done := make(chan [4]bool, 1)
	go func() {
		done <- [4]bool{match(t, stars+"b", long), match(t, stars, long), match(t, doubles+"b", deep), match(t, doubles+"c", deep)}
	}()
	select {
	case got := <-done:
		if want := [4]bool{false, true, false, true}; got != want {
			t.Errorf("rows = %v, want %v", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the match did not end")
	}
}
