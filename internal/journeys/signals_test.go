package journeys

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-files/lock"
	testenv "github.com/birdtable/go-testkit"
)

// bigRepo is the fixture with files enough that reading them takes a while:
// each is read, parsed and checked before the next, and the run looks at its
// context between files. The first file read is go-proc's hold, which keeps the
// run where it reads until the returned release is called, so an interrupt sent
// first always lands while the run is still reading, however loaded the host.
func bigRepo(t *testing.T) (repo string, release func()) {
	t.Helper()
	repo = genFixture(t, false)
	dir := filepath.Join(repo, "rulebook/language/go/rules")
	hold, release := testenv.HoldRead(t)
	testenv.Link(t, hold, filepath.Join(dir, "a-hold.yaml"))
	for i := range 600 {
		body := fmt.Sprintf("part: go\nconcern: c%d\n", i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("c%d.yaml", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo, release
}

// signalWait bounds one signal journey from the run's start to its end.
const signalWait = 30 * time.Second

// TestAnInterruptStopsTheRunCleanly: the system's interrupt (go-proc's
// Running.Interrupt: SIGINT on Unix, a break on Windows), sent once the run
// holds the lock, stops it through its context: it ends on failed, says it was
// stopped and how to run it again, exits 1, and leaves the lock free and no
// file aside (go.concurrency.4). Risk: a run killed mid-write that leaves a
// torn file or a held lock, which no in-process test can send.
func TestAnInterruptStopsTheRunCleanly(t *testing.T) {
	t.Parallel()
	stoppedBy(t, (*testenv.Running).Interrupt)
}

// TestATerminateStopsTheRunCleanly: SIGTERM (go-proc's Running.Terminate),
// sent once the run holds the lock, stops it as the interrupt does: the kit
// cancels the run's context on either. A system with no SIGTERM,
// Windows, answers Terminate with errors.ErrUnsupported and the test sends the
// interrupt instead, on that answer and not on the system's name: a skip is
// the kit's (testenv.Link, testenv.Modes), never t.Skip (tests.admission.5).
// Risk: a run that dies on SIGTERM with a torn file or a held lock, as a
// service manager's stop would leave it.
func TestATerminateStopsTheRunCleanly(t *testing.T) {
	// Not parallel: both signal journeys take the one lock in the state folder
	// the run's environment names, and a test that is not parallel ends before
	// the parallel ones resume, so the interrupt's run never meets this one's.
	stoppedBy(t, (*testenv.Running).Terminate)
}

// stoppedBy starts a run over bigRepo, waits until it holds the lock, ends it
// with send, and holds the run to the stop the journeys above describe. A send
// that is errors.ErrUnsupported is made with the interrupt.
func stoppedBy(t *testing.T, send func(*testenv.Running) error) {
	t.Helper()
	repo, release := bigRepo(t)
	// The program is built before the deadline starts: signalWait bounds the
	// run, from its start to its end, and not the compile, which a loaded
	// machine stretches. Path compiles on first use, so the
	// Start below finds it built, or reports the failed build in its error.
	bin := testenv.Build("cmd/gragas")
	_ = bin.Path()
	ctx, cancel := context.WithTimeout(t.Context(), signalWait)
	defer cancel()
	run, err := bin.Start(ctx, "gen", "--apply", "-C", repo, "--plain")
	if err != nil {
		t.Fatal(err)
	}
	// A run that never takes the lock, or never stops on the interrupt, is
	// killed at the deadline, so the test fails in its own words and not on the
	// package's timeout.
	stuck := time.AfterFunc(signalWait, func() { _ = run.Kill() })
	defer stuck.Stop()
	held := filepath.Join(appFolders(t, map[string]string{stateKey: os.Getenv(envName(stateKey)), homeKey: os.Getenv(envName(homeKey))}).State, "gragas.lock")
	for {
		// The run writes its note beside the lock only once it holds the lock
		// (host/fs's Lock.Take), so the note's presence is the hold. A Take of
		// this test's own could land between the run's create and its flock and
		// make the run exit busy, so none is made before the interrupt.
		if _, err := os.Stat(held + ".holder"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the run never held the lock within %s", signalWait)
		case <-time.After(2 * time.Millisecond):
		}
	}
	err = send(run)
	if errors.Is(err, errors.ErrUnsupported) {
		// No SIGTERM here (Windows): the run is still held and untouched, and
		// the interrupt this system has ends it, so the stop below is measured.
		err = run.Interrupt()
	}
	if err != nil {
		t.Fatal(err)
	}
	// The signal reaches the run's context a moment after it is sent, and no
	// test can see from outside when. The run says so once it has, so the
	// hold is let go only after that line: a run that read on past the signal
	// would end on findings, not on the stop. A run that never
	// says it is killed at the deadline, and the stream then ends.
	heard := false
	for sc := bufio.NewScanner(run.Stderr()); sc.Scan(); {
		if strings.Contains(sc.Text(), "stopping on a signal") {
			heard = true
			break
		}
	}
	if !heard {
		release()
		t.Fatalf("the run never said it heard the signal\n%s", run.Wait().Out())
	}
	release()
	res := run.Wait()
	all := res.Out()
	if res.Code != 1 {
		t.Fatalf("ended %d, want exit 1 (killed at %s: a code of -1)\n%s", res.Code, signalWait, all)
	}
	if !strings.Contains(all, "failed     stopped by a signal") || !strings.Contains(all, "Run it again: `gragas gen --apply ") {
		t.Errorf("the run did not say it was stopped and how to go on:\n%s", all)
	}
	l, err := lock.Take(held, held+".holder")
	if err != nil {
		t.Errorf("the lock is still held after the run: %v", err)
	} else {
		_ = l.Release()
	}
	aside, err := filepath.Glob(filepath.Join(repo, "rulebook", "*", ".*.tmp"))
	if err != nil || len(aside) > 0 {
		t.Errorf("files left aside: %v %v", aside, err)
	}
}
