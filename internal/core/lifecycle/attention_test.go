package lifecycle_test

import (
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// attentive is a book of rules built by edit, each with a source, a pair and an
// owner unless edit takes them away, under a governance that asks for a decision.
func attentive(rules ...model.Rule) model.Book {
	b := book()
	b.Identity.Governance = model.Governance{DecisionRequired: true, TrialReleases: 3, ReviewDays: 90}
	b.Rules = rules
	return b
}

func whole(id string, st model.Status, edit ...func(*model.Rule)) model.Rule {
	r := model.Rule{ID: model.RuleID(id), Statement: "z", Strength: model.Must, Status: st, Origin: model.Authored, Owner: "owner",
		Sources: []model.Citation{{Ref: "effective-go", Backs: model.BacksRule}}, Pairs: []model.Pair{{Number: 1}},
		Reasoning: &model.Reasoning{Decision: "taken"}}
	for _, e := range edit {
		e(&r)
	}
	return r
}

func attentionKindsOf(l model.AttentionListing, id string) []model.AttentionKind {
	var out []model.AttentionKind
	for _, i := range l.Items {
		if string(i.Rule) == id {
			out = append(out, i.Kind)
		}
	}
	return out
}

// TestAttendNamesEveryKindOfAttention: a table over the kinds the listing
// makes, each row a rule that has the one fault, and the test says it ran every
// kind. Risk: a kind that is declared and never produced, a rule listed under
// a kind it does not have, an active rule that is not overdue until it is.
func TestAttendNamesEveryKindOfAttention(t *testing.T) {
	t.Parallel()
	type row struct {
		kind model.AttentionKind
		rule model.Rule
	}
	rows := []row{
		{model.AttentionOverdue, whole("go.errors.1", model.Active, func(r *model.Rule) { r.ReviewBy = "2026-01-01" })},
		{model.AttentionNoSource, whole("go.errors.1", model.Proposed, func(r *model.Rule) { r.Sources = nil })},
		{model.AttentionNoPair, whole("go.errors.1", model.Proposed, func(r *model.Rule) { r.Pairs = nil })},
		{model.AttentionNoDecision, whole("go.errors.1", model.Proposed, func(r *model.Rule) { r.Reasoning = nil })},
		{model.AttentionNoOwner, whole("go.errors.1", model.Proposed, func(r *model.Rule) { r.Owner = "" })},
	}
	var ran atomic.Int32
	t.Cleanup(func() {
		// every kind but the one only a whole proposed book lists
		if want := len(model.AttentionKinds()) - 1; int(ran.Load()) != want {
			t.Errorf("ran %d of %d kinds", ran.Load(), want)
		}
	})
	for _, r := range rows {
		t.Run(string(r.kind), func(t *testing.T) {
			t.Parallel()
			ran.Add(1)
			got := attentionKindsOf(lifecycle.Attend(attentive(r.rule), "2026-11-16"), "go.errors.1")
			if !slices.Equal(got, []model.AttentionKind{r.kind}) {
				t.Fatalf("kinds = %v, want only %q", got, r.kind)
			}
		})
	}
}

// TestAttendLeavesAWholeRuleAndARetiredOneAlone: a rule that misses nothing is
// not listed in a book that has an active rule, and a retired rule that misses
// everything asks for nothing. Risk: noise that hides the rules that need you.
func TestAttendLeavesAWholeRuleAndARetiredOneAlone(t *testing.T) {
	t.Parallel()
	b := attentive(whole("go.errors.1", model.Active, func(r *model.Rule) { r.ReviewBy = "2027-01-01" }),
		whole("go.errors.2", model.Retired, func(r *model.Rule) { r.Sources, r.Pairs, r.Owner = nil, nil, "" }))
	got := lifecycle.Attend(b, "2026-11-16")
	if len(got.Items) != 0 || got.AllProposed {
		t.Fatalf("listed %+v (all proposed %v), want nothing", got.Items, got.AllProposed)
	}
}

// TestAttendListsTheWholeBookWhenEveryRuleIsProposed: every rule is listed, a
// whole one as nothing missing. Risk: a book of proposed rules that shows a
// short list and hides the rest.
func TestAttendListsTheWholeBookWhenEveryRuleIsProposed(t *testing.T) {
	t.Parallel()
	b := attentive(whole("go.errors.1", model.Proposed), whole("go.errors.2", model.Proposed, func(r *model.Rule) { r.Owner = "" }))
	got := lifecycle.Attend(b, "2026-11-16")
	if !got.AllProposed {
		t.Fatal("all proposed = false")
	}
	if !slices.Equal(attentionKindsOf(got, "go.errors.1"), []model.AttentionKind{model.AttentionComplete}) ||
		!slices.Equal(attentionKindsOf(got, "go.errors.2"), []model.AttentionKind{model.AttentionNoOwner}) {
		t.Fatalf("items = %+v", got.Items)
	}
}

// TestAttendGrowsTheOverdueListWith400DaysOfClock: three active rules confirmed
// to dates a year apart; today moved 400 days on, the overdue list grows by the
// rules whose date it passes, a literal count. Risk: a date judged against the
// wrong day, or against none.
func TestAttendGrowsTheOverdueListWith400DaysOfClock(t *testing.T) {
	t.Parallel()
	b := attentive(
		whole("go.errors.1", model.Active, func(r *model.Rule) { r.ReviewBy = "2026-12-01" }),
		whole("go.errors.2", model.Active, func(r *model.Rule) { r.ReviewBy = "2027-06-01" }),
		whole("go.errors.3", model.Active, func(r *model.Rule) { r.ReviewBy = "2027-12-31" }))
	today := "2026-11-16"
	later := mustAdd(t, today, 400) // 2027-12-21
	overdue := func(day string) []string {
		var out []string
		for _, i := range lifecycle.Attend(b, day).Items {
			if i.Kind == model.AttentionOverdue {
				out = append(out, string(i.Rule))
			}
		}
		return out
	}
	if got := overdue(today); len(got) != 0 {
		t.Fatalf("on %s overdue = %v, want none", today, got)
	}
	if later != "2027-12-21" {
		t.Fatalf("400 days after %s = %s, want 2027-12-21", today, later)
	}
	if got := overdue(later); !slices.Equal(got, []string{"go.errors.1", "go.errors.2"}) {
		t.Fatalf("on %s overdue = %v, want go.errors.1 and go.errors.2", later, got)
	}
}

func mustAdd(t *testing.T, day string, days int) string {
	t.Helper()
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		t.Fatal(err)
	}
	return d.AddDate(0, 0, days).Format(time.DateOnly)
}

// TestTheAttentionListingReadsNoClock: the files of the listing, below the
// port that gives the day, call no wall clock and start no timer (S55). Risk:
// a date judged against the machine's day inside the context, which no test
// can move.
func TestTheAttentionListingReadsNoClock(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"attention.go", "../../model/attention.go", "../../cli/cmd/attention/command.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"time.Now", "time.Since", "time.Until", "time.After", "time.Tick", "time.NewTimer", "time.NewTicker", "time.AfterFunc"} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s calls %s", f, banned)
			}
		}
	}
}
