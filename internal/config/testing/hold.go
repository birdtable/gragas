package configtest

import (
	"os"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"
)

// holdDrain is how long the probe run's output pipes are read once its tree
// is killed (go-proc's Cmd.Drain, a zero as none).
const holdDrain = 10 * time.Second

// A Hold is what this system does with a run held to a memory limit, as
// go-proc answers it (host/tool's memoryHold).
type Hold string

// The three answers go-proc gives a run held to a memory limit here: its
// tree held to a bound while it runs (a delegated cgroup on Linux, the Job
// Object on Windows), its peak judged after the run alone (macOS), or
// refused before it starts (Linux with no delegated cgroup).
const (
	HoldsTree   Hold = "held"
	JudgesAfter Hold = "judged after"
	HoldsNone   Hold = "refused"
)

// HoldOf is what this system does with a run whose tree is bounded to
// treeMemory bytes, as go-proc's Holds answers it for a run of this test's
// own binary: a test asks the library, never the GOOS, which answer to
// expect, as the Steps adapter itself does. Only a test of a run held to a
// memory limit that is set asks it: a run with none, the default,
// asks go-proc nothing and runs on every host.
func HoldOf(t testing.TB, treeMemory uint64) Hold {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := proc.Cmd{Path: exe, Args: []string{"-test.run=^$"}, Timeout: time.Minute, MaxOutput: 1 << 10, Drain: holdDrain, ReadPeak: true}
	c.Limits.TreeMemory = treeMemory
	switch hold, _, err := proc.HoldsTreeMemory(c); {
	case err != nil:
		t.Fatalf("go-proc refused the probe's own Cmd: %v", err)
	case hold == proc.TreeMemoryHeld:
		return HoldsTree
	case hold == proc.TreeMemoryJudgedAfter:
		return JudgesAfter
	}
	return HoldsNone
}
