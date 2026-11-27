package lifecycle_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// lent is a parent's rule go.errors.1 with one binding, lint-a config
// wrap-errors at warning, as an overlay borrows it.
func lent(edit ...func(*lifecycle.Lent)) lifecycle.Lent {
	l := lifecycle.Lent{From: "the parent base 1.2.0", Rule: model.Rule{
		ID: "go.errors.1", Status: model.Active, Origin: model.Authored,
		Bindings: []model.Binding{lintBinding(model.SeverityWarning)},
	}}
	for _, e := range edit {
		e(&l)
	}
	return l
}

func borrowedFiles() model.PlanFiles {
	f := files()
	f.Bindings = "book/bindings"
	return f
}

// held is a Compare that holds the entry, the rung and the run point, and
// ranks the severity alone, as the guard it stands for: link's Tighten,
// which the operations hand in and link's tests hold field by field.
func held(was, next model.Binding) (bool, string, string) {
	rank := map[model.Severity]int{model.SeverityInfo: 0, model.SeverityWarning: 1, model.SeverityError: 2}
	switch {
	case was.Tool != next.Tool || was.Target != next.Target || was.Key != next.Key:
		return false, "entry", "the entry would move"
	case was.Rung != next.Rung:
		return false, "rung", "the rung would change"
	case was.RunPoint != next.RunPoint:
		return false, "run_point", "the run point would change"
	case was.Remedy != next.Remedy:
		return false, "remedy", "the remedy cannot be compared"
	case rank[next.Severity] < rank[was.Severity]:
		return false, "severity", "the severity would be weaker"
	}
	return reflect.DeepEqual(was, next), "", ""
}

// TestABindOfABorrowedRuleWritesARecordUnderTheBorrowersPrefix. Risk: a
// tightening written into the parent's rule, or under the parent's id.
func TestABindOfABorrowedRuleWritesARecordUnderTheBorrowersPrefix(t *testing.T) {
	t.Parallel()
	b := book()
	b.BindingRecords = []model.Opaque{{Path: "bindings/go-errors-1.yaml"}}
	req := model.BindRule{ID: "go.errors.1", Binding: lintBinding(model.SeverityError), Files: borrowedFiles()}
	req.Binding.Options = map[string]any{"ignore": []any{"fmt"}}
	p, err := lifecycle.PlanBindBorrowed(b, lent(), req, held, moment())
	if err != nil {
		t.Fatal(err)
	}
	want := "subject: go.errors.1\ntool: lint-a\ntarget: config\nkey: wrap-errors\nvalue: true\noptions:\n  ignore: [fmt]\nseverity: error\nrung: 1\nrun_point: commit\n"
	if len(p.Delta.Changes) != 1 {
		t.Fatalf("changes = %+v", p.Delta.Changes)
	}
	c := p.Delta.Changes[0]
	if c.Entity != "record:bindings/go-errors-1-2.yaml" || c.Before != "" || c.After != want {
		t.Errorf("change = %+v, want a new record beside the one there, holding\n%s", c, want)
	}
	if p.Record.Kind != "bind" || p.Record.Entity != "rule:go.errors.1" || p.Record.Decision != "" {
		t.Errorf("record = %+v", p.Record)
	}
	all := fmt.Sprint(p.Sections)
	for _, w := range []string{"which the parent base 1.2.0 lends", "book/bindings/go-errors-1-2.yaml: writes a record of go.errors.1, under this rulebook's prefix", "stands on no decision"} {
		if !strings.Contains(all, w) {
			t.Errorf("sections %s do not say %q", all, w)
		}
	}
	// a binding at an entry the rule holds none at adds one, which tightens
	other := lintBinding(model.SeverityInfo)
	other.Key = "wrap-other"
	if _, err := lifecycle.PlanBindBorrowed(book(), lent(), model.BindRule{ID: "go.errors.1", Binding: other, Files: borrowedFiles()}, held, moment()); err != nil {
		t.Errorf("a binding at a new entry: %v", err)
	}
	// the binding the rule holds already plans nothing
	same, err := lifecycle.PlanBindBorrowed(book(), lent(), model.BindRule{ID: "go.errors.1", Binding: lintBinding(model.SeverityWarning), Files: borrowedFiles()}, held, moment())
	if err != nil || len(same.Delta.Changes) != 0 {
		t.Errorf("the same binding: %+v, %v", same.Delta, err)
	}
}

