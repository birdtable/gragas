package journeys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"

	"github.com/birdtable/go-files/filestest"
)

// The plants of Fable's second read at 28748847: the three-way
// rule holds each lifecycle value whole, never a sum GuardEdit names, and
// reads each side where its rulebook stands. Each passed before; each stands
// beside a control that passes, so a guard that refuses every merge fails.

// newStatus is the status of rule new's rule, newArgs's, as rule new writes
// it, and as a hand edit sets it.
const (
	newStatus  = "statement: \"Return errors, do not panic.\"\n    strength: must\n    status: proposed"
	handStatus = "statement: \"Return errors, do not panic.\"\n    strength: must\n    status: active"
)

// ruleNew runs rule new with newArgs and --apply.
func ruleNew(t *testing.T) func(string, map[string]string) {
	t.Helper()
	return func(dir string, env map[string]string) {
		if r := run(t, dir, env, append(newArgs(), "--apply")...); r.code != 0 {
			t.Fatalf("rule new: exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	}
}

// aFile writes a text file of its own, no rulebook file.
func aFile(t *testing.T, name string) func(string, map[string]string) {
	t.Helper()
	return func(dir string, _ map[string]string) { write(t, dir, name, "a file of its own\n") }
}

// TestAMergeHoldsARuleAddedWhole (Fable at 28748847, 1a): a rule either side
// adds is held field by field, not as its id. Its status changed by hand in
// the merge is refused, whether the other side added it or HEAD did.
// Risk: a new rule's lifecycle field set by hand under cover of its rule new.
func TestAMergeHoldsARuleAddedWhole(t *testing.T) {
	t.Parallel()
	dir, env := branched(t, ruleNew(t), aFile(t, "main.txt"))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "control, the merge of the other side's rule new", staged(t, dir, env))
	edit(t, dir, "language/go/rules/errors.yaml", newStatus, handStatus)
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "the other side's new rule's status by hand", staged(t, dir, env),
		"the merge leaves status of rule:go.errors.4 at active, and the other side's history leaves it at proposed")

	dir, env = branched(t, aFile(t, "side.txt"), ruleNew(t))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "control, a file merged into HEAD's rule new", staged(t, dir, env))
	edit(t, dir, "language/go/rules/errors.yaml", newStatus, handStatus)
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "HEAD's new rule's status by hand", staged(t, dir, env),
		"the merge leaves status of rule:go.errors.4 at active, and HEAD holds proposed")
}

// overlayBranched is the overlay fixture, its parent fetched, on main and a
// branch side from it, made by onSide and committed; then onMain on main,
// committed. The repository is left on main.
func overlayBranched(t *testing.T, onSide, onMain func(dir string, env map[string]string)) (dir string, env map[string]string) {
	t.Helper()
	dir, env = overlayRepo(t), ruleEnv(t)
	env["GRAGAS_NAMES_FILE"] = ""
	fetchParent(t, env)
	gitIn(t, dir, env, "init", "-q")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "branch", "-M", "main")
	gitIn(t, dir, env, "checkout", "-q", "-b", "side")
	onSide(dir, env)
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "main")
	onMain(dir, env)
	commitAll(t, dir, env)
	return dir, env
}

// exclude runs rule exclude of rule under decision, a slug the fixture holds.
func exclude(t *testing.T, rule, decision string) func(string, map[string]string) {
	t.Helper()
	return func(dir string, env map[string]string) {
		if r := run(t, dir, env, "rule", "exclude", rule, "--decision", decision, "--apply"); r.code != 0 {
			t.Fatalf("rule exclude %s: exit %d\n%s%s", rule, r.code, r.stdout, r.stderr)
		}
	}
}

// bothAppended resolves a merge where both sides appended to name: HEAD's
// file, then what the other side added since the merge base.
func bothAppended(t *testing.T, dir string, env map[string]string, ours map[string]string, names ...string) {
	t.Helper()
	for _, name := range names {
		base, theirs := gitIn(t, dir, env, "show", "main~1:"+name), gitIn(t, dir, env, "show", "side:"+name)
		if !strings.HasPrefix(theirs, base) {
			t.Fatalf("the other side's %s does not extend the merge base's", name)
		}
		write(t, dir, name, ours[name]+theirs[len(base):])
	}
	gitIn(t, dir, env, "add", "-A")
}

// The three cases of Fable at 28748847, 1b and 1c, one test each so they run
// in parallel and share no wall time: exclusions are held one by one, by
// overlay, rule and number. Risk of each: an exclusion's record rewritten by
// hand under cover of another's.

