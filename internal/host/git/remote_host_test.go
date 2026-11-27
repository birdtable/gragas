package git_test

import (
	"bytes"
	"encoding/pem"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	"github.com/birdtable/go-proc/gitenv"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/git"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// The fetch from a host,
// against servers on the loopback alone: git's http-backend behind a TLS
// server with a CA of the test's own, handed to git by http.sslCAInfo in a
// test alone (git.WithCA), and an ssh that is a stand-in here and the
// ssh server in this process in remote_ssh_test.go.

// testUser and testSecret are what the test's server asks for and its
// helper answers: a placeholder, never a real credential.
const (
	testUser   = "gragas-test"
	testSecret = "test-only-placeholder"
)

// oddFolder is a folder name that holds a space, $, a backtick and ;, each
// of which the shell would read unquoted.
const oddFolder = "a b$c`d;e"

// hostAdapter is the adapter of w with set applied to its config, with the
// groups on the way to every program and file it names, and to the
// account's known_hosts, holding root and this account alone.
func (w remoteWorld) hostAdapter(t *testing.T, set func(*git.Config)) *git.Git {
	t.Helper()
	cfg := git.Config{Git: configtest.Git(t), Releases: w.rels, Home: w.home, Run: "run-1"}
	if set != nil {
		set(&cfg)
	}
	g, err := git.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// backend is git's http-backend serving every repository in root, run once
// per request as a CGI program with no config of this account's or the
// machine's: the request's method, path, query and headers go to it as CGI
// variables and its body on its standard input, and its header lines and
// body come back. It is written here rather than taken from net/http/cgi,
// which gosec blocklists, so suppressions.txt does not grow.
func backend(t *testing.T, root string) http.Handler {
	t.Helper()
	ctx := t.Context()
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		cmd := exec.CommandContext(ctx, "git", "http-backend")
		cmd.Dir, cmd.Stdin = root, r.Body
		cmd.Env = []string{
			"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GATEWAY_INTERFACE=CGI/1.1", "REMOTE_ADDR=127.0.0.1", "REQUEST_METHOD=" + r.Method, "PATH_INFO=" + r.URL.Path,
			"QUERY_STRING=" + r.URL.RawQuery, "CONTENT_TYPE=" + r.Header.Get("Content-Type"),
			"HTTP_CONTENT_ENCODING=" + r.Header.Get("Content-Encoding"), "GIT_PROTOCOL=" + r.Header.Get("Git-Protocol"),
		}
		if r.ContentLength >= 0 {
			cmd.Env = append(cmd.Env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
		}
		out, err := cmd.Output()
		head, body, found := bytes.Cut(out, []byte("\r\n\r\n"))
		if err != nil || !found {
			http.Error(rw, "http-backend failed", http.StatusInternalServerError)
			return
		}
		status := http.StatusOK
		for line := range strings.SplitSeq(string(head), "\r\n") {
			k, v, _ := strings.Cut(line, ": ")
			if k != "Status" {
				rw.Header().Add(k, v)
				continue
			}
			code, _, _ := strings.Cut(v, " ")
			if n, err := strconv.Atoi(code); err == nil {
				status = n
			}
		}
		rw.WriteHeader(status)
		_, _ = rw.Write(body)
	})
}

// The four tests that serve a host on the loopback run in every tier: the
// gate no longer blocks the network of its members, so a
// listener refused fails the test like any other listen error.
// TestAFetchFromAnHTTPSHostIsTheTagsFiles,
// TestAnHTTPSFetchIsRefusedWhatItDoesNotPin,
// TestAnHTTPSFetchAsksTheNamedHelperAlone and
// TestAFetchOverSSHFromALocalServer.

// TestTheLoopbackTestsHaveNoEarlyEndOnARefusedListen: the files of the four
// tests above name no refusal of a permission (syscall.EPERM,
// syscall.EACCES, fs.ErrPermission, os.ErrPermission), so a listener the
// tier refuses fails them and is never logged and passed.
// Risk: an early end comes back, and the tier runs none of the four.
func TestTheLoopbackTestsHaveNoEarlyEndOnARefusedListen(t *testing.T) {
	t.Parallel()
	refusals := map[string][]string{"syscall": {"EPERM", "EACCES"}, "fs": {"ErrPermission"}, "os": {"ErrPermission"}}
	for _, name := range []string{"remote_host_test.go", "remote_ssh_test.go", "remote_ssh_agent_test.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && slices.Contains(refusals[pkg.Name], sel.Sel.Name) {
				t.Errorf("%s names %s.%s: a refused listen must fail the test, not end it early", name, pkg.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// loopback is a listener on 127.0.0.1. An error fails t.
func loopback(t *testing.T) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("a listener on 127.0.0.1: %v", err)
	}
	return ln
}

// hostOn serves h over TLS on the loopback, and answers its base URL and
// the file holding its certificate, the test's CA.
func hostOn(t *testing.T, h http.Handler) (base, ca string) {
	t.Helper()
	ln := loopback(t)
	srv := httptest.NewUnstartedServer(h)
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.StartTLS()
	t.Cleanup(srv.Close)
	ca = filepath.Join(filestest.Private(t), "ca.pem")
	writeFile(t, ca, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})))
	return srv.URL, ca
}

