package coreapi_test

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The commit ids of the planted history: B is the merge base, O the commit
// HEAD holds, T the commit a merge brings in, and X one commit between B and T.
const (
	cB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cO = "oooooooooooooooooooooooooooooooooooooooo"
	cX = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	cT = "tttttttttttttttttttttttttttttttttttttttt"
)

// fixtures are two rulebook.yaml at one depth and none in force: the tree of
// a repository before the switch.
func fixtures() map[string]string {
	return map[string]string{"fixture/base/rulebook.yaml": "format: 3\n", "fixture/other/rulebook.yaml": "format: 3\n"}
}

// initOver is the files of a tree of the fixtures once gragas init has made
// the rulebook at its root, as the command wrote them.
func initOver(t *testing.T) (*porttest.World, model.Root, map[string]string) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/history.schema.json")
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", fixtures())
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "A Person", Email: "a@example.test"}})
	newInit(t, w, root, initForm()).Apply(t.Context(), "")
	files := map[string]string{}
	es, err := w.Ports().Tree.List(root, "", model.Every)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if !e.Dir {
			files[e.Path] = read(t, w, root, e.Path)
		}
	}
	if _, ok := files["rulebook.yaml"]; !ok || files["history.jsonl"] == "" {
		t.Fatalf("init made %v", files)
	}
	return w, root, files
}

// initReplay replays a line as gragas init, the one command planted history
// holds.
func initReplay(root model.Root) coreapi.Replayer {
	return func(_ context.Context, p port.Ports, _ string) (*coreapi.Change, *model.Refusal, error) {
		c, err := coreapi.InitRulebook(p, root, initForm(), coreapi.Lines{Command: "gragas init --kind rulebook", Applies: "gragas init --kind rulebook --apply", Who: "gragas init"}, "run-1", "v0.0.0-test")
		return c, nil, err
	}
}

// with is files and the changes on top of them.
func with(files map[string]string, plus map[string]string) map[string]string {
	out := maps.Clone(files)
	maps.Copy(out, plus)
	return out
}

// mergeOver judges a merge of T into O, O's tree and the merge base's as the
// test hands them, over the history the test plants; it answers the staged
// guard's refusal and error.
func mergeOver(t *testing.T, w *porttest.World, root model.Root, r porttest.Repo) (*model.Refusal, error) {
	t.Helper()
	r.Who = &model.Identity{Name: "Ada", Email: "ada@example.org"}
	r.Merging = []string{"theirs"}
	w.SeedRepo(root, r)
	_, ref, err := coreapi.GuardStaged(t.Context(), w.Ports(), coreapi.StagedAsk{Root: root, Replay: initReplay(root), Run: "guard-1", Engine: "v0.0.0-test"})
	return ref, err
}

func refusedWith(t *testing.T, what string, ref *model.Refusal, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: error %v", what, err)
	}
	if ref == nil || !strings.Contains(ref.Observed(), want) {
		t.Fatalf("%s: refusal = %v, want one saying %q", what, ref, want)
	}
}

func allowedMerge(t *testing.T, what string, ref *model.Refusal, err error) {
	t.Helper()
	if err != nil || ref != nil {
		t.Fatalf("%s: ref = %v, err = %v, want it allowed", what, ref, err)
	}
}

// TestAMergeOverABaseWithTwoBooksIsJudged plants the merge rule
// in the core: the base holds two fixture books and none in force, the other
// side makes the book with gragas init, and the merge takes it. Planted with
// the real init as the replayer, so a forged digest is found by replaying.
// Risk: a branch cut before the switch is refused at its next merge of
// release-1; or a forged line, or a tree with two books after, passes.
func TestAMergeOverABaseWithTwoBooksIsJudged(t *testing.T) {
	t.Parallel()
	_, _, made := initOver(t)
	base, ours := fixtures(), with(fixtures(), map[string]string{"note.txt": "main moved on\n"})
	merged := with(made, map[string]string{"note.txt": "main moved on\n"})
	repo := func(theirs, index map[string]string) porttest.Repo {
		return porttest.Repo{
			Heads:   map[string]porttest.Rev{"HEAD": {Commit: cO, Files: ours}, "theirs": {Commit: cT, Files: theirs}},
			Revs:    []porttest.Rev{{Commit: cB, Files: base}},
			Parents: map[string][]string{cB: {}, cO: {cB}, cT: {cB}},
			Bases:   map[[2]string]string{{"HEAD", "theirs"}: cB},
			Index:   index,
		}
	}
	t.Run("the book made on the other side", func(t *testing.T) {
		t.Parallel()
		w, root, _ := initOver(t)
		ref, err := mergeOver(t, w, root, repo(made, merged))
		allowedMerge(t, "the merge", ref, err)
	})
	t.Run("a forged plan digest in the commit brought in", func(t *testing.T) {
		t.Parallel()
		w, root, _ := initOver(t)
		forged := with(made, map[string]string{"history.jsonl": strings.Replace(made["history.jsonl"], `"plan":"`, `"plan":"x`, 1)})
		ref, err := mergeOver(t, w, root, repo(forged, with(forged, map[string]string{"note.txt": "main moved on\n"})))
		refusedWith(t, "a forged digest", ref, err, "names the plan")
	})
	t.Run("a forged plan digest in the merge alone", func(t *testing.T) {
		t.Parallel()
		w, root, _ := initOver(t)
		forged := with(merged, map[string]string{"history.jsonl": strings.Replace(made["history.jsonl"], `"plan":"`, `"plan":"x`, 1)})
		ref, err := mergeOver(t, w, root, repo(made, forged))
		refusedWith(t, "a forged digest", ref, err, "is new in this merge")
	})
	t.Run("two books after", func(t *testing.T) {
		t.Parallel()
		w, root, _ := initOver(t)
		// no merge base is seeded: the error comes before one is asked for
		third := with(merged, map[string]string{"fixture/third/rulebook.yaml": "format: 3\n"})
		delete(third, "rulebook.yaml")
		r := repo(made, third)
		r.Bases = nil
		_, err := mergeOver(t, w, root, r)
		if !errors.Is(err, coreapi.ErrTwoBooks) {
			t.Fatalf("two books after: err = %v, want ErrTwoBooks", err)
		}
	})
}