// TestARecordTheBorrowerHoldsIsWhatTheNextChangeIsHeldTo. Risk: a second
// bind compared with the lender's binding, so it undoes the borrower's own
// tightening.
func TestARecordTheBorrowerHoldsIsWhatTheNextChangeIsHeldTo(t *testing.T) {
	t.Parallel()
	const text = "subject: go.errors.1\n..."
	l := lent(func(l *lifecycle.Lent) {
		l.Records = []model.Tightening{{Path: "bindings/go-errors-1.yaml", Subject: "go.errors.1", Binding: lintBinding(model.SeverityError), Text: text}}
	})
	_, err := lifecycle.PlanBindBorrowed(book(), l, model.BindRule{ID: "go.errors.1", Binding: lintBinding(model.SeverityWarning), Files: borrowedFiles()}, held, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "refused at its severity") {
		t.Errorf("back to the lender's severity: %q, want it refused at the severity", r.Msg)
	}
	keep := model.SeverityError
	remedy := "wrap it"
	_, err = lifecycle.PlanRebindBorrowed(book(), l, model.RebindRule{ID: "go.errors.1", Tool: "lint-a", Target: "config", Key: "wrap-errors", Severity: &keep, Remedy: &remedy, Files: borrowedFiles()}, held, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "refused at its remedy") {
		t.Errorf("a remedy change: %v", r.Msg)
	}
	// the record held as it stands plans nothing, and the record's text is
	// what a change of it is written against
	p, err := lifecycle.PlanBindBorrowed(book(), l, model.BindRule{ID: "go.errors.1", Binding: lintBinding(model.SeverityError), Files: borrowedFiles()}, held, moment())
	if err != nil || len(p.Delta.Changes) != 0 {
		t.Errorf("the record as it stands: %+v, %v", p.Delta, err)
	}
}

// TestEachWayOfWeakeningABorrowedRuleIsRefusedNamingTheField. Risk: a
// borrower that loosens, moves or unbinds a rule it was lent.
func TestEachWayOfWeakeningABorrowedRuleIsRefusedNamingTheField(t *testing.T) {
	t.Parallel()
	str := func(s string) *string { return &s }
	tt := func(s model.TargetType) *model.TargetType { return &s }
	sev := func(s model.Severity) *model.Severity { return &s }
	rebind := func(edit func(*model.RebindRule)) model.RebindRule {
		r := model.RebindRule{ID: "go.errors.1", Tool: "lint-a", Target: "config", Key: "wrap-errors", Files: borrowedFiles()}
		edit(&r)
		return r
	}
	bind := func(edit func(*model.Binding)) model.BindRule {
		b := lintBinding(model.SeverityWarning)
		edit(&b)
		return model.BindRule{ID: "go.errors.1", Binding: b, Files: borrowedFiles()}
	}
	for name, c := range map[string]struct {
		plan       func(lifecycle.Lent) error
		field      string
		hint       string
		importHint string
	}{
		"a weaker severity": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanRebindBorrowed(book(), l, rebind(func(r *model.RebindRule) { r.Severity = sev(model.SeverityInfo) }), held, moment())
			return err
		}, "severity", "gragas rule exclude go.errors.1", "import without the part"},
		"a move to another tool": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanRebindBorrowed(book(), l, rebind(func(r *model.RebindRule) { r.ToTool = str("lint-b") }), held, moment())
			return err
		}, "entry", "at the other entry", "at the other entry"},
		"a move to another target": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanRebindBorrowed(book(), l, rebind(func(r *model.RebindRule) { r.ToTarget = tt(model.TargetSource) }), held, moment())
			return err
		}, "entry", "at the other entry", "at the other entry"},
		"a rung change": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanBindBorrowed(book(), l, bind(func(b *model.Binding) { b.Rung = 3 }), held, moment())
			return err
		}, "rung", "keeps its rung and run point", "keeps its rung and run point"},
		"a run point change": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanBindBorrowed(book(), l, bind(func(b *model.Binding) { b.RunPoint = model.RunPush }), held, moment())
			return err
		}, "run point", "keeps its rung and run point", "keeps its rung and run point"},
		"a change that cannot be compared": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanRebindBorrowed(book(), l, rebind(func(r *model.RebindRule) { r.Remedy = str("ignore it") }), held, moment())
			return err
		}, "remedy", "gragas rule exclude", "import without the part"},
		"an unbind": {func(l lifecycle.Lent) error {
			_, err := lifecycle.PlanUnbindBorrowed(l, model.UnbindRule{ID: "go.errors.1", Tool: "lint-a", Target: "config", Key: "wrap-errors"})
			return err
		}, "unbinding it at lint-a config wrap-errors would weaken it", "gragas rule exclude", "import without the part"},
	} {
		for _, imported := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, imported %v", name, imported), func(t *testing.T) {
				t.Parallel()
				l := lent(func(l *lifecycle.Lent) {
					if imported {
						l.Imported, l.From = true, "the import other 0.3.0"
					}
				})
				r := refusalOf(t, c.plan(l))
				if r.Word != "" {
					t.Errorf("word = %q, want refused", r.Word)
				}
				if !strings.Contains(r.Msg, c.field) || !strings.Contains(r.Msg, l.From) {
					t.Errorf("msg = %q, want it to name %q and the lender", r.Msg, c.field)
				}
				hint := c.hint
				if imported {
					hint = c.importHint
				}
				if !strings.Contains(r.Hint, hint) {
					t.Errorf("hint = %q, want %q", r.Hint, hint)
				}
			})
		}
	}
}

