package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The commits of the check tests: the merge base, and a commit on the branch.
const (
	checkBase = "1111111111111111111111111111111111111111"
	checkHead = "2222222222222222222222222222222222222222"
)

// checkFiles are the guard book's files below the root, with rules as the
// rule file's text and history as history.jsonl's ("" for none).
func checkFiles(rules, history string) map[string]string {
	out := map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure,
		"book/language/go/rules/errors.yaml": rules,
	}
	if history != "" {
		out["book/history.jsonl"] = history
	}
	return out
}

// checkWorld is a world whose working folder holds the guard's book with no
// finding of validate's, and whose history is r.
func checkWorld(t *testing.T, r porttest.Repo) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC))
	root := w.Seed(guardRoot, checkFiles(guardRules, ""))
	w.SeedRepo(root, r)
	return w, root
}

// checkRepo is a repository whose merge base with origin/main holds base and
// whose HEAD, one commit on, holds head. head nil is HEAD at the merge base.
func checkRepo(base, head map[string]string) porttest.Repo {
	r := porttest.Repo{
		Heads:   map[string]porttest.Rev{"origin/main": {Commit: checkBase, Files: base}},
		Bases:   map[[2]string]string{{"origin/main", "HEAD"}: checkBase},
		Parents: map[string][]string{checkBase: {}},
	}
	if head == nil {
		r.Heads["HEAD"] = porttest.Rev{Commit: checkBase, Files: base}
		return r
	}
	r.Heads["HEAD"] = porttest.Rev{Commit: checkHead, Files: head}
	r.Parents[checkHead] = []string{checkBase}
	return r
}

// noReplay is a replayer the test does not expect to be asked.
func noReplay(t *testing.T) coreapi.Replayer {
	t.Helper()
	return func(_ context.Context, _ port.Ports, line string) (*coreapi.Change, *model.Refusal, error) {
		t.Errorf("replayed %q, and no line was to be replayed", line)
		return nil, nil, errors.New("not replayed")
	}
}

// check runs CheckRulebook in w at root with base as the base ref, over the
// world's history.
func check(t *testing.T, w *porttest.World, root model.Root, base string) (model.RulebookCheck, *model.Refusal) {
	t.Helper()
	p := w.Ports()
	ask := coreapi.CheckAsk{Root: root, BaseRef: base, Replay: noReplay(t),
		OpenHistory: func() (port.History, error) { return p.History, nil }}
	out, ref, err := coreapi.CheckRulebook(context.Background(), p, ask)
	if err != nil {
		t.Fatal(err)
	}
	return out, ref
}

func TestACleanBranchEndsDoneAndSaysWhatItCannotGiveYet(t *testing.T) {
	t.Parallel()
	w, root := checkWorld(t, checkRepo(checkFiles(guardRules, ""), nil))
	out, ref := check(t, w, root, "origin/main")
	if ref != nil || out.Word() != model.Done || len(out.Gate) != 0 {
		t.Fatalf("a clean branch: %+v, validate %+v, refused %v", out.Gate, out.Validation.Findings, ref)
	}
	if out.Base != checkBase || out.Commits != 0 || out.Validation.Rules != 1 {
		t.Errorf("base %q, %d commits, %d rules; want the merge base, 0 and 1", out.Base, out.Commits, out.Validation.Rules)
	}
	if len(out.Unavailable) == 0 {
		t.Error("nothing is said to be not available yet")
	}
	for _, u := range out.Unavailable {
		if u.Value != nil || strings.TrimSpace(u.Reason) == "" {
			t.Errorf("%s: value %v, reason %q; want null and why", u.Name, u.Value, u.Reason)
		}
	}
	doc := coreapi.CheckDoc(out, 50)
	if doc.Word != "done" || doc.Command != "check" || doc.Base != checkBase || doc.Gate == nil {
		t.Errorf("the document: %+v", doc)
	}
}

