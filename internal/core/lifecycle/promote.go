package lifecycle

import (
	"fmt"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/model"
)

// The two plans that carry a rule up its life. Promote moves the status one
// step and stands on a decision; confirm moves the date the rule is next read.
// Both take the day from the Moment the caller took once, never from a clock
// of their own, so the plan shown and the plan applied agree.

// noPresetDiff is what the consumer section says while no binding exists to
// change a preset.
const noPresetDiff = "none: no bindings yet"

// PlanPromote plans the promotion of a rule by one step (S24): a must or a
// should goes proposed to trial, and trial to active once a release has gone
// by and the governance's window is not past; a may goes straight to active.
// It refuses a rule without a source, an example pair or a decision, and says
// which. A rule that is active already plans nothing.
func PlanPromote(b model.Book, r model.PromoteRule, m Moment) (Plan, error) {
	rule, err := ruleOf(b, r.ID)
	if err != nil {
		return Plan{}, err
	}
	if rule.Status == model.Retired {
		return Plan{}, refuse("bring it back with gragas rule restore "+r.ID, "%s is retired, and a retired rule is not promoted", r.ID)
	}
	if rule.Status == model.Active {
		return nothing("promote", rule, fmt.Sprintf("%s is active already", r.ID)), nil
	}
	if err := checkRequired(b, rule, r); err != nil {
		return Plan{}, err
	}
	next, err := nextStatus(rule, r, b.Identity.Governance)
	if err != nil {
		return Plan{}, err
	}
	before := lifecycleOf(rule)
	slug := decisionSlug(r.Why, "promote", r.ID)
	d, changes, written := whyChanges(r.Why, slug, m, r.Files)
	entity := "rule:" + r.ID
	after := before
	after.Status = next
	d.Changes = append(d.Changes, model.Change{Entity: entity, Field: "status", Before: string(before.Status), After: string(next)})
	changes = append([]string{promotion(rule, next)}, changes...)
	written = append([]string{r.Files.Rules + ": sets status of " + r.ID}, written...)
	written = append(written, r.Files.History+": appends one entry (promote)")
	consumers := r.PresetDiff
	if consumers == "" {
		consumers = noPresetDiff
	}
	return Plan{Delta: d, Record: Record{Kind: "promote", Entity: entity, Before: &before, After: after, Decision: slug},
		Sections: sections(changes, written, consumers)}, nil
}

// promotion is the line of the plan that says the step.
func promotion(rule model.Rule, next model.Status) string {
	switch {
	case next == model.Trial:
		return fmt.Sprintf("%s: %s, then trial. A %s goes through trial before it is active", rule.ID, rule.Status, rule.Strength)
	case rule.Status == model.Trial:
		return fmt.Sprintf("%s: trial, then active", rule.ID)
	}
	return fmt.Sprintf("%s: %s, then active. A %s has no trial", rule.ID, rule.Status, rule.Strength)
}

// checkRequired holds the rule to S24's fields. Every missing one is named in
// the one refusal, so a person fixes them in one pass.
func checkRequired(b model.Book, rule model.Rule, r model.PromoteRule) error {
	id := string(rule.ID)
	var missing []string
	var hints []string
	if len(rule.Sources) == 0 {
		missing = append(missing, "source")
		hints = append(hints, "cite one with gragas rule edit "+id+" --cite <source slug>")
	}
	// A must or a should needs its pair before trial; a may, once it is bound.
	if len(rule.Pairs) == 0 && (rule.Strength != model.May || len(rule.Bindings) > 0) {
		missing = append(missing, "example pair")
		hints = append(hints, "add good.* and bad.* under pairs/"+id+"/1/ at the rulebook's root, and list the pair in the rule")
	}
	if len(missing) > 0 {
		return refuse(strings.Join(hints, "; "), "%s has no %s, and a rule is not promoted without it (S24)", id, strings.Join(missing, ", no "))
	}
	return checkWhy(b, r.Why, "promote", id)
}

// nextStatus is the one step a rule takes, or the refusal that says why it
// does not yet.
func nextStatus(rule model.Rule, r model.PromoteRule, g model.Governance) (model.Status, error) {
	switch {
	case rule.Strength == model.May:
		return model.Active, nil
	case rule.Status == model.Proposed:
		return model.Trial, nil
	case r.Releases < 1:
		return "", refuse("promote it again after a release has gone by", "%s is in trial and no release has gone by since it entered, so it stays there (S24)", rule.ID)
	case g.TrialReleases > 0 && r.Releases > g.TrialReleases:
		return "", refuse("retire it with gragas rule retire "+string(rule.ID)+", or bring the window up in the rulebook's governance", "the trial of %s is past its window of %d releases: %d have gone by", rule.ID, g.TrialReleases, r.Releases)
	}
	return model.Active, nil
}

// PlanConfirm plans the confirmation of an active rule: its review-by moves
// to the day of the Moment and the governance's review cadence after it, and
// the history entry names the reviewer, the one who runs the command. A rule
// already confirmed to that day plans nothing.
func PlanConfirm(b model.Book, r model.ConfirmRule, m Moment) (Plan, error) {
	rule, err := ruleOf(b, r.ID)
	if err != nil {
		return Plan{}, err
	}
	days := b.Identity.Governance.ReviewDays
	switch {
	case rule.Status != model.Active:
		return Plan{}, refuse("only an active rule is confirmed: promote it with gragas rule promote "+r.ID, "the status of %s is %s, and only an active rule is confirmed", r.ID, rule.Status)
	case days < 1:
		return Plan{}, refuse("set review_days in the rulebook's governance", "the governance gives no review cadence, so there is no date to move %s to", r.ID)
	}
	day, err := time.Parse("2006-01-02", m.Date)
	if err != nil {
		return Plan{}, refuse("take the day as YYYY-MM-DD", "the day of the change %q is not a date", m.Date)
	}
	next := day.AddDate(0, 0, days).Format("2006-01-02")
	if rule.ReviewBy == next {
		return nothing("confirm", rule, fmt.Sprintf("%s is confirmed to %s already", r.ID, next)), nil
	}
	before := lifecycleOf(rule)
	after := before
	after.ReviewBy = next
	entity := "rule:" + r.ID
	d := model.Delta{Changes: []model.Change{{Entity: entity, Field: "review_by", Before: rule.ReviewBy, After: next}}}
	changes := []string{fmt.Sprintf("%s: confirmed by %s on %s, next review %s (%d days)", r.ID, m.Who, m.Date, next, days)}
	written := []string{r.Files.Rules + ": sets review_by of " + r.ID, r.Files.History + ": appends one entry (confirm)"}
	return Plan{Delta: d, Record: Record{Kind: "confirm", Entity: entity, Before: &before, After: after},
		Sections: sections(changes, written, "none: a confirmation changes no binding")}, nil
}