// TestACommitWithTwoBooksBeforeAndAfterIsNoChangeToJudge: a commit a merge
// brings in whose tree holds more than one book before and after makes no
// rulebook and changes none, as one with none before and after does, so it is
// passed over; the commit that makes the book is judged from nothing. Two
// sides of one merge:
//
//	an ordinary commit then the commit that makes the book: a branch cut
//	before the switch carries an ordinary commit over the two fixture books,
//	then the commit that makes the book;
//	the branch that made the book takes release-1 in: it takes in release-1,
//	which still holds two, with a commit of its own over them.
//
// Risk: the switch's own branch cannot take release-1 in, and a branch that
// carries one ordinary commit before the switch cannot merge it, and with
// neither way to accept the refusal.
func TestACommitWithTwoBooksBeforeAndAfterIsNoChangeToJudge(t *testing.T) {
	t.Parallel()
	_, _, made := initOver(t)
	base := fixtures()
	ordinary := with(base, map[string]string{"x.txt": "an ordinary commit\n"})
	t.Run("an ordinary commit then the commit that makes the book", func(t *testing.T) {
		t.Parallel()
		w, root, _ := initOver(t)
		ours := with(base, map[string]string{"note.txt": "main moved on\n"})
		ref, err := mergeOver(t, w, root, porttest.Repo{
			Heads:   map[string]porttest.Rev{"HEAD": {Commit: cO, Files: ours}, "theirs": {Commit: cT, Files: with(made, map[string]string{"x.txt": "an ordinary commit\n"})}},
			Revs:    []porttest.Rev{{Commit: cB, Files: base}, {Commit: cX, Files: ordinary}},
			Parents: map[string][]string{cB: {}, cO: {cB}, cX: {cB}, cT: {cX}},
			Bases:   map[[2]string]string{{"HEAD", "theirs"}: cB},
			Index:   with(made, map[string]string{"x.txt": "an ordinary commit\n", "note.txt": "main moved on\n"}),
		})
		allowedMerge(t, "the merge", ref, err)
	})
	t.Run("the branch that made the book takes release-1 in", func(t *testing.T) {
		t.Parallel()
		w, root, _ := initOver(t)
		ours := made
		ref, err := mergeOver(t, w, root, porttest.Repo{
			Heads:   map[string]porttest.Rev{"HEAD": {Commit: cO, Files: ours}, "theirs": {Commit: cT, Files: ordinary}},
			Revs:    []porttest.Rev{{Commit: cB, Files: base}},
			Parents: map[string][]string{cB: {}, cO: {cB}, cT: {cB}},
			Bases:   map[[2]string]string{{"HEAD", "theirs"}: cB},
			Index:   with(made, map[string]string{"x.txt": "an ordinary commit\n"}),
		})
		allowedMerge(t, "the merge", ref, err)
	})
}

// TestACommitThatGoesFromOneBookToTwoIsRefused: a commit a merge brings in
// that takes the one book out and leaves two at one depth is not "two books
// before and after", so it is judged and refused, naming the commit; it is
// not passed over because it ends in two.
// Risk: the pass-over loosens to any commit that leaves two books, and the
// refusal rests on the base's lines alone.
func TestACommitThatGoesFromOneBookToTwoIsRefused(t *testing.T) {
	t.Parallel()
	w, root, made := initOver(t)
	left := fixtures()
	ours := with(made, map[string]string{"note.txt": "main moved on\n"})
	ref, err := mergeOver(t, w, root, porttest.Repo{
		Heads:   map[string]porttest.Rev{"HEAD": {Commit: cO, Files: ours}, "theirs": {Commit: cT, Files: left}},
		Revs:    []porttest.Rev{{Commit: cB, Files: made}},
		Parents: map[string][]string{cB: {}, cO: {cB}, cT: {cB}},
		Bases:   map[[2]string]string{{"HEAD", "theirs"}: cB},
		Index:   ours,
	})
	refusedWith(t, "a commit from one book to two", ref, err, "the merge brings in commit tttttttttttt, which is refused")
}
