package link

import (
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
)

// Derive is the standing and the marks of every rule of the book, in the
// order of its rules (S32, spec 17.2 and 17.3). It is a function of each
// rule's status, strength, bindings and pairs and of the grid, and of
// nothing else: it reads no file and calls no port, so retiring a rule
// changes no row but its own.
//
// A binding at rung 0 to 5 is effective when the grid says pass for that
// binding, found by what decides it (model.Binding.Ref), not by its place: a
// cell recorded for a binding since unbound or changed matches nothing, and
// the binding reads as not run. A binding at rung 6 is effective when its pair
// exists, since a review is never pass or fail. A binding in an open entry
// conflict of the chain (S8) is blocked: it is no effective binding and no
// unproven one, its rule reads from its other bindings, and the rule carries the
// blocked mark, so a must whose only binding is in a conflict is "planned,
// blocked".
//
// A rule whose status and strength cannot be held (a trial on a may, which goes
// to active at once; a strength that is none of the three) is refused as
// invalid input from its status and strength alone, whatever the bindings and
// the grid say: it has no row, and the error names each. The rows returned
// beside an error are for the error's report only; a caller that paints them
// paints a book with rules missing.
func Derive(book model.Book, grid model.Grid, chain model.Chain) ([]model.Held, error) {
	rows := make([]model.Held, 0, len(book.Rules))
	var refused []error
	for _, r := range book.Rules {
		h, err := derive(r, grid, chain)
		if err != nil {
			refused = append(refused, err)
			continue
		}
		rows = append(rows, h)
	}
	return rows, errors.Join(refused...)
}

func derive(r model.Rule, grid model.Grid, chain model.Chain) (model.Held, error) {
	switch r.Status {
	case model.Retired:
		return model.Held{Rule: r.ID, Standing: model.StandingRetired}, nil
	case model.Proposed:
		return model.Held{Rule: r.ID, Standing: model.StandingProposed}, nil
	case model.Active, model.Trial:
	default:
		return model.Held{}, fmt.Errorf("rule %s: status %q is not one of proposed, trial, active, retired", r.ID, r.Status)
	}
	switch r.Strength {
	case model.Must, model.Should:
	case model.May:
		if r.Status == model.Trial {
			return model.Held{}, fmt.Errorf("rule %s: a trial on a may is refused: a may goes to active at once", r.ID)
		}
	default:
		return model.Held{}, fmt.Errorf("rule %s: strength %q is not one of must, should, may", r.ID, r.Strength)
	}
	var strong, reviewed, unproven, blocked bool
	for _, b := range r.Bindings {
		switch {
		case b.Rung < model.MaxRung && chain.Blocks(b.Ref(r.ID)):
			blocked = true
		case !effective(r, b, grid):
			unproven = true
		case b.Rung == model.MaxRung:
			reviewed = true
		default:
			strong = true
		}
	}
	var s model.Standing
	switch {
	case strong:
		s = model.StandingEnforced
	case reviewed:
		s = model.StandingReviewed
	case r.Strength == model.May:
		s = model.StandingAdvisory
	default:
		s = model.StandingPlanned
	}
	h := model.Held{Rule: r.ID, Standing: s}
	if r.Status == model.Trial {
		h.Marks = append(h.Marks, model.MarkTrial)
	}
	if blocked {
		h.Marks = append(h.Marks, model.MarkBlocked)
	}
	if unproven {
		h.Marks = append(h.Marks, model.MarkUnproven)
	}
	return h, nil
}

// effective says whether the binding holds its rule: proven at rung 0 to 5,
// its pair standing at rung 6.
func effective(r model.Rule, b model.Binding, grid model.Grid) bool {
	if b.Rung == model.MaxRung {
		for _, p := range r.Pairs {
			if p.Number == b.Pair && b.Pair != 0 {
				return true
			}
		}
		return false
	}
	return grid[b.Ref(r.ID)] == model.TruthPass
}
