package model_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestAnOpenProposalAloneMoves (spec 16.1): of the sixteen moves
// between the four states, the three out of open are allowed, and every
// other is refused, staying put included, as is a move to or from a state
// that is not one. Planted failure: a final state let move back to open.
func TestAnOpenProposalAloneMoves(t *testing.T) {
	t.Parallel()
	allowed := map[[2]model.ProposalState]bool{
		{model.ProposalOpen, model.ProposalAccepted}:  true,
		{model.ProposalOpen, model.ProposalDeclined}:  true,
		{model.ProposalOpen, model.ProposalWithdrawn}: true,
	}
	for _, from := range model.ProposalStates() {
		for _, to := range model.ProposalStates() {
			if got := from.CanBecome(to); got != allowed[[2]model.ProposalState{from, to}] {
				t.Errorf("%s.CanBecome(%s) = %v", from, to, got)
			}
		}
	}
	if model.ProposalOpen.CanBecome("merged") || model.ProposalState("merged").CanBecome(model.ProposalOpen) {
		t.Error("a state that is not one moved")
	}
}
