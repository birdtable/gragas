package lifecycle_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func termFiles() model.PlanFiles {
	return model.PlanFiles{Terms: "rulebook/terms.yaml", History: "rulebook/history.jsonl"}
}

// termBook is the book of the rule tests with three terms: gate (a literal),
// wrap (used by two rules, once in a statement and once in the reasoning) and
// old (retired).
func termBook() model.Book {
	b := book()
	b.Terms = []model.Term{
		{Slug: "gate", Definition: "A check that stops a push.", Literal: "make check"},
		{Slug: "wrap", Definition: "Add what was being done to an error."},
		{Slug: "old", Definition: "A word that went.", Status: model.Retired},
	}
	b.Rules = append(b.Rules,
		model.Rule{ID: "go.errors.5", Statement: "Wrap an error with {wrap}.", Strength: model.Should, Status: model.Active, Origin: model.Authored},
		model.Rule{ID: "go.style.2", Statement: "Keep it short.", Strength: model.Should, Status: model.Proposed, Origin: model.Authored,
			Reasoning: &model.Reasoning{Pros: []string{"A reader sees where to {wrap}."}}},
		model.Rule{ID: "go.style.3", Statement: "Name a {gate} by its literal.", Strength: model.Should, Status: model.Active, Origin: model.Authored},
	)
	return b
}

// TestANewTermWithALiteralIsAddedAndRecorded: the entry, the record and the
// plan screen of a new term; the literal is what the names check lets a
// statement use through {slug}.
// Planted failure: the literal is dropped, or the record names no status.
func TestANewTermWithALiteralIsAddedAndRecorded(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanTermNew(termBook(), model.NewTerm{Slug: "header", Definition: "The name a request carries.", Literal: "X-Request-Id", Files: termFiles()}, moment())
	if err != nil {
		t.Fatal(err)
	}
	c := changeFor(t, p, "term:header", "")
	if c.Before != "" || c.After != "slug: header\ndefinition: The name a request carries.\nliteral: X-Request-Id\n" {
		t.Errorf("change = %+v", c)
	}
	if p.Record.Kind != "new" || p.Record.Entity != "term:header" || p.Record.Before != nil ||
		p.Record.After != (model.Lifecycle{Status: model.Active, Literal: "X-Request-Id"}) {
		t.Errorf("record = %+v", p.Record)
	}
	var screen strings.Builder
	for _, s := range p.Sections {
		screen.WriteString(s.Title + "\n" + strings.Join(s.Lines, "\n") + "\n")
	}
	for _, want := range []string{"new term header", "X-Request-Id", "rulebook/terms.yaml: adds term header", "rulebook/history.jsonl: appends one entry (new)"} {
		if !strings.Contains(screen.String(), want) {
			t.Errorf("the plan screen lacks %q:\n%s", want, screen.String())
		}
	}
}

func TestANewTermWithNoLiteralWritesNoLiteralLine(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanTermNew(termBook(), model.NewTerm{Slug: "header", Definition: "The name a request carries.", Files: termFiles()}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "term:header", ""); strings.Contains(c.After, "literal") {
		t.Errorf("after = %q", c.After)
	}
}

