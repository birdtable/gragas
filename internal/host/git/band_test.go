package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// The stand-in's object names: a commit, and a file's blob.
const (
	standCommit = "1111111111111111111111111111111111111111"
	standBlob   = "3333333333333333333333333333333333333333"
)

// standIn is the rules of a stand-in for git at version ver, which answer
// each verb of the table as git does for a repository with one commit,
// tagged v1, holding the file a of two bytes; promisor, when not "", is what
// its config prints for the promisor keys. over, when set, answers every
// verb but version in place of git's answer.
func standIn(ver, promisor string, over *Answer) []Rule {
	answer := func(a Answer) Answer {
		if over != nil {
			return *over
		}
		return a
	}
	prom := Answer{Code: 1}
	if promisor != "" {
		prom = Answer{Out: promisor + "\x00"}
	}
	return []Rule{
		{End: " version", Answer: Answer{Out: "git version " + ver + "\n"}},
		{Has: []string{"user"}, Answer: Answer{Out: "user.name\nAda\x00user.email\na@example.com\x00"}},
		{Has: []string{"extensions"}, Answer: answer(prom)},
		{End: "--is-shallow-repository", Answer: answer(Answer{Out: "false\n"})},
		{End: "--is-inside-work-tree --show-prefix", Answer: answer(Answer{Out: "true\n\n"})},
		{End: "--show-prefix", Answer: answer(Answer{Out: "\n"})},
		{Has: []string{"rev-parse --verify"}, Answer: answer(Answer{Out: standCommit + "\n"})},
		{Has: []string{"merge-base"}, Answer: answer(Answer{Out: standCommit + "\n"})},
		{Has: []string{"ls-tree"}, Answer: answer(Answer{Out: "100644 blob " + standBlob + " 2\ta\x00"})},
		{Has: []string{"ls-files"}, Answer: answer(Answer{Out: "100644 " + standBlob + " 2 0\ta\x00"})},
		{Has: []string{onObjects}, Stdin: standBlob, Answer: answer(Answer{Out: standBlob + " blob 2\nxx\n"})},
		{Has: []string{onObjects}, Answer: answer(standObject())},
		{Has: []string{"cat-file"}, Answer: answer(Answer{Out: "x\n"})},
		{Has: []string{"for-each-ref"}, Answer: answer(Answer{Out: "refs/tags/v1\x00commit\x00" + standCommit + "\x00\x00\n"})},
		{Has: []string{onRange}, Answer: answer(standRange())},
	}
}

// onRange and onObjects are what the stand-in's rules for range-commits and
// commit-objects match.
const (
	onRange   = "rev-list"
	onObjects = "cat-file --batch"
)

// standRange is the stand-in's answer to range-commits, and standObject to
// commit-objects: the one commit, whose message is "one\n".
func standRange() Answer { return Answer{Out: standCommit + "\n"} }

func standObject() Answer {
	return Answer{Out: standCommit + " commit 12\ntree x\n\none\n\n"}
}

// with is rules with the answer of the rule that first matches has set to
// a; the rules handed in are left as they were.
func with(t *testing.T, rules []Rule, has string, a Answer) []Rule {
	t.Helper()
	out := slices.Clone(rules)
	for i, r := range out {
		if len(r.Has) > 0 && r.Has[0] == has && r.Stdin == "" {
			out[i].Answer = a
			return out
		}
	}
	t.Fatalf("no rule of the stand-in matches %q", has)
	return nil
}

// withBlobs is rules with the answer to the batch read of blobs set to a.
func withBlobs(t *testing.T, rules []Rule, a Answer) []Rule {
	t.Helper()
	out := slices.Clone(rules)
	for i, r := range out {
		if len(r.Has) > 0 && r.Has[0] == onObjects && r.Stdin == standBlob {
			out[i].Answer = a
			return out
		}
	}
	t.Fatal("no rule of the stand-in answers the batch read of blobs")
	return nil
}

