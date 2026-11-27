package plugin_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/plugin"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// session answers the one operation this command calls, and keeps what the
// command said. The rest of kit.Session is nil: a call to it fails the test.
type session struct {
	kit.Session
	answer model.PluginTest
	list   model.PluginsListing
	shown  model.PluginApproval
	apply  bool
	given  []model.Digest
	err    error
	json   bool
	asked  []string
	added  []string
	// upgraded is each upgrade's source and ref, joined by a space.
	upgraded []string
	doc      any
	ending   *kit.Ending
}

func (s *session) PluginTest(_ context.Context, dir string) (model.PluginTest, error) {
	s.asked = append(s.asked, dir)
	return s.answer, s.err
}
func (s *session) PluginUpgrade(_ context.Context, _ kit.Cmd, source, ref string) int {
	s.upgraded = append(s.upgraded, source+" "+ref)
	return 0
}
func (s *session) PluginAdd(_ context.Context, _ kit.Cmd, dir string) int {
	s.added = append(s.added, dir)
	return 0
}
func (s *session) JSON() bool       { return s.json }
func (s *session) ApplyFlag() *bool { return &s.apply }
func (s *session) Applying() bool   { return s.apply }
func (s *session) Document(v any)   { s.doc = v }
func (s *session) Exit(int) error   { return nil }
func (s *session) End(e kit.Ending) int {
	s.ending = &e
	return exit.Done
}

func run(t *testing.T, s *session, args ...string) {
	t.Helper()
	c := plugin.New(s)
	c.SetArgs(args)
	c.SilenceUsage, c.SilenceErrors = true, true
	if err := c.Execute(); err != nil {
		t.Fatalf("plugin %v: %v", args, err)
	}
}

func answer(statuses ...string) model.PluginTest {
	a := model.PluginTest{Schema: model.PluginTestSchema, Bundle: "plugins/lint-a/2.0.0"}
	for i, st := range statuses {
		c := model.PluginCheck{N: i + 1, Name: "check", Status: st}
		if st != model.CheckPassed {
			c.What = []string{"because", "and more"}
		}
		a.Checks = append(a.Checks, c)
	}
	return a
}

// TestTheRowIsOneThatTheRegistryTakes: the declaration passes the registry's
// rules and names test and contract.
func TestTheRowIsOneThatTheRegistryTakes(t *testing.T) {
	t.Parallel()
	row := plugin.Row()
	if _, err := kit.NewRegistry([]kit.Command{row}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(row.Then, []string{"test", "contract", "list", "approve", "add", "upgrade", "record"}) {
		t.Errorf("Then = %v, want [test contract list approve add upgrade record]", row.Then)
	}
}

// TestTheHelpSaysWhatRunsWithoutApproval (Plugins.Try's fourth condition):
// plugin test runs a module nobody approved, so
// its help says so: the module runs in the sandbox without approval, and no
// step a plan answers is run. Risk: a person runs plugin test on a bundle
// they do not trust, thinking nothing of it runs.
func TestTheHelpSaysWhatRunsWithoutApproval(t *testing.T) {
	t.Parallel()
	c, _, err := plugin.New(&session{}).Find([]string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Join(strings.Fields(c.Long), " ")
	for _, want := range []string{
		"runs the bundle's module in the sandbox",
		"without asking for approval",
		"None of the steps a plan answers is run",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("plugin test's help lacks %q:\n%s", want, c.Long)
		}
	}
}

// TestPluginTestTranscript: the command asks for the bundle once and ends on
// the word the answer holds, findings over not ready over done; a bundle that
// cannot be read is failed.
func TestPluginTestTranscript(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		answer model.PluginTest
		want   model.Word
		in     string
	}{
		{"every check passed", answer(slices.Repeat([]string{model.CheckPassed}, model.PluginChecks)...), model.Done, "23 passed, 0 failed, 0 not run, 0 not asked"},
		{"passed or not asked", answer(append(slices.Repeat([]string{model.CheckPassed}, 18), slices.Repeat([]string{model.CheckNotAsked}, 5)...)...), model.Done, "18 passed, 0 failed, 0 not run, 5 not asked"},
		{"too few checks, all passed", answer(model.CheckPassed, model.CheckPassed), model.NotReady, "2 passed, 0 failed, 0 not run, 0 not asked"},
		{"one not run", answer(model.CheckPassed, model.CheckNotRun), model.NotReady, "1 passed, 0 failed, 1 not run, 0 not asked"},
		{"one failed", answer(model.CheckFailed, model.CheckNotRun), model.Findings, "0 passed, 1 failed, 1 not run, 0 not asked"},
	} {
		s := &session{answer: c.answer}
		run(t, s, "test", "plugins/lint-a/2.0.0")
		if len(s.asked) != 1 || s.asked[0] != "plugins/lint-a/2.0.0" {
			t.Errorf("%s: asked %v", c.name, s.asked)
		}
		if s.ending == nil || s.ending.Word != c.want || !strings.Contains(s.ending.What, c.in) {
			t.Errorf("%s: ending %+v, want %s saying %q", c.name, s.ending, c.want, c.in)
		}
		if len(s.ending.Lead) != 2 {
			t.Errorf("%s: the board has %d blocks, want a heading and the table", c.name, len(s.ending.Lead))
		}
	}
	js := &session{json: true, answer: answer(model.CheckFailed)}
	run(t, js, "test", "plugins/lint-a/2.0.0")
	if _, ok := js.doc.(model.PluginTest); !ok || js.ending.Word != model.Findings {
		t.Errorf("--json: doc %T, ending %+v, want the document and findings", js.doc, js.ending)
	}
	bad := &session{err: os.ErrNotExist}
	run(t, bad, "test", "nowhere")
	if bad.ending == nil || bad.ending.Word != model.Failed {
		t.Errorf("an unreadable bundle: ending %+v, want failed", bad.ending)
	}
}
