package attention_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/attention"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain registers -update, which this package keeps no goldens for, so
// go test -update over internal/cli runs here too, and starts from the
// shared test setup.
func TestMain(m *testing.M) {
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "write the goldens: this package keeps none")
	}
	os.Exit(configtest.Main(m))
}

// session is a run that keeps what the command said instead of drawing it, and
// answers Attention with what the test set.
type session struct {
	kit.Contracts // never called by this command
	kit.Records   // nor this
	kit.Explains  // nor this
	kit.Projects  // nor this
	kit.Plugins   // nor this
	json          bool
	got           model.AttentionListing
	err           error
	doc           any
	ending        *kit.Ending
}

func (s *session) Say(string)                         {}
func (s *session) JSON() bool                         { return s.json }
func (s *session) Document(v any)                     { s.doc = v }
func (s *session) SARIF() bool                        { return false }
func (s *session) Encoded([]byte)                     {}
func (s *session) Build() kit.Build                   { return kit.Build{} }
func (s *session) Line(argv []string) (string, error) { return strings.Join(argv, " "), nil }
func (s *session) Validate(context.Context) (model.Validation, error) {
	return model.Validation{}, nil
}
func (s *session) Tools(context.Context) (model.ToolsListing, error) {
	return model.ToolsListing{}, nil
}
func (s *session) Import(context.Context, kit.Cmd, model.ImportInput) int { return 0 }
func (s *session) Attention(context.Context) (model.AttentionListing, error) {
	return s.got, s.err
}
func (s *session) Bind(context.Context, kit.Cmd, model.BindRule) int     { return 0 }
func (s *session) Rebind(context.Context, kit.Cmd, model.RebindRule) int { return 0 }
func (s *session) Unbind(context.Context, kit.Cmd, model.UnbindRule) int { return 0 }
func (s *session) Exit(code int) error {
	if code == exit.Done {
		return nil
	}
	return exitCode(code)
}
func (s *session) End(e kit.Ending) int {
	s.ending = &e
	c, _ := exit.Code(e.Word)
	return c
}

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func run(t *testing.T, s *session) int {
	t.Helper()
	c := attention.New(s)
	c.SetArgs([]string{}) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	if err := c.ExecuteContext(context.Background()); err != nil {
		if code, ok := errors.AsType[exitCode](err); ok {
			return int(code)
		}
		t.Fatal(err)
	}
	return exit.Done
}

func item(rule string, k model.AttentionKind) model.AttentionItem {
	return model.AttentionItem{Rule: model.RuleID(rule), Kind: k, Status: model.Proposed, Why: "why " + rule, Fix: "fix " + rule}
}

func notes(e *kit.Ending) []string {
	var out []string
	for _, b := range e.Lead {
		if n, ok := b.(model.Note); ok {
			out = append(out, n.Text)
		}
	}
	return out
}

func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	if _, err := kit.NewRegistry([]kit.Command{attention.Row()}); err != nil {
		t.Fatal(err)
	}
	if r := attention.Row(); r.State != kit.Built || r.Order != attention.Order || r.Context != kit.InRulebook || r.Ends != kit.EndsListing {
		t.Errorf("row = %+v", r)
	}
}

// TestTheListingGroupsByKindAndSaysWhatIsNotKnown: a table a kind that has
// items, in the order of the kinds, and the override line last, whatever is
// listed; the closing word is done.
func TestTheListingGroupsByKindAndSaysWhatIsNotKnown(t *testing.T) {
	t.Parallel()
	s := &session{got: model.AttentionListing{Items: []model.AttentionItem{
		item("go.errors.1", model.AttentionOverdue), item("go.errors.1", model.AttentionNoOwner), item("go.errors.2", model.AttentionNoOwner)}}}
	if code := run(t, s); code != exit.Done || s.ending.Word != model.Done || s.ending.What != "2 rules need attention, 3 items" {
		t.Fatalf("exit %d, %+v", code, s.ending)
	}
	var heads []string
	for _, b := range s.ending.Lead {
		if h, ok := b.(model.Heading); ok {
			heads = append(heads, h.Text)
		}
	}
	if strings.Join(heads, "|") != "overdue review-by (1)|owner missing (2)" {
		t.Errorf("headings = %q", heads)
	}
	if n := notes(s.ending); n[len(n)-1] != attention.OverrideLine {
		t.Errorf("notes = %q, want the override line last", n)
	}
}

