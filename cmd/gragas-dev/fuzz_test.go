package main

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fuzzRepo is a folder with three packages holding five fuzz targets.
func fuzzRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, "internal/a/a_test.go", "package a\n\nfunc FuzzOne(f *testing.F) {}\nfunc FuzzTwo(f *testing.F) {}\nfunc TestNotFuzz(t *testing.T) {}\n")
	put(t, dir, "internal/b/b_test.go", "package b\n\nfunc FuzzThree(f *testing.F) {}\n")
	put(t, dir, "cmd/c/c_test.go", "package c\n\nfunc FuzzFour(f *testing.F) {}\nfunc FuzzFive(f *testing.F) {}\n")
	put(t, dir, "internal/b/b.go", "package b\n\nfunc FuzzNotATest() {}\n")
	return dir
}

// TestEveryFuzzTargetRunsAtMostJobsAtOnce: all five targets run, never more
// than jobs together, and jobs together when the work allows it.
// Risk: a pool that stays serial, or one that ignores its limit.
func TestEveryFuzzTargetRunsAtMostJobsAtOnce(t *testing.T) {
	t.Parallel()
	var now, peak atomic.Int32
	var seen sync.Map
	var closeOnce sync.Once
	both := make(chan struct{})
	run := func(_ context.Context, _ string, tg fuzzTarget, _ time.Duration, _ int) (string, error) {
		n := now.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		seen.Store(tg.name, true)
		if n == 2 {
			closeOnce.Do(func() { close(both) })
		}
		if tg.name == "FuzzFour" || tg.name == "FuzzFive" {
			<-both
		}
		now.Add(-1)
		return "PASS " + tg.name, nil
	}
	var out bytes.Buffer
	ok, err := fuzzCommand(t.Context(), fuzzPlan{fuzzRepo(t), time.Minute, time.Second, 2, 8}, run, &out)
	if err != nil || !ok {
		t.Fatalf("ok %v, err %v\n%s", ok, err, out.String())
	}
	// The count the gate runner reads: each target run.
	if _, n := counted(t, out.String(), "tests"); n != 5 {
		t.Errorf("a count of %d tests, want the 5 targets\n%s", n, out.String())
	}
	if peak.Load() != 2 {
		t.Errorf("%d at once at the most, want 2", peak.Load())
	}
	for _, n := range []string{"FuzzOne", "FuzzTwo", "FuzzThree", "FuzzFour", "FuzzFive"} {
		if _, ok := seen.Load(n); !ok {
			t.Errorf("%s never ran", n)
		}
		if !strings.Contains(out.String(), "fuzz: "+n+" ok in ") || !strings.Contains(out.String(), "PASS "+n+"\n") {
			t.Errorf("the line or the result of %s is not kept:\n%s", n, out.String())
		}
	}
	if strings.Contains(out.String(), "FuzzNotATest") || strings.Contains(out.String(), "TestNotFuzz") {
		t.Errorf("a function that is not a fuzz target ran:\n%s", out.String())
	}
}

// TestAFailingFuzzTargetFailsTheStepAndIsNamed: the step fails, the failed
// target's own output is kept, and the closing line names it and its package.
// Risk: a step that passes with a red target, or says only "failed".
func TestAFailingFuzzTargetFailsTheStepAndIsNamed(t *testing.T) {
	t.Parallel()
	run := func(_ context.Context, _ string, tg fuzzTarget, _ time.Duration, _ int) (string, error) {
		if tg.name == "FuzzThree" {
			return "--- FAIL: FuzzThree (0.01s)\n    seed corpus entry 7\n", errors.New("exit status 1")
		}
		return "ok", nil
	}
	var out bytes.Buffer
	ok, err := fuzzCommand(t.Context(), fuzzPlan{fuzzRepo(t), time.Minute, time.Second, 1, 4}, run, &out)
	if err != nil || ok {
		t.Fatalf("ok %v, err %v; want a failed step\n%s", ok, err, out.String())
	}
	for _, want := range []string{"seed corpus entry 7", "fuzz: FuzzThree FAILED after", "the first was FuzzThree in ./internal/b"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "all ok") {
		t.Errorf("a red step says all ok:\n%s", out.String())
	}
}

// firedContext is a target's limit that ends when its signal is closed, with
// the error the wall clock's limit gives, and not before: no verdict reads how
// long a loaded host took to schedule anything.
type firedContext struct {
	context.Context
	fired <-chan struct{}
}

func (c firedContext) Done() <-chan struct{} { return c.fired }

func (c firedContext) Err() error {
	select {
	case <-c.fired:
		return context.DeadlineExceeded
	default:
		return c.Context.Err()
	}
}

