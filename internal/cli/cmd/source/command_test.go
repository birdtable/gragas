package source_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/source"
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
	gotNew      *model.NewSource
	gotEdit     *model.EditSource
}

func (s *session) SourceNew(_ context.Context, _ kit.Cmd, r model.NewSource) int {
	s.gotNew = &r
	return 0
}

func (s *session) Import(context.Context, kit.Cmd, model.ImportInput) int { return 0 }

func (s *session) SourceEdit(_ context.Context, _ kit.Cmd, r model.EditSource) int {
	s.gotEdit = &r
	return 0
}

func (s *session) Exit(code int) error { return nil }

func exec(t *testing.T, s *session, args ...string) error {
	t.Helper()
	c := source.New(s)
	c.SetArgs(args) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	return c.ExecuteContext(context.Background())
}

// TestSourceNewHandsTheRunWhatWasGiven: each flag lands in its field of the
// request, and a flag not given leaves its field empty for the run to ask.
// Risk: a flag wired to the wrong field, or a default that hides a question.
func TestSourceNewHandsTheRunWhatWasGiven(t *testing.T) {
	t.Parallel()
	s := &session{}
	if err := exec(t, s, "new", "--source-slug", "rfc-9110", "--source-kind", "standard", "--title", "HTTP Semantics", "--locator", "RFC 9110", "--attribution", "IETF"); err != nil {
		t.Fatal(err)
	}
	want := model.NewSource{Slug: "rfc-9110", Kind: "standard", Title: "HTTP Semantics", Locator: "RFC 9110", Attribution: "IETF"}
	if s.gotNew == nil || *s.gotNew != want {
		t.Errorf("request %+v, want %+v", s.gotNew, want)
	}
	bare := &session{}
	if err := exec(t, bare, "new"); err != nil {
		t.Fatal(err)
	}
	if bare.gotNew == nil || *bare.gotNew != (model.NewSource{}) {
		t.Errorf("a bare source new hands %+v, want the empty request", bare.gotNew)
	}
}

// TestSourceEditLeavesAFieldAloneUnlessItWasGiven: a field not given is nil, a
// field given empty is a pointer to "", which takes an attribution away.
// Risk: an edit that blanks what it was not asked to change.
func TestSourceEditLeavesAFieldAloneUnlessItWasGiven(t *testing.T) {
	t.Parallel()
	s := &session{}
	if err := exec(t, s, "edit", "effective-go", "--attribution", ""); err != nil {
		t.Fatal(err)
	}
	got := s.gotEdit
	if got == nil || got.Slug != "effective-go" || got.Title != nil || got.Locator != nil || got.Attribution == nil || *got.Attribution != "" {
		t.Errorf("request %+v, want only the attribution, empty", got)
	}
	if err := exec(t, &session{}, "edit"); err == nil {
		t.Error("source edit with no slug ran")
	}
}

// TestEachWordThatWritesSaysSo: new and edit carry the writes annotation and
// take --apply, and only a word that does both writes (cli.commands.1).
// Risk: a word that writes without --apply being the gate.
func TestEachWordThatWritesSaysSo(t *testing.T) {
	t.Parallel()
	for _, c := range source.New(&session{}).Commands() {
		if _, ok := c.Annotations["writes"]; !ok {
			t.Errorf("source %s has no writes annotation", c.Name())
		}
		if c.Flags().Lookup("apply") == nil {
			t.Errorf("source %s takes no --apply", c.Name())
		}
	}
}

// TestTheRowNamesEveryExit: both words can end refused, so the row lists it.
// Planted failure: the row drops refused.
func TestTheRowNamesEveryExit(t *testing.T) {
	t.Parallel()
	row := source.Row()
	for _, w := range []model.Word{model.Done, model.Declined, model.Refused} {
		if !slices.Contains(row.Exits, w) {
			t.Errorf("the row's closing words %v leave out %s", row.Exits, w)
		}
	}
}

// TestSourceNewNamesEveryKindTheFlagTakes: --source-kind's usage points at
// the kinds the help names above it, so that help names every kind the
// model has, one word each, and the usage still says where they are. Risk:
// a kind added to the model that the help never names, so nobody learns
// they may give it (the list left the flag's own line to fit 80 columns).
func TestSourceNewNamesEveryKindTheFlagTakes(t *testing.T) {
	t.Parallel()
	c, _, err := source.New(&session{}).Find([]string{"new"})
	if err != nil {
		t.Fatal(err)
	}
	words := strings.Join(strings.Fields(c.Long), " ")
	for _, k := range model.SourceKinds() {
		if !strings.Contains(words, string(k)) {
			t.Errorf("source new's help does not name the kind %q", k)
		}
	}
	if f := c.Flags().Lookup("source-kind"); f == nil || !strings.Contains(f.Usage, "named above") {
		t.Errorf("--source-kind does not point at the kinds the help names: %+v", f)
	}
}
