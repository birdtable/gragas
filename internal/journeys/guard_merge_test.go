package journeys

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The merge journeys of guard staged run the built binary over
// a real git merge left uncommitted, as the commit hook meets it. Each plant
// is one Fable's read at 0ceb4f67 found passing, or one refusal of the
// three-way rule, beside a control that passes, so a guard that refuses
// every merge fails as surely as one that lets the plant through.

// gitTry runs git as gitIn does and answers its error, for a merge that
// stops on a conflict the journey then resolves by hand.
func gitTry(t *testing.T, dir string, env map[string]string, args ...string) error {
	t.Helper()
	c := exec.CommandContext(t.Context(), "git", "-c", "commit.gpgsign=false")
	c.Args = append(c.Args, args...)
	c.Dir = dir
	c.Env = gitEnv(env)
	return c.Run()
}

// branched is the promote rulebook on main and a branch side from it, made
// by onSide and committed; then onMain on main, committed. The repository is
// left on main.
func branched(t *testing.T, onSide, onMain func(dir string, env map[string]string)) (dir string, env map[string]string) {
	t.Helper()
	dir, env = stagedRepo(t)
	gitIn(t, dir, env, "branch", "-M", "main")
	gitIn(t, dir, env, "checkout", "-q", "-b", "side")
	onSide(dir, env)
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "main")
	onMain(dir, env)
	commitAll(t, dir, env)
	return dir, env
}

// sideMerged is side merged into main and left uncommitted, side holding a
// rule promote of go.errors.2 and main a prose edit of the same file.
func sideMerged(t *testing.T) (dir string, env map[string]string) {
	t.Helper()
	dir, env = branched(t, func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") }, proseEdit(t))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	return dir, env
}

// proseEdit is an edit of a rule's statement, no lifecycle field.
func proseEdit(t *testing.T) func(string, map[string]string) {
	t.Helper()
	return func(dir string, _ map[string]string) {
		edit(t, dir, "language/go/rules/errors.yaml", "statement: Return an error.", "statement: Return an error to the caller.")
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func allowed(t *testing.T, what string, r result) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("%s: exit %d, want done (0)\n%s%s", what, r.code, r.stdout, r.stderr)
	}
}

// TestAMergeJudgesEachCommitItBringsIn (findings 1a, 1b, 1c): a
// merge brings in each of its commits only as that commit would pass on its
// own. A side branch committed with no hook, holding a status changed by
// hand, is refused naming the commit; a stash merged in is refused whether
// the hand edit was staged when it was made or not, and a stash of a promote
// staged whole passes. A tag named MERGE_HEAD is no merge: a forged line
// staged under it is refused as any staged change is, and never as a merge.
// Risk: a hand edit that rides into the rulebook on a merge.
func TestAMergeJudgesEachCommitItBringsIn(t *testing.T) {
	t.Parallel()
	dir, env := branched(t, func(dir string, _ map[string]string) { handPromote(t, dir) }, proseEdit(t))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	refusedNaming(t, "1b: a side branch with a hand edit", staged(t, dir, env), "the merge brings in commit", "status", "rule:go.errors.2", "with no line of history.jsonl")

	stash := func(make func(dir string, env map[string]string)) (string, map[string]string) {
		dir, env := stagedRepo(t)
		make(dir, env)
		gitIn(t, dir, env, "stash", "push", "-q", "-m", "a journey's own")
		gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "stash@{0}")
		return dir, env
	}
	dir, env = stash(func(dir string, env map[string]string) {
		promoteApplied(t, dir, env, "go.errors.2")
		gitIn(t, dir, env, "add", "-A")
	})
	allowed(t, "a stash of a promote staged whole", staged(t, dir, env))
	dir, env = stash(func(dir string, env map[string]string) {
		handPromote(t, dir)
		gitIn(t, dir, env, "add", "-A")
	})
	refusedNaming(t, "1a: a stash of a hand edit staged", staged(t, dir, env), "the merge brings in commit", "status", "rule:go.errors.2")
	dir, env = stash(func(dir string, _ map[string]string) { handPromote(t, dir) })
	refusedNaming(t, "1a: a stash of a hand edit not staged", staged(t, dir, env), "the merge brings in commit", "status", "rule:go.errors.2", "neither side's history records it")

	forged := `"plan":"sha256:` + strings.Repeat("ab", 32) + `"`
	dir, env = branched(t, func(dir string, env map[string]string) {
		promoteApplied(t, dir, env, "go.errors.2")
		write(t, dir, "history.jsonl", planDigest.ReplaceAllString(read(t, dir, "history.jsonl"), forged))
	}, proseEdit(t))
	gitIn(t, dir, env, "tag", "MERGE_HEAD", "side")
	gitIn(t, dir, env, "checkout", "side", "--", "history.jsonl", "decisions.yaml")
	edit(t, dir, "language/go/rules/errors.yaml", "strength: may\n    status: proposed", "strength: may\n    status: active")
	gitIn(t, dir, env, "add", "-A")
	r := staged(t, dir, env)
	refusedNaming(t, "1c: a forged line under a tag named MERGE_HEAD", r, "sha256:"+strings.Repeat("ab", 32), "over the tree before it")
	if strings.Contains(r.stdout+r.stderr, "merge") {
		t.Errorf("1c: a tag named MERGE_HEAD is judged as a merge:\n%s%s", r.stdout, r.stderr)
	}
}

