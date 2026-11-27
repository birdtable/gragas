package tools_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/tools"
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
// answers Tools with what the test set.
type session struct {
	kit.Contracts // never called by this command
	kit.Records   // nor this
	kit.Explains  // nor this
	kit.Projects  // nor this
	kit.Plugins   // nor this
	json          bool
	got           model.ToolsListing
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
func (s *session) Coverage(context.Context) (model.Coverage, error) {
	return model.Coverage{}, nil
}

func (s *session) Tools(context.Context) (model.ToolsListing, error) { return s.got, s.err }
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
	c := tools.New(s)
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

// listing is three tools: lint-a, served by a-lint, lint-q, which only the
// tested set names, and lint-z, which a rule binds to and no plugin serves.
func listing() model.ToolsListing {
	return model.ToolsListing{Rules: 2, Rows: []model.ToolRow{
		{Tool: "lint-a", Tested: "2.5.0", Plugin: &model.ToolPlugin{Name: "a-lint", Version: "1.0.0"}, Entries: []model.ToolEntryRow{
			{Target: model.TargetConfig, Key: "cause", Rules: []model.RuleID{"go.errors.2"}},
			{Target: model.TargetSource, Key: "wrap", Rules: []model.RuleID{"go.errors.1", "go.errors.2"}},
		}},
		{Tool: "lint-q", Tested: "1.0.0", Entries: []model.ToolEntryRow{}},
		{Tool: "lint-z", Entries: []model.ToolEntryRow{{Target: model.TargetSource, Key: "zed", Rules: []model.RuleID{"go.errors.2"}}}},
	}}
}

func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	if _, err := kit.NewRegistry([]kit.Command{tools.Row()}); err != nil {
		t.Fatal(err)
	}
	if r := tools.Row(); r.State != kit.Built || r.Order != tools.Order || r.Context != kit.InBoth || r.Ends != kit.EndsListing {
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

// served is listing with lint-z served by z-lint: every bound tool has a
// plugin.
func served() model.ToolsListing {
	l := listing()
	l.Rows[2].Plugin = &model.ToolPlugin{Name: "z-lint", Version: "0.1.0"}
	return l
}

// TestEachToolHasAHeadingWithItsTestedVersionAndPluginAndARowForEachEntry: a
// tool with no tested version says not tested, one with no plugin says so, one
// no rule binds says that, and each entry shows every rule that holds it. A
// tool a rule binds to with no plugin ends the run not ready, naming it and
// the step that installs one, with the listing drawn above.
// Risk: a listing that ends done while a bound tool is read by nothing.
func TestEachToolHasAHeadingWithItsTestedVersionAndPluginAndARowForEachEntry(t *testing.T) {
	t.Parallel()
	s := &session{got: listing()}
	if code := run(t, s); code != exit.NotReady || s.ending.Word != model.NotReady || s.ending.Next != "gragas plugin add <folder>" ||
		s.ending.What != "3 tools, 3 entries, 2 rules held; no plugin is installed for lint-z, so no check reads its config" {
		t.Fatalf("exit %d, %+v", code, s.ending)
	}
	want := []string{
		"# lint-a, tested 2.5.0, plugin a-lint 1.0.0",
		"config: cause|go.errors.2",
		"source: wrap|go.errors.1, go.errors.2",
		"# lint-q, tested 1.0.0, no plugin",
		"- no rule binds it",
		"# lint-z, not tested, no plugin",
		"source: zed|go.errors.2",
		"- no preset entries read: 0 unexplained (coverage lists them)",
	}
	if got := screen(s.ending); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the screen is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	s = &session{got: served()}
	if code := run(t, s); code != exit.Done || s.ending.Word != model.Done || s.ending.Next != "" || s.ending.What != "3 tools, 3 entries, 2 rules held" {
		t.Errorf("every bound tool served: exit %d, %+v; want done", code, s.ending)
	}
}

// TestTheUnexplainedEntriesAreSaidNotReadAndNoBindingSaysSo: the note that no
// preset entry is read, last and once, and the empty state of a rulebook with no
// binding.
func TestTheUnexplainedEntriesAreSaidNotReadAndNoBindingSaysSo(t *testing.T) {
	t.Parallel()
	s := &session{got: served()}
	run(t, s)
	got := screen(s.ending)
	if n := len(got); n == 0 || !strings.HasPrefix(got[n-1], "- no preset entries read: 0 unexplained") || strings.Count(strings.Join(got, "\n"), "no preset entries read") != 1 {
		t.Errorf("screen = %q", got)
	}
	s = &session{}
	if code := run(t, s); code != exit.Done || s.ending.What != "0 tools, 0 entries, 0 rules held" ||
		strings.Join(screen(s.ending), "|") != "- no bindings: 0 rules held" {
		t.Errorf("exit %d, %+v", code, s.ending)
	}
	s = &session{got: model.ToolsListing{Rows: []model.ToolRow{{Tool: "lint-q", Tested: "1.0.0", Entries: []model.ToolEntryRow{}}}}}
	run(t, s)
	if got := screen(s.ending); !strings.Contains(strings.Join(got, "\n"), "- no bindings: 0 rules held") {
		t.Errorf("a tested tool and no binding: screen = %q", got)
	}
}

// TestJSONIsOneDocumentAndDrawsNothing: the contract version, the counts, an
// empty unexplained list and every tool; and, with a bound tool no plugin
// serves, the same document saying not ready and naming it, under the ending
// not ready.
func TestJSONIsOneDocumentAndDrawsNothing(t *testing.T) {
	t.Parallel()
	s := &session{json: true, got: served()}
	run(t, s)
	d, ok := s.doc.(coreapi.ToolsDoc)
	if !ok || d.Contract != model.ContractVersion || d.Schema != 2 || d.Command != "tools" || d.Word != "done" || d.Entries != 3 || d.Rules != 2 ||
		len(d.Tools) != 3 || d.Unexplained == nil || len(d.Unexplained) != 0 || d.Unserved == nil || len(d.Unserved) != 0 {
		t.Errorf("doc = %+v", s.doc)
	}
	if len(s.ending.Lead) != 0 {
		t.Errorf("--json drew %+v", s.ending.Lead)
	}
	s = &session{json: true, got: listing()}
	code := run(t, s)
	d, ok = s.doc.(coreapi.ToolsDoc)
	if !ok || code != exit.NotReady || d.Word != string(model.NotReady) || strings.Join(d.Unserved, ",") != "lint-z" || s.ending.Word != model.NotReady || len(s.ending.Lead) != 0 {
		t.Errorf("unserved: exit %d, doc %+v, ending %+v", code, s.doc, s.ending)
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
		"no rulebook":   {coreapi.ErrNoBook, model.NotReady, "gragas init"},
		"two rulebooks": {coreapi.ErrTwoBooks, model.NotReady, "gragas init"},
		"findings":      {&coreapi.FindingsError{Count: 3}, model.NotReady, "gragas validate"},
		"an open run":   {&coreapi.OpenRecordError{Run: "r1"}, model.NotReady, coreapi.RecoverLine},
		"an unapproved plugin": {&coreapi.UnapprovedPlugin{Ref: model.PluginRef{Name: "a-lint", Version: "1.0.0"}, Err: errors.New("not approved")},
			model.NotReady, "gragas plugin approve a-lint@1.0.0"},
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