// methods runs each History method that reads objects once, and answers
// each one's error, by name.
func methods(ctx context.Context, g *Git, root model.Root) map[string]error {
	out := map[string]error{}
	_, out["Base"] = g.Base(ctx, root, "v1", "main")
	_, out["Files at a rev"] = g.Files(ctx, root, "v1")
	_, out["Files of the index"] = g.Files(ctx, root, "")
	_, out["Blob"] = g.Blob(ctx, root, "v1", "a")
	_, out["Blobs"] = g.Blobs(ctx, root, "v1", []string{"a"})
	_, out["Tags"] = g.Tags(ctx, root)
	_, out["Messages"] = g.Messages(ctx, root, standCommit, standCommit)
	return out
}

// verbsRun is the verbs of g's runs, in order.
func verbsRun(g *Git) []string {
	var out []string
	for _, r := range g.Ran() {
		out = append(out, r.Verb)
	}
	return out
}

// TestEachBandRunsWhatItsVersionAllows (history-version-bands): before 2.38 the methods that
// read objects are not ready, naming the version found and 2.38, and Who
// still runs; from 2.38 to 2.43 each reads the promisor keys first, runs
// without --no-lazy-fetch when none is set and is not ready when one is;
// from 2.44 every verb that reads objects runs with --no-lazy-fetch. The
// version is read once. Planted failures: the band of 2.43 given
// --no-lazy-fetch; the promisor check skipped; the version read each call.
func TestEachBandRunsWhatItsVersionAllows(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	t.Run("2.37 is not ready, and Who runs", func(t *testing.T) {
		g := program(t, standIn("2.37.1", "", nil)...)
		for name, err := range methods(t.Context(), g, root) {
			nr, ok := errors.AsType[*NotReady](err)
			if !ok || !strings.Contains(nr.Observed(), "2.37.1") || !strings.Contains(nr.Observed(), "2.38") || !strings.Contains(nr.Hint(), Setting) {
				t.Errorf("%s on 2.37.1: %v, want not ready naming 2.37.1 and 2.38", name, err)
			}
		}
		if got := verbsRun(g); !slices.Equal(got, []string{"version"}) {
			t.Errorf("on 2.37.1 the runs were %q, want the version alone", got)
		}
		if id, err := g.Who(t.Context(), root); err != nil || id.Name != "Ada" {
			t.Errorf("Who on 2.37.1: %+v, %v", id, err)
		}
	})
	t.Run("2.43 with no promisor runs without --no-lazy-fetch", func(t *testing.T) {
		for _, ver := range []string{"2.43.0", "2.38.0", "2.39.5 (Apple Git-154)", "2.40.1.windows.1"} {
			partialRuns(t, root, ver)
		}
	})
	t.Run("2.43 with a promisor remote is not ready", func(t *testing.T) {
		for _, keys := range []string{"extensions.partialclone\norigin", "remote.origin.promisor\ntrue", "remote.x.promisor\nfalse"} {
			g := program(t, standIn("2.43.0", keys, nil)...)
			for name, err := range methods(t.Context(), g, root) {
				nr, ok := errors.AsType[*NotReady](err)
				if !ok || !strings.Contains(nr.Observed(), "promisor") || !strings.Contains(nr.Observed(), "2.44") {
					t.Errorf("%s on 2.43.0 with %s: %v, want not ready naming the promisor and 2.44", name, keys, err)
				}
			}
			for _, r := range g.Ran() {
				if r.Verb != "version" && r.Verb != "config-promisor" {
					t.Errorf("on 2.43.0 with %s, %s ran", keys, r.Verb)
				}
			}
		}
	})
	t.Run("2.44 and later take --no-lazy-fetch on every verb that reads objects", func(t *testing.T) {
		for _, ver := range []string{"2.44.0", "2.56.0", "3.0.0", "2.45.0.rc1"} {
			noLazyRuns(t, root, ver)
		}
	})
}

