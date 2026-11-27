package git_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/port/contracttest"
	"golang.org/x/crypto/ssh"
)

// The ssh fetch against a server in this process, on the loopback TCP:
// the machine's ssh runs as the adapter runs it, and
// the other end is an x/crypto/ssh server that answers an exec request for
// git-upload-pack by running git's own upload-pack on the repository, so no
// sshd is needed and nothing here builds for one system alone. The adapter
// hands ssh no key of its own (-F none, BatchMode), so this server takes a
// client with no key; the fetch with a key held by an agent is
// remote_ssh_agent_test.go's, on every system.

// sshServer is a loopback ssh server and the host key it answers with.
type sshServer struct {
	port string
	host ssh.PublicKey
}

// known is the known_hosts line of the server's host key.
func (s sshServer) known() string { return knownLineOf(s.host, s.port) }

// knownLineOf is the known_hosts line of key for the loopback at port.
func knownLineOf(key ssh.PublicKey, port string) string {
	return "[127.0.0.1]:" + port + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + "\n"
}

// newKey is an ed25519 key made for the test: its public half and its signer.
func newKey(t *testing.T) (ssh.PublicKey, ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return sp, signer, priv
}

// program is the machine's program name, by its resolved path; none fails t.
func program(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("this test runs the machine's %s, and there is none: %v", name, err)
	}
	return resolved(t, p)
}

// serveSSH starts a loopback ssh server for the repository at repo and stops
// it when t ends. With a nil key it takes any client; with one, only a client
// that proves it.
func serveSSH(t *testing.T, repo string, key ssh.PublicKey) sshServer {
	t.Helper()
	ln := loopback(t)
	host, hostSigner, _ := newKey(t)
	cfg := &ssh.ServerConfig{}
	cfg.AddHostKey(hostSigner)
	if key == nil {
		cfg.NoClientAuth = true
	} else {
		cfg.PublicKeyCallback = func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(k.Marshal(), key.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("the key is not the test's")
		}
	}
	var wg sync.WaitGroup
	var logs syncBuf
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
		if t.Failed() {
			t.Logf("the ssh server said:\n%s", logs.String())
		}
	})
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() { serveConn(t, conn, cfg, repo, &logs) })
		}
	})
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return sshServer{port: port, host: host}
}

// sourcePath is the path every test URL carries. A native path never goes into
// the URL: a Windows folder has a drive colon the adapter refuses in a URL
// path, and a Linux folder may hold one too. The server answers an exec of
// exactly this name with the repository it was started for.
const sourcePath = "/source.git"

// serveConn answers one ssh connection: each session channel's exec request
// for git-upload-pack of repo runs git's upload-pack on it.
func serveConn(t *testing.T, conn net.Conn, cfg *ssh.ServerConfig, repo string, logs *syncBuf) {
	t.Helper()
	defer func() { _ = conn.Close() }()
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		logs.write("handshake: " + err.Error() + "\n")
		return
	}
	defer func() { _ = sc.Close() }()
	go ssh.DiscardRequests(reqs)
	var wg sync.WaitGroup
	defer wg.Wait()
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			logs.write("accept: " + err.Error() + "\n")
			continue
		}
		wg.Go(func() { serveSession(t, ch, requests, repo, logs) })
	}
}

// serveSession answers the requests of one session channel.
func serveSession(t *testing.T, ch ssh.Channel, requests <-chan *ssh.Request, repo string, logs *syncBuf) {
	t.Helper()
	defer func() { _ = ch.Close() }()
	for r := range requests {
		if r.Type != "exec" {
			_ = r.Reply(false, nil)
			continue
		}
		var p struct{ Command string }
		if err := ssh.Unmarshal(r.Payload, &p); err != nil || p.Command != "git-upload-pack '"+sourcePath+"'" {
			logs.write("refused exec " + p.Command + "\n")
			_ = r.Reply(false, nil)
			continue
		}
		_ = r.Reply(true, nil)
		cmd := exec.CommandContext(t.Context(), "git")
		cmd.Args = append(cmd.Args, "upload-pack", repo)
		cmd.Env = []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "LC_ALL=C"}
		cmd.Stdout, cmd.Stderr = ch, ch.Stderr()
		in, err := cmd.StdinPipe()
		if err != nil {
			logs.write("stdin: " + err.Error() + "\n")
			return
		}
		go func() {
			_, _ = io.Copy(in, ch)
			_ = in.Close()
		}()
		code := uint32(0)
		if err := cmd.Run(); err != nil {
			code = 1
			logs.write("upload-pack: " + err.Error() + "\n")
		}
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
		return
	}
}

// syncBuf is a buffer several connections write to at once.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) write(text string) { _, _ = s.Write([]byte(text)) }

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// fetchesTheRelease: with the server's host key in the account's known_hosts,
// the fetch from s through the machine's ssh is made once, and the tag holds
// the release's files. set changes the adapter's config before it is made.
func fetchesTheRelease(t *testing.T, s sshServer, w remoteWorld, set func(*git.Config)) {
	t.Helper()
	from := "ssh://127.0.0.1:" + s.port + sourcePath
	ssh := program(t, "ssh")
	writeFile(t, filepath.Join(w.home, ".ssh", "known_hosts"), s.known())
	g := w.hostAdapter(t, func(cfg *git.Config) {
		cfg.SSH = ssh
		if set != nil {
			set(cfg)
		}
	})
	root, err := g.Fetch(t.Context(), from, "v1")
	if err != nil {
		t.Fatalf("Fetch(%s): %v", from, err)
	}
	if got := fileSet(t, string(root)); !mapsEqual(got, contracttest.TheRelease()) {
		t.Errorf("the fetch holds %q, want %q", got, contracttest.TheRelease())
	}
	if rs := fetchRuns(g); len(rs) != 1 || rs[0].Verb != "fetch-tag-ssh" {
		t.Errorf("the fetch ran %s, want fetch-tag-ssh once", git.Shown(rs))
	}
}

// TestAFetchOverSSHFromALocalServer: through the machine's ssh and a loopback
// ssh server, a host whose key the account's known_hosts holds is fetched,
// and the tag holds the release's files. With no key for the host in
// known_hosts, or another key, the fetch is refused with the ssh-keyscan
// line, never fetched, and known_hosts is left as it was. Planted failure:
// StrictHostKeyChecking=accept-new in sshOptions (the unknown key is added and
// fetched).
func TestAFetchOverSSHFromALocalServer(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	s := serveSSH(t, w.src, nil)
	kh := filepath.Join(w.home, ".ssh", "known_hosts")
	ssh := program(t, "ssh")
	from := "ssh://127.0.0.1:" + s.port + sourcePath
	other, _, _ := newKey(t)
	for _, c := range []struct{ name, known string }{
		{"no key for the host", ""},
		{"another key", knownLineOf(other, s.port)},
	} {
		t.Run(c.name, func(t *testing.T) {
			writeFile(t, kh, c.known)
			g := w.hostAdapter(t, func(cfg *git.Config) { cfg.SSH = ssh })
			got, err := g.Fetch(t.Context(), from, "v1")
			r, ok := errors.AsType[*git.Refused](err)
			if !ok || !strings.Contains(r.Hint(), "ssh-keyscan -p "+s.port+" 127.0.0.1") {
				t.Errorf("Fetch = %q, %v; want refused with the ssh-keyscan line", got, err)
			}
			if b, err := os.ReadFile(kh); err != nil || string(b) != c.known {
				t.Errorf("known_hosts is %q, %v after the fetch, want %q", b, err, c.known)
			}
		})
	}
	fetchesTheRelease(t, s, w, nil)
}
