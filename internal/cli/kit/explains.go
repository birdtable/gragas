package kit

import (
	"context"

	"github.com/birdtable/gragas/internal/model"
)

// Explains are the operations of explain and resolve, each one operation of the
// core, called once. They read the rulebook of the working folder and change
// nothing.
type Explains interface {
	// ExplainRule says what the rulebook holds of the rule with the id or alias.
	// An id no rule holds is an error the command words.
	ExplainRule(ctx context.Context, asked string) (model.Explained, error)
	// ExplainEntry says which rules hold the entry key of tool; none is the
	// unmapped answer, not an error.
	ExplainEntry(ctx context.Context, tool, key string) (model.Explained, error)
	// Resolve lists the effective rulebook of the folder's chain with where each
	// entry came from. A parent that is not here as a value is an error.
	Resolve(ctx context.Context) (model.ResolveReport, error)
}