func TestANewTermTwiceIsNothingToDoAndAClashIsRefused(t *testing.T) {
	t.Parallel()
	same := model.NewTerm{Slug: "gate", Definition: "A check that stops a push.", Literal: "make check", Files: termFiles()}
	p, err := lifecycle.PlanTermNew(termBook(), same, moment())
	if err != nil || len(p.Delta.Changes) != 0 || p.Record.Kind != "new" || p.Record.Entity != "term:gate" {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	for name, tc := range map[string]struct {
		r         model.NewTerm
		hint, msg string
	}{
		"other words":  {model.NewTerm{Slug: "gate", Definition: "Something else.", Files: termFiles()}, "gragas term edit gate", "defined already"},
		"retired slug": {model.NewTerm{Slug: "old", Definition: "Back again.", Files: termFiles()}, "pick another slug", "retired"},
		"not a slug":   {model.NewTerm{Slug: "Two Words", Definition: "x", Files: termFiles()}, "lower-case words", "not a slug"},
		"no meaning":   {model.NewTerm{Slug: "fresh", Definition: " ", Files: termFiles()}, "--term-definition", "definition is empty"},
		"two lines":    {model.NewTerm{Slug: "fresh", Definition: "a\nb", Files: termFiles()}, "--term-definition", "more than one line"},
		"literal line": {model.NewTerm{Slug: "fresh", Definition: "x", Literal: "a\nb", Files: termFiles()}, "one line", "literal runs over"},
	} {
		_, err := lifecycle.PlanTermNew(termBook(), tc.r, moment())
		t.Run(name, func(t *testing.T) { t.Parallel(); refusal(t, err, tc.hint, tc.msg) })
	}
}

// TestATermEditChangesAddsAndTakesAwayTheLiteral: the lifecycle field of a
// term; the definition is prose and no plan rewrites it.
// Planted failure: the before value is not the book's, so the file is read
// wrong at render.
func TestATermEditChangesAddsAndTakesAwayTheLiteral(t *testing.T) {
	t.Parallel()
	text := func(s string) *string { return &s }
	for _, tc := range []struct {
		name, slug string
		literal    *string
		before     string
		after      string
		line       string
	}{
		{"change", "gate", text("gate run"), "make check", "gate run", "the literal goes from make check to gate run"},
		{"add", "wrap", text("errors.Wrap"), "", "errors.Wrap", "stands for errors.Wrap"},
		{"take away", "gate", text(""), "make check", "", "the literal make check is taken away"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := lifecycle.PlanTermEdit(termBook(), model.EditTerm{Slug: tc.slug, Literal: tc.literal, Files: termFiles()}, moment())
			if err != nil {
				t.Fatal(err)
			}
			if c := changeFor(t, p, "term:"+tc.slug, "literal"); c.Before != tc.before || c.After != tc.after {
				t.Errorf("change = %+v", c)
			}
			if p.Record.Kind != "edit" || p.Record.Before == nil || p.Record.Before.Literal != tc.before || p.Record.After.Literal != tc.after ||
				p.Record.After.Status != model.Active || p.Record.Before.Status != model.Active {
				t.Errorf("record = %+v", p.Record)
			}
			if got := p.Sections[0].Lines[0]; !strings.Contains(got, tc.line) {
				t.Errorf("changes line = %q, want it to say %q", got, tc.line)
			}
		})
	}
}

