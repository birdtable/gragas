package lifecycle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// twoCites is the fixture book with go.errors.1 citing two sources, one with a
// place and words whose texts need quoting, and go.style.1 citing the first.
func twoCites() model.Book {
	b := book()
	b.Sources = append(b.Sources, model.Source{Slug: "go-faq", Kind: model.SourceGuide, Title: "Go FAQ", Locator: "https://go.dev/doc/faq"})
	b.Rules[0].Sources = []model.Citation{
		{Ref: "effective-go", Backs: model.BacksRule},
		{Ref: "go-faq", Where: "Why: errors", Says: "it's \"values\"", Backs: model.BacksCheck},
	}
	b.Rules[2].Sources = []model.Citation{{Ref: "effective-go", Backs: model.BacksRule}}
	return b
}

func lines(p lifecycle.Plan) string {
	var out []string
	for _, s := range p.Sections {
		out = append(out, s.Lines...)
	}
	return strings.Join(out, "\n")
}

// TestAnUnciteOfOneOfTwoCitesPlansOneRemovalAndAnEditEntry: uncite on a rule
// with two cites plans one change that takes the citation out, the record as
// its before and nothing after, and an edit entry; the record's texts are
// quoted as the file holds them; the source no rule cites now is named and
// stays in the book.
// Planted failure: the record is the after, so format adds it again; or the
// where and says are left out, so format finds no record to take out.
func TestAnUnciteOfOneOfTwoCitesPlansOneRemovalAndAnEditEntry(t *testing.T) {
	t.Parallel()
	r := editReq("go.errors.1")
	r.Uncite = []string{"go-faq", "go-faq"}
	p, err := lifecycle.PlanRuleEdit(twoCites(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Delta.Changes) != 1 {
		t.Fatalf("changes = %+v; want one removal for the source asked for twice", p.Delta.Changes)
	}
	want := "ref: go-faq\nwhere: \"Why: errors\"\nsays: \"it's \\\"values\\\"\"\nbacks: check"
	if c := p.Delta.Changes[0]; c.Entity != "rule:go.errors.1" || c.Field != "sources{}" || c.Before != want || c.After != "" {
		t.Errorf("change = %+v; want the record %q as the before, and no after", c, want)
	}
	if p.Record.Kind != "edit" || p.Record.Entity != "rule:go.errors.1" {
		t.Errorf("entry = %+v; want an edit entry of the rule", p.Record)
	}
	got := lines(p)
	for _, w := range []string{"sources of go.errors.1 loses go-faq, which backs the check", "no rule cites go-faq now; it stays in the sources of the book"} {
		if !strings.Contains(got, w) {
			t.Errorf("the plan lacks %q:\n%s", w, got)
		}
	}
}

// TestAnUnciteOfASourceAnotherRuleCitesDoesNotSayNoRuleCitesIt: the line for
// a source no rule cites is written only when no other rule does.
func TestAnUnciteOfASourceAnotherRuleCitesDoesNotSayNoRuleCitesIt(t *testing.T) {
	t.Parallel()
	r := editReq("go.errors.1")
	r.Uncite = []string{"effective-go"}
	p, err := lifecycle.PlanRuleEdit(twoCites(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.errors.1", "sources{}"); c.Before != "ref: effective-go\nbacks: rule" {
		t.Errorf("change = %+v", c)
	}
	if strings.Contains(lines(p), "no rule cites") {
		t.Errorf("go.style.1 still cites effective-go, and the plan says none does:\n%s", lines(p))
	}
}

// TestTheLastCiteOfARulePastProposedIsRefusedNamingTheRule: an uncite that
// would leave an active or a trial rule with no source is refused, naming the
// rule and its status, with a hint to cite another first; with a cite of
// another in the same edit it is planned; a proposed rule may lose its last.
// Planted failure: the last-cite check in checkUncite is skipped, and the
// active and trial cases plan the removal (the guards table in the PR body
// has the red line).
func TestTheLastCiteOfARulePastProposedIsRefusedNamingTheRule(t *testing.T) {
	t.Parallel()
	for _, st := range []model.Status{model.Active, model.Trial} {
		b := twoCites()
		b.Rules[0].Status = st
		r := editReq("go.errors.1")
		r.Uncite = []string{"effective-go", "go-faq"}
		_, err := lifecycle.PlanRuleEdit(b, r, moment())
		var ref *lifecycle.Refusal
		if !errors.As(err, &ref) || !strings.Contains(ref.Msg, "go.errors.1 is "+string(st)) || !strings.Contains(ref.Msg, "last source") ||
			!strings.Contains(ref.Hint, "--cite") || !strings.Contains(ref.Hint, "proposed") {
			t.Errorf("%s: err = %v; want a refusal naming the rule, its status and its last source, with a hint", st, err)
		}
		r.Cite = []string{"effective-go"}
		r.Uncite = []string{"go-faq"}
		b.Rules[0].Sources = b.Rules[0].Sources[1:]
		if _, err := lifecycle.PlanRuleEdit(b, r, moment()); err != nil {
			t.Errorf("%s: a cite of another in the same edit: err = %v; want a plan", st, err)
		}
	}
	b := twoCites()
	b.Rules[0].Status = model.Proposed
	r := editReq("go.errors.1")
	r.Uncite = []string{"effective-go", "go-faq"}
	if p, err := lifecycle.PlanRuleEdit(b, r, moment()); err != nil || len(p.Delta.Changes) != 2 {
		t.Errorf("a proposed rule: changes = %+v, err = %v; want both taken out", p.Delta.Changes, err)
	}
}

// TestAnUnciteTheBookOrTheRuleCannotMeanIsRefused: a source the book lacks and
// the rule does not cite is refused naming both; one passed to --cite and to
// --uncite at once is refused; a source the book holds and the rule does not
// cite plans nothing; a dangling citation of a source the book lacks may still
// be taken out.
func TestAnUnciteTheBookOrTheRuleCannotMeanIsRefused(t *testing.T) {
	t.Parallel()
	var ref *lifecycle.Refusal
	r := editReq("go.errors.1")
	r.Uncite = []string{"missing"}
	if _, err := lifecycle.PlanRuleEdit(twoCites(), r, moment()); !errors.As(err, &ref) || !strings.Contains(ref.Msg, "missing") || !strings.Contains(ref.Msg, "go.errors.1") {
		t.Errorf("a source neither holds: err = %v", err)
	}
	r.Uncite, r.Cite = []string{"go-faq"}, []string{"go-faq"}
	if _, err := lifecycle.PlanRuleEdit(twoCites(), r, moment()); !errors.As(err, &ref) || !strings.Contains(ref.Hint, "not both") {
		t.Errorf("cite and uncite at once: err = %v", err)
	}
	r = editReq("go.style.1")
	r.Uncite = []string{"go-faq"}
	if p, err := lifecycle.PlanRuleEdit(twoCites(), r, moment()); err != nil || len(p.Delta.Changes) != 0 {
		t.Errorf("a source the rule does not cite: changes = %+v, err = %v; want nothing to do", p.Delta.Changes, err)
	}
	b := twoCites()
	b.Rules[0].Sources = append(b.Rules[0].Sources, model.Citation{Ref: "gone", Backs: model.BacksRule})
	r = editReq("go.errors.1")
	r.Uncite = []string{"gone"}
	if p, err := lifecycle.PlanRuleEdit(b, r, moment()); err != nil || len(p.Delta.Changes) != 1 {
		t.Errorf("a dangling citation: changes = %+v, err = %v; want it taken out", p.Delta.Changes, err)
	}
}
