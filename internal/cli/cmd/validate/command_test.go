package validate_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/validate"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// session is a run that keeps what the command said instead of drawing it,
// and answers Validate with what the test set.
type session struct {
	kit.Contracts // never called by this command
	kit.Records   // nor this
	kit.Explains  // nor this
	kit.Projects  // nor this
	kit.Plugins   // nor this
	json, sarif   bool
	got           model.Validation
	err           error
	doc           any
	encoded       []byte
	build         kit.Build
	ending        *kit.Ending
}

func (s *session) Say(string)                         {}
func (s *session) JSON() bool                         { return s.json }
func (s *session) Document(v any)                     { s.doc = v }
func (s *session) SARIF() bool                        { return s.sarif }
func (s *session) Encoded(doc []byte)                 { s.encoded = doc }
func (s *session) Build() kit.Build                   { return s.build }
func (s *session) Line(argv []string) (string, error) { return strings.Join(argv, " "), nil }
func (s *session) Validate(context.Context) (model.Validation, error) {
	return s.got, s.err
}
func (s *session) Coverage(context.Context) (model.Coverage, error) {
	return model.Coverage{}, nil
}

func (s *session) Tools(context.Context) (model.ToolsListing, error) {
	return model.ToolsListing{}, nil
}
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
	c := validate.New(s)
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

func findings(n int) []model.Finding {
	var out []model.Finding
	for i := range n {
		out = append(out, model.Finding{Entity: fmt.Sprintf("a.yaml:%d", i+1), Msg: "wrong", Fix: "write it right"})
	}
	return out
}

func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	if _, err := kit.NewRegistry([]kit.Command{validate.Row()}); err != nil {
		t.Fatal(err)
	}
	if r := validate.Row(); r.State != kit.Built || r.Order != validate.Order {
		t.Errorf("row = %+v", r)
	}
}

// TestAnyFindingEndsFindingsAndANoticeNeverDoes: the word follows the
// findings alone (S27).
func TestAnyFindingEndsFindingsAndANoticeNeverDoes(t *testing.T) {
	t.Parallel()
	notice := []model.Notice{{Entity: "t.yaml:3", Msg: "stale"}}
	s := &session{got: model.Validation{Rules: 2, Notices: notice}}
	if code := run(t, s); code != exit.Done || s.ending.Word != model.Done {
		t.Errorf("a notice alone: exit %d, %+v", code, s.ending)
	}
	s = &session{got: model.Validation{Rules: 2, Findings: findings(1), Notices: notice}}
	if code := run(t, s); code != exit.Failed || s.ending.Word != model.Findings {
		t.Errorf("a finding: exit %d, %+v", code, s.ending)
	}
}

// TestAListOfTwoHundredFindingsShowsABoundedNumberAndTheCount: never cut
// without saying so.
func TestAListOfTwoHundredFindingsShowsABoundedNumberAndTheCount(t *testing.T) {
	t.Parallel()
	s := &session{got: model.Validation{Rules: 5, Findings: findings(200)}}
	run(t, s)
	var table model.Table
	var notes []string
	for _, b := range s.ending.Lead {
		switch b := b.(type) {
		case model.Table:
			table = b
		case model.Note:
			notes = append(notes, b.Text)
		}
	}
	if len(table.Rows) != validate.Limit || !slices.Contains(notes, "50 of 200 findings shown. Fix these and run it again to see the rest.") {
		t.Errorf("%d rows, notes %q", len(table.Rows), notes)
	}
	if !strings.Contains(s.ending.What, "200 findings") {
		t.Errorf("the sentence beside the word is %q", s.ending.What)
	}
	s = &session{json: true, got: s.got}
	run(t, s)
	d, ok := s.doc.(coreapi.Validation)
	if !ok || d.Total != 200 || d.Shown != validate.Limit || len(d.Findings) != validate.Limit {
		t.Errorf("json doc = %+v", s.doc)
	}
}

// TestAnEmptyRulebookIsDoneAndSaysWhatToRunNext.
func TestAnEmptyRulebookIsDoneAndSaysWhatToRunNext(t *testing.T) {
	t.Parallel()
	s := &session{got: model.Validation{}}
	if code := run(t, s); code != exit.Done || s.ending.Word != model.Done || s.ending.What != "0 rules, 0 findings, 0 notices" || s.ending.Next == "" {
		t.Errorf("exit %d, %+v", code, s.ending)
	}
}

// TestARulebookTheCoreCannotReadEndsNotReadyWithTheNextLine: an open run
// names journal recover; a newer format names its version.
func TestARulebookTheCoreCannotReadEndsNotReadyWithTheNextLine(t *testing.T) {
	t.Parallel()
	s := &session{err: &coreapi.OpenRecordError{Run: "r1"}}
	if code := run(t, s); code != exit.NotReady || s.ending.Word != model.NotReady || s.ending.Next != coreapi.RecoverLine {
		t.Errorf("open run: exit %d, %+v", code, s.ending)
	}
	s = &session{err: fmt.Errorf("%w: rulebook.yaml is written in format 4 and this engine reads up to format 3", coreapi.ErrNotReady)}
	if code := run(t, s); code != exit.NotReady || !strings.Contains(s.ending.What, "format 4") || !strings.Contains(s.ending.What, "format 3") {
		t.Errorf("newer format: exit %d, %+v", code, s.ending)
	}
}

