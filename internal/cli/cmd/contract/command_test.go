package contract_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/contract"
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

// session keeps what the command asked and said, and answers from the fields
// a test sets.
type session struct {
	kit.Records  // never called by this command
	kit.Explains // nor this
	kit.Projects // nor this
	kit.Plugins  // nor this
	check        model.ContractCheck
	fields       model.ContractFields
	fixture      model.ContractFixture
	err          error

	asked  []string
	json   bool
	said   []string
	doc    any
	ending *kit.Ending
}

func (s *session) Say(line string) { s.said = append(s.said, line) }
func (s *session) JSON() bool      { return s.json }
func (s *session) Document(v any)  { s.doc = v }
func (s *session) SARIF() bool     { return false }
func (s *session) Encoded([]byte)  {}
func (s *session) Exit(code int) error {
	if code == exit.Done {
		return nil
	}
	return exitCode(code)
}
func (s *session) Build() kit.Build                   { return kit.Build{} }
func (s *session) Line(argv []string) (string, error) { return strings.Join(argv, " "), nil }
func (s *session) End(e kit.Ending) int {
	s.ending = &e
	return exit.Done
}

// Migrate is the plan form, which this command never runs.
func (s *session) Migrate(context.Context, kit.Cmd, model.InitForm) int { return 0 }

// GuardEdit and Aside are the guard's, which this command never runs.
func (s *session) GuardEdit(context.Context) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) GuardStaged(context.Context, bool) (model.Guard, *model.Refusal, error) {
	return model.Guard{}, nil, nil
}
func (s *session) Aside(string) {}

// SourceNew and SourceEdit are the plan forms of the source commands, which
// this command never runs.
func (s *session) SourceNew(context.Context, kit.Cmd, model.NewSource) int           { return 0 }
func (s *session) SourceEdit(context.Context, kit.Cmd, model.EditSource) int         { return 0 }
func (s *session) Import(context.Context, kit.Cmd, model.ImportInput) int            { return 0 }
func (s *session) DecisionNew(context.Context, kit.Cmd, model.NewDecisionRecord) int { return 0 }
func (s *session) DecisionSupersede(context.Context, kit.Cmd, model.SupersedeDecision) int {
	return 0
}
func (s *session) Undo(context.Context, kit.Cmd) int                     { return 0 }
func (s *session) Bind(context.Context, kit.Cmd, model.BindRule) int     { return 0 }
func (s *session) Rebind(context.Context, kit.Cmd, model.RebindRule) int { return 0 }
func (s *session) Unbind(context.Context, kit.Cmd, model.UnbindRule) int { return 0 }

func (s *session) Validate(context.Context) (model.Validation, error) { return model.Validation{}, nil }
func (s *session) Coverage(context.Context) (model.Coverage, error) {
	return model.Coverage{}, nil
}

func (s *session) Tools(context.Context) (model.ToolsListing, error) {
	return model.ToolsListing{}, nil
}

func (s *session) ContractValidate(_ context.Context, file, kind string) (model.ContractCheck, error) {
	s.asked = append(s.asked, "validate "+file+" "+kind)
	return s.check, s.err
}

func (s *session) ContractFields(_ context.Context, entity string) (model.ContractFields, error) {
	s.asked = append(s.asked, "fields "+entity)
	return s.fields, s.err
}

func (s *session) ContractFixture(_ context.Context, out string, apply bool) (model.ContractFixture, error) {
	s.asked = append(s.asked, "fixture "+out+" "+map[bool]string{true: "apply", false: "dry"}[apply])
	return s.fixture, s.err
}

type exitCode int

func (e exitCode) Error() string { return "exit" }

func run(t *testing.T, s *session, args ...string) {
	t.Helper()
	c := contract.New(s)
	c.SetArgs(append([]string{}, args...))
	c.SilenceUsage, c.SilenceErrors = true, true
	if err := c.Execute(); err != nil {
		t.Fatalf("contract %v: %v", args, err)
	}
}

// TestTheRowIsOneThatTheRegistryTakes: the declaration passes the registry's
// rules, is built and names the three verbs.
func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	row := contract.Row()
	if _, err := kit.NewRegistry([]kit.Command{row}); err != nil {
		t.Fatal(err)
	}
	if row.State != kit.Built || !slices.Equal(row.Then, []string{"validate", "fields", "fixture"}) || !row.SARIF {
		t.Errorf("row = %+v", row)
	}
}

// TestValidateEndsOnTheWordTheCheckGives: done, findings, usage naming the
// kinds, and not ready naming both versions. Risk: a newer major ending as
// findings, or a bad kind as a failure.
func TestValidateEndsOnTheWordTheCheckGives(t *testing.T) {
	t.Parallel()
	kinds := []string{"bundle", "report"}
	for _, tc := range []struct {
		name  string
		check model.ContractCheck
		word  model.Word
		has   []string
	}{
		{"valid", model.ContractCheck{Kind: "bundle", Contract: "1.0"}, model.Done, []string{"bundle", "0 findings"}},
		{"findings", model.ContractCheck{Kind: "bundle", Contract: "1.0", Findings: []model.Finding{{Entity: "rules[0]", Msg: "bad"}}}, model.Findings, []string{"1 finding"}},
		{"unknown kind", model.ContractCheck{Usage: `"x" is not a kind of document`, Known: kinds}, model.Usage, []string{"kinds are listed above"}},
		{"newer major", model.ContractCheck{Kind: "report", NotReady: &model.ContractNotReady{Found: "2.0", Reads: "1.0"}}, model.NotReady, []string{"2.0", "1.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &session{check: tc.check}
			run(t, s, "validate", "doc.json")
			if s.ending == nil || s.ending.Word != tc.word {
				t.Fatalf("ending = %+v, want %s", s.ending, tc.word)
			}
			for _, w := range tc.has {
				if !strings.Contains(s.ending.What, w) {
					t.Errorf("what = %q, want %q", s.ending.What, w)
				}
			}
		})
	}
}

