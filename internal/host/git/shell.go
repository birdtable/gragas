package git

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// A fetch from a host:
// the URL it is handed, and the two lines git hands its shell, the ssh
// command and the credential helper. Each line is built here, by one
// function, from fixed words and paths the walk has checked, and checked
// again by run at its slot by building it once more from what it holds.

// errBadURL is a source that is no URL this adapter fetches from.
var errBadURL = errors.New("not a host URL this adapter fetches from")

// errNoQuote is a path that cannot go in one pair of single quotes: it
// holds a single quote, which ends them, or a control character.
var errNoQuote = errors.New("a path git hands its shell holds a single quote or a control character")

// maxURL is the longest URL a fetch takes.
const maxURL = 2048

// parseURL checks a host source on its parsed form: https://host[:port]/path
// or ssh://[user@]host[:port]/path. The host is letters, digits, dots and
// dashes, and starts with a letter or a digit; the port is 1 to 65535; only
// ssh names a user, of letters, digits, dots, underscores and dashes, not
// starting with a dash, and nothing names a password. The path has one or
// more names of letters, digits and . _ ~ + -, none of them . or .., and no
// query, fragment or percent sign, so git and the host read the same path.
func parseURL(s string) (value, error) {
	bad := func(why string) (value, error) {
		return value{}, fmt.Errorf("%w: %s %s", errBadURL, strconv.Quote(clip([]byte(s))), why)
	}
	if len(s) > maxURL {
		return bad("is longer than " + strconv.Itoa(maxURL) + " bytes")
	}
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok || (scheme != "https" && scheme != "ssh") {
		return bad("is not an https:// or ssh:// URL")
	}
	authority, path, ok := strings.Cut(rest, "/")
	if !ok {
		return bad("names no path")
	}
	hostPort := authority
	if user, after, has := strings.Cut(authority, "@"); has {
		switch {
		case scheme != "ssh":
			return bad("names a user, which only an ssh URL may")
		case strings.Contains(user, ":"):
			return bad("holds a password: a URL names where a secret is asked for, never the secret")
		case !nameOK(user, "._-"):
			return bad("names a user that is not letters, digits, dots, underscores and dashes, or starts with a dash")
		}
		hostPort = after
	}
	host, port, hasPort := strings.Cut(hostPort, ":")
	if !nameOK(host, ".-") || len(host) > 253 {
		return bad("names a host that is not letters, digits, dots and dashes, starting with a letter or a digit")
	}
	if hasPort {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return bad("names a port that is not a number from 1 to 65535")
		}
	}
	for name := range strings.SplitSeq(path, "/") {
		if name == "" || name == "." || name == ".." || strings.IndexFunc(name, func(r rune) bool { return !pathRune(r) }) >= 0 {
			return bad("has a path that is not names of letters, digits and . _ ~ + -, none of them empty, . or ..")
		}
	}
	return value{kind: kindURL, text: s}, nil
}

// nameOK is whether s is one or more ASCII letters, digits and the runes of
// extra, starting with a letter or a digit.
func nameOK(s, extra string) bool {
	if s == "" || !isAlnum(rune(s[0])) {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool { return !isAlnum(r) && !strings.ContainsRune(extra, r) }) < 0
}

func isAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func pathRune(r rune) bool { return isAlnum(r) || strings.ContainsRune("._~+-", r) }

// quoted is p in one pair of single quotes, for a line git hands its shell:
// inside them the shell reads every byte as it is, so a space, $, a
// backtick or ; reaches the program unchanged. A path that holds a single
// quote, which would end them, or a control character, or that is no
// absolute, clean path, is refused.
func quoted(p string) (string, error) {
	if unquotable(p) {
		return "", fmt.Errorf("%w: %q", errNoQuote, p)
	}
	if _, err := parsePath(p); err != nil {
		return "", err
	}
	return "'" + p + "'", nil
}

// unquotable is whether p holds a single quote or a control character, so
// it cannot go in one pair of single quotes.
func unquotable(p string) bool {
	return strings.ContainsRune(p, '\'') || strings.ContainsFunc(p, unicode.IsControl)
}

// helperLine is the credential.helper line of the helper at the checked
// path: git runs a helper that starts with ! as a shell line, with get
// after it, and the path is in one pair of single quotes.
func helperLine(helper string) (value, error) {
	q, err := quoted(helper)
	if err != nil {
		return value{}, err
	}
	return value{kind: kindHelper, text: "!" + q}, nil
}

// helperOK is whether line is a line helperLine builds.
func helperOK(line string) bool {
	p, ok := strings.CutPrefix(line, "!'")
	if !ok {
		return false
	}
	if p, ok = strings.CutSuffix(p, "'"); !ok {
		return false
	}
	v, err := helperLine(p)
	return err == nil && v.text == line
}

// sshOptions are the options of every ssh a fetch runs, after its path:
// no config file, so no ProxyCommand, ProxyJump or other host's settings;
// no prompt; a host key checked against the account's known_hosts alone
// (GlobalKnownHostsFile=none: from OpenSSH 8.5, ssh clears the list and
// opens no file of that name; an older ssh opens a file named none in the
// folder it runs in, the cache repository, this account's and 0700) and
// never added or changed there; no forwarding of the agent
// or of a port; a connection that is not made in 30 seconds, or a host
// that answers no keepalive for a minute, dropped, inside the fetch's
// deadline.
// The agent and the default identity files may authenticate; nothing else
// is set.
func sshOptions(knownHosts string) []string {
	return []string{
		"-F", "none", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
		"-o", "'UserKnownHostsFile=" + knownHosts + "'", "-o", "GlobalKnownHostsFile=none",
		"-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "-o", "UpdateHostKeys=no",
		"-o", "ConnectTimeout=30", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=4",
	}
}

// sshLine is the core.sshCommand line of the ssh at the checked path with
// the known_hosts file at kh: the path in one pair of single quotes, then
// the fixed options. ssh reads UserKnownHostsFile's value as a list of
// words with %-tokens and ${} variables, so kh goes to it in double quotes
// with \, " and % escaped, inside the shell's single quotes; a kh that holds
// ${, which ssh cannot read as it is, is refused.
func sshLine(ssh, kh string) (value, error) {
	q, err := quoted(ssh)
	if err != nil {
		return value{}, err
	}
	if _, err := quoted(kh); err != nil {
		return value{}, err
	}
	if strings.Contains(kh, "${") {
		return value{}, fmt.Errorf("%w: %q holds ${, which ssh reads as a variable", errNoQuote, kh)
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(kh)
	return value{kind: kindSSH, text: q + " " + strings.Join(sshOptions(`"`+esc+`"`), " ")}, nil
}

// sshOK is whether line is a line sshLine builds: the path and known_hosts
// file read back from it build it again, byte for byte.
func sshOK(line string) bool {
	rest, ok := strings.CutPrefix(line, "'")
	if !ok {
		return false
	}
	ssh, rest, ok := strings.Cut(rest, "'")
	if !ok {
		return false
	}
	_, kh, ok := strings.Cut(rest, `UserKnownHostsFile="`)
	if !ok {
		return false
	}
	var b strings.Builder
	for i := 0; i < len(kh); i++ {
		switch {
		case kh[i] == '\\' && i+1 < len(kh):
			i++
			b.WriteByte(kh[i])
		case kh[i] == '%' && i+1 < len(kh) && kh[i+1] == '%':
			i++
			b.WriteByte('%')
		case kh[i] == '"':
			v, err := sshLine(ssh, b.String())
			return err == nil && v.text == line
		default:
			b.WriteByte(kh[i])
		}
	}
	return false
}