// partialRuns: on ver, a git from 2.38 to 2.43, every method runs with no
// promisor key, without --no-lazy-fetch, reading the version once and the
// keys once per method.
func partialRuns(t *testing.T, root model.Root, ver string) {
	t.Helper()
	g := program(t, standIn(ver, "", nil)...)
	for name, err := range methods(t.Context(), g, root) {
		if err != nil {
			t.Errorf("%s on %s: %v", name, ver, err)
		}
	}
	for _, r := range g.Ran() {
		if slices.Contains(r.Argv, "--no-lazy-fetch") {
			t.Errorf("on %s the run %s took --no-lazy-fetch, which it lacks", ver, r.Verb)
		}
	}
	if n := strings.Count(strings.Join(verbsRun(g), " "), "version"); n != 1 {
		t.Errorf("on %s the version was read %d times", ver, n)
	}
	if n := strings.Count(strings.Join(verbsRun(g), " "), "config-promisor"); n != 7 {
		t.Errorf("on %s the promisor keys were read %d times, want once per method, 7", ver, n)
	}
}

// noLazyRuns: on ver, from 2.44, every verb that reads objects takes
// --no-lazy-fetch first, and the promisor keys are never read.
func noLazyRuns(t *testing.T, root model.Root, ver string) {
	t.Helper()
	g := program(t, standIn(ver, "", nil)...)
	for name, err := range methods(t.Context(), g, root) {
		if err != nil {
			t.Errorf("%s on %s: %v", name, ver, err)
		}
	}
	for _, r := range g.Ran() {
		reads := verbs()[r.Verb].objects
		if got := len(r.Argv) > 1 && r.Argv[1] == "--no-lazy-fetch"; got != reads {
			t.Errorf("on %s the run %s: %q; --no-lazy-fetch first is %v, want %v", ver, r.Verb, r.Argv, got, reads)
		}
		if r.Verb == "config-promisor" {
			t.Errorf("on %s the promisor keys were read", ver)
		}
		// the table is not its own witness: the batch reads of objects are named here
		if (r.Verb == verbBlobs || r.Verb == verbCommits || r.Verb == verbBlob) && !slices.Contains(r.Argv, "--no-lazy-fetch") {
			t.Errorf("on %s the run %s: %q took no --no-lazy-fetch, and it reads objects", ver, r.Verb, r.Argv)
		}
	}
}

// TestAVersionGitDoesNotPrintIsFailed: what is not one version line fails
// with the run id, and nothing that reads objects runs. Planted failure: a
// version read by its first number alone.
func TestAVersionGitDoesNotPrintIsFailed(t *testing.T) {
	t.Parallel()
	for _, line := range []string{"git version 2.56", "git version 2.56.0-dirty", "2.56.0", "git version 2.56.0 extra", "git version v2.56.0", ""} {
		g := program(t, Always(Answer{Out: line + "\n"}))
		_, err := g.Files(t.Context(), model.Root(filestest.Private(t)), "")
		if f, ok := errors.AsType[*Failed](err); !ok || f.Run() != "run-7" {
			t.Errorf("git version printing %q: %v, want failed with the run id", line, err)
		}
		if got := verbsRun(g); !slices.Equal(got, []string{"version"}) {
			t.Errorf("git version printing %q, and the runs were %q", line, got)
		}
	}
}

// TestEveryHistoryMethodEndsAtTheDeadlineFailedWithTheRunID: a git that
// hangs past the deadline in any verb after the version ends each method
// failed, at the deadline, with the run id. Planted failure: the deadline
// left off the verbs that read objects.
func TestEveryHistoryMethodEndsAtTheDeadlineFailedWithTheRunID(t *testing.T) {
	t.Parallel()
	g := program(t, standIn("2.56.0", "", &Answer{Hang: true})...)
	g.wait = 200 * time.Millisecond
	for name, err := range methods(t.Context(), g, model.Root(filestest.Private(t))) {
		if f, ok := errors.AsType[*Failed](err); !ok || f.Run() != "run-7" || !errors.Is(err, errDeadline) || !strings.Contains(err.Error(), "run-7") {
			t.Errorf("%s with a hung git: %v, want failed at the deadline naming run-7", name, err)
		}
	}
}

