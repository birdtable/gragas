package journeys

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/sandbox"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The gate's network block is go-proc's sandbox,
// and go-proc's own suite proves it on each system, row by row: every way
// to the network, to another process's socket and to its terminal
// (go-proc's sandbox/cmd/netblock, proof_test.go and rows_linux_test.go).
// The journey proves only that gragas-dev's blocked child is that block:
// the built gragas-dev, started with blockWord, runs the test binary behind
// it as the start tests run the unpacked binary, and the test binary, as the
// row child, dials the loopback and a Unix socket on a path made outside. It
// names no system: what Check says is held to the Blocks go-proc gives, and
// a host that gives none refuses the child by name.

// blockWord is gragas-dev's hidden command that makes it the blocked child,
// internal/exit's BlockedWord: the journeys reach gragas-dev only as its
// binary, so the word is written here as the binary takes it.
const blockWord = "network-blocked"

// The row child's exits: the test binary, rerun as the helper, ends 0 when
// its test passes and 1 when it fails.
const (
	rowDone   = 0
	rowFailed = 1
)

// TestTheHelperRunsOnlyWhenRerun is the row child: the test binary rerun by
// testenv.Rerun, alone or behind the block. It dials one address and writes
// how it went to the file its first argument names. Args are the report
// file, then "dial", the network and the address. In a run go test started
// it is given nothing and does nothing.
func TestTheHelperRunsOnlyWhenRerun(t *testing.T) {
	t.Parallel() // in a rerun child this is the only test that runs
	args, ok := configtest.Helper()
	if !ok {
		return
	}
	if len(args) != 4 || args[1] != "dial" {
		t.Fatalf("the row child was given %q", args)
	}
	c, err := (&net.Dialer{}).DialContext(t.Context(), args[2], args[3])
	if err == nil {
		err = c.Close()
	}
	if err := os.WriteFile(args[0], []byte(outcome(err)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// outcome is how a row's dial went: ok, refused (a permission error, which a
// block gives as EPERM or EACCES by system and kernel), or the error.
func outcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, fs.ErrPermission):
		return "refused"
	}
	return "other " + err.Error()
}

// The Blocks go-proc's Check gives where it gives one, fixed here and never
// read from the host as the expectation: Linux closes every way out, and
// macOS's profile leaves signals and the service manager open
// (go-proc/sandbox's Check).
func theBlocksGiven() []sandbox.Block {
	everyWayClosed := sandbox.Block{Network: true, Daemons: true, Terminal: true, Signals: true, ServiceManager: true}
	profileOfMacOS := sandbox.Block{Network: true, Daemons: true, Terminal: true}
	return []sandbox.Block{everyWayClosed, profileOfMacOS}
}

// theBlockedChild opens what the blocked child says when it runs nothing.
const theBlockedChild = "gragas-dev " + blockWord + ": "

// gate is the built gragas-dev, and this test binary by its absolute path,
// the program it runs behind the block.
type gate struct{ dev, self string }

// buildGate is the built gragas-dev and this test binary.
func buildGate(t *testing.T) gate {
	t.Helper()
	dev := testenv.Build("cmd/gragas-dev").Path()
	if dev == "" {
		t.Fatalf("gragas-dev did not build:\n%s", configtest.GoSaid(testenv.Build("cmd/gragas-dev").Run().Stderr))
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return gate{dev: dev, self: self}
}

// row runs the row child, behind the block as a start test runs or with the
// network open, and returns whether it reported, what, what the command
// said, its exit code and the rerun's error (a child that failed is no error
// there, only a code).
func (g gate) row(t *testing.T, blocked bool, args ...string) (said string, ran bool, out string, code int, runErr error) {
	t.Helper()
	report := filepath.Join(t.TempDir(), "row")
	line := append([]string{report}, args...)
	var b []byte
	if blocked {
		code, b, runErr = configtest.RerunVia(t.Context(), []string{g.dev, blockWord, "--"}, line...)
	} else {
		code, b, runErr = configtest.Rerun(t.Context(), line...)
	}
	got, err := os.ReadFile(report)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return string(got), err == nil, string(b), code, runErr
}

// socketPath is a path for a Unix socket, short on every system, in a folder
// made for the test (testenv.ShortDir).
func socketPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testenv.ShortDir(t), "s.sock")
}

