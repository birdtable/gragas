package conflicts_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/conflicts"
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

// session is a run that keeps what the command said instead of drawing it, and
// answers Conflicts with what the test set.
type session struct {
	kit.Contracts // never called by this command
	kit.Records   // nor this
	kit.Explains  // nor this
	kit.Projects  // nor this
	kit.Plugins   // nor this
	json, sarif   bool
	got           model.ConflictsListing
	err           error
	doc           any
	encoded       []byte
	ending        *kit.Ending
}

func (s *session) Say(string)                         {}
func (s *session) JSON() bool                         { return s.json }
func (s *session) Document(v any)                     { s.doc = v }
func (s *session) SARIF() bool                        { return s.sarif }
func (s *session) Encoded(b []byte)                   { s.encoded = b }
func (s *session) Build() kit.Build                   { return kit.Build{Version: "v1.2.3"} }
func (s *session) Line(argv []string) (string, error) { return strings.Join(argv, " "), nil }
func (s *session) Validate(context.Context) (model.Validation, error) {
	return model.Validation{}, nil
}
func (s *session) Tools(context.Context) (model.ToolsListing, error) {
	return model.ToolsListing{}, nil
}
func (s *session) Conflicts(context.Context) (model.ConflictsListing, error) {
	return s.got, s.err
}
func (s *session) Coverage(context.Context) (model.Coverage, error) {
	return model.Coverage{}, nil
}
func (s *session) Undo(context.Context, kit.Cmd) int { return 0 }
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

// The plan forms are the changes of a run, which this command never makes.
func (s *session) Migrate(context.Context, kit.Cmd, model.InitForm) int              { return 0 }
func (s *session) SourceNew(context.Context, kit.Cmd, model.NewSource) int           { return 0 }
func (s *session) SourceEdit(context.Context, kit.Cmd, model.EditSource) int         { return 0 }
func (s *session) Import(context.Context, kit.Cmd, model.ImportInput) int            { return 0 }
func (s *session) DecisionNew(context.Context, kit.Cmd, model.NewDecisionRecord) int { return 0 }
func (s *session) DecisionSupersede(context.Context, kit.Cmd, model.SupersedeDecision) int {
	return 0
}
func (s *session) Bind(context.Context, kit.Cmd, model.BindRule) int     { return 0 }
func (s *session) Rebind(context.Context, kit.Cmd, model.RebindRule) int { return 0 }
func (s *session) Unbind(context.Context, kit.Cmd, model.UnbindRule) int { return 0 }

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func run(t *testing.T, s *session) int {
	t.Helper()
	c := conflicts.New(s)
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

func side(rule string, value any, sev model.Severity) model.ConflictSide {
	return model.ConflictSide{Rule: model.RuleID(rule), Value: value, Severity: sev}
}

func open() model.Conflict {
	return model.Conflict{
		Entry:   model.EntryRef{Tool: "lint-a", Target: model.TargetSource, Key: "wrap"},
		Differs: []string{model.DiffValue, model.DiffSeverity},
		Sides:   []model.ConflictSide{side("go.errors.1", true, model.SeverityError), side("go.errors.2", false, model.SeverityWarning)},
	}
}

func resolved() model.Conflict {
	c := open()
	c.Entry.Key = "cause"
	c.Winner = "go.errors.2"
	return c
}

func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	if _, err := kit.NewRegistry([]kit.Command{conflicts.Row()}); err != nil {
		t.Fatal(err)
	}
	if r := conflicts.Row(); r.State != kit.Built || r.Order != conflicts.Order || r.Context != kit.InRulebook || r.Ends != kit.EndsListing || !r.SARIF {
		t.Errorf("row = %+v", r)
	}
}

// screen is what the lead drew, line by line: a heading as its text, a table row
// as its cells joined by a bar, a note as its text.
func screen(e *kit.Ending) []string {
	var out []string
	for _, b := range e.Lead {
		switch b := b.(type) {
		case model.Heading:
			out = append(out, "# "+b.Text)
		case model.Table:
			for _, r := range b.Rows {
				out = append(out, strings.Join(r, "|"))
			}
		case model.Note:
			out = append(out, "- "+b.Text)
		}
	}
	return out
}

