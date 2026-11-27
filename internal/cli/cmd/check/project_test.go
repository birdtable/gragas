package check_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// weaker is one finding of the project check, of the first kind.
func weaker() model.GateFinding {
	return model.GateFinding{Kind: model.GateToolConfig, Entity: "rule:go.errors.1", Msg: "examplelint holds errcheck weaker than the rule", Fix: "set errcheck to error"}
}

// inProject is a session below a pin, with what the project check answers.
func inProject(p model.ProjectCheck) inBook {
	s := newInBook()
	s.pinned = true
	s.project = p
	return s
}

// asked says which of the three checks a run reached.
func asked(s inBook) (project, rulebook, old bool) { return *s.projectAsked, *s.asked, *s.oldRan }

func TestCheckBelowAPinIsTheProjectCheck(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{})
	runCheck(t, s)
	if project, rulebook, old := asked(s); !project || rulebook || old {
		t.Errorf("reached project %t, rulebook %t, old %t; want only the project check", project, rulebook, old)
	}
}

// TestCheckInAFolderWithItsOwnRulebookIsTheGate: a pin above does not win
// over the folder's own rulebook.yaml.
func TestCheckInAFolderWithItsOwnRulebookIsTheGate(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{})
	s.here = true
	runCheck(t, s)
	if project, rulebook, old := asked(s); project || !rulebook || old {
		t.Errorf("reached project %t, rulebook %t, old %t; want only the rulebook's gate", project, rulebook, old)
	}
}

// TestCheckAtARootWithAPinAndABookBelowIsTheProjectCheck: the folder holds no
// rulebook.yaml of its own, so a book below (one, or two as gragas's fixtures
// make) never makes the answer of the rulebook's check the project's.
func TestCheckAtARootWithAPinAndABookBelowIsTheProjectCheck(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{"one book below": nil, "two books below": coreapi.ErrTwoBooks} {
		s := inProject(model.ProjectCheck{})
		s.err = err
		runCheck(t, s)
		if project, rulebook, old := asked(s); !project || rulebook || old {
			t.Errorf("%s: reached project %t, rulebook %t, old %t; want only the project check", name, project, rulebook, old)
		}
	}
}

func TestCheckWithDashCIsTheOldCheckEvenBelowAPin(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{})
	runCheck(t, s, "-C", "../other")
	if project, rulebook, old := asked(s); project || rulebook || !old {
		t.Errorf("reached project %t, rulebook %t, old %t; want only the old check", project, rulebook, old)
	}
}

// TestCheckWithNoPinAndOneBookBelowIsTheGate and its twin with none: branch c.
func TestCheckWithNoPinIsTheRulebooksOrTheOldCheck(t *testing.T) {
	t.Parallel()
	s := newInBook() // one book below: CheckRulebook answers
	runCheck(t, s)
	if project, rulebook, old := asked(s); project || !rulebook || old {
		t.Errorf("one book: reached project %t, rulebook %t, old %t; want only the rulebook's gate", project, rulebook, old)
	}
	s = newInBook()
	s.err = coreapi.ErrNoBook
	runCheck(t, s)
	if project, rulebook, old := asked(s); project || !rulebook || !old {
		t.Errorf("no book: reached project %t, rulebook %t, old %t; want the old check after the rulebook's was asked", project, rulebook, old)
	}
}

func TestCheckProjectWithAToolNotCheckedIsNotReady(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{NotChecked: []model.NotChecked{{Tool: "examplelint", Bindings: 36, Msg: "the pinned linter was not found"}}})
	runCheck(t, s, "--fail-on", "tool-config")
	e := *s.ended
	if e.Word != model.NotReady || !strings.Contains(e.What, "examplelint: 36 bindings not judged") || e.Next == "" {
		t.Errorf("ended %+v; want not ready naming the tool and its count, and the next line", e)
	}
}

func TestCheckProjectWithFindingsAndAToolNotChecked(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{Findings: []model.GateFinding{weaker()},
		NotChecked: []model.NotChecked{{Tool: "examplelint", Bindings: 2, Msg: "unread"}}})
	runCheck(t, s, "--fail-on", "tool-config")
	e := *s.ended
	if e.Word != model.NotReady || !strings.Contains(e.What, "1 finding") || !strings.Contains(e.What, "examplelint: 2 bindings") {
		t.Errorf("ended %+v; want not ready, with the findings counted and the tool named", e)
	}
	if text := strings.Join(strings.Fields(sprint(e.Lead)), " "); !strings.Contains(text, "Findings") || !strings.Contains(text, "Not checked") {
		t.Errorf("the listing holds %s; want the findings and the tools both", text)
	}
}

