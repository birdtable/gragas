package tool_test

import (
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
)

// hog is a tool body that holds a 4 MiB string, doubled from one byte by the
// shell alone, so it needs no PATH: the shell holds tens of MiB at its peak
// doing so, and prints "done" once it has.
func hog() string { return answering(configtest.Memory(4), configtest.Out("done")) }

// The three answers go-proc gives a run held to a memory limit here
// (memoryHold): its tree held to a bound while it runs, its peak judged after
// the run alone, or refused before it starts.
const (
	holdsTree   = configtest.HoldsTree
	judgesAfter = configtest.JudgesAfter
	holdsNone   = configtest.HoldsNone
)

// holding is what this system does with a run held to a memory limit of
// mib, as go-proc's Holds answers it for a run of this test's own binary
// bounded as the adapter bounds it (configtest.HoldOf): the test asks the
// library, never the GOOS, which answer to expect.
func holding(t *testing.T, mib int) configtest.Hold {
	t.Helper()
	return configtest.HoldOf(t, tool.TreeMemory(mib))
}

// TestARunThatGivesNoReadingOfItsPeakIsOverItsLimit: a run whose peak
// memory cannot be read is not counted, since the limit was not seen to
// hold; a peak over the limit is not counted; a peak at the limit is.
// boundary: the limit, 512 MiB
// Risk: a run with no reading counted as one held under its limit.
func TestARunThatGivesNoReadingOfItsPeakIsOverItsLimit(t *testing.T) {
	t.Parallel()
	if _, ok := (&tool.Ended{Why: tool.EndedCap}).NotRun(); ok {
		t.Error("a run past its cap made a not-run cell")
	}
	e := tool.OverMemory("lint-a", 0, "", false, false, 512)
	if e == nil || e.Why != tool.EndedMemory || !strings.Contains(e.Says, "gave no reading of its peak memory against the memory limit of 512 MiB") {
		t.Fatalf("a run with no reading: %+v, want ended on memory", e)
	}
	if cell, ok := e.NotRun(); !ok || cell != "not run: the memory limit of 512 MiB was hit" {
		t.Errorf("the cell is %q, %v", cell, ok)
	}
	if e := tool.OverMemory("lint-a", 513, "tree-charged", true, false, 512); e == nil || e.Why != tool.EndedMemory || e.Says != "held 513 MiB at its peak (tree-charged), over the memory limit of 512 MiB" {
		t.Errorf("a peak one past the limit: %+v, want ended on memory, its kind named", e)
	}
	if e := tool.OverMemory("lint-a", 513, "process-resident", true, true, 512); e == nil || !strings.HasSuffix(e.Says, "; this system judges memory after the run, and only a sandboxed run caps it") {
		t.Errorf("a peak past the limit, judged after: %+v, want it to say nothing held it", e)
	}
	if e := tool.OverMemory("lint-a", 512, "tree-charged", true, false, 512); e != nil {
		t.Errorf("a peak at the limit: %+v, want held", e)
	}
}

// TestARunThatLeftAProcessIsNotCounted: a run held to a limit whose tree
// held a process when the tool exited is not counted, since that process's
// peak was never read, and it says so. Risk: a hog left in the background
// passes the limit, since its peak is never read.
func TestARunThatLeftAProcessIsNotCounted(t *testing.T) {
	t.Parallel()
	e := tool.LeftUnread("lint-a", 512)
	if e == nil || e.Why != tool.EndedMemory || !strings.Contains(e.Error(), "left a process of its tree running when it exited, whose peak memory was never read") {
		t.Fatalf("a run that left a process: %+v, want ended on memory, saying its peak was never read", e)
	}
	if cell, ok := e.NotRun(); !ok || cell != "not run: the memory limit of 512 MiB was hit" {
		t.Errorf("the cell is %q, %v", cell, ok)
	}
}
