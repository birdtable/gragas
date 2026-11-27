package git

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/gitenv"
)

// TestEachHostFetchRowPinsItsOptions: each option a host
// fetch pins is in its row of the table, one subtest per row and option,
// so taking any one away fails its own subtest. Planted failure: each
// option in turn left off fetchHTTP or fetchSSH.
func TestEachHostFetchRowPinsItsOptions(t *testing.T) {
	t.Parallel()
	every := []string{"protocol.allow=never", "credential.helper=", "transfer.fsckObjects=true", "gc.auto=0", "maintenance.auto=false"}
	https := []string{
		"protocol.https.allow=always", "http.sslVerify=true", "http.proxy=", "http.followRedirects=false",
		"http.extraHeader=", "http.cookieFile=", "http.lowSpeedLimit=1000", "http.lowSpeedTime=30",
	}
	rows := map[string][]string{
		verbFetch:       slices.Concat(every, []string{"protocol.file.allow=always"}),
		verbFetchHTTPS:  slices.Concat(every, https),
		verbFetchHelper: slices.Concat(every, https, []string{"credential.helper=" + slotHelper}),
		verbFetchSSH:    slices.Concat(every, []string{"protocol.ssh.allow=always", "ssh.variant=ssh", "core.sshCommand=" + slotSSH}),
	}
	for name, pins := range rows {
		args := verbs()[name].args
		for _, pin := range pins {
			t.Run(name+"/"+pin, func(t *testing.T) {
				t.Parallel()
				i := slices.Index(args, pin)
				if i < 1 || args[i-1] != "-c" {
					t.Errorf("%s runs %q, without -c %s", name, args, pin)
				}
			})
		}
		// no protocol but the row's own is allowed
		for _, a := range args {
			if strings.HasPrefix(a, "protocol.") && strings.HasSuffix(a, ".allow=always") && !slices.Contains(pins, a) {
				t.Errorf("%s allows %s too", name, a)
			}
		}
	}
}

// TestAHostFetchReadsNothingOfTheAccountsButWhatItNames: the cache's init
// and every fetch run with GIT_CONFIG_GLOBAL at the null device; the https
// fetch with no helper with go-proc's gitenv.NoHome at the run's own empty
// folder, so curl reads no ~/.netrc; the ssh fetch alone is handed the
// agent's socket. Planted failures: noAccount off a row; noHome off the
// https row; agent on the https row. The home is given in the model's slash
// form and wanted as gitenv.Home hands it, in this system's form.
func TestAHostFetchReadsNothingOfTheAccountsButWhatItNames(t *testing.T) {
	t.Parallel()
	home := filepath.ToSlash(abs("/work/scratch/me"))
	empty := abs("/work/scratch/run/home")
	in, err := parsePath(empty)
	if err != nil {
		t.Fatal(err)
	}
	withHome, err := gitenv.Home(filepath.FromSlash(home))
	if err != nil {
		t.Fatal(err)
	}
	noHome, err := gitenv.NoHome(empty)
	if err != nil {
		t.Fatal(err)
	}
	g := &Git{cfg: Config{Home: home, Agent: "/work/scratch/agent.sock"}}
	for _, c := range []struct {
		name        string
		home, agent bool
	}{
		{verbInit, true, false},
		{verbFetch, true, false},
		{verbFetchHTTPS, false, false},
		{verbFetchHelper, true, false},
		{verbFetchSSH, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			v := verbs()[c.name]
			put := input{hooks: in}
			if v.noHome {
				put.home = in
			}
			env, err := g.env(v, put)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(env, wantGlobal(t)) {
				t.Errorf("%s runs with %q, which reads the account's git config", c.name, env)
			}
			want := map[bool][]string{true: withHome, false: noHome}[c.home]
			for _, kv := range want {
				if !slices.Contains(env, kv) {
					t.Errorf("%s runs with %q, want %s", c.name, env, kv)
				}
			}
			if got := slices.Contains(env, "SSH_AUTH_SOCK=/work/scratch/agent.sock"); got != c.agent {
				t.Errorf("%s runs with %q: the agent's socket handed is %v, want %v", c.name, env, got, c.agent)
			}
		})
	}
}