// TestAnOpenConflictEndsOnFindingsWithItsRulesAndTheThreeResolutions: the
// heading names the entry and what differs, each rule is a row with what it
// sets, and the three resolutions follow; a resolved conflict is a note and
// not a table. The word is findings, exit 1, and the next line the first
// resolution that is a command.
func TestAnOpenConflictEndsOnFindingsWithItsRulesAndTheThreeResolutions(t *testing.T) {
	t.Parallel()
	s := &session{got: model.ConflictsListing{Conflicts: []model.Conflict{open(), resolved()}}}
	if code := run(t, s); code != 1 || s.ending.Word != model.Findings || s.ending.What != "1 conflict open, 1 resolved" || s.ending.Next != "gragas rebind" {
		t.Fatalf("exit %d, %+v", code, s.ending)
	}
	got := screen(s.ending)
	want := []string{
		"# lint-a source: wrap differs in value, severity",
		"go.errors.1|true|-|error",
		"go.errors.2|false|-|warning",
		"- resolved: lint-a source: cause, a decision names go.errors.2",
		"# Resolve each in one of three ways",
	}
	if len(got) != len(want)+3 || strings.Join(got[:len(want)], "\n") != strings.Join(want, "\n") {
		t.Errorf("the screen is\n%s\nwant\n%s\nand the three resolutions", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i, line := range coreapi.ResolutionLines() {
		if got[len(want)+i] != "- "+line {
			t.Errorf("resolution %d is %q, want %q", i+1, got[len(want)+i], line)
		}
	}
}

// TestNoConflictEndsDoneAndSaysSo, and a listing with only resolved ones is
// done too, with no resolution offered. The plant is the open conflict above,
// which ends findings.
func TestNoConflictEndsDoneAndSaysSo(t *testing.T) {
	t.Parallel()
	s := &session{got: model.ConflictsListing{Conflicts: []model.Conflict{}}}
	if code := run(t, s); code != exit.Done || s.ending.Word != model.Done || s.ending.What != "no conflicts" || s.ending.Next != "" || len(s.ending.Lead) != 0 {
		t.Errorf("exit %d, %+v", code, s.ending)
	}
	s = &session{got: model.ConflictsListing{Conflicts: []model.Conflict{resolved()}}}
	if code := run(t, s); code != exit.Done || s.ending.Word != model.Done || s.ending.What != "0 conflicts open, 1 resolved" {
		t.Errorf("only a resolved one: exit %d, %+v", code, s.ending)
	}
	if got := strings.Join(screen(s.ending), "\n"); strings.Contains(got, "Resolve each") {
		t.Errorf("a resolution is offered with none open:\n%s", got)
	}
}

// TestJSONIsOneDocumentAndDrawsNothing: the word, the counts and the three
// resolutions while one is open.
func TestJSONIsOneDocumentAndDrawsNothing(t *testing.T) {
	t.Parallel()
	s := &session{json: true, got: model.ConflictsListing{Conflicts: []model.Conflict{open(), resolved()}}}
	run(t, s)
	d, ok := s.doc.(coreapi.ConflictsDoc)
	if !ok || d.Contract != model.ContractVersion || d.Command != "conflicts" || d.Word != "findings" || d.Open != 1 || d.Resolved != 1 ||
		len(d.Conflicts) != 2 || len(d.Resolutions) != 3 {
		t.Errorf("doc = %+v", s.doc)
	}
	if len(s.ending.Lead) != 0 {
		t.Errorf("--json drew %+v", s.ending.Lead)
	}
}

// TestSARIFIsTheEncodedDocumentAndDrawsNothing: one result for the open
// conflict, none for the resolved one.
func TestSARIFIsTheEncodedDocumentAndDrawsNothing(t *testing.T) {
	t.Parallel()
	s := &session{sarif: true, got: model.ConflictsListing{Conflicts: []model.Conflict{open(), resolved()}}}
	if code := run(t, s); code != 1 || strings.Count(string(s.encoded), `"ruleId"`) != 1 || len(s.ending.Lead) != 0 {
		t.Errorf("exit %d, results %d, drew %+v", code, strings.Count(string(s.encoded), `"ruleId"`), s.ending.Lead)
	}
}

// TestAFolderThatCannotBeListedEndsOnTheRightWordWithTheRightNextLine.
func TestAFolderThatCannotBeListedEndsOnTheRightWordWithTheRightNextLine(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		err  error
		word model.Word
		next string
	}{
		"no rulebook":             {coreapi.ErrNoBook, model.NotReady, "gragas init"},
		"two rulebooks":           {coreapi.ErrTwoBooks, model.NotReady, "gragas init"},
		"findings":                {&coreapi.FindingsError{Count: 3}, model.NotReady, "gragas validate"},
		"a tool no plugin serves": {&coreapi.UnservedTools{Tools: []string{"lint-a"}}, model.NotReady, "gragas plugin add <folder>"},
		"an unapproved plugin": {&coreapi.UnapprovedPlugin{Ref: model.PluginRef{Name: "a-lint", Version: "1.0.0"}, Err: errors.New("not approved")},
			model.NotReady, "gragas plugin approve a-lint@1.0.0"},
		"an open run":   {&coreapi.OpenRecordError{Run: "r1"}, model.NotReady, coreapi.RecoverLine},
		"anything else": {errors.New("disk"), model.Failed, ""},
	} {
		s := &session{err: c.err}
		code := run(t, s)
		want, _ := exit.Code(c.word)
		if code != want || s.ending.Word != c.word || s.ending.Next != c.next {
			t.Errorf("%s: exit %d, %+v; want %s and %q", name, code, s.ending, c.word, c.next)
		}
	}
}

// The guard hook is not this command's: it answers nothing.
func (s *session) GuardEdit(context.Context) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) GuardStaged(context.Context, bool) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) Aside(string) {}

func (s *session) CheckRulebook(context.Context) (model.RulebookCheck, *model.Refusal, error) {
	return model.RulebookCheck{}, nil, nil
}

// Attention is never called by this command.
func (s *session) Attention(context.Context) (model.AttentionListing, error) {
	return model.AttentionListing{}, nil
}
