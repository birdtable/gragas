package tool_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/port"
)

// stepFound is a Tool over a project root and a program lint-a with body,
// found by the settings outside the root: the run of a step of it is held to
// what the Scratch says, which stepBoundTo gives it.
func stepFound(t *testing.T, body string) (*tool.Tool, string) {
	t.Helper()
	root := private(t)
	prog := program(t, private(t)+"/lint-a", body)
	return stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": prog}}, nil), root
}

// stepBoundTo is the runner of tl bound to a scratch folder held to seconds
// and mib, a memory limit of 0 being none.
func stepBoundTo(t *testing.T, tl *tool.Tool, root string, seconds, mib int) port.StepRunner {
	t.Helper()
	s := stepScope(t, root, false)
	s.Scratch.Seconds, s.Scratch.MemoryMiB = seconds, mib
	return bind(t, tl, s)
}

// lastRan is the record of the newest run tl started.
func lastRan(tl *tool.Tool) tool.Ran {
	r := tl.Ran()
	return r[len(r)-1]
}

// TestAStepsRunPastItsMemoryLimitIsNotRunWithTheLimitNamed (replaces
// TestARunPastItsMemoryLimitIsNotRunWithTheLimitNamed): a step
// whose peak memory passed its scratch folder's limit, or whose tree passed the
// bound go-proc held it to, is ended on memory, and its cell is "not run" with
// the limit named. Where this system can neither hold nor read a run's memory,
// the step is refused before it starts, by the host's own words with the limit
// named and the delegation fix, matching port.ErrNotHeld, and nothing runs. The
// controls are the same step held to no limit, and, where a limit is held or
// judged, held to 512 MiB it stays below: each is counted.
// Planted: bound.run ignoring Scratch.MemoryMiB (the limit row red, the hog
// counted).
func TestAStepsRunPastItsMemoryLimitIsNotRunWithTheLimitNamed(t *testing.T) {
	t.Parallel()
	tl, root := stepFound(t, hog())
	answer := holding(t, 8)
	_, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 8))
	if answer == holdsNone {
		var r *tool.Refused
		if !errors.Is(err, port.ErrRefused) || !errors.Is(err, port.ErrNotHeld) || !errors.As(err, &r) ||
			!strings.Contains(r.Observed(), "is held to the memory limit of 8 MiB, and this system can neither hold the run's tree to it nor read its peak memory after it") ||
			!strings.Contains(r.Hint(), "Delegate=yes and DelegateSubgroup=app (docs/checks.md, The delegated cgroup)") || len(tl.Ran()) != 1 {
			t.Fatalf("held to a limit this system neither holds nor reads: %v, runs %+v; want refused before the start by name, with the fix", err, tl.Ran())
		}
	} else {
		var e *tool.Ended
		if !errors.As(err, &e) || e.Why != tool.EndedMemory {
			t.Fatalf("past the limit (%s): %v, want ended on memory", answer, err)
		}
		if cell, ok := e.NotRun(); !ok || cell != "not run: the memory limit of 8 MiB was hit" {
			t.Errorf("the cell is %q, %v; want not run with the limit named", cell, ok)
		}
		if got := lastRan(tl); got.Shape != "step" || got.Ended != tool.EndedMemory || got.JudgedAfter != (answer == judgesAfter) {
			t.Errorf("the record (%s): %+v", answer, got)
		}
	}
	if res, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 0)); err != nil || text(t, res.Stdout) != "done" {
		t.Errorf("the control, held to no limit: %v, %v", res.Stdout, err)
	}
	if answer == holdsNone {
		return
	}
	if res, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 512)); err != nil || text(t, res.Stdout) != "done" || lastRan(tl).JudgedAfter != (answer == judgesAfter) {
		t.Errorf("the control, held to 512 MiB (%s): %v, %v, %+v; want counted", answer, res.Stdout, err, lastRan(tl))
	}
}

// TestAStepsRunHeldToALimitRecordsThePeakGoProcRead (replaces
// TestARunHeldToALimitRecordsThePeakGoProcRead): where this
// system holds or reads a run's memory, a step held to a memory limit asks
// go-proc for its peak, and its record carries it and its kind: above 0 and at
// most the 512 MiB the hog stays below. A step held to no limit asks for none,
// and records -1 and no kind. Where this system does neither, the step held to
// a limit is refused before it starts and records nothing.
// Planted: memoryHold not setting ReadPeak (a step held to a limit is refused
// or records no peak; the first row red), and bound.run ignoring
// Scratch.MemoryMiB (the first row red, -1 recorded).
func TestAStepsRunHeldToALimitRecordsThePeakGoProcRead(t *testing.T) {
	t.Parallel()
	tl, root := stepFound(t, hog())
	answer := holding(t, 512)
	_, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 512))
	switch got := lastRan(tl); {
	case answer == holdsNone:
		if !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 1 {
			t.Errorf("held to a limit where nothing is held or read: %v, runs %+v; want refused before the start", err, tl.Ran())
		}
	case err != nil || got.PeakMiB < 1 || got.PeakMiB > 512 || got.PeakKind == "":
		t.Errorf("held to 512 MiB (%s): %v, the record's peak %d MiB of %q; want counted, with the hog's peak above 0 and at most 512, its kind named", answer, err, got.PeakMiB, got.PeakKind)
	}
	if _, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 0)); err != nil || lastRan(tl).PeakMiB != -1 || lastRan(tl).PeakKind != "" {
		t.Errorf("held to no limit: %v, peak %d of %q; want run with no reading, -1", err, lastRan(tl).PeakMiB, lastRan(tl).PeakKind)
	}
}

