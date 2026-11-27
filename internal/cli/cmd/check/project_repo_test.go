package check_test

import (
	"reflect"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// The three repository checks in the command: their findings in
// the --json document, and the closing word under --fail-on.

// repoFound is one finding of each of the three repository checks, after one
// of the project's own.
func repoFound() []model.GateFinding {
	return []model.GateFinding{weaker(),
		{Kind: model.GateNames, Entity: "z.md:1", File: "z.md", Line: 1, Msg: "a name of the list (go.names.1, gragas names)", Fix: "take the name out"},
		{Kind: model.GateTrackedFiles, Entity: "B.md", File: "B.md", Msg: "the path differs from b.md only by case (go.files.1, gragas fold)", Fix: "rename one of the two"},
		{Kind: model.GateCounts, Entity: "a.go:3", File: "a.go", Line: 3, Msg: "a directive the base does not have (go.debt.1, gragas directive)", Fix: "take the directive out"}}
}

// TestTheProjectDocumentHoldsTheThreeRepositoryChecksAsFindings: under --json
// a finding of the names, the tracked files or the counts is an entry of the
// one findings list, a model.GateFinding as the project's own are, in the
// order the check gave them.
// Planted failure: the document keeps only the kinds of #1726.
func TestTheProjectDocumentHoldsTheThreeRepositoryChecksAsFindings(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{Findings: repoFound()})
	s.json = true
	runCheck(t, s)
	got, ok := (*s.written).(coreapi.ProjectDoc)
	if !ok || got.Word != "done" || !reflect.DeepEqual(got.Findings, repoFound()) {
		t.Errorf("the document is %+v; want a done one whose findings are the four", *s.written)
	}
}

// TestFailOnEndsTheRunOnEachOfTheThreeRepositoryKinds: with findings listed
// the run ends done, and findings when --fail-on names the kind of one of
// them; a kind none of them has leaves it done.
// Planted failure: --fail-on refuses the three kinds, or passes them by.
func TestFailOnEndsTheRunOnEachOfTheThreeRepositoryKinds(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, failOn string
		want         model.Word
	}{
		{"no flag ends done", "", model.Done},
		{"names ends findings", "names", model.Findings},
		{"tracked-files ends findings", "tracked-files", model.Findings},
		{"counts ends findings", "counts", model.Findings},
		{"one kind of two ends findings", "exemption,counts", model.Findings},
		{"a kind with no finding ends done", "exemption", model.Done},
	} {
		s := inProject(model.ProjectCheck{Findings: repoFound()[1:]})
		var args []string
		if c.failOn != "" {
			args = []string{"--fail-on", c.failOn}
		}
		runCheck(t, s, args...)
		e := *s.ended
		if e.Word != c.want {
			t.Errorf("%s: ended %q, want %q", c.name, e.Word, c.want)
		}
		if text := sprint(e.Lead); !strings.Contains(text, "z.md:1") || !strings.Contains(text, "a.go:3") {
			t.Errorf("%s: the listing %s lacks the findings", c.name, text)
		}
	}
}

// TestARepositoryCheckNotMadeEndsTheRunNotReady: a row with the check behind
// it, as a refused names list or a build with no git gives, ends the run not
// ready whatever --fail-on names, and the listing says the check and its
// reason.
// Planted failure: a check that was not made ends done.
func TestARepositoryCheckNotMadeEndsTheRunNotReady(t *testing.T) {
	t.Parallel()
	row := model.NotChecked{Tool: "gragas names", Bindings: 1, Msg: "the names list was refused"}
	s := inProject(model.ProjectCheck{Findings: repoFound()[2:], NotChecked: []model.NotChecked{row}})
	runCheck(t, s, "--fail-on", "counts")
	e := *s.ended
	if e.Word != model.NotReady || !strings.Contains(e.What, "gragas names") {
		t.Errorf("ended %+v; want not ready naming the check", e)
	}
	if text := sprint(e.Lead); !strings.Contains(text, "the names list was refused") {
		t.Errorf("the listing %s lacks the row's reason", text)
	}
}