// TestAMergeHoldsALineAndItsChangeTogether (finding 2, A38): the
// merge of a branch's promote passes whole; refused are its change taken
// without its line (2a), its line without its change (2b), a line the
// other side does not hold, its line twice, a last line with no newline, a
// field HEAD changed put back, and a field neither side changed.
// Risk: a merge that splits a line from its change, or makes a change no
// line records.
func TestAMergeHoldsALineAndItsChangeTogether(t *testing.T) {
	t.Parallel()
	dir, env := sideMerged(t)
	allowed(t, "the merge of a branch's promote", staged(t, dir, env))

	for _, tc := range []struct {
		name  string
		plant func(dir string, env map[string]string)
		words []string
	}{
		{"2a: the change without its line", func(dir string, _ map[string]string) {
			// HEAD holds no history.jsonl: the side's is its first line
			if err := os.Remove(filepath.Join(dir, "history.jsonl")); err != nil {
				t.Fatal(err)
			}
		}, []string{"the merge leaves out a line the other side's history.jsonl holds"}},
		{"2b: the line without its change", func(dir string, env map[string]string) {
			edit(t, dir, "language/go/rules/errors.yaml", "strength: may\n    status: active", "strength: may\n    status: proposed")
		}, []string{"the merge leaves status of rule:go.errors.2 at the merge base's value", "the other side's history leaves it at active"}},
		{"a line the other side does not hold", func(dir string, _ map[string]string) {
			h := read(t, dir, "history.jsonl")
			last := h[strings.LastIndex(strings.TrimSuffix(h, "\n"), "\n")+1:]
			write(t, dir, "history.jsonl", h+otherWho(last))
		}, []string{"line 2 of history.jsonl is new in this merge"}},
		{"its line twice", func(dir string, _ map[string]string) {
			h := read(t, dir, "history.jsonl")
			write(t, dir, "history.jsonl", h+h[strings.LastIndex(strings.TrimSuffix(h, "\n"), "\n")+1:])
		}, []string{"line 2 of history.jsonl is the other side's, out of that side's order or twice"}},
		{"a last line with no newline", func(dir string, _ map[string]string) {
			write(t, dir, "history.jsonl", strings.TrimSuffix(read(t, dir, "history.jsonl"), "\n"))
		}, []string{"line 1 of history.jsonl is not ended by a newline"}},
		{"a field neither side changed", func(dir string, _ map[string]string) {
			edit(t, dir, "language/go/rules/errors.yaml", "strength: must\n    status: proposed\n    origin: authored\n    sources:\n      - ref: effective-go\n        backs: rule\n    pairs:",
				"strength: must\n    status: active\n    origin: authored\n    sources:\n      - ref: effective-go\n        backs: rule\n    pairs:")
		}, []string{"the merge changes status of rule:go.errors.1 to active, and neither side's history records it"}},
	} {
		dir, env := sideMerged(t)
		tc.plant(dir, env)
		gitIn(t, dir, env, "add", "-A")
		refusedNaming(t, tc.name, staged(t, dir, env), tc.words...)
	}

	dir, env = branched(t, proseEdit(t), func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") })
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "the merge of a branch's prose into HEAD's promote", staged(t, dir, env))
	edit(t, dir, "language/go/rules/errors.yaml", "strength: may\n    status: active", "strength: may\n    status: proposed")
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "HEAD's change put back", staged(t, dir, env), "the merge leaves status of rule:go.errors.2 at the merge base's value", "HEAD holds active")
}

