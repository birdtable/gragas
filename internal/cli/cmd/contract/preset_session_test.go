package contract_test

import (
	"context"

	"github.com/birdtable/gragas/internal/cli/kit"
)

// Preset is the one method of kit.Session this package does not draw through.
func (s *session) Preset(context.Context, kit.Cmd, bool) int { return 0 }