// needs is h behind basic auth for testUser and testSecret.
func needs(h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != testUser || p != testSecret {
			rw.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(rw, "who are you", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(rw, r)
	})
}

// helperIn writes a credential helper in a folder named oddFolder in dir,
// which answers testUser and testSecret to get and appends each action it
// is asked to mark, and answers its path.
func helperIn(t *testing.T, dir, mark string) string {
	t.Helper()
	folder := filepath.Join(dir, oddFolder)
	mkdir(t, folder)
	// a built program, not a shell script: it marks each action it is asked, and answers only get
	answer := "username=" + testUser + "\npassword=" + testSecret + "\n"
	return configtest.Fake(t, folder, "helper",
		configtest.Step{Write: mark, First: true, Append: true},
		configtest.Step{OnlyFirst: "get"},
		configtest.Out(answer))
}

// fetchRuns is the runs of g whose verb starts fetch-tag.
func fetchRuns(g *git.Git) []git.Ran {
	var out []git.Ran
	for _, r := range g.Ran() {
		if strings.HasPrefix(r.Verb, "fetch-tag") {
			out = append(out, r)
		}
	}
	return out
}

// TestAFetchFromAnHTTPSHostIsTheTagsFiles: a tag fetched from a loopback
// https host holds the release's files, with the account's own git config
// and home unread: a ~/.gitconfig that rewrites the URL to a closed port
// and turns the certificate check off, and a ~/.netrc and ~/_netrc that
// answer the host, change nothing, and a host that asks for a credential gets none.
// Every run of the fetch, the cache's init with it, runs with
// GIT_CONFIG_GLOBAL at the null device. Planted failures: noHome off the
// https row (curl answers the locked host from ~/.netrc); noAccount off it
// (the run's environment; with gitenv.NoHome git would not find
// ~/.gitconfig either, so the helper row, which keeps the home, is where
// the rewrite shows it).
func TestAFetchFromAnHTTPSHostIsTheTagsFiles(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	base, ca := hostOn(t, backend(t, w.base))
	locked, lockedCA := hostOn(t, needs(backend(t, w.base)))
	writeFile(t, filepath.Join(w.home, ".gitconfig"), "[url \"https://127.0.0.1:1/\"]\n\tinsteadOf = "+base+"/\n[http]\n\tsslVerify = false\n")
	host := strings.TrimPrefix(locked, "https://")
	host, _, _ = strings.Cut(host, ":")
	// curl reads _netrc too on Windows, so both answer the locked host.
	for _, n := range []string{".netrc", "_netrc"} {
		writeFile(t, filepath.Join(w.home, n), "machine "+host+" login "+testUser+" password "+testSecret+"\n")
		if err := os.Chmod(filepath.Join(w.home, n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	g := git.WithCA(w.hostAdapter(t, nil), ca)
	root, err := g.Fetch(t.Context(), base+"/src", "v1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := fileSet(t, string(root)); !mapsEqual(got, contracttest.TheRelease()) {
		t.Errorf("the fetch holds %q, want %q", got, contracttest.TheRelease())
	}
	for _, r := range g.Ran() {
		if (r.Verb == "init-cache" || strings.HasPrefix(r.Verb, "fetch-tag")) && !slices.Contains(r.Env, git.WantGlobal(t)) {
			name, _, _ := strings.Cut(git.WantGlobal(t), "=")
			t.Errorf("%s ran without the %s go-proc's NoGlobalConfig gives, so it reads the account's git config", r.Verb, name)
		}
	}
	// the account's home, as gitenv.Home hands it, is not the fetch's
	account, err := gitenv.Home(w.home)
	if err != nil {
		t.Fatal(err)
	}
	if rs := fetchRuns(g); len(rs) != 1 || rs[0].Verb != "fetch-tag-https" || slices.Contains(rs[0].Env, account[0]) {
		t.Errorf("the fetch ran %s, want fetch-tag-https once, with no home of the account's", git.Shown(rs))
	}
	g = git.WithCA(w.hostAdapter(t, nil), lockedCA)
	if got, err := g.Fetch(t.Context(), locked+"/src", "v1"); err == nil || !isFailed(err) {
		t.Errorf("a fetch with no helper from a host that asks for a credential = %q, %v; want failed", got, err)
	}
}

// TestAnHTTPSFetchIsRefusedWhatItDoesNotPin: with no CA of the test's own,
// the host's certificate is not trusted; a redirect, even to the same
// host, is not followed. Each fails the fetch and leaves no release in
// the cache. Planted failures: the CA handed when unset; followRedirects
// off the https row.
func TestAnHTTPSFetchIsRefusedWhatItDoesNotPin(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	back := backend(t, w.base)
	// git's first request of /moved is its listing of refs, sent on to
	// /src's, a fixed target on the same host
	base, ca := hostOn(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved/info/refs" {
			http.Redirect(rw, r, "/src/info/refs?service=git-upload-pack", http.StatusFound)
			return
		}
		back.ServeHTTP(rw, r)
	}))
	if _, err := git.WithCA(w.hostAdapter(t, nil), ca).Fetch(t.Context(), base+"/src", "v1"); err != nil {
		t.Fatalf("the fetch with the CA: %v", err)
	}
	for _, c := range []struct {
		name, from string
		ca         string
	}{
		{"no CA", base + "/src", ""},
		{"a redirect", base + "/moved", ca},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := w
			w.rels = filepath.Join(filestest.Private(t), "releases")
			g := w.hostAdapter(t, nil)
			if c.ca != "" {
				g = git.WithCA(g, c.ca)
			}
			if got, err := g.Fetch(t.Context(), c.from, "v1"); err == nil || !isFailed(err) {
				t.Errorf("Fetch(%s) = %q, %v; want failed", c.from, got, err)
			}
			entries, err := os.ReadDir(w.rels)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".git") {
					t.Errorf("the cache holds %s after a failed fetch", e.Name())
				}
			}
		})
	}
}