func TestATermEditToTheSameLiteralIsNothingToDoAndWhatItCannotPlanIsRefused(t *testing.T) {
	t.Parallel()
	text := func(s string) *string { return &s }
	p, err := lifecycle.PlanTermEdit(termBook(), model.EditTerm{Slug: "gate", Literal: text("make check"), Files: termFiles()}, moment())
	if err != nil || len(p.Delta.Changes) != 0 || p.Record.Kind != "edit" {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	for name, tc := range map[string]struct {
		r         model.EditTerm
		hint, msg string
	}{
		"unknown":   {model.EditTerm{Slug: "nope", Literal: text("x"), Files: termFiles()}, "gragas term new", "not in the book"},
		"retired":   {model.EditTerm{Slug: "old", Literal: text("x"), Files: termFiles()}, "define a new term", "is retired"},
		"no field":  {model.EditTerm{Slug: "gate", Files: termFiles()}, "--literal", "no field"},
		"two lines": {model.EditTerm{Slug: "gate", Literal: text("a\nb"), Files: termFiles()}, "one line", "runs over"},
	} {
		_, err := lifecycle.PlanTermEdit(termBook(), tc.r, moment())
		t.Run(name, func(t *testing.T) { t.Parallel(); refusal(t, err, tc.hint, tc.msg) })
	}
}

// TestRetiringAnUnusedTermSetsItsStatusAndKeepsTheTerm: nothing is deleted.
// Planted failure: the term is removed from the file, or the status is not
// guarded by a history record.
func TestRetiringAnUnusedTermSetsItsStatusAndKeepsTheTerm(t *testing.T) {
	t.Parallel()
	b := termBook()
	b.Terms = append(b.Terms, model.Term{Slug: "spare", Definition: "Nothing uses it.", Literal: "spare-part"})
	p, err := lifecycle.PlanTermRetire(b, model.RetireTerm{Slug: "spare", Why: newWhy(), Files: termFiles()}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "term:spare", "status"); c.Before != "" || c.After != "retired" {
		t.Errorf("change = %+v", c)
	}
	for _, c := range p.Delta.Changes {
		if c.Field == "" && strings.HasPrefix(c.Entity, "term:") {
			t.Errorf("a retirement adds or removes a term: %+v", c)
		}
	}
	changeFor(t, p, "decision:decide-retire-spare", "")
	want := lifecycle.Record{Kind: "retire", Entity: "term:spare", Decision: "decide-retire-spare",
		Before: &model.Lifecycle{Status: model.Active, Literal: "spare-part"}, After: model.Lifecycle{Status: model.Retired, Literal: "spare-part"}}
	if p.Record.Kind != want.Kind || p.Record.Entity != want.Entity || p.Record.Decision != want.Decision || p.Record.After != want.After || *p.Record.Before != *want.Before {
		t.Errorf("record = %+v, want %+v", p.Record, want)
	}
}

func TestRetiringATermARuleUsesIsRefusedAndNamesTheRules(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.PlanTermRetire(termBook(), model.RetireTerm{Slug: "wrap", Why: newWhy(), Files: termFiles()}, moment())
	refusal(t, err, "gragas rule replace", "go.errors.5, go.style.2")
	if !strings.Contains(err.Error(), "wrap is used by 2 rule(s)") {
		t.Errorf("err = %v", err)
	}
	_, err = lifecycle.PlanTermRetire(termBook(), model.RetireTerm{Slug: "gate", Why: newWhy(), Files: termFiles()}, moment())
	refusal(t, err, "gragas rule replace", "go.style.3")
}

// TestRetiringATermIsRefusedWhereverARuleWritesIt: a rule that writes {spare}
// in its statement, its reasoning's definition, a pro or a con uses the term,
// and retiring spare is refused and names that rule. Each row is a book whose
// one rule writes the term in that place alone.
// Planted failure: rulesUsing reading no longer one of the four places; the
// row of that place then plans the retirement.
func TestRetiringATermIsRefusedWhereverARuleWritesIt(t *testing.T) {
	t.Parallel()
	const uses = "Keep {spare} out of it."
	for name, r := range map[string]model.Rule{
		"statement":  {Statement: uses},
		"definition": {Statement: "Keep it short.", Reasoning: &model.Reasoning{Definition: uses}},
		"pros":       {Statement: "Keep it short.", Reasoning: &model.Reasoning{Pros: []string{"A pro.", uses}}},
		"cons":       {Statement: "Keep it short.", Reasoning: &model.Reasoning{Cons: []string{"A con.", uses}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := termBook()
			b.Terms = append(b.Terms, model.Term{Slug: "spare", Definition: "A word one rule writes."})
			r.ID, r.Strength, r.Status, r.Origin = "go.style.9", model.Should, model.Active, model.Authored
			b.Rules = append(b.Rules, r)
			_, err := lifecycle.PlanTermRetire(b, model.RetireTerm{Slug: "spare", Why: newWhy(), Files: termFiles()}, moment())
			refusal(t, err, "gragas rule replace", "go.style.9")
		})
	}
}

func TestRetiringATermTwiceIsNothingToDoAndAnUnknownTermIsRefused(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanTermRetire(termBook(), model.RetireTerm{Slug: "old", Why: newWhy(), Files: termFiles()}, moment())
	if err != nil || len(p.Delta.Changes) != 0 || p.Record.Kind != "retire" || p.Record.After.Status != model.Retired {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	_, err = lifecycle.PlanTermRetire(termBook(), model.RetireTerm{Slug: "nope", Why: newWhy(), Files: termFiles()}, moment())
	refusal(t, err, "terms.yaml", "not in the book")
}

// TestRetiringATermWithNoDecisionOrAWrongOneIsRefused: retire is the step that
// takes a word out of use, so the history says why; new and edit stand on no
// decision.
// Planted failure: the plan is made with no decision, or with one the book
// does not hold.
func TestRetiringATermWithNoDecisionOrAWrongOneIsRefused(t *testing.T) {
	t.Parallel()
	b := termBook()
	b.Terms = append(b.Terms, model.Term{Slug: "spare", Definition: "Nothing uses it."})
	for name, tc := range map[string]struct {
		why       model.Why
		hint, msg string
	}{
		"none":        {model.Why{}, "--decision <slug>", "stands on a decision, and none is given"},
		"not in book": {model.Why{Cite: "nope"}, "gragas decision list", "not in the book"},
		"both":        {model.Why{Cite: "taken", New: &model.NewDecision{Title: "t", Text: "x"}}, "--decision-text", "both cited and new"},
		"taken slug":  {model.Why{New: &model.NewDecision{Slug: "taken", Title: "t", Text: "x"}}, "--decision-slug", "is taken"},
		"empty title": {model.Why{New: &model.NewDecision{Text: "x"}}, "title and a text", "no title"},
	} {
		_, err := lifecycle.PlanTermRetire(b, model.RetireTerm{Slug: "spare", Why: tc.why, Files: termFiles()}, moment())
		t.Run(name, func(t *testing.T) { t.Parallel(); refusal(t, err, tc.hint, tc.msg) })
	}
	cited, err := lifecycle.PlanTermRetire(b, model.RetireTerm{Slug: "spare", Why: model.Why{Cite: "taken"}, Files: termFiles()}, moment())
	if err != nil || cited.Record.Decision != "taken" {
		t.Fatalf("a cited decision: %+v, %v", cited.Record, err)
	}
	for _, c := range cited.Delta.Changes {
		if strings.HasPrefix(c.Entity, "decision:") {
			t.Errorf("a cited decision is added again: %+v", c)
		}
	}
}
