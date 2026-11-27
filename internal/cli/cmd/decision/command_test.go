package decision_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/decision"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// session keeps the request a word hands the run, and plans nothing.
type session struct {
	kit.Session // every other method is the run's, which these tests never reach
	gotNew      *model.NewDecisionRecord
	gotSup      *model.SupersedeDecision
}

func (s *session) DecisionNew(_ context.Context, _ kit.Cmd, r model.NewDecisionRecord) int {
	s.gotNew = &r
	return 0
}

func (s *session) DecisionSupersede(_ context.Context, _ kit.Cmd, r model.SupersedeDecision) int {
	s.gotSup = &r
	return 0
}

func (s *session) Exit(code int) error { return nil }

func exec(t *testing.T, s *session, args ...string) error {
	t.Helper()
	c := decision.New(s)
	c.SetArgs(args) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	return c.ExecuteContext(context.Background())
}

// TestDecisionNewHandsTheRunWhatWasGiven: each flag lands in its field, a
// repeated flag lands as a list in the order given, and a flag not given
// leaves its field empty for the run to ask.
// Risk: a flag wired to the wrong field, or a list that keeps only its last.
func TestDecisionNewHandsTheRunWhatWasGiven(t *testing.T) {
	t.Parallel()
	s := &session{}
	if err := exec(t, s, "new", "--decision-slug", "small", "--title", "Small", "--text", "Keep it small.",
		"--rejected", "big: slow", "--rejected", "none: unsafe", "--cite-source", "effective-go", "--cite-rule", "go.errors.1", "--cite-rule", "go.errors.2", "--cite-binding", "go.errors.1 revive x"); err != nil {
		t.Fatal(err)
	}
	g := s.gotNew
	if g == nil || g.Slug != "small" || g.Title != "Small" || g.Text != "Keep it small." ||
		!slices.Equal(g.Rejected, []string{"big: slow", "none: unsafe"}) || !slices.Equal(g.Sources, []string{"effective-go"}) ||
		!slices.Equal(g.Rules, []model.RuleID{"go.errors.1", "go.errors.2"}) || !slices.Equal(g.Bindings, []string{"go.errors.1 revive x"}) {
		t.Errorf("request %+v", g)
	}
	bare := &session{}
	if err := exec(t, bare, "new"); err != nil {
		t.Fatal(err)
	}
	if b := bare.gotNew; b == nil || b.Slug != "" || b.Title != "" || len(b.Rejected) != 0 {
		t.Errorf("a bare decision new hands %+v, want the empty request", b)
	}
}

// TestDecisionSupersedeTakesTheOldSlugAsItsArgument: the argument is the old
// decision and the flags are the new one's words.
// Risk: the old and the new slug swapped, or a run with no old decision.
func TestDecisionSupersedeTakesTheOldSlugAsItsArgument(t *testing.T) {
	t.Parallel()
	s := &session{}
	if err := exec(t, s, "supersede", "wrap", "--decision-slug", "wrap-with-context", "--title", "T", "--text", "X.", "--rejected", "bare: no"); err != nil {
		t.Fatal(err)
	}
	g := s.gotSup
	if g == nil || g.Old != "wrap" || g.Slug != "wrap-with-context" || g.Title != "T" || g.Text != "X." || !slices.Equal(g.Rejected, []string{"bare: no"}) {
		t.Errorf("request %+v", g)
	}
	if err := exec(t, &session{}, "supersede"); err == nil {
		t.Error("decision supersede with no slug ran")
	}
}

// TestEachWordThatWritesSaysSo: new and supersede carry the writes annotation
// and take --apply (cli.commands.1).
// Risk: a word that writes without --apply being the gate.
func TestEachWordThatWritesSaysSo(t *testing.T) {
	t.Parallel()
	for _, c := range decision.New(&session{}).Commands() {
		if _, ok := c.Annotations["writes"]; !ok {
			t.Errorf("decision %s has no writes annotation", c.Name())
		}
		if c.Flags().Lookup("apply") == nil {
			t.Errorf("decision %s takes no --apply", c.Name())
		}
	}
}

// TestTheRowNamesEveryExitAndEveryFileTheFamilyWrites: both words can end
// refused, and supersede rewrites the rules files of the rules that move, so
// the row lists refused among its closing words and a rules file among its
// writes.
// Planted failure: the row drops refused, or drops the rules files.
func TestTheRowNamesEveryExitAndEveryFileTheFamilyWrites(t *testing.T) {
	t.Parallel()
	row := decision.Row()
	for _, w := range []model.Word{model.Done, model.Declined, model.Refused} {
		if !slices.Contains(row.Exits, w) {
			t.Errorf("the row's closing words %v leave out %s", row.Exits, w)
		}
	}
	for _, f := range []string{"decisions.yaml", "history.jsonl"} {
		if !slices.Contains(row.Writes, f) {
			t.Errorf("the row's writes %v leave out %s", row.Writes, f)
		}
	}
	if !slices.ContainsFunc(row.Writes, func(w string) bool { return strings.Contains(w, "rules files") }) {
		t.Errorf("the row's writes %v leave out the rules files supersede rewrites", row.Writes)
	}
}