func TestCheckProjectWithARowAndNoToolBehindItIsDone(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{NotChecked: []model.NotChecked{{Msg: "names: no list named"}}})
	runCheck(t, s)
	if e := *s.ended; e.Word != model.Done {
		t.Errorf("ended %+v; want done: no tool is behind the row", e)
	}
}

// TestTheProjectEndingTable: the same findings with and without --fail-on, a
// kind that is not of the set, an empty value, and the pin that cannot be read.
func TestTheProjectEndingTable(t *testing.T) {
	t.Parallel()
	found := model.ProjectCheck{Findings: []model.GateFinding{weaker()}}
	notReady := errors.Join(coreapi.ErrNotReady, errors.New(".gragas/pin.yaml:1: the pin is refused"))
	for _, c := range []struct {
		name string
		p    model.ProjectCheck
		err  error
		args []string
		want model.Word
		asks bool // whether the project check was asked: a usage error asks nothing
	}{
		{"findings, no flag", found, nil, nil, model.Done, true},
		{"findings, flag names their kind", found, nil, []string{"--fail-on", "tool-config"}, model.Findings, true},
		{"findings, flag names another kind", found, nil, []string{"--fail-on", "exemption"}, model.Done, true},
		{"findings, two kinds with a comma", found, nil, []string{"--fail-on", "exemption,tool-config"}, model.Findings, true},
		{"a kind that is not of the set", found, nil, []string{"--fail-on", "style"}, model.Usage, false},
		{"an empty value", found, nil, []string{"--fail-on", ""}, model.Usage, false},
		{"the pin cannot be read", model.ProjectCheck{}, notReady, nil, model.NotReady, true},
		{"a clean run with the flag", model.ProjectCheck{}, nil, []string{"--fail-on", "tool-config"}, model.Done, true},
	} {
		s := inProject(c.p)
		s.projectErr = c.err
		runCheck(t, s, c.args...)
		if got := s.ended.Word; got != c.want || *s.projectAsked != c.asks {
			t.Errorf("%s: ended %q (project asked %t); want %q (asked %t)", c.name, got, *s.projectAsked, c.want, c.asks)
		}
	}
}

// TestTheProjectDocumentHoldsTheFindingsTheirKindsAndTheRowsNotChecked: under
// --json the run hands the root one document with the words of the closing
// word, the findings in the order given, and what was not checked. The root
// writes it; its text is compared where it is written (internal/cli/root).
func TestTheProjectDocumentHoldsTheFindingsTheirKindsAndTheRowsNotChecked(t *testing.T) {
	t.Parallel()
	p := model.ProjectCheck{
		Findings: []model.GateFinding{weaker(), {Kind: model.GateExemption, Entity: "gate/exemptions.yaml:4", Msg: "the row expired on 2026-10-01"}},
		Exempt:   []string{"arch.ports.24 is exempt until 2026-12-31"},
		NotChecked: []model.NotChecked{{Tool: "examplelint", Bindings: 36, Msg: "the pinned linter was not found"},
			{Msg: "names: no list named"}},
	}
	s := inProject(p)
	s.json = true
	runCheck(t, s, "--fail-on", "exemption")
	want := coreapi.ProjectDoc{Schema: coreapi.CheckSchema, Contract: model.ContractVersion, Command: "check", Word: "not ready",
		Findings: p.Findings, Exempt: p.Exempt, NotChecked: p.NotChecked}
	if got, ok := (*s.written).(coreapi.ProjectDoc); !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("the document is %+v; want %+v", *s.written, want)
	}
	if e := *s.ended; len(e.Lead) != 0 {
		t.Errorf("under --json the run drew %v", e.Lead)
	}
}

// TestTheProjectDocumentHoldsEmptyListsNeverNil: a clean run, whose lists the
// root writes as [] and never as null.
func TestTheProjectDocumentHoldsEmptyListsNeverNil(t *testing.T) {
	t.Parallel()
	s := inProject(model.ProjectCheck{})
	s.json = true
	runCheck(t, s)
	got, ok := (*s.written).(coreapi.ProjectDoc)
	if !ok || got.Findings == nil || got.Exempt == nil || got.NotChecked == nil || got.Word != "done" {
		t.Errorf("the document is %+v; want a done one with three empty lists", *s.written)
	}
}

// sprint is the listing as one string.
func sprint(blocks []model.Block) string { return fmt.Sprint(blocks) }
