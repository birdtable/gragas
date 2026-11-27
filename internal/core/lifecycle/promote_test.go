package lifecycle_test

import (
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// ready is a book with one rule, go.errors.9, of the strength and status asked
// for, with a source and an example pair, and a governance of a three-release
// trial window and a ninety-day review.
func ready(s model.Strength, st model.Status, edit ...func(*model.Rule)) model.Book {
	b := book()
	r := model.Rule{ID: "go.errors.9", Statement: "z", Strength: s, Status: st, Origin: model.Authored,
		Sources: []model.Citation{{Ref: "effective-go", Backs: model.BacksRule}}, Pairs: []model.Pair{{Number: 1}}}
	for _, e := range edit {
		e(&r)
	}
	b.Rules = append(b.Rules, r)
	b.Identity.Governance = model.Governance{DecisionRequired: true, TrialReleases: 3, ReviewDays: 90}
	return b
}

func promote(releases int) model.PromoteRule {
	return model.PromoteRule{ID: "go.errors.9", Why: model.Why{Cite: "taken"}, Releases: releases, Files: files()}
}

// TestPromoteMovesEveryStrengthAndStatusOneStep: a table of the three
// strengths and the four statuses, each row the answer of S24, and the test
// says it ran every row. Risk: a must that skips its trial, a may that waits
// for one, a retired rule that comes back by promote.
func TestPromoteMovesEveryStrengthAndStatusOneStep(t *testing.T) {
	t.Parallel()
	type row struct {
		s        model.Strength
		st       model.Status
		releases int
		want     model.Status // "" is nothing to do; "refused" is a refusal
	}
	rows := []row{
		{model.Must, model.Proposed, 0, model.Trial}, {model.Must, model.Trial, 1, model.Active}, {model.Must, model.Active, 1, ""}, {model.Must, model.Retired, 1, "refused"},
		{model.Should, model.Proposed, 0, model.Trial}, {model.Should, model.Trial, 1, model.Active}, {model.Should, model.Active, 1, ""}, {model.Should, model.Retired, 1, "refused"},
		{model.May, model.Proposed, 0, model.Active}, {model.May, model.Trial, 0, model.Active}, {model.May, model.Active, 0, ""}, {model.May, model.Retired, 0, "refused"},
	}
	if len(rows) != len(model.Strengths())*len(model.Statuses()) {
		t.Fatalf("the table has %d rows, want every strength and status: %d", len(rows), len(model.Strengths())*len(model.Statuses()))
	}
	var ran atomic.Int32
	t.Cleanup(func() {
		if int(ran.Load()) != len(rows) {
			t.Errorf("ran %d of %d rows", ran.Load(), len(rows))
		}
	})
	for _, r := range rows {
		t.Run(string(r.s)+"-"+string(r.st), func(t *testing.T) {
			t.Parallel()
			ran.Add(1)
			p, err := lifecycle.PlanPromote(ready(r.s, r.st), promote(r.releases), moment())
			switch r.want {
			case "refused":
				refusal(t, err, "gragas rule restore go.errors.9", "is retired")
			case "":
				if err != nil || len(p.Delta.Changes) != 0 || p.Record.Kind != "promote" || !strings.Contains(p.Sections[0].Lines[0], "active already") {
					t.Errorf("plan = %+v, err = %v, want nothing to do", p, err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				c := changeFor(t, p, "rule:go.errors.9", "status")
				if c.Before != string(r.st) || c.After != string(r.want) || p.Record.Kind != "promote" || p.Record.After.Status != r.want || p.Record.Before.Status != r.st {
					t.Errorf("change = %+v, record = %+v, want %s to %s", c, p.Record, r.st, r.want)
				}
				if p.Record.Decision != "taken" {
					t.Errorf("the record names the decision %q, want taken", p.Record.Decision)
				}
			}
		})
	}
}

// TestPromoteRefusesWhatIsMissingAndNamesIt: a must without an example pair is
// refused naming the example pair; likewise a source and a decision; two gaps
// are named in one refusal. Risk: a rule promoted without its reason, its test
// or its record.
func TestPromoteRefusesWhatIsMissingAndNamesIt(t *testing.T) {
	t.Parallel()
	noPair := func(r *model.Rule) { r.Pairs = nil }
	noSource := func(r *model.Rule) { r.Sources = nil }
	bound := func(r *model.Rule) { r.Pairs = nil; r.Bindings = []model.Binding{{}} }
	for _, tc := range []struct {
		name      string
		b         model.Book
		req       model.PromoteRule
		hint, msg string
	}{
		{"must without a pair", ready(model.Must, model.Proposed, noPair), promote(0), "pairs/", "no example pair"},
		{"should without a pair", ready(model.Should, model.Proposed, noPair), promote(0), "pairs/", "no example pair"},
		{"must without a source", ready(model.Must, model.Proposed, noSource), promote(0), "gragas rule edit go.errors.9 --cite", "no source"},
		{"may without a source", ready(model.May, model.Proposed, noSource), promote(0), "--cite", "no source"},
		{"must without a decision", ready(model.Must, model.Proposed), model.PromoteRule{ID: "go.errors.9", Files: files()}, "--decision", "decision"},
		{"may without a decision", ready(model.May, model.Proposed), model.PromoteRule{ID: "go.errors.9", Files: files()}, "--decision", "decision"},
		{"may once bound needs a pair", ready(model.May, model.Proposed, bound), promote(0), "pairs/", "no example pair"},
		{"two gaps in one refusal", ready(model.Must, model.Proposed, noPair, noSource), promote(0), "--cite", "no source, no example pair"},
		{"a decision the book lacks", ready(model.Must, model.Proposed), model.PromoteRule{ID: "go.errors.9", Why: model.Why{Cite: "nowhere"}, Files: files()}, "gragas decision list", "nowhere"},
		{"a rule the book lacks", ready(model.Must, model.Proposed), model.PromoteRule{ID: "go.errors.8", Why: model.Why{Cite: "taken"}, Files: files()}, "gragas rule list", "go.errors.8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := lifecycle.PlanPromote(tc.b, tc.req, moment())
			refusal(t, err, tc.hint, tc.msg)
		})
	}
	// a may with no binding needs no pair, and goes straight to active
	noBinding := ready(model.May, model.Proposed, noPair)
	if p, err := lifecycle.PlanPromote(noBinding, promote(0), moment()); err != nil || changeFor(t, p, "rule:go.errors.9", "status").After != "active" {
		t.Errorf("an unbound may with source and decision: plan = %+v, err = %v, want active", p, err)
	}
}

// TestATrialMovesToActiveOnlyAfterAReleaseAndInsideTheWindow: the release
// count is the request's, and the window is the governance's trial_releases.
// Risk: a trial that ends before one release, or one promoted past its window.
func TestATrialMovesToActiveOnlyAfterAReleaseAndInsideTheWindow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		releases  int
		active    bool
		hint, msg string
	}{
		{0, false, "after a release has gone by", "no release has gone by"},
		{1, true, "", ""},
		{3, true, "", ""},
		{4, false, "gragas rule retire go.errors.9", "past its window of 3 releases: 4 have gone by"},
	} {
		p, err := lifecycle.PlanPromote(ready(model.Must, model.Trial), promote(tc.releases), moment())
		if !tc.active {
			refusal(t, err, tc.hint, tc.msg)
			continue
		}
		if err != nil || changeFor(t, p, "rule:go.errors.9", "status").After != "active" {
			t.Errorf("releases %d: plan = %+v, err = %v, want active", tc.releases, p, err)
		}
	}
	// a governance that gives no window has none to be past
	b := ready(model.Must, model.Trial)
	b.Identity.Governance.TrialReleases = 0
	if _, err := lifecycle.PlanPromote(b, promote(40), moment()); err != nil {
		t.Errorf("no window: %v", err)
	}
}