// TestEveryHistoryMethodPastItsCapFails: a git that prints past the cap in
// any verb after the version fails each method, never cut and read. Planted
// failure: the listing verbs given no cap.
func TestEveryHistoryMethodPastItsCapFails(t *testing.T) {
	t.Parallel()
	g := program(t, standIn("2.56.0", "", &Answer{Zeros: 4200000})...)
	for name, err := range methods(t.Context(), g, model.Root(filestest.Private(t))) {
		if f, ok := errors.AsType[*Failed](err); !ok || f.Run() != "run-7" || !errors.Is(err, errPastCap) {
			t.Errorf("%s with a git past its cap: %v, want failed past the cap", name, err)
		}
	}
}

// TestABlobPastTheCapIsFailedBeforeItIsRead: a file the listing says is
// past the cap is failed without cat-file running, and content of another
// length than the listing's is failed. Planted failures: the size not
// checked before the read; the length not checked after.
func TestABlobPastTheCapIsFailedBeforeItIsRead(t *testing.T) {
	t.Parallel()
	root := model.Root(filestest.Private(t))
	big := with(t, standIn("2.56.0", "", nil), "ls-tree", Answer{Out: fmt.Sprintf("100644 blob %s %d\ta\x00", standBlob, maxListing+1)})
	g := program(t, big...)
	if _, err := g.Blob(t.Context(), root, "v1", "a"); !errors.Is(err, errPastCap) || !isFailed(err) {
		t.Errorf("a blob past the cap: %v, want failed past the cap", err)
	}
	if slices.Contains(verbsRun(g), "blob") {
		t.Error("cat-file ran for a blob past the cap")
	}
	short := with(t, standIn("2.56.0", "", nil), "cat-file", Answer{Out: "x"})
	if _, err := program(t, short...).Blob(t.Context(), root, "v1", "a"); !isFailed(err) {
		t.Errorf("a blob of another length than its listing: %v, want failed", err)
	}
}

func isFailed(err error) bool {
	_, ok := errors.AsType[*Failed](err)
	return ok
}

// TestAValueOutsideItsSlotIsRefused: a verb's slots take only values of
// their kind, checked again at the slot, and in their number; a verb that
// reads objects runs only once the band is read. Each is refused before
// anything starts. Planted failures: the kind not checked; a value made
// outside parseRev not checked again; the band not checked.
func TestAValueOutsideItsSlotIsRefused(t *testing.T) {
	t.Parallel()
	g := program(t, standIn("2.56.0", "", nil)...)
	dir := filestest.Private(t)
	if _, err := g.run(t.Context(), verbTree, dir, value{kind: kindOID, text: standCommit}); !errors.Is(err, errNoBand) {
		t.Errorf("a verb that reads objects before the band: %v, want refused", err)
	}
	g.band = bandNoLazy
	rev, err := parseRev("v1")
	if err != nil {
		t.Fatal(err)
	}
	oid := value{kind: kindOID, text: standCommit}
	cases := map[string]struct {
		verb string
		vals []value
	}{
		"no value":                        {verbResolve, nil},
		"an object name in a rev slot":    {verbResolve, []value{oid}},
		"a rev in an object name slot":    {verbTree, []value{rev}},
		"one value too many":              {verbResolve, []value{rev, rev}},
		"a value for a verb with none":    {verbTags, []value{rev}},
		"a rev made outside parseRev":     {verbResolve, []value{{kind: kindRev, text: "--output=/work/scratch/x"}}},
		"an object name not hex":          {verbTree, []value{{kind: kindOID, text: strings.Repeat("g", 40)}}},
		"a value of no kind":              {verbResolve, []value{{text: "v1"}}},
		"two object names for merge-base": {verbBase, []value{oid}},
	}
	for name, c := range cases {
		if _, err := g.run(t.Context(), c.verb, dir, c.vals...); !errors.Is(err, errSlots) {
			t.Errorf("%s: %v, want refused as not fitting the slots", name, err)
		}
	}
	if len(g.Ran()) != 0 {
		t.Errorf("a refused run started: %s", Shown(g.Ran()))
	}
}
