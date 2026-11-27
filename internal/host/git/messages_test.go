package git

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// TestARefusedRangeRunsNoGit: an end of a range that is not a full object
// name, a ref name, a short name, capitals, an option, a range or nothing,
// is refused on its parsed form as a bad rev before any git runs, the
// version included. Planted failure: the ends left for git to judge.
func TestARefusedRangeRunsNoGit(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	g := program(t, standIn("2.56.0", "", nil)...)
	ends := []string{
		"v1", "main", "HEAD", standCommit[:12], "ABCDEF" + standCommit[6:], "--all", "-n1", "--output=/x",
		"^" + standCommit, standCommit + "..", standCommit + "\n", standCommit + "^{commit}", "",
	}
	for _, end := range ends {
		for _, pair := range [][2]string{{end, standCommit}, {standCommit, end}} {
			if msgs, err := g.Messages(t.Context(), root, pair[0], pair[1]); !errors.Is(err, errBadRev) || msgs != nil {
				t.Errorf("Messages(%q, %q) = %q, %v; want a bad rev and no list", pair[0], pair[1], msgs, err)
			}
		}
	}
	if ran := verbsRun(g); len(ran) != 0 {
		t.Errorf("a refused range ran %q, want nothing", ran)
	}
}

// TestARangeReachesGitAfterTheEndOfOptions: each end is checked as a commit
// of the repository by exactly its name, the range is handed to rev-list
// after --end-of-options, base as ^base, and each commit it lists is read
// as its object by name on standard input, never on the command line.
// Planted failure: the ends put before --end-of-options.
func TestARangeReachesGitAfterTheEndOfOptions(t *testing.T) {
	t.Parallel()
	g := program(t, standIn("2.56.0", "", nil)...)
	msgs, err := g.Messages(t.Context(), model.Root(filestest.Private(t)), standCommit, standCommit)
	if want := []model.Message{{Commit: standCommit, Text: "one\n"}}; err != nil || !slices.Equal(msgs, want) {
		t.Fatalf("Messages = %q, %v; want %q", msgs, err, want)
	}
	argv := map[string][]string{}
	for _, r := range g.Ran() {
		argv[r.Verb] = r.Argv
	}
	rng := argv[verbRange]
	end := slices.Index(rng, "--end-of-options")
	if end < 0 || !slices.Equal(rng[end+1:], []string{"^" + standCommit, standCommit, "--"}) {
		t.Errorf("rev-list ran as %q, want the range alone after --end-of-options", rng)
	}
	if objs := argv[verbCommits]; slices.ContainsFunc(objs, func(a string) bool { return strings.Contains(a, standCommit) }) {
		t.Errorf("cat-file ran as %q, want the commits on standard input alone", objs)
	}
	if n := strings.Count(strings.Join(verbsRun(g), " "), "resolve-commit"); n != 2 {
		t.Errorf("the ends were checked as commits %d times, want 2", n)
	}
}

// TestARangePastItsCapIsFailedNotCut: a range or its objects printed past
// the listing's cap fail Messages with the run id, never a part of the
// list. Planted failure: either verb given no cap.
func TestARangePastItsCapIsFailedNotCut(t *testing.T) {
	t.Parallel()
	in := standIn("2.56.0", "", nil)
	for name, c := range map[string]struct{ on, run, verb string }{
		"the range":   {onRange, "run-7", verbRange},
		"the objects": {onObjects, "run-7", verbCommits},
	} {
		g := program(t, with(t, in, c.on, Answer{Zeros: 4200000})...)
		msgs, err := g.Messages(t.Context(), model.Root(filestest.Private(t)), standCommit, standCommit)
		if f, ok := errors.AsType[*Failed](err); !ok || f.Run() != c.run || !errors.Is(err, errPastCap) || msgs != nil {
			t.Errorf("%s past its cap: %q, %v; want failed past the cap naming %s, and no list", name, msgs, err, c.run)
		}
		if got := verbsRun(g); len(got) == 0 || got[len(got)-1] != c.verb {
			t.Errorf("%s: the runs were %q, want %s last", name, got, c.verb)
		}
	}
}