// TestAPromotionRecordsANewDecisionAndSaysThePresetDiff: a new decision is in
// the delta; the consumer section says "none: no bindings yet" until a diff is
// given, and is left out then, and shows a diff when there is one.
func TestAPromotionRecordsANewDecisionAndSaysThePresetDiff(t *testing.T) {
	t.Parallel()
	req := promote(0)
	req.Why = model.Why{New: &model.NewDecision{Title: "Try it", Text: "Let it run a release."}}
	p, err := lifecycle.PlanPromote(ready(model.Must, model.Proposed), req, moment())
	if err != nil {
		t.Fatal(err)
	}
	changeFor(t, p, "decision:decide-promote-go-errors-9", "")
	if p.Record.Decision != "decide-promote-go-errors-9" {
		t.Errorf("record = %+v", p.Record)
	}
	for _, s := range p.Sections {
		if s.Title == "Consumer actions" {
			t.Errorf("a plan with no diff prints a consumer section: %+v", s)
		}
	}
	req.PresetDiff = "go.errors.9 ships at warning in the go preset"
	p, err = lifecycle.PlanPromote(ready(model.Must, model.Proposed), req, moment())
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(p.Sections, func(s model.Section) bool { return s.Title == "Consumer actions" }); i < 0 || p.Sections[i].Lines[0] != req.PresetDiff {
		t.Errorf("sections = %+v, want the preset diff in the consumer section", p.Sections)
	}
}