// TestAStatusCommittedByHandIsAGateFindingNamingTheCommit (agents.guards.1):
// the hook bypassed, the commit is held to its own entries as guard staged
// would have held it.
func TestAStatusCommittedByHandIsAGateFindingNamingTheCommit(t *testing.T) {
	t.Parallel()
	byHand := strings.Replace(guardRules, "status: proposed", "status: active", 1)
	w, root := checkWorld(t, checkRepo(checkFiles(guardRules, ""), checkFiles(byHand, "")))
	out, ref := check(t, w, root, "origin/main")
	if ref != nil {
		t.Fatalf("refused: %v", ref.Observed())
	}
	if out.Word() != model.Findings || out.Commits != 1 {
		t.Fatalf("word %s, %d commits; want findings and 1", out.Word(), out.Commits)
	}
	i := slices.IndexFunc(out.Gate, func(g model.GateFinding) bool { return g.Kind == model.GateUnrecorded })
	if i < 0 {
		t.Fatalf("no unrecorded finding: %+v", out.Gate)
	}
	if g := out.Gate[i]; !strings.Contains(g.Msg, "commit "+checkHead[:12]) || !strings.Contains(g.Msg, "active") || g.Fix == "" {
		t.Errorf("the finding %+v; want the commit, the value and a fix", g)
	}
}

// TestAProseEditCommittedIsNoGateFinding is the control: a commit that
// changes no lifecycle field passes the gate.
func TestAProseEditCommittedIsNoGateFinding(t *testing.T) {
	t.Parallel()
	reworded := strings.Replace(guardRules, "An error says what failed.", "An error says what failed, and where.", 1)
	w, root := checkWorld(t, checkRepo(checkFiles(guardRules, ""), checkFiles(reworded, "")))
	out, ref := check(t, w, root, "origin/main")
	if ref != nil || out.Word() != model.Done || len(out.Gate) != 0 {
		t.Errorf("a prose edit: %+v, validate %+v, refused %v", out.Gate, out.Validation.Findings, ref)
	}
}

func TestALineOfTheMergeBaseRewrittenIsAGateFinding(t *testing.T) {
	t.Parallel()
	w, root := checkWorld(t, checkRepo(checkFiles(guardRules, "{\"n\":1}\n"), checkFiles(guardRules, "{\"n\":2}\n")))
	out, ref := check(t, w, root, "origin/main")
	if ref != nil {
		t.Fatalf("refused: %v", ref.Observed())
	}
	if !slices.ContainsFunc(out.Gate, func(g model.GateFinding) bool { return g.Kind == model.GateRewritten }) {
		t.Errorf("no rewritten finding: %+v", out.Gate)
	}
	if out.Word() != model.Findings {
		t.Errorf("word %s, want findings", out.Word())
	}
}

// failingBase is a history whose Base fails with err.
type failingBase struct {
	port.History
	err error
}

func (f failingBase) Base(context.Context, model.Root, string, string) (string, error) {
	return "", f.err
}

// gitSays is an adapter's refusal, known by its methods as core knows it.
type gitSays struct{ word model.Word }

func (g gitSays) Error() string    { return "git says no" }
func (g gitSays) Word() model.Word { return g.word }
func (g gitSays) Observed() string { return "the repository is a shallow clone" }
func (g gitSays) Hint() string     { return "git fetch --unshallow" }