// TestABorrowedRuleTheLenderRetiredTakesNoBinding, and a bad field is named
// as for a rule of the rulebook's own.
func TestABorrowedRuleTheLenderRetiredTakesNoBinding(t *testing.T) {
	t.Parallel()
	retired := lent(func(l *lifecycle.Lent) { l.Rule.Status = model.Retired })
	_, err := lifecycle.PlanBindBorrowed(book(), retired, model.BindRule{ID: "go.errors.1", Binding: lintBinding(model.SeverityError), Files: borrowedFiles()}, held, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "retired in the parent base 1.2.0") {
		t.Errorf("msg = %q", r.Msg)
	}
	bad := lintBinding(model.SeverityError)
	bad.Options = map[string]any{"a\nb": 1}
	_, err = lifecycle.PlanBindBorrowed(book(), lent(), model.BindRule{ID: "go.errors.1", Binding: bad, Files: borrowedFiles()}, held, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "is not a word") {
		t.Errorf("an option's name with a newline: %q", r.Msg)
	}
}

// TestAWeakRecordIsNoGroundToWeakenTheLender. Risk: a stale or weak record
// the borrower holds taken for what the rule is held to, so a change that
// tightens the record writes a binding weaker than the lender's.
func TestAWeakRecordIsNoGroundToWeakenTheLender(t *testing.T) {
	t.Parallel()
	l := lent(func(l *lifecycle.Lent) {
		l.Rule.Bindings = []model.Binding{lintBinding(model.SeverityError)}
		l.Records = []model.Tightening{{Path: "bindings/go-errors-1.yaml", Subject: "go.errors.1", Binding: lintBinding(model.SeverityInfo), Text: "subject: go.errors.1\n..."}}
	})
	warn, errSev := model.SeverityWarning, model.SeverityError
	rebind := func(s *model.Severity) (lifecycle.Plan, error) {
		return lifecycle.PlanRebindBorrowed(book(), l, model.RebindRule{ID: "go.errors.1", Tool: "lint-a", Target: "config", Key: "wrap-errors", Severity: s, Files: borrowedFiles()}, held, moment())
	}
	_, err := rebind(&warn)
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "refused at its severity") {
		t.Errorf("a rebind to warning over a record at info, the lender at error: %q", r.Msg)
	}
	_, err = lifecycle.PlanBindBorrowed(book(), l, model.BindRule{ID: "go.errors.1", Binding: lintBinding(model.SeverityWarning), Files: borrowedFiles()}, held, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "refused at its severity") {
		t.Errorf("a bind at warning over a record at info, the lender at error: %q", r.Msg)
	}
	p, err := rebind(&errSev)
	if err != nil || len(p.Delta.Changes) == 0 {
		t.Errorf("a rebind back to exactly the lender's binding: %+v, %v", p.Delta, err)
	}
}
