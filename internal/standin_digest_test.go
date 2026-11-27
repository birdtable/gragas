package internal

// The stand-in's digest (gragas#2041): standin.wasm is never committed, so the tree holds its
// sha256 in gate/standin.digest, and the push builds the module from the source in the tree with the one
// builder (configtest.StandinWasm: the pinned Go, -trimpath -buildvcs=false -ldflags=-buildid=) and holds
// the two equal. The member `standin` runs these with -count=1, since Go's test cache keys on the files a
// test reads and not on the packages the stand-in imports or the toolchain that builds it.

import (
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/digest"
)

// standinDigestFile is the tree's record of the module's digest: comment lines, then one line in the
// prefixed form of internal/digest.
const standinDigestFile = "gate/standin.digest"

// heldStandinDigest is the digest of text, the content of the file: its one line that is no comment.
// A file with none, or with two, holds none.
func heldStandinDigest(text string) (string, bool) {
	var lines []string
	for l := range strings.SplitSeq(text, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 || !strings.HasPrefix(lines[0], digest.Prefix) || len(lines[0]) != len(digest.Prefix)+64 {
		return "", false
	}
	return lines[0], true
}

// standinDigestProblems holds the digest of built, the bytes a build of the source gave, to the file's
// text, and says both digests on a difference.
func standinDigestProblems(built []byte, file string) []string {
	held, ok := heldStandinDigest(file)
	if !ok {
		return []string{standinDigestFile + " holds no one line of " + digest.Prefix + " and 64 hex digits"}
	}
	if got := digest.Prefixed(built); got != held {
		return []string{"standin.wasm built from the tree is " + got + ", and " + standinDigestFile + " holds " + held +
			": when the source or the Go pin moved on purpose, write the first in the file"}
	}
	return nil
}

// TestTheStandinModuleIsTheOneTheTreeHolds is the member's first check: the module the source builds to
// is the digest the tree holds, red on any difference. It builds twice, in two folders, so a build that
// does not repeat is red here and not at the next person's machine.
func TestTheStandinModuleIsTheOneTheTreeHolds(t *testing.T) {
	t.Parallel()
	built := configtest.StandinWasm(t)
	t.Logf("standin.wasm digest: %s", digest.Prefixed(built))
	for _, p := range standinDigestProblems(built, read(t, standinDigestFile)) {
		t.Error(p)
	}
	if again := configtest.StandinWasm(t); digest.Prefixed(again) != digest.Prefixed(built) {
		t.Errorf("two builds of the same source gave %s and %s: the build does not repeat, so no digest can be held", digest.Prefixed(built), digest.Prefixed(again))
	}
}

// TestAChangedStandinSourceIsRedAgainstTheHeldDigest plants the change the member exists for: the
// stand-in's source changed (one answer's text, through go build's overlay, the tree untouched) while
// the digest the tree holds was not written again. The clean control is the tree's own source, green
// against the file; each other plant is a file that holds no digest.
func TestAChangedStandinSourceIsRedAgainstTheHeldDigest(t *testing.T) {
	t.Parallel()
	file := read(t, standinDigestFile)
	if p := standinDigestProblems(configtest.StandinWasm(t), file); len(p) != 0 {
		t.Fatalf("the clean control is red: %v", p)
	}
	changed := configtest.StandinWasmChanged(t, "answer.go", "the standin serves source only", "the standin serves sources only")
	got := strings.Join(standinDigestProblems(changed, file), "\n")
	for _, want := range []string{digest.Prefixed(changed), "holds " + func() string { h, _ := heldStandinDigest(file); return h }()} {
		if !strings.Contains(got, want) {
			t.Errorf("a changed source with the digest not written again says %q, want %q in it", got, want)
		}
	}
	for name, text := range map[string]string{
		"no line":      "# only a comment\n",
		"two lines":    digest.Prefixed([]byte("a")) + "\n" + digest.Prefixed([]byte("b")) + "\n",
		"no prefix":    strings.TrimPrefix(digest.Prefixed([]byte("a")), digest.Prefix) + "\n",
		"a short hash": digest.Prefix + "abcd\n",
	} {
		if len(standinDigestProblems(changed, text)) == 0 {
			t.Errorf("a digest file with %s is not red", name)
		}
	}
}
