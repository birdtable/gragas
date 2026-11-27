package root

import (
	"testing"

	"github.com/birdtable/gragas/internal/cli/kit"
)

// TestTheKitsWritesAnnotationIsTheRoots: a command package marks a command
// that writes with kit.Writes, and the root reads the annotation as writes.
func TestTheKitsWritesAnnotationIsTheRoots(t *testing.T) {
	t.Parallel()
	if kit.Writes != writes {
		t.Errorf("kit.Writes = %q, the root's annotation is %q", kit.Writes, writes)
	}
}
