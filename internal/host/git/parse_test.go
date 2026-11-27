package git

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// A commit's object name and a blob's, for the seeds.
const (
	seedCommit = "0123456789abcdef0123456789abcdef01234567"
	seedBlob   = "89abcdef0123456789abcdef0123456789abcdef"
)

// heldPath fails t when p is a path the readers should never answer.
func heldPath(t *testing.T, p string) {
	t.Helper()
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsFunc(p, unicode.IsControl) || strings.Contains(p, `\`) {
		t.Fatalf("a path %q is empty, absolute, or holds a control character or a backslash", p)
	}
	for part := range strings.SplitSeq(p, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			t.Fatalf("a path %q has the part %q", p, part)
		}
	}
}

// heldOID fails t when s is not 40 or 64 lower-case hex digits.
func heldOID(t *testing.T, s string) {
	t.Helper()
	if len(s) != 40 && len(s) != 64 || strings.Trim(s, "0123456789abcdef") != "" {
		t.Fatalf("an object name %q is not 40 or 64 hex digits", s)
	}
}

// FuzzParseTree (tests.generated.1): whatever ls-tree prints, the reader
// does not panic, and each file it answers has a held path, an object name
// and a size of zero or more, each path after the one before it in git's
// order.
func FuzzParseTree(f *testing.F) {
	for _, seed := range []string{
		"100644 blob " + seedBlob + " 2\ta\x00100755 blob " + seedBlob + " 10\td/run.sh\x00",
		"120000 blob " + seedBlob + " 1\tlink\x00160000 commit " + seedCommit + " -\tsub\x00",
		"",
		"100644 blob " + seedBlob + " 2\ta",
		"100644 blob " + seedBlob + " 2\t../x\x00",
		"100644 blob " + seedBlob + " 2\t.git/config\x00",
		"100644 blob " + seedBlob + " 2\ta\x1b[31m\x00",
		"100644 blob " + seedBlob + " -1\ta\x00",
		"100644 blob " + seedBlob + " 02\ta\x00",
		"040000 tree " + seedBlob + " -\td\x00",
		"100644 commit " + seedBlob + " 2\ta\x00",
		"100644 blob " + seedBlob + strings.Repeat("0", 24) + " 2\ta\x00100644 blob " + seedBlob + " 2\tb\x00",
		"100644 blob " + strings.ToUpper(seedBlob) + " 2\ta\x00",
		"100644 blob " + seedBlob + " 2\t\xff\x00",
		"100644 blob " + seedBlob + " 2\tzz\x00120000 blob " + seedBlob + " 1\tld\x00",
		"100644 blob " + seedBlob + " 2\ta\x00100644 blob " + seedBlob + " 2\ta\x00",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		files, err := parseTree(out)
		if err != nil {
			if files != nil {
				t.Fatalf("files %+v beside %v", files, err)
			}
			return
		}
		for i, e := range files {
			heldPath(t, e.path)
			heldOID(t, e.oid.text)
			if e.size < 0 {
				t.Fatalf("a size %d", e.size)
			}
			if i > 0 && files[i-1].path >= e.path {
				t.Fatalf("%q after %q, out of git's order", e.path, files[i-1].path)
			}
		}
	})
}

// FuzzParseIndex (tests.generated.1): whatever ls-files prints, the reader
// does not panic; each file it answers has a held path and object name, at
// most once; an unmerged index answers no files.
func FuzzParseIndex(f *testing.F) {
	for _, seed := range []string{
		"100644 " + seedBlob + " 2 0\ta\x00100755 " + seedBlob + " 0 0\tb/c\x00",
		"100644 " + seedBlob + " 2 1\ta\x00100644 " + seedBlob + " 2 2\ta\x00100644 " + seedBlob + " 2 3\ta\x00",
		"160000 " + seedCommit + " - 0\tsub\x00120000 " + seedBlob + " 1 0\tl\x00",
		"",
		"100644 " + seedBlob + " 2 0\ta\x00100644 " + seedBlob + " 2 0\ta\x00",
		"100644 " + seedBlob + " 2 4\ta\x00",
		"100644 " + seedBlob + " 2 00\ta\x00",
		"100644 " + seedBlob + " 2 0\ta\nb\x00",
		"100644 " + seedBlob + " 2 0\t/abs\x00",
		"100644 " + seedBlob + " - 0\ta\x00",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		files, unmerged, err := parseIndex(out)
		if err != nil {
			if files != nil || unmerged {
				t.Fatalf("files %+v, unmerged %v beside %v", files, unmerged, err)
			}
			return
		}
		seen := map[string]bool{}
		for _, e := range files {
			heldPath(t, e.path)
			heldOID(t, e.oid.text)
			if seen[e.path] {
				t.Fatalf("the path %q twice", e.path)
			}
			seen[e.path] = true
		}
	})
}

// FuzzParseTags (tests.generated.1): whatever for-each-ref prints, the
// reader does not panic, and each tag it answers has a held name and the
// object name of a commit.
func FuzzParseTags(f *testing.F) {
	for _, seed := range []string{
		"refs/tags/v1\x00commit\x00" + seedCommit + "\x00\x00\nrefs/tags/v2\x00tag\x00" + seedBlob + "\x00commit\x00" + seedCommit + "\n",
		"refs/tags/release/v1\x00commit\x00" + seedCommit + "\x00\x00\n",
		"",
		"refs/tags/t\x00tree\x00" + seedBlob + "\x00\x00\n",
		"refs/tags/t\x00tag\x00" + seedBlob + "\x00tree\x00" + seedBlob + "\n",
		"refs/heads/main\x00commit\x00" + seedCommit + "\x00\x00\n",
		"refs/tags/\x00commit\x00" + seedCommit + "\x00\x00\n",
		"refs/tags/v1\x00commit\x00" + seedCommit + "\x00\x00",
		"refs/tags/v1\x00commit\x00" + seedCommit + "\n",
		"refs/tags/a\x1bb\x00commit\x00" + seedCommit + "\x00\x00\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		tags, err := parseTags(out)
		if err != nil {
			if tags != nil {
				t.Fatalf("tags %+v beside %v", tags, err)
			}
			return
		}
		for _, tag := range tags {
			heldPath(t, tag.Name)
			heldOID(t, tag.Commit)
		}
	})
}

// FuzzParseBases (tests.generated.1): whatever merge-base prints, the
// reader does not panic, and each base it answers is an object name of one
// length.
func FuzzParseBases(f *testing.F) {
	for _, seed := range []string{
		seedCommit + "\n", seedCommit + "\n" + seedBlob + "\n", "", seedCommit, seedCommit + "\n\n",
		seedCommit + strings.Repeat("0", 24) + "\n" + seedCommit + "\n", "-" + seedCommit[1:] + "\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		bases, err := parseBases(out)
		if err != nil {
			return
		}
		for _, b := range bases {
			heldOID(t, b.text)
			if len(b.text) != len(bases[0].text) {
				t.Fatalf("bases of two lengths: %+v", bases)
			}
		}
	})
}

// FuzzParseRev (tests.generated.1): whatever a caller hands as a rev, the
// check does not panic, and a rev it passes cannot be read as an option or
// as more than a name, and passes again at the slot.
func FuzzParseRev(f *testing.F) {
	for _, seed := range []string{
		"v1", "main", "release/v1.2", seedCommit, "-n", "--output=x", "v1..v2", "v1^", "v1~1", "HEAD@{1}", "v1:a",
		":/x", "a b", "a\x00b", "/abs", ".hidden", "x.lock", "x/", "a//b", "a/.b", "\x1b[31m", "é",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := parseRev(s)
		if err != nil {
			return
		}
		if v.text != s || v.kind != kindRev || !v.ok() {
			t.Fatalf("parseRev(%q) = %+v", s, v)
		}
		if strings.HasPrefix(s, "-") || strings.ContainsAny(s, " \t\n\x00:^~?*[\\") || strings.Contains(s, "..") || strings.Contains(s, "@{") {
			t.Fatalf("the rev %q passed and could be read as more than a name", s)
		}
		if strings.ContainsFunc(s, func(r rune) bool { return r > unicode.MaxASCII || unicode.IsControl(r) }) {
			t.Fatalf("the rev %q passed with a byte that is not printable ASCII", s)
		}
	})
}

// FuzzParseVersion (tests.generated.1): whatever git version prints, the
// reader does not panic, and a version it answers is one line of three
// numbers git printed.
func FuzzParseVersion(f *testing.F) {
	for _, seed := range []string{
		"git version 2.56.0\n", "git version 2.39.5 (Apple Git-154)\n", "git version 2.40.1.windows.1\n", "git version 2.45.0.rc1\n",
		"git version 2.56\n", "git version 2.56.0", "2.56.0\n", "git version 99999.0.0\n", "git version 2.56.0\nmore\n", "",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		v, err := parseVersion(out)
		if err != nil {
			return
		}
		if v.major < 0 || v.minor < 0 || v.patch < 0 || !strings.HasPrefix(string(out), "git version "+v.text) {
			t.Fatalf("parseVersion(%q) = %+v", out, v)
		}
	})
}

// FuzzParsePath (tests.generated.1): whatever path a setting or a caller
// hands, the check does not panic, and a path it passes is absolute,
// clean, UTF-8 and free of control characters, and passes again.
func FuzzParsePath(f *testing.F) {
	for _, seed := range []string{
		"/work/scratch/git", "/", "relative", "/a/../b", "/a/./b", "/a//b", "/a/", "/a\x00b", "/a\nb", "\x1b[31m", "/é", "",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := parsePath(s)
		if err != nil {
			if !errors.Is(err, errBadPath) {
				t.Fatalf("parsePath(%q) refused with %v, not a bad path", s, err)
			}
			return
		}
		if v.text != s || v.kind != kindPath || !v.ok() {
			t.Fatalf("parsePath(%q) = %+v", s, v)
		}
		if !filepath.IsAbs(s) || filepath.Clean(s) != s || !utf8.ValidString(s) || strings.ContainsFunc(s, unicode.IsControl) {
			t.Fatalf("the path %q passed and is not absolute, clean, UTF-8 and free of control characters", s)
		}
		if again, err := parsePath(v.text); err != nil || again.text != v.text {
			t.Fatalf("the path %q passed once and not again: %v", s, err)
		}
	})
}

// FuzzParseTag (tests.generated.1): whatever name a caller hands as a tag,
// the check does not panic, and a name it passes is a rev parseRev passes
// that names one tag: not HEAD, not @, and no ref of another kind.
func FuzzParseTag(f *testing.F) {
	for _, seed := range []string{"v1", "v1.2.0", "release/v1", "HEAD", "@", "refs/heads/main", "refs/tags/v1", "-n", "v1..v2", "v1^{}", seedCommit} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := parseTag(s)
		if err != nil {
			return
		}
		if v.text != s || v.kind != kindTag || !v.ok() {
			t.Fatalf("parseTag(%q) = %+v", s, v)
		}
		if _, err := parseRev(s); err != nil {
			t.Fatalf("the tag %q passed, and parseRev refuses it: %v", s, err)
		}
		if s == "HEAD" || s == "@" || strings.HasPrefix(s, "refs/") {
			t.Fatalf("the tag %q passed and names no tag", s)
		}
	})
}

// FuzzParseSource (tests.generated.1): whatever a caller hands as where a
// release is fetched from, the check does not panic. A source it passes
// is a path parsePath passes, with no transport helper's ::, or a host URL
// that parseURL passes as it is; a host source it refuses is
// refused as no host URL, never as a path.
func FuzzParseSource(f *testing.F) {
	for _, seed := range []string{"/repo/mirror.git", "https://example.org/r.git", "ssh://host/r.git", "ssh://git@host:22/r.git", "https://u@h/r", "ext::sh -c x", "/a::b", "host:path", "relative", "file:///repo/r.git", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := parseSource(s)
		if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "ssh://") {
			if err != nil {
				if !errors.Is(err, errBadURL) {
					t.Fatalf("parseSource(%q): %v, want refused as no host URL", s, err)
				}
				return
			}
			if v.kind != kindURL || v.text != s || !v.ok() {
				t.Fatalf("the host source %q passed as %+v", s, v)
			}
			return
		}
		if err != nil {
			return
		}
		if p, perr := parsePath(s); perr != nil || p.text != v.text || strings.Contains(s, "::") {
			t.Fatalf("the source %q passed as %+v, and is no path or holds ::", s, v)
		}
	})
}
