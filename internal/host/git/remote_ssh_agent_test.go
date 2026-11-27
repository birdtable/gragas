package git_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-socket/listen"
	"github.com/birdtable/go-socket/peer"
	testenv "github.com/birdtable/go-testkit"
	"github.com/birdtable/gragas/internal/host/git"
	"golang.org/x/crypto/ssh/agent"
)

// The ssh fetch with the key held by an agent, on
// every system: the agent's address is what listen.Local gives for a socket
// named "agent" in a folder (a path here, a pipe where there are no Unix
// sockets), the address Config.Agent hands ssh as SSH_AUTH_SOCK. The agent is
// in this process (x/crypto/ssh/agent), and the server is remote_ssh_test.go's,
// set to take only the key the agent holds. The fetch with no key is
// remote_ssh_test.go's.

// agentFolder is a folder of its own for t's agent, removed after t:
// testenv's short folder, which is not under the test's own temp folder,
// since a folder under t.TempDir() is as long as the host's
// temp root and testenv's folders below it, which a stacked worktree can take
// past a socket's path. Which root is short on each system is go-testkit's to
// say.
func agentFolder(t *testing.T) string {
	t.Helper()
	return testenv.ShortDir(t)
}

// agentAddress is the address of the test's agent, from listen.Local in a
// folder of its own: go-socket refuses by name an address its system cannot
// hold, so no length is counted here.
func agentAddress(t *testing.T) string {
	t.Helper()
	addr, err := listen.Local(agentFolder(t), "agent")
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

// bindAgent binds the agent's address. Listen takes a ctx with a deadline, the
// span it waits on to prove no one else holds the address, so it has one.
func bindAgent(t *testing.T, addr string) (*listen.Listener, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	return listen.Listen(ctx, addr)
}

// TestTheAgentSocketIsInAFolderOfItsOwnNotTheTestsTemp: the agent's address is
// not under the test's own folder, whose path is as long as the host's temp
// root and the test's name, which a stacked worktree can take past what a
// socket's path holds, and the agent still answers. Which root
// is short on each system is go-testkit's to say, so no variable and no length
// is named here. Planted: agentFolder gives t.TempDir()
// (the address is under it).
func TestTheAgentSocketIsInAFolderOfItsOwnNotTheTestsTemp(t *testing.T) {
	t.Parallel()
	addr := agentAddress(t)
	if own := filepath.Dir(t.TempDir()); strings.HasPrefix(addr, own) {
		t.Errorf("the agent's address %s is under the test's folder %s, want a folder of its own", addr, own)
	}
	ln, err := bindAgent(t, addr)
	if err != nil {
		t.Fatalf("the agent's address %s: %v", addr, err)
	}
	_ = ln.Close()
}

// newAgentWorld starts an agent that holds one key, on a socket in
// agentFolder, and a loopback ssh server for w's repository that takes only
// that key; both stop when t ends. It answers the server and the socket.
func newAgentWorld(t *testing.T, w remoteWorld) (sshServer, string) {
	t.Helper()
	pub, _, priv := newKey(t)
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	sock := agentAddress(t)
	ln, err := bindAgent(t, sock)
	if err != nil {
		t.Fatalf("the agent's address %s: %v", sock, err)
	}
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
				defer func() { _ = conn.Close() }()
				_ = agent.ServeAgent(keyring, conn)
			})
		}
	})
	return serveSSH(t, w.src, pub), sock
}

// agentSSH is the ssh that reaches the test's agent, as programs.ssh would
// name it: the command go-socket's peer.AgentSSHCommand gives, which is a
// path, when it gives one, else the machine's ssh, which reads SSH_AUTH_SOCK's
// socket itself. Which ssh that is on which system is
// go-socket's to say, never this test's: no system is named here.
func agentSSH(t *testing.T) string {
	t.Helper()
	cmd, err := peer.AgentSSHCommand()
	if err != nil {
		t.Fatalf("the ssh that reaches the agent: %v", err)
	}
	if cmd == "" {
		return program(t, "ssh")
	}
	return resolved(t, cmd)
}

// fetchesWithTheAgent: with the host's key in the account's known_hosts, the
// fetch from s through the ssh that reaches the agent (agentSSH) and the
// agent at sock is made once, and the tag holds the release's files.
func fetchesWithTheAgent(t *testing.T, s sshServer, sock string, w remoteWorld) {
	t.Helper()
	ssh := agentSSH(t)
	fetchesTheRelease(t, s, w, func(cfg *git.Config) {
		cfg.SSH = ssh
		cfg.Agent = sock
	})
}

// TestAFetchOverSSHWithTheAgentsKey: through the ssh that reaches the agent
// and a loopback ssh server that takes only one key, held by an agent the
// adapter is given, the host is fetched and the tag holds the release's
// files. Planted failures: no SSH_AUTH_SOCK handed (the server takes no other
// key, and the fetch fails); the machine's ssh in agentSSH where go-socket
// names another (on Windows, Git for Windows' ssh cannot open the agent's
// pipe, and the fetch fails).
func TestAFetchOverSSHWithTheAgentsKey(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	s, sock := newAgentWorld(t, w)
	fetchesWithTheAgent(t, s, sock, w)
}
