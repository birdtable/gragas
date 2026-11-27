package journeys

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureBooks writes a rulebook.yaml in each of the folders, as the two
// fixture books of a repository sit at one depth, and commits them: a tree
// with no one book in force.
func fixtureBooks(t *testing.T, dir string, env map[string]string, folders ...string) {
	t.Helper()
	for _, f := range folders {
		if err := os.MkdirAll(filepath.Join(dir, f), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, dir, f+"/rulebook.yaml", "format: 3\n")
	}
	commitAll(t, dir, env)
}

// TestGuardStagedJudgesTheCommitThatMakesTheRulebookOverTwoFixtureBooks
// : a tree before that holds two rulebook.yaml at one depth and
// none in force, and a tree after with exactly one, is the commit that makes the
// rulebook: its history is replayed from a tree with nothing in it, as for a
// tree with no book. Two before and two after are still refused as not ready,
// and one book before in another folder is still the move refusal.
// Risk: the commit that makes the rulebook beside fixture books cannot be
// judged, so it is red at landing with no way to accept it; or the road widens
// and a tree with two books, or a moved book, passes unjudged.
func TestGuardStagedJudgesTheCommitThatMakesTheRulebookOverTwoFixtureBooks(t *testing.T) {
	t.Parallel()
	t.Run("two before, one after", func(t *testing.T) {
		t.Parallel()
		dir := newFolder(t)
		env := ruleEnv(t)
		env["GRAGAS_NAMES_FILE"] = ""
		fixtureBooks(t, dir, env, "fixture/base", "fixture/other")
		if r := run(t, dir, env, append([]string{"init"}, append(initFlags(), "--apply")...)...); r.code != 0 {
			t.Fatalf("init --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
		gitIn(t, dir, env, "add", "-A")
		allowed(t, "the commit that makes the rulebook", staged(t, dir, env))
		// the replay is no pass for any line: a hand-edited one is refused
		edit(t, dir, "history.jsonl", `"plan":"`, `"plan":"x`)
		gitIn(t, dir, env, "add", "-A")
		refusedNaming(t, "a forged digest in that commit", staged(t, dir, env), "names the plan")
	})
	t.Run("two before, two after", func(t *testing.T) {
		t.Parallel()
		dir, env := stagedRepo(t)
		gitIn(t, dir, env, "rm", "-rq", ".")
		commitAll(t, dir, env)
		fixtureBooks(t, dir, env, "fixture/base", "fixture/other")
		write(t, dir, "fixture/other/note.txt", "a change\n")
		gitIn(t, dir, env, "add", "-A")
		if r := staged(t, dir, env); r.code != 4 {
			t.Fatalf("two books before and after: exit %d, want not ready (4)\n%s%s", r.code, r.stdout, r.stderr)
		}
	})
	t.Run("one before in another folder", func(t *testing.T) {
		t.Parallel()
		dir, env := stagedRepo(t)
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
		commitAll(t, dir, env)
		for _, e := range entries {
			if e.Name() != ".git" {
				gitIn(t, dir, env, "mv", "book/"+e.Name(), e.Name())
			}
		}
		refusedNaming(t, "a book moved to the root", staged(t, dir, env), "moves the rulebook from book to the root")
	})
}

// beforeTheSwitch is a repository on main whose tree holds two fixture books
// and no book in force, a branch side from it made by onSide, and main moved
// on by a file; side is merged into main and left uncommitted, as the hook
// meets it.
func beforeTheSwitch(t *testing.T, onSide func(dir string, env map[string]string)) (dir string, env map[string]string) {
	t.Helper()
	dir, env = newFolder(t), ruleEnv(t)
	env["GRAGAS_NAMES_FILE"] = ""
	fixtureBooks(t, dir, env, "fixture/base", "fixture/other")
	gitIn(t, dir, env, "branch", "-M", "main")
	gitIn(t, dir, env, "checkout", "-q", "-b", "side")
	onSide(dir, env)
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "main")
	write(t, dir, "note.txt", "main moved on\n")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	return dir, env
}

// makesTheBook is the side that makes the rulebook with init, as release-1
// does at the switch.
func makesTheBook(t *testing.T) func(string, map[string]string) {
	t.Helper()
	return func(dir string, env map[string]string) {
		if r := run(t, dir, env, append([]string{"init"}, append(initFlags(), "--apply")...)...); r.code != 0 {
			t.Fatalf("init --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	}
}

// TestGuardStagedJudgesAMergeWhoseMergeBaseHoldsTwoBooks: a
// branch cut before the switch, its tree and the merge base's with two
// fixture books and no book in force, merging a branch that makes the book is
// judged, as the commit that makes the book is: the merge base is read as a
// tree with nothing in it. A forged plan digest in the commit brought in is
// still refused, and two books after are still not ready.
// Risk: every branch cut before the switch is refused at its next merge of
// release-1, with no way to accept it; or the road widens and a merge over a
// base with two books passes unjudged.
func TestGuardStagedJudgesAMergeWhoseMergeBaseHoldsTwoBooks(t *testing.T) {
	t.Parallel()
	t.Run("the book made on the other side", func(t *testing.T) {
		t.Parallel()
		dir, env := beforeTheSwitch(t, makesTheBook(t))
		allowed(t, "a merge over a base with two books", staged(t, dir, env))
	})
	t.Run("a forged plan digest in the commit brought in", func(t *testing.T) {
		t.Parallel()
		dir, env := beforeTheSwitch(t, func(dir string, env map[string]string) {
			makesTheBook(t)(dir, env)
			edit(t, dir, "history.jsonl", `"plan":"`, `"plan":"x`)
		})
		refusedNaming(t, "a forged digest", staged(t, dir, env), "names the plan")
	})
	t.Run("two books after", func(t *testing.T) {
		t.Parallel()
		dir, env := beforeTheSwitch(t, func(dir string, env map[string]string) {
			if err := os.MkdirAll(filepath.Join(dir, "fixture/third"), 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, dir, "fixture/third/rulebook.yaml", "format: 3\n")
		})
		if r := staged(t, dir, env); r.code != 4 {
			t.Fatalf("two books after the merge: exit %d, want not ready (4)\n%s%s", r.code, r.stdout, r.stderr)
		}
	})
}

// TestGuardStagedJudgesTheMigrateOverAFormat2Tree: the commit
// that migrates a format 2 rulebook has a tree before with no rulebook.yaml,
// which is no book, or, beside two fixture books, no one book. Its line is a
// migrate, which plans over the format 2 folder, so the tree before is not
// emptied: the line is replayed over it, its digest held, and the commit
// allowed. A forged digest in that commit is still refused.
// Risk: the guard replays the migrate over a tree with no file, where it has
// nothing to do, and refuses the switch to the format at landing with no way
// to accept it.
func TestGuardStagedJudgesTheMigrateOverAFormat2Tree(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		fixtures []string
	}{
		{"no book before", nil},
		{"two fixture books before", []string{"fixture/base", "fixture/other"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := v2Folder(t)
			env := ruleEnv(t)
			env["GRAGAS_NAMES_FILE"] = ""
			commitAll(t, dir, env)
			if len(c.fixtures) > 0 {
				fixtureBooks(t, dir, env, c.fixtures...)
			}
			if r := run(t, dir, env, append(append([]string{"migrate"}, migrateFlags()...), "--apply")...); r.code != 0 {
				t.Fatalf("migrate --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			gitIn(t, dir, env, "add", "-A")
			allowed(t, "the commit that migrates the rulebook", staged(t, dir, env))
			edit(t, dir, "history.jsonl", `"plan":"`, `"plan":"x`)
			gitIn(t, dir, env, "add", "-A")
			refusedNaming(t, "a forged digest in that commit", staged(t, dir, env), "names the plan")
		})
	}
}
