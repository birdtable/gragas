package git_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/host/git"
	"golang.org/x/crypto/ssh"
)

// treeEnds is how long the killed tree may take to be gone once the fetch has
// come back: the kernel drops a killed process in well under a second, so a
// minute is for a machine at a load of hundreds, not for the product.
const treeEnds = time.Minute

// TestAFetchAtItsDeadlineEndsItsSSH: a fetch whose ssh answers
// and never closes is ended at the fetch's deadline, failed as past it, and
// the ssh git started is gone when Fetch returns: git's whole tree ends with
// the run, not git alone. The machine's ssh reaches a loopback server that
// answers git-upload-pack with the first bytes of a packet line and nothing
// more, so git waits on the rest, and holds the connection open: only an
// ssh that ended closes it, which the server sees on every system, with no
// pid read and no signal sent. Planted failure: the run's tree
// not ended at the deadline (only git killed), so the ssh stays connected
// past the fetch.
func TestAFetchAtItsDeadlineEndsItsSSH(t *testing.T) {
	t.Parallel()
	// The fetch is a long verb, held to twelve times the wait; the runs
	// before it are real git's. A deadline that came before the ssh reached
	// the server's git-upload-pack says nothing of what the test asks, only
	// that the machine was slow to start a git, so the fetch is tried again
	// with twice the wait, and the assertions are made on the first that
	// reached it: no run is held to the speed of the machine.
	ssh := program(t, "ssh")
	for wait := time.Second; ; wait *= 2 {
		w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
		s, reached, gone := serveHanging(t)
		writeFile(t, filepath.Join(w.home, ".ssh", "known_hosts"), s.known())
		g := git.WithWait(w.hostAdapter(t, func(cfg *git.Config) { cfg.SSH = ssh }), wait)
		got, err := g.Fetch(t.Context(), "ssh://127.0.0.1:"+s.port+sourcePath, "v1")
		if !errors.Is(err, git.ErrDeadline) {
			t.Fatalf("Fetch = %q, %v; want failed at the deadline", got, err)
		}
		select {
		case <-reached:
		default:
			if wait > 30*time.Second {
				t.Fatalf("the ssh never reached the server's git-upload-pack, even given a wait of %s", wait)
			}
			continue
		}
		// The tree is killed before the run comes back; the kernel may take
		// a moment to drop a killed process and close its connection, so it
		// is waited for until treeEnds, a bound well past that, whatever the
		// load is: a planted failure must end the test, not hang it.
		select {
		case <-gone:
		case <-time.After(treeEnds):
			t.Fatalf("the ssh git started is still connected %s after the fetch failed", treeEnds)
		}
		return
	}
}

// serveHanging starts a loopback ssh server that takes any client, answers
// its exec with the first bytes of a packet line and never more, and holds
// each connection until its client closes it, or t ends. reached is closed
// at the first exec, and gone when a client that reached one closes its
// connection.
func serveHanging(t *testing.T) (s sshServer, reached, gone <-chan struct{}) {
	t.Helper()
	ln := loopback(t)
	host, hostSigner, _ := newKey(t)
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(hostSigner)
	r, g := make(chan struct{}), make(chan struct{})
	var once struct{ r, g sync.Once }
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				if hang(t.Context(), conn, cfg, func() { once.r.Do(func() { close(r) }) }) {
					once.g.Do(func() { close(g) })
				}
			})
		}
	})
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return sshServer{port: port, host: host}, r, g
}

// hang serves one connection as serveHanging says, calling execd at each
// exec, and answers whether its client reached an exec and then closed the
// connection itself: one closed because ctx ended is not.
func hang(ctx context.Context, conn net.Conn, cfg *ssh.ServerConfig, execd func()) bool {
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return false
	}
	go ssh.DiscardRequests(reqs)
	var hit atomic.Bool
	go func() {
		for nc := range chans {
			if nc.ChannelType() != "session" {
				_ = nc.Reject(ssh.UnknownChannelType, "only sessions")
				continue
			}
			ch, requests, err := nc.Accept()
			if err != nil {
				continue
			}
			go func() {
				for req := range requests {
					if req.Type != "exec" {
						_ = req.Reply(false, nil)
						continue
					}
					_ = req.Reply(true, nil)
					hit.Store(true)
					execd()
					// a packet line of 0x40 bytes, of which only the length comes
					_, _ = ch.Write([]byte("0040"))
				}
			}()
		}
	}()
	_ = sc.Wait()
	return hit.Load() && ctx.Err() == nil
}