// TestAMergeHoldsEachExclusionWholeLiftDeleted: the lifted lines of one
// exclusion deleted by hand in a merge that takes the other side's exclude
// are refused, though the count stays the other side's.
// Risk: an exclusion's record rewritten by hand under cover of another's.
func TestAMergeHoldsEachExclusionWholeLiftDeleted(t *testing.T) {
	t.Parallel()
	dir, env := overlayBranched(t, exclude(t, "std.errors.4", "house-call"), aFile(t, "main.txt"))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "control, the merge of the other side's exclude", staged(t, dir, env))
	edit(t, dir, "exclusions.yaml", "    when: \"2026-10-22T09:00:00Z\"\n    lifted:\n      decision: wrap-errors-again\n      who: alex\n      when: \"2026-11-02T09:00:00Z\"\n", "    when: \"2026-10-22T09:00:00Z\"\n")
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a lift deleted by hand", staged(t, dir, env),
		"the merge changes the exclusion fixture-base/std.errors.3/1 of exclusions.yaml to", "neither side's history records it")
}

// TestAMergeHoldsEachExclusionWholeTwoRules: two sides excluding two rules
// merge.
// Risk: an exclusion's record rewritten by hand under cover of another's, a
// guard that refuses every merge of two excludes.
func TestAMergeHoldsEachExclusionWholeTwoRules(t *testing.T) {
	t.Parallel()
	var ours map[string]string
	dir, env := overlayBranched(t, exclude(t, "std.errors.4", "house-call"), func(dir string, env map[string]string) {
		exclude(t, "std.errors.1", "house-call")(dir, env)
		ours = map[string]string{"exclusions.yaml": read(t, dir, "exclusions.yaml"), "history.jsonl": read(t, dir, "history.jsonl")}
	})
	_ = gitTry(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	bothAppended(t, dir, env, ours, "exclusions.yaml", "history.jsonl")
	allowed(t, "control, two sides excluding two rules", staged(t, dir, env))
}

// TestAMergeHoldsEachExclusionWholeTwoDecisions: two sides excluding one
// rule under two decisions are refused.
// Risk: an exclusion's record rewritten by hand under cover of another's.
func TestAMergeHoldsEachExclusionWholeTwoDecisions(t *testing.T) {
	t.Parallel()
	var ours map[string]string
	dir, env := overlayBranched(t, exclude(t, "std.errors.4", "house-call"), func(dir string, env map[string]string) {
		exclude(t, "std.errors.4", "wrap-errors-again")(dir, env)
		ours = map[string]string{"history.jsonl": read(t, dir, "history.jsonl")}
	})
	_ = gitTry(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	gitIn(t, dir, env, "checkout", "--ours", "--", "exclusions.yaml")
	bothAppended(t, dir, env, ours, "history.jsonl")
	refusedNaming(t, "two sides excluding one rule two ways", staged(t, dir, env),
		"both sides change the exclusion fixture-base/std.errors.4/1 of exclusions.yaml since the merge base",
		"drop one side's commit that changes the exclusion, then merge again")
}

// TestAMergeReadsEachSideWhereItsRulebookStands (Fable at 28748847, 1d): a
// rulebook HEAD moved in a commit made with no hook is never read as absent
// at the merge base, which would let the other side's change pass as the
// merge base's: the merge is refused, naming both folders.
// Risk: a side read where its rulebook is not, as an empty one.
func TestAMergeReadsEachSideWhereItsRulebookStands(t *testing.T) {
	t.Parallel()
	dir, env := branched(t, func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") }, func(dir string, env map[string]string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, "book"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != ".git" {
				gitIn(t, dir, env, "mv", e.Name(), "book/"+e.Name())
			}
		}
	})
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	refusedNaming(t, "a move committed on HEAD with no hook", staged(t, dir, env),
		"the merge base holds the rulebook in the root, and the merge in book")
}

// hookedGit runs git in dir with init's two git hooks in place, pre-commit
// and pre-merge-commit, each calling the built binary, and answers what git
// printed and how it ended.
func hookedGit(t *testing.T, dir string, env map[string]string, args ...string) (string, error) {
	t.Helper()
	hooks := t.TempDir()
	bin, err := shell.Quote(shell.POSIX, []string{filepath.ToSlash(gragasBin(t))})
	if err != nil {
		t.Fatal(err)
	}
	hook, err := os.ReadFile("testdata/hook.sh") // a script of sh, which git runs on every system it runs on
	if err != nil {
		t.Fatal(err)
	}
	for name, line := range map[string]string{"pre-commit": "guard staged", "pre-merge-commit": "guard staged --merge-hook"} {
		filestest.WriteProgram(t, filepath.Join(hooks, name), fmt.Appendf(nil, string(hook), bin, line))
	}
	c := commandOf(t, "git", dir, env, append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + filepath.ToSlash(hooks)}, args...)...)
	c.Env = append(c.Env, gitSettings(env)...)
	out, err := c.CombinedOutput()
	return string(out), err
}