// TestValidatePassesTheAsFlag: --as reaches the operation.
func TestValidatePassesTheAsFlag(t *testing.T) {
	t.Parallel()
	s := &session{check: model.ContractCheck{Kind: "plan"}}
	run(t, s, "validate", "p.json", "--as", "plan")
	if !slices.Equal(s.asked, []string{"validate p.json plan"}) {
		t.Errorf("asked %v", s.asked)
	}
}

// TestFieldsListsATableAndAnUnknownEntityIsUsage.
func TestFieldsListsATableAndAnUnknownEntityIsUsage(t *testing.T) {
	t.Parallel()
	s := &session{fields: model.ContractFields{Entity: "rule", Contract: "1.0", Fields: []model.ContractField{{Name: "id", Type: "string", Since: "1.0", Default: "required"}}}}
	run(t, s, "fields", "rule")
	if s.ending == nil || s.ending.Word != model.Done || !strings.Contains(s.ending.What, "1 field") || len(s.ending.Lead) == 0 {
		t.Errorf("ending = %+v", s.ending)
	}
	u := &session{fields: model.ContractFields{Usage: "no entity x", Known: []string{"rule"}}}
	run(t, u, "fields", "x")
	if u.ending == nil || u.ending.Word != model.Usage {
		t.Errorf("ending = %+v", u.ending)
	}
}

// TestFixtureNeedsOutAndOnlyApplyWrites: no --out is usage; a dry run asks
// with apply false and ends on the apply line; --apply asks with apply true.
func TestFixtureNeedsOutAndOnlyApplyWrites(t *testing.T) {
	t.Parallel()
	none := &session{}
	run(t, none, "fixture")
	if none.ending == nil || none.ending.Word != model.Usage || len(none.asked) != 0 {
		t.Errorf("no --out: ending %+v, asked %v", none.ending, none.asked)
	}
	dry := &session{fixture: model.ContractFixture{Out: "d", Files: []string{"a.json"}}}
	run(t, dry, "fixture", "--out", "d")
	if !slices.Equal(dry.asked, []string{"fixture d dry"}) || dry.ending.Word != model.Declined || !strings.HasSuffix(dry.ending.Next, "--apply") {
		t.Errorf("dry run: asked %v, ending %+v", dry.asked, dry.ending)
	}
	dryJSON := &session{json: true, fixture: model.ContractFixture{Out: "d", Files: []string{"a.json"}}}
	run(t, dryJSON, "fixture", "--out", "d")
	if dryJSON.doc == nil || dryJSON.ending == nil || dryJSON.ending.Word != model.Declined || !strings.HasSuffix(dryJSON.ending.Next, "--apply") {
		t.Errorf("dry run under --json: doc %v, ending %+v, want the document and declined", dryJSON.doc, dryJSON.ending)
	}
	wetJSON := &session{json: true, fixture: model.ContractFixture{Out: "d", Files: []string{"a.json"}, Wrote: true}}
	run(t, wetJSON, "fixture", "--out", "d", "--apply")
	if wetJSON.ending == nil || wetJSON.ending.Word != model.Done {
		t.Errorf("apply under --json: ending %+v, want done", wetJSON.ending)
	}
	wet := &session{fixture: model.ContractFixture{Out: "d", Files: []string{"a.json"}, Wrote: true}}
	run(t, wet, "fixture", "--out", "d", "--apply")
	if !slices.Equal(wet.asked, []string{"fixture d apply"}) || wet.ending.Word != model.Done {
		t.Errorf("apply: asked %v, ending %+v", wet.asked, wet.ending)
	}
}

// TestFixtureCarriesTheWritesAnnotation: the verb that writes says so.
func TestFixtureCarriesTheWritesAnnotation(t *testing.T) {
	t.Parallel()
	for _, c := range contract.New(&session{}).Commands() {
		_, marked := c.Annotations[kit.Writes]
		if marked != (c.Name() == "fixture") {
			t.Errorf("%s: writes annotation %v", c.Name(), marked)
		}
	}
}

// TestAFolderAnotherRunHoldsEndsBusyAndAnyOtherFailureFailed: the word follows
// the error's identity, never its text (go.errors.8).
func TestAFolderAnotherRunHoldsEndsBusyAndAnyOtherFailureFailed(t *testing.T) {
	t.Parallel()
	busy := &session{err: fmt.Errorf("held elsewhere: %w", coreapi.ErrFolderBusy)}
	run(t, busy, "fixture", "--out", "d", "--apply")
	if busy.ending == nil || busy.ending.Word != model.Busy {
		t.Errorf("a busy folder: ending %+v", busy.ending)
	}
	other := &session{err: errors.New("the folder is busy, says its text only")}
	run(t, other, "fixture", "--out", "d", "--apply")
	if other.ending == nil || other.ending.Word != model.Failed {
		t.Errorf("another failure: ending %+v", other.ending)
	}
}

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