// TestUnderSARIFEveryFindingIsInTheEncodedDocument: --sarif answers with the
// contract's SARIF of every finding, past the listing's limit, and no JSON
// document; the word is still the findings'. Planted failure: the listing's
// limit applied to SARIF.
func TestUnderSARIFEveryFindingIsInTheEncodedDocument(t *testing.T) {
	t.Parallel()
	s := &session{sarif: true, build: kit.Build{Version: "1.2.3"}, got: model.Validation{Rules: 2, Findings: findings(validate.Limit + 1)}}
	if code := run(t, s); code != exit.Failed || s.ending.Word != model.Findings || s.doc != nil {
		t.Fatalf("exit %d, ending %+v, document %v; want findings and no JSON document", code, s.ending, s.doc)
	}
	want, err := coreapi.ValidationSARIF(s.got, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if string(s.encoded) != string(want) || strings.Count(string(s.encoded), `"ruleId"`) != validate.Limit+1 {
		t.Errorf("encoded %d bytes with %d results, want the %d findings as ValidationSARIF writes them", len(s.encoded), strings.Count(string(s.encoded), `"ruleId"`), validate.Limit+1)
	}
}

// TestUnderSARIFAFindingSARIFCannotHoldEndsFailedWithNothingWritten: a
// finding with no message is one SARIF refuses, so nothing is written.
func TestUnderSARIFAFindingSARIFCannotHoldEndsFailedWithNothingWritten(t *testing.T) {
	t.Parallel()
	s := &session{sarif: true, got: model.Validation{Findings: []model.Finding{{Entity: "a.yaml:1", File: "a.yaml", Line: 1}}}}
	if code := run(t, s); code != exit.Failed || s.ending.Word != model.Failed || s.encoded != nil || !strings.Contains(s.ending.What, "it has no message") {
		t.Errorf("exit %d, ending %+v, encoded %q; want failed, naming why, and nothing written", code, s.ending, s.encoded)
	}
}

func (s *session) GuardEdit(context.Context) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) GuardStaged(context.Context, bool) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) Aside(string) {}

// The plan forms are the changes of a run, which this command never makes.
func (s *session) Migrate(context.Context, kit.Cmd, model.InitForm) int                    { return 0 }
func (s *session) SourceNew(context.Context, kit.Cmd, model.NewSource) int                 { return 0 }
func (s *session) SourceEdit(context.Context, kit.Cmd, model.EditSource) int               { return 0 }
func (s *session) Import(context.Context, kit.Cmd, model.ImportInput) int                  { return 0 }
func (s *session) DecisionNew(context.Context, kit.Cmd, model.NewDecisionRecord) int       { return 0 }
func (s *session) DecisionSupersede(context.Context, kit.Cmd, model.SupersedeDecision) int { return 0 }
func (s *session) Undo(context.Context, kit.Cmd) int                                       { return 0 }
func (s *session) Bind(context.Context, kit.Cmd, model.BindRule) int                       { return 0 }
func (s *session) Rebind(context.Context, kit.Cmd, model.RebindRule) int                   { return 0 }
func (s *session) Unbind(context.Context, kit.Cmd, model.UnbindRule) int                   { return 0 }

func (s *session) Conflicts(context.Context) (model.ConflictsListing, error) {
	return model.ConflictsListing{}, nil
}

func (s *session) CheckRulebook(context.Context) (model.RulebookCheck, *model.Refusal, error) {
	return model.RulebookCheck{}, nil, nil
}

// Attention is never called by this command.
func (s *session) Attention(context.Context) (model.AttentionListing, error) {
	return model.AttentionListing{}, nil
}

// TestValidatePrintsTheHeldCount: validate says in one line how many migrated
// rules the prose lint holds, with findings or none, when the count is zero
// too, and the document carries the count. Risk: an owner who cannot see how
// much of the conversion is left to reword.
func TestValidatePrintsTheHeldCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		got  model.Validation
		want string
	}{
		{model.Validation{Rules: 3}, "0 migrated rules held from the prose lint until reworded; the count only falls."},
		{model.Validation{Rules: 3, Held: 1}, "1 migrated rule held from the prose lint until reworded; the count only falls."},
		{model.Validation{Rules: 1130, Held: 1129, Findings: findings(2)}, "1,129 migrated rules held from the prose lint until reworded; the count only falls."},
	}
	for _, c := range cases {
		s := &session{got: c.got}
		run(t, s)
		var held []string
		for _, b := range s.ending.Lead {
			if n, ok := b.(model.Note); ok && strings.Contains(n.Text, "held from the prose lint") {
				held = append(held, n.Text)
			}
		}
		if len(held) != 1 || held[0] != c.want {
			t.Errorf("held %d: the lines are %q, want the one %q", c.got.Held, held, c.want)
		}
		s = &session{json: true, got: c.got}
		run(t, s)
		if d, ok := s.doc.(coreapi.Validation); !ok || d.Held != c.got.Held {
			t.Errorf("held %d: json doc = %+v", c.got.Held, s.doc)
		}
	}
}