// TestTheBlockedChildRefusesTheLoopbackAndAPathSocket: behind gragas-dev's blocked child, a dial to the loopback is
// refused, and so is a Unix socket on a path made outside the block (EACCES
// from Landlock ABI 9, EPERM below it and on macOS), where the same dials
// with the network open answer, so a refusal is the block's and not the
// host's. What Check says the block closes is one of the Blocks go-proc
// gives. Where the host gives no block, the blocked child is refused by
// name, and the row child never runs.
//
// What it costs when this fails: a start test runs the unpacked binary with
// the network open, or behind a block that leaves a path socket answering.
func TestTheBlockedChildRefusesTheLoopbackAndAPathSocket(t *testing.T) {
	t.Parallel()
	g := buildGate(t)
	block, err := sandbox.Check(sandbox.Folders{}, "")
	if err != nil {
		if !errors.Is(err, sandbox.ErrNoBlock) {
			t.Fatalf("sandbox.Check: %v, want a block or ErrNoBlock", err)
		}
		said, ran, out, code, runErr := g.row(t, true, "dial", "tcp4", "127.0.0.1:1")
		if ran || runErr != nil || code != rowFailed || !strings.Contains(out, theBlockedChild) {
			t.Errorf("with no block here (%v), the blocked child ran %v (%q) and said %q, exit %d, %v; want it refused by name, and nothing run", err, ran, said, out, code, runErr)
		}
		return
	}
	if given := theBlocksGiven(); !slices.Contains(given, block) {
		t.Errorf("sandbox.Check says the block closes %+v, which is none of the Blocks go-proc gives: %+v", block, given)
	}
	refusedByBlock := []string{"refused"}
	lc := &net.ListenConfig{}
	path := socketPath(t)
	pathed, err := lc.Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pathed.Close() }()
	// Behind a block already, as a gate member may be, no inet socket is
	// made here: the dial goes to an address no listener holds, and its
	// open control is that outer block's refusal.
	dialTo, openDial := "127.0.0.1:1", refusedByBlock
	if tcp, lerr := lc.Listen(t.Context(), "tcp4", "127.0.0.1:0"); lerr == nil {
		defer func() { _ = tcp.Close() }()
		dialTo, openDial = tcp.Addr().String(), []string{"ok"}
	} else if !errors.Is(lerr, fs.ErrPermission) {
		t.Fatalf("a loopback listener here: %v; want it made, or refused by a block", lerr)
	} else {
		t.Log("behind a network block already: no inet socket is made here, so the loopback's open control is the outer block's refusal")
	}
	for _, r := range []struct {
		claim       string
		args        []string
		open, block []string
	}{
		{"a dial to the loopback", []string{"dial", "tcp4", dialTo}, openDial, refusedByBlock},
		{"a Unix socket on a path, made outside", []string{"dial", "unix", path}, []string{"ok"}, refusedByBlock},
	} {
		if said, ran, out, code, runErr := g.row(t, true, r.args...); !ran || code != rowDone || runErr != nil || !slices.Contains(r.block, said) {
			t.Errorf("%s, blocked: %q (ran %v, exit %d, %v: %s), want one of %q", r.claim, said, ran, code, runErr, out, r.block)
		}
		if said, ran, out, code, runErr := g.row(t, false, r.args...); !ran || code != rowDone || runErr != nil || !slices.Contains(r.open, said) {
			t.Errorf("%s, the open control: %q (ran %v, exit %d, %v: %s), want one of %q", r.claim, said, ran, code, runErr, out, r.open)
		}
	}
}
