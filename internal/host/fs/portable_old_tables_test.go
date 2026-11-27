package fs_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/birdtable/go-files/confine"
)

// TestPortableRefusesWhatTheOldPathTablesRefused: the names the old tables of the apply
// package's unsafePath refuse, TestAPathAMacDiskFoldsToDotGitIsRefused's, and of the guard's newName, as
// TestANewNameThatFoldsToOneTheLoaderReadsIsRefusedOnEveryHost holds them, are each refused by go-files'
// confine.Portable, in at least one segment, with an error that matches fs.ErrInvalid. It is the proof that
// comes before the old functions go: Portable refuses at least what they refuse. The names are written out, as the
// pure packages that hold the old tables do not import go-files.
// Risk: the swap lets a name through that the old functions refused, which is a plan writing under .git or a
// new name a host reads as another.
func TestPortableRefusesWhatTheOldPathTablesRefused(t *testing.T) {
	t.Parallel()
	for _, p := range []string{
		// apply's TestAPathAMacDiskFoldsToDotGitIsRefused.
		".g\u200cit/config", ".git\u200d/config", ".\u200egit/x", ".g\u202ait/x", ".g\u206fit/x", "\ufeff.git/x",
		"rules/a\u200c.yaml",
		"GIT~1/config", "git~1/config", "x/Git~2", "git~1 /x", "GIT~12/x",
		// the ticket's planted names: .GIT, git~1, an HFS+ ignorable inside .git, an end in a dot.
		".GIT/x", "a/.Git/x", ".git./x", ".git /x", "x/git~1",
		// the guard's newName: an end in a dot or a space, a short name, a colon, a backslash, the reserved set.
		"book/history.jsonl.", "book/bindings/x.yaml ", "book/history.jsonl:x", "book/BINDIN~1/x.yaml",
		`book/bindings\x.yaml`, `a*b`, `a?b`, `a"b`, `a<b`, `a>b`, `a|b`,
		// apply's backslash, colon and NUL.
		"a\x00b", "a:b",
	} {
		refused := false
		for seg := range strings.SplitSeq(p, "/") {
			if _, err := confine.Portable(seg); err != nil {
				if !errors.Is(err, fs.ErrInvalid) {
					t.Errorf("%q: Portable's refusal of %q does not match fs.ErrInvalid: %v", p, seg, err)
				}
				refused = true
			}
		}
		if !refused {
			t.Errorf("%q: no segment is refused by Portable, which the old tables refuse", p)
		}
	}
}

// TestPortableGivesOneKeyToNamesADiskThatFoldsCaseReadsAsOne: the guard's
// "spelled otherwise" pairs and the tracked-files check's case pairs are each one key to Portable, so the callers
// that take their grouping from it group what the old folded and fold grouped; and the names that are two names
// stay two. The fake and the adapter agree on these, and on the three the contract case writes out.
// Risk: two tracked paths a disk reads as one are not grouped, or two a disk keeps apart are.
func TestPortableGivesOneKeyToNamesADiskThatFoldsCaseReadsAsOne(t *testing.T) {
	t.Parallel()
	one := [][]string{
		{"history.jsonl", "History.jsonl", "HISTORY.JSONL"},
		{"Readme.md", "README.md", "ReadMe.MD"},
		{"bindings", "Bindings"},
		{"\u00e9", "e\u0301", "\u00c9"},
		{"k", "K", "\u212a"},
		{"\u00df", "ss", "SS", "\u1e9e"},
	}
	for _, names := range one {
		first, err := confine.Portable(names[0])
		if err != nil {
			t.Fatalf("%q: %v", names[0], err)
		}
		for _, n := range names[1:] {
			if k, err := confine.Portable(n); err != nil || k != first {
				t.Errorf("%q: key %q, error %v, want the key %q of %q", n, k, err, first, names[0])
			}
		}
	}
	a, _ := confine.Portable("keep.md")
	b, _ := confine.Portable("keeps.md")
	if a == b {
		t.Errorf("keep.md and keeps.md are one key %q", a)
	}
}