// TestAHostURLIsHeldToItsShape: parseURL takes https and ssh URLs of a
// host, an optional port and plain names, and a user on ssh alone, and
// nothing else. Planted failure: a user taken on https.
func TestAHostURLIsHeldToItsShape(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"https://example.com/book", "https://example.com:8443/a/b.git", "ssh://example.com/book",
		"ssh://git@example.com:2222/team/book.git", "ssh://a.b_c-d@127.0.0.1/x", "https://h/x~y+z",
	} {
		if v, err := parseURL(s); err != nil || v.kind != kindURL || !v.ok() {
			t.Errorf("parseURL(%q) = %+v, %v; want it taken", s, v, err)
		}
	}
	for _, s := range []string{
		"http://example.com/book", "https://me@example.com/book", "ssh://me:pw@example.com/b",
		"ssh://-o@example.com/b", "ssh://-oProxyCommand=x/b", "https://example.com:0/b", "https://example.com:65536/b",
		"https://example.com:08/b", "https://example.com:/b", "https://example.com", "https://example.com/",
		"https://example.com/a//b", "https://example.com/./b", "https://example.com/a/..", "https://example.com/a?b",
		"https://example.com/a#b", "https://example.com/a%2e", "https://exa mple.com/a", "https://ex_ample.com/a",
		"https://[::1]/a", "ssh://.example.com/a", "https://example.com/" + strings.Repeat("a", maxURL),
		"https://example.com/a\nb", "file:///a", "ext::sh",
	} {
		if v, err := parseURL(s); !errors.Is(err, errBadURL) {
			t.Errorf("parseURL(%q) = %+v, %v; want refused", s, v, err)
		}
	}
}

// boundary: maxURL
// TestAHostURLIsHeldToItsCap: a URL of maxURL bytes is taken, and one byte
// more is refused. Planted failure: > made >= in parseURL.
func TestAHostURLIsHeldToItsCap(t *testing.T) {
	t.Parallel()
	head := "https://example.com/"
	at := head + strings.Repeat("a", maxURL-len(head))
	if v, err := parseURL(at); err != nil || !v.ok() {
		t.Errorf("parseURL of %d bytes = %+v, %v; want it taken", len(at), v, err)
	}
	if v, err := parseURL(at + "a"); !errors.Is(err, errBadURL) {
		t.Errorf("parseURL of %d bytes = %+v, %v; want refused", len(at)+1, v, err)
	}
}

// abs is the slash path p, under the neutral root /work/scratch, as an
// absolute path of this system: as it is where it is absolute already, and
// otherwise (Windows, where a rooted path is not, and would be refused
// before the check a test means to reach) on drive C: with this system's
// separator. Nothing here is opened.
func abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return `C:` + filepath.FromSlash(p)
}

// TestAShellPathIsQuotedOrRefused: a path with a space, $, a backtick and ;
// goes in one pair of single quotes as it is; one with a single quote, a
// newline or another control character, or that is no absolute, clean
// path, is refused. Planted failure: the quote check dropped.
func TestAShellPathIsQuotedOrRefused(t *testing.T) {
	t.Parallel()
	odd := abs("/work/scratch/a b$c`d;e/prog")
	if q, err := quoted(odd); err != nil || q != "'"+odd+"'" {
		t.Errorf("quoted(%q) = %q, %v", odd, q, err)
	}
	for _, p := range []string{
		abs("/work/scratch/it's/prog"), abs("/work/scratch/a\nb"), abs("/work/scratch/a\tb"), abs("/work/scratch/a\x7fb"),
		"rel/prog", abs("/work/scratch/../prog"), "",
	} {
		if q, err := quoted(p); err == nil {
			t.Errorf("quoted(%q) = %q, want refused", p, q)
		}
	}
	for _, p := range []string{abs("/work/scratch/it's/prog"), abs("/work/scratch/a\nb")} {
		if _, err := quoted(p); !errors.Is(err, errNoQuote) {
			t.Errorf("quoted(%q): %v, want errNoQuote", p, err)
		}
	}
}