// TestAnHTTPSFetchAsksTheNamedHelperAlone: a host that asks for a
// credential is answered by the helper programs.credential names, at a path
// whose folder holds a space, $, a backtick and ;, which reaches it
// unchanged; with no helper named, the fetch fails and the helper is never
// asked. The helper's line clears every helper before it. The account's
// ~/.gitconfig, which rewrites the URL to a closed port and names a helper
// of its own, is not read, and its helper never asked. Planted failures:
// the helper's path handed to the shell unquoted; noAccount off the helper
// row (the rewrite goes to port 1).
func TestAnHTTPSFetchAsksTheNamedHelperAlone(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	base, ca := hostOn(t, needs(backend(t, w.base)))
	mark := filepath.Join(w.base, "asked")
	helper := helperIn(t, w.base, mark)
	ownMark := filepath.Join(w.base, "own-asked")
	own := configtest.Fake(t, w.base, "own-helper", configtest.Step{Write: ownMark, First: true, Append: true})
	// the path in git's config syntax, where \ and " are escaped, so the
	// file is one git reads wherever the path holds a backslash
	ownValue := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(own)
	writeFile(t, filepath.Join(w.home, ".gitconfig"),
		"[url \"https://127.0.0.1:1/\"]\n\tinsteadOf = "+base+"/\n[credential]\n\thelper = !'"+ownValue+"'\n")
	defer func() {
		if b, err := os.ReadFile(ownMark); err == nil {
			t.Errorf("the account's own helper was asked: %q", b)
		}
	}()
	g := git.WithCA(w.hostAdapter(t, nil), ca)
	if got, err := g.Fetch(t.Context(), base+"/src", "v1"); err == nil || !isFailed(err) {
		t.Errorf("a fetch with no helper = %q, %v; want failed", got, err)
	}
	if b, err := os.ReadFile(mark); err == nil {
		t.Errorf("the helper was asked with none named: %q", b)
	}
	g = git.WithCA(w.hostAdapter(t, func(c *git.Config) { c.Credential = helper }), ca)
	root, err := g.Fetch(t.Context(), base+"/src", "v1")
	if err != nil {
		t.Fatalf("the fetch with the helper: %v", err)
	}
	if got := fileSet(t, string(root)); !mapsEqual(got, contracttest.TheRelease()) {
		t.Errorf("the fetch holds %q, want %q", got, contracttest.TheRelease())
	}
	if b, err := os.ReadFile(mark); err != nil || !strings.HasPrefix(string(b), "get\n") {
		t.Errorf("the helper was asked %q, %v; want get first", b, err)
	}
	rs := fetchRuns(g)
	if len(rs) != 1 || rs[0].Verb != "fetch-tag-https-helper" {
		t.Fatalf("the fetch ran %s, want fetch-tag-https-helper once", git.Shown(rs))
	}
	clear, named := slices.Index(rs[0].Argv, "credential.helper="), slices.Index(rs[0].Argv, "credential.helper=!'"+helper+"'")
	if clear < 0 || named < clear {
		t.Errorf("the fetch ran %q: want the helpers cleared, then the named one in single quotes", rs[0].Argv)
	}
}