// TestAMergeRefusesWhatNoLineRecords: a merge of two branches
// at once is refused, as is one over a HEAD that rewrote a line of the merge
// base, and one where both sides changed one field to two values: no line
// records the value the merge picks.
// Risk: a merge judged against the wrong side, or a pick no command made.
func TestAMergeRefusesWhatNoLineRecords(t *testing.T) {
	t.Parallel()
	dir, env := branched(t, func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") }, proseEdit(t))
	gitIn(t, dir, env, "checkout", "-q", "-b", "other", "main~1")
	write(t, dir, "notes.txt", "a file of the other branch\n")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "main")
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side", "other")
	refusedNaming(t, "two branches at once", staged(t, dir, env), "the merge brings in 2 branches at once", "merge one branch at a time")

	dir, env = stagedRepo(t)
	gitIn(t, dir, env, "branch", "-M", "main")
	promoteApplied(t, dir, env, "go.errors.2")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "-b", "side")
	promoteApplied(t, dir, env, "go.errors.1")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "main")
	rewritten := otherWho(read(t, dir, "history.jsonl"))
	write(t, dir, "history.jsonl", rewritten)
	commitAll(t, dir, env)
	_ = gitTry(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	gitIn(t, dir, env, "checkout", "side", "--", "language/go/rules/errors.yaml", "decisions.yaml")
	side := gitIn(t, dir, env, "show", "side:history.jsonl")
	write(t, dir, "history.jsonl", rewritten+side[strings.Index(side, "\n")+1:])
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a HEAD that rewrote the merge base's line", staged(t, dir, env), "HEAD's history.jsonl does not keep every line the merge base has")

	dir, env = branched(t, func(dir string, env map[string]string) {
		if r := run(t, dir, env, with([]string{"rule", "retire", "go.errors.2"}, append(why(), "--apply")...)...); r.code != 0 {
			t.Fatalf("rule retire: exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	}, func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") })
	ours := read(t, dir, "history.jsonl")
	_ = gitTry(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	gitIn(t, dir, env, "checkout", "--ours", "--", "language/go/rules/errors.yaml", "decisions.yaml")
	// each side's decision is kept, as each side's history lines are: HEAD's
	// file, then the other side's entries below its head (the merge base holds
	// no decisions.yaml, so each side made the file)
	sideFile := gitIn(t, dir, env, "show", "side:decisions.yaml")
	_, entries, _ := strings.Cut(sideFile, "decisions:\n")
	write(t, dir, "decisions.yaml", read(t, dir, "decisions.yaml")+entries)
	theirs := gitIn(t, dir, env, "show", "side:history.jsonl")
	write(t, dir, "history.jsonl", ours+theirs)
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "both sides change one field", staged(t, dir, env), "both sides change status of rule:go.errors.2", "HEAD to active and the other side to retired")
}

// TestAMergeRefusesWhatItCannotJudgeInOrder: the other side's
// two lines merged in the order that side wrote them pass, and swapped are
// refused, since each line is made again over the tree before it; a commit
// the merge brings in that itself merges three branches is refused, as the
// merge staged is; and a MERGE_HEAD naming an earlier commit whose rulebook
// this engine cannot read is refused, never judged against an empty base.
// Risk: lines replayed out of the order that makes their plans, or a merge
// judged against a base that was not read.
func TestAMergeRefusesWhatItCannotJudgeInOrder(t *testing.T) {
	t.Parallel()
	dir, env := branched(t, func(dir string, env map[string]string) {
		promoteApplied(t, dir, env, "go.errors.2")
		commitAll(t, dir, env)
		promoteApplied(t, dir, env, "go.errors.1")
	}, proseEdit(t))
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	allowed(t, "control, the other side's two lines in its order", staged(t, dir, env))
	h := read(t, dir, "history.jsonl")
	lines := strings.SplitAfter(strings.TrimSuffix(h, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("the merge holds %d lines, want the other side's two:\n%s", len(lines), h)
	}
	last, before := lines[len(lines)-1]+"\n", lines[len(lines)-2]
	write(t, dir, "history.jsonl", strings.Join(lines[:len(lines)-2], "")+last+before)
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "the other side's lines swapped", staged(t, dir, env), "out of that side's order or twice")

	dir, env = branched(t, func(dir string, env map[string]string) { promoteApplied(t, dir, env, "go.errors.2") }, proseEdit(t))
	for _, b := range []string{"x", "y"} {
		gitIn(t, dir, env, "checkout", "-q", "-b", b, "main~1")
		write(t, dir, b+".txt", "a file of branch "+b+"\n")
		commitAll(t, dir, env)
	}
	gitIn(t, dir, env, "checkout", "-q", "side")
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "-m", "three at once", "x", "y")
	gitIn(t, dir, env, "checkout", "-q", "main")
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	refusedNaming(t, "a commit brought in that merges three", staged(t, dir, env), "the merge brings in commit", "it merges 3 branches at once")

	dir, env = stagedRepo(t)
	edit(t, dir, "rulebook.yaml", "format: 3", "format: 99")
	commitAll(t, dir, env)
	newer := strings.TrimSpace(gitIn(t, dir, env, "rev-parse", "HEAD"))
	edit(t, dir, "rulebook.yaml", "format: 99", "format: 3")
	commitAll(t, dir, env)
	promoteApplied(t, dir, env, "go.errors.2")
	gitIn(t, dir, env, "add", "-A")
	allowed(t, "control, the promote staged with no merge", staged(t, dir, env))
	write(t, dir, filepath.Join(".git", "MERGE_HEAD"), newer+"\n")
	refusedNaming(t, "a merge base this engine cannot read", staged(t, dir, env), "the merge base's rulebook could not be read")
}
