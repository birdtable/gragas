package root

import (
	"context"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestTheFormOfEachRuleWordAsksEachQuestionBesideTheFlagThatAnswersIt walks
// the form of every rule word at a terminal: every question is put in the
// order of the form, with the flag that answers it in a script beside it
// (cli.environment.14), and a flag the command lacks would end the run.
// Risk: a question that names a flag the command does not take, or one that
// is skipped.
func TestTheFormOfEachRuleWordAsksEachQuestionBesideTheFlagThatAnswersIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		answers []string
		flags   []string
	}{
		{"new", []string{"rule", "new"}, []string{"go.errors", "Return errors.", "must", "own", "Panics", "Panics hide failures."},
			[]string{"--concern", "--statement", "--strength", "--source", "--decision-title", "--decision-text"}},
		{"replace", []string{"rule", "replace", "go.errors.1"}, []string{"Wrap with the call.", "must", "Replace it", "It says more."},
			[]string{"--statement", "--strength", "--decision-title", "--decision-text"}},
		{"retire", []string{"rule", "retire", "go.errors.1"}, []string{"Retire it", "It no longer holds."},
			[]string{"--decision-title", "--decision-text"}},
		{"restore", []string{"rule", "restore", "go.errors.1"}, []string{"2027-01-31", "Back", "It holds again."},
			[]string{"--review-by", "--decision-title", "--decision-text"}},
		{"term retire", []string{"term", "retire", "spare"}, []string{"Retire it", "Nothing uses it."},
			[]string{"--decision-title", "--decision-text"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, _ := process(t, world(t), tc.args...)
			p.Config = config.Config{StateDir: t.TempDir()}
			p, sc := attach(t, p)
			sc.out.SeedTerminal(porttest.Term{Asking: true, Answers: tc.answers})
			sc.out.Seed(".", map[string]string{"x": "y"})
			p.World = sc.out.Ports()
			p.History = func(string) (port.History, error) { return nil, ErrNoGit }
			Execute(context.Background(), p)
			if len(sc.out.Asked) < len(tc.flags) {
				t.Fatalf("asked %d questions, want %d: %q", len(sc.out.Asked), len(tc.flags), sc.out.Asked)
			}
			for i, flag := range tc.flags {
				if want := "(" + flag + " answers it in a script)"; !strings.Contains(sc.out.Asked[i], want) {
					t.Errorf("question %d = %q, want it to name %s", i+1, sc.out.Asked[i], flag)
				}
			}
		})
	}
}

// TestARuleWordOffATerminalWithoutAFlagEndsUsageNamingIt: nothing is asked of
// nobody; the flag that gives the answer is named.
func TestARuleWordOffATerminalWithoutAFlagEndsUsageNamingIt(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "rule", "retire", "go.errors.1")
	p.History = func(string) (port.History, error) { return nil, ErrNoGit }
	p, sc := attach(t, p)
	code := Execute(context.Background(), p)
	if code != exit.Usage || !strings.Contains(sc.output(), "--decision-title") {
		t.Errorf("exit %d, said %q; want usage naming --decision-title", code, sc.output())
	}
	if len(sc.out.Asked) > 0 {
		t.Errorf("a question was put to nobody: %q", sc.out.Asked)
	}
}

// TestARuleWordWhenTheSettingsNameNoGitEndsNotReadyNamingTheKey: no git means
// no name for the history line; the sentence names programs.git (exit 4).
func TestARuleWordWhenTheSettingsNameNoGitEndsNotReadyNamingTheKey(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "rule", "retire", "go.errors.1", "--decision", "taken")
	p, sc := attach(t, p)
	sc.out.Seed(".", map[string]string{"x": "y"})
	p.World = sc.out.Ports()
	p.History = func(string) (port.History, error) { return nil, ErrNoGit }
	code := Execute(context.Background(), p)
	if code != exit.NotReady || !strings.Contains(sc.output(), "programs.git") {
		t.Errorf("exit %d, said %q, diag %q; want not ready naming programs.git", code, sc.output(), sc.diag.Said)
	}
}