// TestTheShellLinesAreBuiltAndCheckedTheSameWay: a helper or ssh line
// helperLine or sshLine builds passes its check, with a known_hosts path
// that holds \, " and %; a known_hosts that holds ${ is refused; and a
// line with anything added, taken away or changed fails the check. Planted
// failure: sshOK taking any line that starts with a quoted path.
func TestTheShellLinesAreBuiltAndCheckedTheSameWay(t *testing.T) {
	t.Parallel()
	h, err := helperLine(abs("/work/scratch/a b$c`d;e/helper"))
	if err != nil || h.text != "!'"+abs("/work/scratch/a b$c`d;e/helper")+"'" || !helperOK(h.text) {
		t.Fatalf("helperLine = %+v, %v", h, err)
	}
	for _, bad := range []string{
		"!'" + abs("/work/scratch/h") + "' ; rm x", "'" + abs("/work/scratch/h") + "'", "!" + abs("/work/scratch/h"),
		"!'" + abs("/work/scratch/h"), "!'" + abs("/work/scratch/it's") + "'", "!'rel/h'", "",
	} {
		if helperOK(bad) {
			t.Errorf("helperOK(%q): taken", bad)
		}
	}
	ssh := abs("/work/scratch/bin/ssh")
	for _, kh := range []string{
		abs("/work/scratch/me/.ssh/known_hosts"), abs(`/work/scratch/a\b"c%d e/known_hosts`), abs("/work/scratch/%%h/known_hosts"),
	} {
		v, err := sshLine(ssh, kh)
		if err != nil || !sshOK(v.text) {
			t.Errorf("sshLine(%q) = %q, %v; want a line its check takes", kh, v.text, err)
			continue
		}
		if !strings.HasPrefix(v.text, "'"+ssh+"' -F none -o BatchMode=yes -o StrictHostKeyChecking=yes -o 'UserKnownHostsFile=\"") ||
			!strings.HasSuffix(v.text, "\"' -o GlobalKnownHostsFile=none -o ForwardAgent=no -o ClearAllForwardings=yes -o UpdateHostKeys=no"+
				" -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=4") {
			t.Errorf("sshLine(%q) = %q", kh, v.text)
		}
		for _, bad := range []string{v.text + " -o ProxyCommand=x", strings.Replace(v.text, "BatchMode=yes", "BatchMode=no", 1),
			strings.Replace(v.text, " -o ForwardAgent=no", "", 1), strings.Replace(v.text, "-F none", "-F /work/scratch/c", 1), v.text[:len(v.text)-1],
			strings.Replace(v.text, " -o GlobalKnownHostsFile=none", "", 1),
			strings.Replace(v.text, "GlobalKnownHostsFile=none", "GlobalKnownHostsFile=/work/scratch/ssh_known_hosts", 1),
			strings.Replace(v.text, " -o ConnectTimeout=30", "", 1), strings.Replace(v.text, "ServerAliveInterval=15", "ServerAliveInterval=0", 1),
			strings.Replace(v.text, " -o ServerAliveCountMax=4", "", 1)} {
			if sshOK(bad) {
				t.Errorf("sshOK(%q): taken", bad)
			}
		}
	}
	// the escape itself, written out, not read back through sshOK: where the
	// path is the slash path as written, the escaped text is written out too;
	// where abs moved it onto a drive, the same three rules are applied to it
	kh, want := abs(`/work/scratch/a\b"c%d/known_hosts`), `/work/scratch/a\\b\"c%%d/known_hosts`
	if kh != `/work/scratch/a\b"c%d/known_hosts` {
		want = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(kh)
	}
	if v, err := sshLine(ssh, kh); err != nil || !strings.Contains(v.text, ` -o 'UserKnownHostsFile="`+want+`"' `) {
		t.Errorf("sshLine(%q) = %q, %v; want UserKnownHostsFile=\"%s\"", kh, v.text, err, want)
	}
	if v, err := sshLine(ssh, abs("/work/scratch/${USER}/known_hosts")); err == nil {
		t.Errorf("sshLine with ${ = %q, want refused", v.text)
	}
	if v, err := sshLine(abs("/work/scratch/bin/it's"), abs("/work/scratch/me/known_hosts")); err == nil {
		t.Errorf("sshLine of a quoted ssh = %q, want refused", v.text)
	}
}