// TestAShellPathThatCannotBeQuotedIsRefusedBeforeAnythingRuns: an ssh or
// credential setting that holds a single quote or a control character, or
// a link to a path that holds one, is refused, naming the setting, and
// nothing runs. Planted failure: the resolved path not checked again (the
// link case runs git).
func TestAShellPathThatCannotBeQuotedIsRefusedBeforeAnythingRuns(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	quote := filepath.Join(w.base, "it's")
	mkdir(t, quote)
	prog := filepath.Join(quote, "prog")
	filestest.WriteProgram(t, prog, []byte("prog\n"))
	ln := filepath.Join(w.base, "ln")
	link(t, prog, ln)
	writeFile(t, filepath.Join(w.home, ".ssh", "known_hosts"), "")
	cases := []struct {
		name, from, setting string
		set                 func(*git.Config, string)
	}{
		{"credential", "https://127.0.0.1:1/src", git.CredentialSetting, func(c *git.Config, p string) { c.Credential = p }},
		{"ssh", "ssh://127.0.0.1:1/src", git.SSHSetting, func(c *git.Config, p string) { c.SSH = p }},
	}
	for _, c := range cases {
		for _, p := range []struct{ name, path string }{{"a quote", prog}, {"a newline", w.base + "/a\nb"}, {"a link to a quote", ln}} {
			t.Run(c.name+"/"+p.name, func(t *testing.T) {
				g := w.hostAdapter(t, func(cfg *git.Config) { c.set(cfg, p.path) })
				if got, err := g.Fetch(t.Context(), c.from, "v1"); !isRefused(err) || !strings.Contains(err.Error(), c.setting) {
					t.Errorf("Fetch with %s at %q = %q, %v; want refused naming %s", c.setting, p.path, got, err, c.setting)
				}
				if len(g.Ran()) != 0 {
					t.Errorf("a refused setting ran %q", ran(g))
				}
			})
		}
	}
}