// TestAStepsRunInAScratchFolderWithNoMemoryLimitRunsOnEveryHost (replaces
// TestARunInAScratchFolderWithNoMemoryLimitRunsOnEveryHost): gragas sets no
// memory limit of its own, so a step held
// to a scratch folder with a deadline and a memory limit of 0, the default, asks
// go-proc for no TreeMemory and no peak (memoryHold), and starts whatever this
// system answers for a run held to a limit: it is counted, its record reads no
// peak (-1, no kind) and is not judged after. The control is the same step held
// to 512 MiB, which goes by the host's answer, refused by name where nothing
// holds or reads a run's memory.
// Planted: memoryHold taking 0 for a limit (l.mib < 0; the record reads a peak,
// or the step is refused where nothing is held).
func TestAStepsRunInAScratchFolderWithNoMemoryLimitRunsOnEveryHost(t *testing.T) {
	t.Parallel()
	tl, root := stepFound(t, echo)
	res, err := stepRunOf(t, stepBoundTo(t, tl, root, 60, 0))
	ran := tl.Ran()
	if err != nil || len(ran) != 2 {
		t.Fatalf("held to no memory limit: %v, %v, runs %+v; want counted on every host", res.Stdout, err, ran)
	}
	if got := ran[1]; got.PeakMiB != -1 || got.PeakKind != "" || got.JudgedAfter {
		t.Errorf("held to no memory limit: the record %+v; want no peak asked and nothing judged", got)
	}
	_, err = stepRunOf(t, stepBoundTo(t, tl, root, 60, 512))
	if holding(t, 512) == holdsNone {
		if !errors.Is(err, port.ErrNotHeld) || len(tl.Ran()) != 3 {
			t.Errorf("the control, held to 512 MiB where nothing holds or reads a run's memory: %v, runs %+v; want refused by name before the start", err, tl.Ran())
		}
		return
	}
	if err != nil {
		t.Errorf("the control, held to 512 MiB: %v, want counted", err)
	}
}

// TestAStepPastItsDeadlineIsEndedAndTheRunSaysSo (replaces
// TestAToolPastItsCapOrItsDeadlineIsEndedAndTheRunSaysSo, its deadline half, and
// TestARunPastItsScratchDeadlineIsNotRunWithTheLimitNamed): a step that never
// ends is ended by the deadline, a real process killed, as the host's wait ends
// it (Bound) or as the scratch folder's, lower than the host's, does; the end is
// an *Ended at the deadline with no output taken as a result, the cell names the
// deadline "not run: the deadline of 1s was hit", the record says so, and the
// run does not go on to this test's own deadline. The output cap's half is
// TestAStreamIsCutAtItsCap's, which holds a step.
// Planted: bound.run starting the step with a deadline of an hour (the first
// half red), and the scratch folder's seconds never lowering it (the second
// half red).
func TestAStepPastItsDeadlineIsEndedAndTheRunSaysSo(t *testing.T) {
	t.Parallel()
	tl, root := stepFound(t, answering(configtest.Spin()))
	tl.Bound(300*time.Millisecond, 1024)
	// The step's own deadline ends it, long before this test's.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, err := stepBoundTo(t, tl, root, 60, 0).Run(ctx, 1, 1, runOf())
	var e *tool.Ended
	if !errors.As(err, &e) || e.Why != tool.EndedDeadline {
		t.Fatalf("past the host's deadline: %v, want ended at the deadline", err)
	}
	if got := lastRan(tl); got.Shape != "step" || got.Ended != tool.EndedDeadline {
		t.Errorf("the record: %+v", got)
	}
	tl, root = stepFound(t, answering(configtest.Spin()))
	tl.Bound(time.Minute, 1024)
	ctx, cancel = context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, err = stepBoundTo(t, tl, root, 1, 0).Run(ctx, 1, 1, runOf())
	if !errors.As(err, &e) || e.Why != tool.EndedDeadline {
		t.Fatalf("past the scratch deadline: %v, want ended at the deadline", err)
	}
	if cell, ok := e.NotRun(); !ok || cell != "not run: the deadline of 1s was hit" {
		t.Errorf("the cell is %q, %v", cell, ok)
	}
	if ctx.Err() != nil {
		t.Error("the step went on to this test's deadline: the scratch folder's was not applied")
	}
}
