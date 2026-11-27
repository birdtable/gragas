package version_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/version"
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

// session is a run that keeps what the command said instead of drawing it.
type session struct {
	kit.Contracts // never called by this command
	kit.Records   // nor this
	kit.Explains  // nor this
	kit.Projects  // nor this
	kit.Plugins   // nor this
	json          bool
	build         kit.Build
	said          []string
	doc           any
	ending        *kit.Ending
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
func (s *session) Build() kit.Build                   { return s.build }
func (s *session) Line(argv []string) (string, error) { return strings.Join(argv, " "), nil }

func (s *session) Validate(context.Context) (model.Validation, error) {
	return model.Validation{}, nil
}

func (s *session) Coverage(context.Context) (model.Coverage, error) {
	return model.Coverage{}, nil
}

func (s *session) Tools(context.Context) (model.ToolsListing, error) {
	return model.ToolsListing{}, nil
}

func (s *session) End(e kit.Ending) int {
	s.ending = &e
	return exit.Done
}

type exitCode int

func (e exitCode) Error() string { return "exit" }

func run(t *testing.T, s *session, args ...string) {
	t.Helper()
	c := version.New(s)
	c.SetArgs(append([]string{}, args...)) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SilenceUsage, c.SilenceErrors = true, true
	if err := c.Execute(); err != nil {
		t.Fatalf("version %v: %v", args, err)
	}
}

// TestTheRowIsOneThatTheRegistryTakes: the declaration passes the registry's
// rules and claims a slot of its own.
func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	row := version.Row()
	if _, err := kit.NewRegistry([]kit.Command{row}); err != nil {
		t.Fatal(err)
	}
	if row.Order != version.Order || row.Order == 0 || row.State != kit.Built || !row.Menu.Offered {
		t.Errorf("row = %+v, want a built row that claims its slot and is offered by the menu", row)
	}
}

// TestVersionSaysTheBuildAsLines: a tagged build names its tag and commit; a
// build no tag reaches says so, and the commit it lacks.
func TestVersionSaysTheBuildAsLines(t *testing.T) {
	t.Parallel()
	tagged := &session{build: kit.Build{Version: "v1.2.3", Commit: "abc123", Line: "v1.2.3 (abc123)"}}
	run(t, tagged)
	text := strings.Join(tagged.said, "\n")
	for _, want := range []string{"gragas v1.2.3 (abc123)", "tag       v1.2.3", "commit    abc123", "format    ", "contract  "} {
		if !strings.Contains(text, want) {
			t.Errorf("a tagged build says\n%s\nwant %q", text, want)
		}
	}
	if tagged.ending == nil || tagged.ending.What != "this is the build above" {
		t.Errorf("ending = %+v", tagged.ending)
	}
	untagged := &session{build: kit.Build{Version: "v0.0.0-dev", Line: "v0.0.0-dev"}}
	run(t, untagged)
	text = strings.Join(untagged.said, "\n")
	for _, want := range []string{"none, no release tag reaches this build", "unknown, the build recorded none"} {
		if !strings.Contains(text, want) {
			t.Errorf("an untagged build says\n%s\nwant %q", text, want)
		}
	}
}

// TestVersionUnderJSONAnswersOneDocumentAndDrawsNothing: the document
// carries the toolchain go-run's version.Build read and, for a
// build no tag reaches, no tag with the mark that says so. Planted failure:
// the toolchain read from the running binary in place of the build's.
func TestVersionUnderJSONAnswersOneDocumentAndDrawsNothing(t *testing.T) {
	t.Parallel()
	s := &session{json: true, build: kit.Build{Version: "v0.0.0-dev", Go: "go1.27.3"}}
	run(t, s)
	v, ok := s.doc.(coreapi.Version)
	if !ok || v.Tag != nil || v.Commit != nil || v.Notes["tag"] != "no release tag reaches this build" || v.Notes["commit"] == "" {
		t.Errorf("document = %+v, want an untagged build with its reasons", s.doc)
	}
	if v.Go != "go1.27.3" {
		t.Errorf("the document's toolchain is %q, want the build's go1.27.3", v.Go)
	}
	if len(s.said) != 0 || s.ending != nil {
		t.Errorf("under --json it said %v and ended %+v", s.said, s.ending)
	}
}

// TestVersionTakesNoArgument.
func TestVersionTakesNoArgument(t *testing.T) {
	t.Parallel()
	c := version.New(&session{})
	c.SetArgs([]string{"extra"})
	c.SilenceUsage, c.SilenceErrors = true, true
	if err := c.Execute(); err == nil {
		t.Error("version took an argument")
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