// standinSSH writes an ssh in a folder named oddFolder in dir that writes
// each of its arguments on a line of its own to args and its environment
// to env, prints stderr to its stderr and exits 255, as ssh does when it
// cannot connect, and answers its path: a built program, not a
// script one system's shell runs.
func standinSSH(t *testing.T, dir, args, env, stderr string) string {
	t.Helper()
	folder := filepath.Join(dir, oddFolder)
	mkdir(t, folder)
	return configtest.Fake(t, folder, "ssh",
		configtest.Step{Write: args, Args: true},
		configtest.Step{Write: env, Env: true},
		configtest.Err(stderr+"\n"),
		configtest.Exit(255))
}

// sshEscaped is p as ssh is to read it inside UserKnownHostsFile's double
// quotes: \ as \\, " as \" and % as %%. It is the test's own, written from
// ssh_config(5) and shared with no product code, so a wrong escape in
// sshLine fails here; every Windows path holds a \.
func sshEscaped(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch r {
		case '\\', '"':
			b.WriteRune('\\')
		case '%':
			b.WriteRune('%')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestAnSSHFetchRunsTheNamedSSHWithItsOptions: git runs the ssh
// programs.ssh names, at a path whose folder holds a space, $, a backtick
// and ;, with exactly the fixed options and the account's known_hosts
// alone (GlobalKnownHostsFile=none), escaped as ssh reads it, the stall
// pins (ConnectTimeout, ServerAliveInterval, ServerAliveCountMax), and the
// agent's socket in its environment. ssh's own line for a host key
// known_hosts does not hold refuses the fetch with the ssh-keyscan line
// for the host and its port; any other line, and that line with more on
// it, fails it. Planted failures: an option dropped from sshOptions; the
// line matched as a part of a line.
func TestAnSSHFetchRunsTheNamedSSHWithItsOptions(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, stderr string
		refused      bool
	}{
		{"an unknown host key", "Host key verification failed.", true},
		{"no connection", "ssh: connect to host 127.0.0.1 port 2222: Connection refused", false},
		{"the line inside another", "not: Host key verification failed.", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
			kh := filepath.Join(w.home, ".ssh", "known_hosts")
			writeFile(t, kh, "")
			args, env := filepath.Join(w.base, "args"), filepath.Join(w.base, "env")
			ssh := standinSSH(t, w.base, args, env, c.stderr)
			agent := filepath.Join(w.base, "agent.sock")
			g := w.hostAdapter(t, func(cfg *git.Config) { cfg.SSH, cfg.Agent = ssh, agent })
			got, err := g.Fetch(t.Context(), "ssh://git@127.0.0.1:2222/src.git", "v1")
			r, isR := errors.AsType[*git.Refused](err)
			keyscan := isR && strings.Contains(r.Hint(), "ssh-keyscan -p 2222 127.0.0.1")
			switch {
			case c.refused && (!isRefused(err) || !keyscan):
				t.Errorf("Fetch = %q, %v; want refused with the ssh-keyscan line", got, err)
			case !c.refused && (!isFailed(err) || keyscan):
				t.Errorf("Fetch = %q, %v; want failed, with no ssh-keyscan line", got, err)
			}
			b, err := os.ReadFile(args)
			if err != nil {
				t.Fatalf("the stand-in ssh did not run: %v", err)
			}
			lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
			want := []string{
				"-F", "none", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
				"-o", "UserKnownHostsFile=\"" + sshEscaped(resolved(t, kh)) + "\"", "-o", "GlobalKnownHostsFile=none",
				"-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "-o", "UpdateHostKeys=no",
				"-o", "ConnectTimeout=30", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=4",
			}
			if len(lines) < len(want) || !slices.Equal(lines[:len(want)], want) {
				t.Errorf("ssh ran with %q, want it to start %q", lines, want)
			}
			for _, pin := range []struct{ opt, why string }{
				{"GlobalKnownHostsFile=none", "the system known_hosts would be trusted too"},
				{"ConnectTimeout=30", "a host that never takes the connection holds ssh past the deadline"},
				{"ServerAliveInterval=15", "a host that stalls holds ssh past the deadline"},
				{"ServerAliveCountMax=4", "a host that stalls is dropped later than a minute"},
			} {
				if i := slices.Index(lines, pin.opt); i < 1 || lines[i-1] != "-o" {
					t.Errorf("ssh ran with %q, without -o %s: %s", lines, pin.opt, pin.why)
				}
			}
			if !slices.Contains(lines, "git@127.0.0.1") || !slices.Contains(lines, "2222") {
				t.Errorf("ssh ran with %q, not to git@127.0.0.1 at 2222", lines)
			}
			e, err := os.ReadFile(env)
			if err != nil {
				t.Fatalf("the stand-in ssh wrote no environment: %v", err)
			}
			entries := strings.Split(string(e), "\n")
			switch {
			case slices.Contains(entries, "SSH_AUTH_SOCK="+agent):
			case slices.ContainsFunc(entries, func(kv string) bool { return strings.HasPrefix(kv, "SSH_AUTH_SOCK=") }):
				t.Errorf("ssh ran with SSH_AUTH_SOCK holding another value, want the agent's socket")
			default:
				t.Errorf("ssh ran without SSH_AUTH_SOCK, want the agent's socket")
			}
		})
	}
}