// TestConfirmMovesReviewByByTheCadenceAndRecordsTheReviewer: the date is the
// moment's day and the governance's days; the entry is the moment's who.
// Risk: a date read from the clock, a cadence of zero, or no reviewer.
func TestConfirmMovesReviewByByTheCadenceAndRecordsTheReviewer(t *testing.T) {
	t.Parallel()
	b := ready(model.Must, model.Active, func(r *model.Rule) { r.ReviewBy = "2026-09-01" })
	p, err := lifecycle.PlanConfirm(b, model.ConfirmRule{ID: "go.errors.9", Files: files()}, moment())
	if err != nil {
		t.Fatal(err)
	}
	// 2026-11-08 and ninety days
	if c := changeFor(t, p, "rule:go.errors.9", "review_by"); c.Before != "2026-09-01" || c.After != "2027-02-06" {
		t.Errorf("change = %+v", c)
	}
	if p.Record.Kind != "confirm" || p.Record.After.ReviewBy != "2027-02-06" || p.Record.Before.ReviewBy != "2026-09-01" || p.Record.After.Status != model.Active {
		t.Errorf("record = %+v", p.Record)
	}
	if !strings.Contains(p.Sections[0].Lines[0], "Ada <ada@example.org>") {
		t.Errorf("the plan does not name the reviewer: %+v", p.Sections[0])
	}
	// the same day again: nothing to do
	b = ready(model.Must, model.Active, func(r *model.Rule) { r.ReviewBy = "2027-02-06" })
	if p, err := lifecycle.PlanConfirm(b, model.ConfirmRule{ID: "go.errors.9", Files: files()}, moment()); err != nil || len(p.Delta.Changes) != 0 || !strings.Contains(p.Sections[0].Lines[0], "already") {
		t.Errorf("a second confirm: plan = %+v, err = %v, want nothing to do", p, err)
	}
}

func TestConfirmRefusesWhatItCannotMove(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		b         model.Book
		m         lifecycle.Moment
		hint, msg string
	}{
		{"a proposed rule", ready(model.Must, model.Proposed), moment(), "gragas rule promote go.errors.9", "only an active rule is confirmed"},
		{"a trial rule", ready(model.Must, model.Trial), moment(), "gragas rule promote", "status of go.errors.9 is trial"},
		{"a retired rule", ready(model.Must, model.Retired), moment(), "gragas rule promote", "is retired"},
		{"no cadence", func() model.Book {
			b := ready(model.Must, model.Active)
			b.Identity.Governance.ReviewDays = 0
			return b
		}(), moment(), "review_days", "no review cadence"},
		{"a day that is not a date", ready(model.Must, model.Active), lifecycle.Moment{Who: "Ada", Date: "soon"}, "YYYY-MM-DD", "not a date"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := lifecycle.PlanConfirm(tc.b, model.ConfirmRule{ID: "go.errors.9", Files: files()}, tc.m)
			refusal(t, err, tc.hint, tc.msg)
		})
	}
	if _, err := lifecycle.PlanConfirm(ready(model.Must, model.Active), model.ConfirmRule{ID: "go.nope.1", Files: files()}, moment()); err == nil {
		t.Error("a rule the book lacks was confirmed")
	}
}