// TestAFuzzTargetThatHangsPastItsTimePlusGraceFails: a run that never
// returns is stopped at fuzztime plus grace and fails, named. The limit ends
// on a signal once the hung target is running, so a target that returns at
// once is never late and none is early, whatever the host's load (gragas#741).
// Risk: one hung target holding the release tier until the job is killed.
func TestAFuzzTargetThatHangsPastItsTimePlusGraceFails(t *testing.T) {
	t.Parallel()
	running := make(chan struct{})
	fire := make(chan struct{})
	run := func(ctx context.Context, _ string, tg fuzzTarget, _ time.Duration, _ int) (string, error) {
		if tg.name != "FuzzFour" {
			return "", nil
		}
		close(running)
		<-ctx.Done()
		return "partial output\n", ctx.Err()
	}
	limit := func(ctx context.Context, tg fuzzTarget) (context.Context, context.CancelFunc) {
		if tg.name != "FuzzFour" {
			return context.WithCancel(ctx)
		}
		return firedContext{ctx, fire}, func() {}
	}
	go func() {
		<-running
		close(fire)
	}()
	var out bytes.Buffer
	ok, err := fuzzCommandLimited(t.Context(), fuzzPlan{fuzzRepo(t), 40 * time.Millisecond, 40 * time.Millisecond, 2, 8}, run, limit, &out)
	if err != nil || ok {
		t.Fatalf("ok %v, err %v; want a failed step\n%s", ok, err, out.String())
	}
	for _, want := range []string{"fuzz: FuzzFour FAILED after", "did not stop within 40ms of its 40ms", "the first was FuzzFour in ./cmd/c", "partial output"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "fuzz: FuzzFive FAILED") {
		t.Errorf("a target that returned at once failed:\n%s", out.String())
	}
}

// TestEachFuzzTargetStillFuzzesForTheWholeFuzzTime: the run is handed the
// full fuzztime whatever jobs is, and the go command line carries it, for
// that one target alone.
// Risk: parallelism paid for by dividing the time.
func TestEachFuzzTargetStillFuzzesForTheWholeFuzzTime(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var got []time.Duration
	run := func(_ context.Context, _ string, _ fuzzTarget, ft time.Duration, _ int) (string, error) {
		mu.Lock()
		got = append(got, ft)
		mu.Unlock()
		return "", nil
	}
	var out bytes.Buffer
	if ok, err := fuzzCommand(t.Context(), fuzzPlan{fuzzRepo(t), time.Minute, time.Second, 4, 16}, run, &out); err != nil || !ok {
		t.Fatalf("ok %v, err %v", ok, err)
	}
	if len(got) != 5 || slices.IndexFunc(got, func(d time.Duration) bool { return d != time.Minute }) >= 0 {
		t.Errorf("fuzz times %v, want five of 1m0s", got)
	}
	args := fuzzArgs(fuzzTarget{name: "FuzzOne", pkg: "./internal/a"}, time.Minute, 4)
	want := []string{"-run", "^$", "-fuzz", "^FuzzOne$", "-fuzztime", "1m0s", "-parallel", "4", "./internal/a"}
	if !slices.Equal(args, want) {
		t.Errorf("go %q, want %q", args, want)
	}
}

func TestTheDefaultNumberOfFuzzJobsIsAQuarterOfTheCPUsAtLeastOne(t *testing.T) {
	t.Parallel()
	for cpus, want := range map[int]int{1: 1, 3: 1, 4: 1, 8: 2, 15: 3, 16: 4} {
		if got := fuzzJobs(cpus); got != want {
			t.Errorf("fuzzJobs(%d) = %d, want %d", cpus, got, want)
		}
	}
}

// TestEachFuzzTargetGetsTheCPUsDividedByJobsAsWorkers: the workers each run
// may start are cpus / jobs, at least one, so the step's total effort is
// the CPUs, not jobs times the CPUs. It is a stated trade (wall time per
// target is the measure, not executions), held so it cannot drop unseen.
// Risk: a worker count that moves silently, up into oversubscription or
// down into a thinner fuzz.
func TestEachFuzzTargetGetsTheCPUsDividedByJobsAsWorkers(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ jobs, cpus, want int }{{3, 15, 5}, {4, 16, 4}, {1, 8, 8}, {4, 2, 1}} {
		var mu sync.Mutex
		var got []int
		run := func(_ context.Context, _ string, _ fuzzTarget, _ time.Duration, workers int) (string, error) {
			mu.Lock()
			got = append(got, workers)
			mu.Unlock()
			return "", nil
		}
		var out bytes.Buffer
		if ok, err := fuzzCommand(t.Context(), fuzzPlan{fuzzRepo(t), time.Minute, time.Second, c.jobs, c.cpus}, run, &out); err != nil || !ok {
			t.Fatalf("jobs %d, cpus %d: ok %v, err %v", c.jobs, c.cpus, ok, err)
		}
		if slices.IndexFunc(got, func(w int) bool { return w != c.want }) >= 0 || len(got) != 5 {
			t.Errorf("jobs %d, cpus %d: workers %v, want five of %d", c.jobs, c.cpus, got, c.want)
		}
	}
}