// TestAWholeProposedBookSaysSoFirst: the one line comes before every table.
func TestAWholeProposedBookSaysSoFirst(t *testing.T) {
	t.Parallel()
	s := &session{got: model.AttentionListing{AllProposed: true, Items: []model.AttentionItem{item("go.errors.1", model.AttentionComplete)}}}
	run(t, s)
	first, ok := s.ending.Lead[0].(model.Note)
	if !ok || !strings.HasPrefix(first.Text, "every rule is proposed") {
		t.Fatalf("lead = %+v", s.ending.Lead)
	}
	if s.ending.What != "0 rules need attention, 1 item" {
		t.Errorf("what = %q: a rule that misses nothing is listed but needs nothing", s.ending.What)
	}
}

// TestNothingToAttendToSaysSo: the empty state is a line, and still done.
func TestNothingToAttendToSaysSo(t *testing.T) {
	t.Parallel()
	s := &session{}
	if code := run(t, s); code != exit.Done || notes(s.ending)[0] != "nothing needs attention" {
		t.Fatalf("exit %d, %+v", code, s.ending)
	}
}

// TestJSONIsTheDocumentAlone: under --json nothing is drawn and the document
// carries the override rate as null with its reason (cli.output.9).
func TestJSONIsTheDocumentAlone(t *testing.T) {
	t.Parallel()
	s := &session{json: true, got: model.AttentionListing{Items: []model.AttentionItem{item("go.errors.1", model.AttentionNoOwner)}}}
	run(t, s)
	doc, ok := s.doc.(coreapi.AttentionDocument)
	if !ok || doc.OverrideRate.Value != nil || doc.OverrideRate.Reason != "no batches given" || len(s.ending.Lead) != 0 {
		t.Fatalf("doc %+v, lead %v", s.doc, s.ending.Lead)
	}
}

// TestARulebookThatIsNotThereOrNotReadableEndsNotReady: a table over the
// errors the session gives; the test says it ran each.
func TestARulebookThatIsNotThereOrNotReadableEndsNotReady(t *testing.T) {
	t.Parallel()
	rows := []struct {
		err  error
		word model.Word
	}{
		{coreapi.ErrNoBook, model.NotReady},
		{fmt.Errorf("%w: newer format", coreapi.ErrNotReady), model.NotReady},
		{errors.New("disk"), model.Failed},
	}
	ran := 0
	for _, r := range rows {
		s := &session{err: r.err}
		run(t, s)
		if s.ending.Word != r.word {
			t.Errorf("%v: word %q, want %q", r.err, s.ending.Word, r.word)
		}
		ran++
	}
	if ran != len(rows) {
		t.Fatalf("ran %d of %d", ran, len(rows))
	}
}

// The plan forms are the changes of a run, which this command never makes.
func (s *session) Migrate(context.Context, kit.Cmd, model.InitForm) int      { return 0 }
func (s *session) SourceNew(context.Context, kit.Cmd, model.NewSource) int   { return 0 }
func (s *session) SourceEdit(context.Context, kit.Cmd, model.EditSource) int { return 0 }

// GuardEdit and Aside are the guard's, which this command never runs.
func (s *session) GuardEdit(context.Context) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) Aside(string) {}

// The rest of the session is never called by this command.
func (s *session) GuardStaged(context.Context, bool) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) DecisionNew(context.Context, kit.Cmd, model.NewDecisionRecord) int       { return 0 }
func (s *session) DecisionSupersede(context.Context, kit.Cmd, model.SupersedeDecision) int { return 0 }
func (s *session) Undo(context.Context, kit.Cmd) int                                       { return 0 }
func (s *session) Conflicts(context.Context) (model.ConflictsListing, error) {
	return model.ConflictsListing{}, nil
}
func (s *session) Coverage(context.Context) (model.Coverage, error) { return model.Coverage{}, nil }
func (s *session) CheckRulebook(context.Context) (model.RulebookCheck, *model.Refusal, error) {
	return model.RulebookCheck{}, nil, nil
}
