package coreapi

import (
	"context"

	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// CheckProjectLoaded is checkProject for the tests, which hand it a loaded
// project whose book is a root of its own, as no pin on disk could name it.
func CheckProjectLoaded(ctx context.Context, p port.Ports, l project.Loaded, ask ProjectAsk) (model.ProjectCheck, error) {
	return checkProject(ctx, p, l, ask)
}