// TestWhatCannotBeJudgedIsNotReadyNeverDone: no base ref, a base ref the
// repository lacks, no one merge base, and git's own not ready each end the
// run on a refusal, with validate's findings still read.
func TestWhatCannotBeJudgedIsNotReadyNeverDone(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		base    string
		history func(port.History) (port.History, error)
		want    model.Word
		says    []string
	}{
		"no base ref": {"", nil, model.NotReady, []string{coreapi.BaseRefKey, coreapi.BaseRefVar}},
		"a base ref the repository lacks": {"origin/gone",
			func(h port.History) (port.History, error) {
				return failingBase{h, fmt.Errorf("the revision %q does not exist: %w", "origin/gone", port.ErrNoRev)}, nil
			}, model.NotReady, []string{"origin/gone", "fetch"}},
		"no one merge base": {"origin/main",
			func(h port.History) (port.History, error) {
				return failingBase{h, fmt.Errorf("two merge bases: %w", port.ErrNoBase)}, nil
			}, model.NotReady, []string{"no one merge base", coreapi.BaseRefKey}},
		"git not ready": {"origin/main",
			func(port.History) (port.History, error) { return nil, gitSays{model.NotReady} }, model.NotReady, []string{"shallow", "unshallow"}},
		"git refused": {"origin/main",
			func(port.History) (port.History, error) { return nil, gitSays{model.Refused} }, model.Refused, []string{"shallow"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, root := checkWorld(t, checkRepo(checkFiles(guardRules, ""), nil))
			p := w.Ports()
			ask := coreapi.CheckAsk{Root: root, BaseRef: c.base, Replay: noReplay(t),
				OpenHistory: func() (port.History, error) { return p.History, nil }}
			if c.history != nil {
				ask.OpenHistory = func() (port.History, error) { return c.history(p.History) }
			}
			out, ref, err := coreapi.CheckRulebook(context.Background(), p, ask)
			if err != nil || ref == nil {
				t.Fatalf("err %v, refusal %v; want a refusal", err, ref)
			}
			said := ref.Observed() + " " + ref.Hint()
			if ref.Word() != c.want {
				t.Errorf("word %s, want %s: %s", ref.Word(), c.want, said)
			}
			for _, s := range c.says {
				if !strings.Contains(said, s) {
					t.Errorf("%q does not say %q", said, s)
				}
			}
			if out.Validation.Rules != 1 || len(out.Gate) != 0 {
				t.Errorf("validate's %d rules, gate %+v; want validate read and no gate", out.Validation.Rules, out.Gate)
			}
		})
	}
}

// TestAFolderWithNoRulebookIsTheOldChecks: the error, so the command falls
// back to the old check: with no base ref named, history is never opened;
// with one named, the merge base holds no rulebook either.
func TestAFolderWithNoRulebookIsTheOldChecks(t *testing.T) {
	t.Parallel()
	plain := map[string]string{"go.mod": "module example\n"}
	w := porttest.NewWorld(t)
	root := w.Seed("/project", plain)
	ask := coreapi.CheckAsk{Root: root, Replay: noReplay(t),
		OpenHistory: func() (port.History, error) { t.Error("history opened"); return nil, errors.New("no") }}
	if _, ref, err := coreapi.CheckRulebook(context.Background(), w.Ports(), ask); !errors.Is(err, coreapi.ErrNoBook) || ref != nil {
		t.Errorf("no base ref: err %v, refusal %v; want ErrNoBook", err, ref)
	}

	w = porttest.NewWorld(t)
	root = w.Seed("/project", plain)
	w.SeedRepo(root, checkRepo(plain, nil))
	p := w.Ports()
	ask = coreapi.CheckAsk{Root: root, BaseRef: "origin/main", Replay: noReplay(t),
		OpenHistory: func() (port.History, error) { return p.History, nil }}
	if _, ref, err := coreapi.CheckRulebook(context.Background(), p, ask); !errors.Is(err, coreapi.ErrNoBook) || ref != nil {
		t.Errorf("a base with no rulebook: err %v, refusal %v; want ErrNoBook", err, ref)
	}
}

// TestARulebookTheBaseHeldIsNeverSwitchedOffByTheBranch: where the merge base
// holds one rulebook, the branch's removing it, adding a second, or committing
// either while the working folder still holds one, is the gate's finding,
// never the old check.
// Risk: a branch turns the gate off by what it holds.
func TestARulebookTheBaseHeldIsNeverSwitchedOffByTheBranch(t *testing.T) {
	t.Parallel()
	book := checkFiles(guardRules, "")
	two := checkFiles(guardRules, "")
	two["stray/rulebook.yaml"] = bookIdentity
	none := map[string]string{"go.mod": "module example\n"}
	for name, c := range map[string]struct {
		folder, head map[string]string
		says         string
	}{
		"the folder's removed":   {none, none, "the working folder holds no rulebook.yaml"},
		"a second in the folder": {two, two, "the working folder holds more than one rulebook.yaml"},
		"HEAD's removed":         {book, none, "HEAD holds no rulebook.yaml"},
		"a second committed":     {book, two, "HEAD holds more than one rulebook.yaml"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.SeedSchemas(allSchemas()...)
			w.Instant(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC))
			root := w.Seed(guardRoot, c.folder)
			w.SeedRepo(root, checkRepo(book, c.head))
			out, ref := check(t, w, root, "origin/main")
			if ref != nil {
				t.Fatalf("refused: %s", ref.Observed())
			}
			if out.Word() != model.Findings || len(out.Gate) != 1 || out.Gate[0].Kind != model.GateBook {
				t.Fatalf("word %s, gate %+v; want findings with the rulebook gone or doubled", out.Word(), out.Gate)
			}
			if g := out.Gate[0]; !strings.Contains(g.Msg, c.says+", where the merge base holds one (book/rulebook.yaml)") {
				t.Errorf("the finding %q; want %q and the base's rulebook", g.Msg, c.says)
			}
		})
	}
}