// TestARangeThatPrintsWhatItDoesNotPrintIsFailed: a commit of the range that
// is no object name, of another length than the range's, listed twice or
// not ended; or an object that is not the commit asked for, not a commit,
// missing, of another size than its line gives, or followed by more: each
// fails Messages, never read in part. Planted failure: the lines read
// without the checks.
func TestARangeThatPrintsWhatItDoesNotPrintIsFailed(t *testing.T) {
	t.Parallel()
	in := standIn("2.56.0", "", nil)
	other := strings.Repeat("2", 40)
	for name, c := range map[string][2]string{
		"a short commit":            {onRange, "1111111\n"},
		"a commit of 64":            {onRange, strings.Repeat("1", 64) + "\n"},
		"one commit twice":          {onRange, standCommit + "\n" + standCommit + "\n"},
		"no newline at the end":     {onRange, standCommit},
		"a commit in capitals":      {onRange, "ABCDEF" + standCommit[6:] + "\n"},
		"another commit's object":   {onObjects, other + " commit 12\ntree x\n\none\n\n"},
		"a tree's object":           {onObjects, standCommit + " tree 12\ntree x\n\none\n\n"},
		"a missing object":          {onObjects, standCommit + " missing\n"},
		"a size past the bytes":     {onObjects, standCommit + " commit 13\ntree x\n\none\n\n"},
		"a size short of the bytes": {onObjects, standCommit + " commit 11\ntree x\n\none\n\n"},
		"more than was asked for":   {onObjects, standCommit + " commit 12\ntree x\n\none\n\nx"},
	} {
		g := program(t, with(t, in, c[0], Answer{Out: c[1]})...)
		if msgs, err := g.Messages(t.Context(), model.Root(filestest.Private(t)), standCommit, standCommit); !isFailed(err) || msgs != nil {
			t.Errorf("%s: %q, %v; want failed and no list", name, msgs, err)
		}
	}
}

// TestAMessageGitLogWouldReadOtherwiseIsHidden: a message holding a NUL, at which git log stops,
// or under a header that names an encoding other than UTF-8, from which git
// log re-codes it, is errHidden naming the commit, and the encoding for a
// header, and no list; the header
// alone is read for the encoding, and utf-8 in any case is UTF-8. Planted
// failure: stored answers the message whatever it holds.
func TestAMessageGitLogWouldReadOtherwiseIsHidden(t *testing.T) {
	t.Parallel()
	in := standIn("2.56.0", "", nil)
	object := func(body string) Answer {
		return Answer{Out: standCommit + " commit " + strconv.Itoa(len(body)) + "\n" + body + "\n"}
	}
	for name, body := range map[string]string{
		"a NUL in the message":     "tree x\n\nvisible line\n\x00HIDDEN\n",
		"a header of ISO-8859-1":   "tree x\nencoding ISO-8859-1\n\none\n",
		"a header of IBM037":       "tree x\nencoding IBM037\n\none\n",
		"a header of ISO-2022-JP":  "tree x\nencoding ISO-2022-JP\n\none\n",
		"a header of UTF-7":        "tree x\nencoding UTF-7\n\none\n",
		"a header with no name":    "tree x\nencoding \n\none\n",
		"a NUL past a blank line":  "tree x\n\none\n\n\x00\n",
		"a NUL as the message end": "tree x\n\none\x00",
	} {
		g := program(t, with(t, in, onObjects, object(body))...)
		msgs, err := g.Messages(t.Context(), model.Root(filestest.Private(t)), standCommit, standCommit)
		if !errors.Is(err, errHidden) || !strings.Contains(err.Error(), standCommit) || msgs != nil {
			t.Errorf("%s: %q, %v; want hidden, naming the commit, and no list", name, msgs, err)
		}
		if enc, ok := strings.CutPrefix(name, "a header of "); ok && (err == nil || !strings.Contains(err.Error(), enc)) {
			t.Errorf("%s: %v; want the refusal to name the encoding %s", name, err, enc)
		}
	}
	for name, c := range map[string][2]string{
		"no header of an encoding":     {"tree x\n\none\n", "one\n"},
		"a header of utf-8":            {"tree x\nencoding utf-8\n\none\n", "one\n"},
		"a header of UTF8":             {"tree x\nencoding UTF8\n\none\n", "one\n"},
		"an encoding line in the text": {"tree x\n\none\nencoding IBM037\n", "one\nencoding IBM037\n"},
		"an encoding line in a header": {"tree x\ngpgsig a\n encoding IBM037\n\none\n", "one\n"},
		"no message":                   {"tree x\n", ""},
		"bytes not UTF-8, kept":        {"tree x\n\n\xff\xfe\n", "\xff\xfe\n"},
	} {
		g := program(t, with(t, in, onObjects, object(c[0]))...)
		want := []model.Message{{Commit: standCommit, Text: c[1]}}
		if msgs, err := g.Messages(t.Context(), model.Root(filestest.Private(t)), standCommit, standCommit); err != nil || !slices.Equal(msgs, want) {
			t.Errorf("%s: %q, %v; want %q", name, msgs, err, want)
		}
	}
}
