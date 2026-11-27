package tool_test

import (
	"testing"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/gragas/internal/host/tool"
)

// TestTheFileNameOfAProgramIsGoProcs: the name the port answers for a program is the file name go-proc
// gives a program of that name on this system, whatever the system adds to it. It
// names no system. Risk: a name a program is never found at, so a path that names the real file is
// refused, or one that does not is passed.
func TestTheFileNameOfAProgramIsGoProcs(t *testing.T) {
	t.Parallel()
	if got, want := tool.FileName("lint-a"), proc.LocalFileName("lint-a"); got != want {
		t.Errorf("FileName(lint-a) = %q, want go-proc's %q", got, want)
	}
	if a, b := tool.FileName("lint-a"), tool.FileName("lint-b"); a == b || len(a) < len("lint-a") {
		t.Errorf("FileName answers %q and %q for two programs", a, b)
	}
}