// TestABranchThatMovesTheBookIsAGateFinding: the shallowest book wins,
// so a branch adding a rulebook.yaml above the base's moves the book; the gate
// names both folders. The same folder and a base with no book give no such
// finding.
// Risk: a branch turns the gate off by holding a shallower book.
func TestABranchThatMovesTheBookIsAGateFinding(t *testing.T) {
	t.Parallel()
	book := checkFiles(guardRules, "")
	up := checkFiles(guardRules, "")
	up["rulebook.yaml"] = bookIdentity
	none := map[string]string{"go.mod": "module example\n"}
	two := checkFiles(guardRules, "")
	two["other/rulebook.yaml"] = bookIdentity // ties with book/ at one depth
	for name, c := range map[string]struct {
		base, head map[string]string
		moved      bool
	}{
		"moved":             {book, up, true},
		"same":              {book, book, false},
		"base without book": {none, up, false},
		"base with two":     {two, up, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.SeedSchemas(allSchemas()...)
			w.Instant(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC))
			root := w.Seed(guardRoot, c.head)
			w.SeedRepo(root, checkRepo(c.base, c.head))
			out, ref := check(t, w, root, "origin/main")
			if ref != nil {
				t.Fatalf("refused: %s", ref.Observed())
			}
			var says string
			for _, g := range out.Gate {
				if g.Kind == model.GateBook {
					says = g.Msg
				}
			}
			want := "HEAD's rulebook is rulebook.yaml, so the old one is no longer judged, where the merge base holds one (book/rulebook.yaml)"
			if c.moved != strings.Contains(says, want) || (!c.moved && says != "") {
				t.Errorf("gate book finding %q; moved = %v, want %q", says, c.moved, want)
			}
		})
	}
}

// TestCheckCountsOnlyReleaseTagsAndSaysWhenItReadNone: a clone with
// no tags and a clone whose only tag is no release both read none, and the
// listing names the lack under "Not available yet"; a release tag is read and
// counted, with nothing said. Planted failure: every tag counted as a release.
func TestCheckCountsOnlyReleaseTagsAndSaysWhenItReadNone(t *testing.T) {
	t.Parallel()
	files := checkFiles(guardRules, "")
	tagged := func(names ...string) porttest.Repo {
		r := checkRepo(files, nil)
		r.Tags = map[string]porttest.Rev{}
		for _, n := range names {
			r.Tags[n] = porttest.Rev{Commit: checkBase, Files: files}
		}
		return r
	}
	for name, c := range map[string]struct {
		tags     []string
		releases int
		said     bool
	}{
		"a clone with no tags":                  {nil, 0, true},
		"a clone with a tag that is no release": {[]string{"backup-2026", "v1", "v1.2.3-rc1"}, 0, true},
		"a clone with a release tag":            {[]string{"v1.2.3"}, 1, false},
		"a release tag among others":            {[]string{"v1.2.3", "latest", "v2.0.0"}, 2, false},
	} {
		w, root := checkWorld(t, tagged(c.tags...))
		out, ref := check(t, w, root, "origin/main")
		if ref != nil || out.Releases != c.releases {
			t.Errorf("%s: read %d release tags (refused %v), want %d", name, out.Releases, ref, c.releases)
		}
		said := slices.ContainsFunc(out.Unavailable, func(u model.Unavailable) bool { return u.Name == lifecycle.NoReleaseRead().Name })
		if said != c.said {
			t.Errorf("%s: the lack of a release is said = %v, want %v: %+v", name, said, c.said, out.Unavailable)
		}
		if doc := coreapi.CheckDoc(out, 50); doc.Releases != c.releases {
			t.Errorf("%s: the document says %d releases, want %d", name, doc.Releases, c.releases)
		}
	}
}
