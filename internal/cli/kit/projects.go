package kit

import (
	"context"

	"github.com/birdtable/gragas/internal/model"
)

// Projects are the operations of check in a project, each one operation of
// the core, called once. They read the project and change nothing.
type Projects interface {
	// HoldsRulebook is whether the working folder itself holds a
	// rulebook.yaml: the first thing check asks, so that inside rulebook/ the
	// rulebook's gate runs even where a pin is above it. It lists nothing
	// below the folder.
	HoldsRulebook(ctx context.Context) (bool, error)
	// HoldsPin is whether a .gragas/pin.yaml lies in the working folder or
	// above it, up to the repository's root.
	HoldsPin(ctx context.Context) (bool, error)
	// CheckProject is check below a pin: the project's tool configs, tier
	// contract and exemptions held to the bindings of the rulebook the pin
	// names. The refusal is a run that cannot be judged; the error is a pin or
	// a book that is not ready to read.
	CheckProject(ctx context.Context) (model.ProjectCheck, *model.Refusal, error)
}