// TestACleanMergeIsJudgedByTheMergeHook: git runs no pre-commit hook for a
// merge it commits itself, and runs pre-merge-commit before it records the
// merge, so init prints that hook's line with --merge-hook. A clean merge
// that changes no lifecycle value and no history line commits at once; one
// that brings in a promote stops, and git commit lands it, the pre-commit
// hook judging it three ways; a hand edit stops at both.
// Risk: a hand edit that rides in on a merge no hook judged.
func TestACleanMergeIsJudgedByTheMergeHook(t *testing.T) {
	t.Parallel()
	merge := []string{"merge", "-q", "--no-ff", "-m", "merge side", "side"}
	finish := []string{"commit", "-q", "--no-edit"}
	merged := func(dir string, env map[string]string) bool {
		t.Helper()
		return gitTry(t, dir, env, "rev-parse", "-q", "--verify", "HEAD^2") == nil
	}
	promote := func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") }

	dir, env := branched(t, proseEdit(t), promote)
	if out, err := hookedGit(t, dir, env, merge...); err != nil || !merged(dir, env) {
		t.Fatalf("the clean merge of prose over HEAD's promote did not commit: %v\n%s", err, out)
	}

	dir, env = branched(t, promote, proseEdit(t))
	out, err := hookedGit(t, dir, env, merge...)
	if err == nil || merged(dir, env) || !strings.Contains(flat(out), "git keeps the merge") {
		t.Fatalf("the clean merge of a promote was not stopped for git commit: %v\n%s", err, out)
	}
	if out, err := hookedGit(t, dir, env, finish...); err != nil || !merged(dir, env) {
		t.Fatalf("git commit of the kept merge of a promote did not commit: %v\n%s", err, out)
	}

	dir, env = branched(t, func(dir string, _ map[string]string) { handPromote(t, dir) }, proseEdit(t))
	head := gitIn(t, dir, env, "rev-parse", "HEAD")
	if out, err := hookedGit(t, dir, env, merge...); err == nil {
		t.Fatalf("the clean merge of a hand edit committed:\n%s", out)
	}
	if out, err := hookedGit(t, dir, env, finish...); err == nil {
		t.Fatalf("git commit of the kept merge of a hand edit committed:\n%s", out)
	}
	if got := gitIn(t, dir, env, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD moved from %s to %s: the refused merge was committed", head, got)
	}
}

// relisted is the fixture's lifted exclusion fixture-base/std.errors.3/1
// written again by hand above itself, with no lifted lines: the loader keeps
// both records, and the rule reads as excluded once more.
func relisted(t *testing.T, dir string) {
	t.Helper()
	const lifted = "  - overlay: fixture-base\n    rule: std.errors.3\n"
	edit(t, dir, "exclusions.yaml", lifted, lifted+"    number: 1\n    decision: house-call\n    who: alex\n    when: \"2026-10-22T09:00:00Z\"\n"+lifted)
}

// TestAMergeHoldsEveryRecordOfAKey (Fable at 57382b31, 1): a key written
// twice is held with every record under it, as the engine reads every one.
// A lifted exclusion written again above itself, not lifted, is refused in
// the merge's own resolution, and a side that brings it in stops at both
// hooks.
// Risk: a rule excluded again by hand under a key the guard read as one.
func TestAMergeHoldsEveryRecordOfAKey(t *testing.T) {
	t.Parallel()
	dir, env := overlayBranched(t, exclude(t, "std.errors.4", "house-call"), aFile(t, "main.txt"))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "control, the merge of the other side's exclude", staged(t, dir, env))
	relisted(t, dir)
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a lifted exclusion written again in the merge", staged(t, dir, env),
		"the merge changes the exclusion fixture-base/std.errors.3/1 of exclusions.yaml to", "neither side's history records it")

	dir, env = overlayBranched(t, func(dir string, _ map[string]string) { relisted(t, dir) }, aFile(t, "main.txt"))
	head := gitIn(t, dir, env, "rev-parse", "HEAD")
	out, err := hookedGit(t, dir, env, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	if err == nil || !strings.Contains(flat(out), "git keeps the merge") {
		t.Fatalf("the clean merge of a lifted exclusion written again was not stopped: %v\n%s", err, out)
	}
	if out, err := hookedGit(t, dir, env, "commit", "-q", "--no-edit"); err == nil {
		t.Fatalf("git commit of the kept merge of a lifted exclusion written again committed:\n%s", out)
	}
	if got := gitIn(t, dir, env, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD moved from %s to %s: the refused merge was committed", head, got)
	}
}

// TestAMergeHoldsEachSourceWhole (Fable at 7a494795, 1): a source's locator
// changed by hand in a merge commit that neither side's history records is
// refused, as a rule's field is. sources.yaml and terms.yaml were guarded in
// a commit of one parent only.
// Risk: a merge repoints what a rule cites with no entry.
func TestAMergeHoldsEachSourceWhole(t *testing.T) {
	t.Parallel()
	dir, env := branched(t, aFile(t, "side.txt"), aFile(t, "main.txt"))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "control, a merge of two files of their own", staged(t, dir, env))
	edit(t, dir, "sources.yaml", "locator: https://go.dev/doc/effective_go\n", "locator: https://example.org/elsewhere\n")
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a source's locator by hand", staged(t, dir, env),
		"the merge changes locator of source:effective-go to https://example.org/elsewhere, and neither side's history records it")
}