// TestAnSSHFetchIsNotReadyWithoutItsProgramOrKnownHosts: with no
// programs.ssh, or no known_hosts in the account's home, an ssh fetch is
// not ready, naming what is missing, and a known_hosts others can write is
// refused; nothing runs. Its group may write it as go-files' trusttest
// plants that, in the system's own form (mode bits or an access list), never
// by a mode only some systems keep. Planted failure:
// known_hosts not walked (the group-writable case runs git).
func TestAnSSHFetchIsNotReadyWithoutItsProgramOrKnownHosts(t *testing.T) {
	t.Parallel()
	w := newRemote(t, map[string]string{"rules.yaml": "one\n"})
	ssh := standinSSH(t, w.base, filepath.Join(w.base, "args"), filepath.Join(w.base, "env"), "")
	kh := filepath.Join(w.home, ".ssh", "known_hosts")
	from := "ssh://127.0.0.1/src"
	g := w.hostAdapter(t, nil)
	if _, err := g.Fetch(t.Context(), from, "v1"); !isNotReady(err) || !strings.Contains(err.Error(), git.SSHSetting) {
		t.Errorf("Fetch with no ssh: %v, want not ready naming %s", err, git.SSHSetting)
	}
	g = w.hostAdapter(t, func(c *git.Config) { c.SSH = ssh })
	_, err := g.Fetch(t.Context(), from, "v1")
	if nr, ok := errors.AsType[*git.NotReady](err); !ok || !strings.Contains(nr.Hint(), "ssh-keyscan 127.0.0.1`") {
		t.Errorf("Fetch with no known_hosts: %v, want not ready with the ssh-keyscan line", err)
	}
	writeFile(t, kh, "")
	trusttest.Loosen(t, kh, trusttest.GroupWrites)
	g = w.hostAdapter(t, func(c *git.Config) { c.SSH = ssh })
	if _, err := g.Fetch(t.Context(), from, "v1"); !isRefused(err) || !strings.Contains(err.Error(), "known_hosts") {
		t.Errorf("Fetch with a known_hosts its group can write: %v, want refused naming known_hosts", err)
	}
	if len(g.Ran()) != 0 {
		t.Errorf("a refused ssh fetch ran %q", ran(g))
	}
}

// TestAnAgentSocketThatIsNoCleanPathIsRefused: New refuses an agent socket
// that is not an absolute, clean path. Planted failure: the socket taken as
// it is.
func TestAnAgentSocketThatIsNoCleanPathIsRefused(t *testing.T) {
	t.Parallel()
	for _, agent := range []string{"agent.sock", "/a/../agent.sock", "-agent"} {
		if _, err := git.New(git.Config{Git: "/work/scratch/git", Run: "run-1", Agent: agent}); err == nil {
			t.Errorf("New with the agent %q: no error, want refused", agent)
		}
	}
}
