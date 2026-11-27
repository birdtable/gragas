package journeys

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoWriterOfHistoryPutsTheRunnersGitIdentityIntoTheBook: each command
// that appends to history.jsonl (init, migrate, and the rule family through
// rule new) runs with a git identity set twice, in the account's home config
// (ruleEnv's Test Person) and in the repository's own (newFolder's A Person),
// and applies. No file it leaves outside .git holds any part of either, and
// each history line names the command as who made it: a
// rulebook is shared and published, and the names rule keeps a person's name
// and address out of it. Planted failure: a writer that reads git's
// user.name or user.email for who, an owner or a decision.
func TestNoWriterOfHistoryPutsTheRunnersGitIdentityIntoTheBook(t *testing.T) {
	t.Parallel()
	person := []string{"Test Person", "test@example.test", "A Person", "a@example.test"}
	for _, tc := range []struct {
		name, who string
		dir       func(t *testing.T) string
		args      []string
	}{
		{"init", "gragas init", newFolder, append([]string{"init"}, initFlags()...)},
		{"migrate", "gragas migrate", v2Folder, append([]string{"migrate"}, migrateFlags()...)},
		{"rule new", "gragas rule new", func(t *testing.T) string {
			t.Helper()
			dir := newFolder(t)
			copyTree(t, filepath.Join(moduleRoot(), "internal/journeys/testdata/history/rulebook"), dir)
			return dir
		}, newArgs()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, env := tc.dir(t), ruleEnv(t)
			env["COLUMNS"], env["GRAGAS_NAMES_FILE"] = "80", ""
			if r := run(t, dir, env, append(tc.args, "--apply")...); r.code != 0 {
				t.Fatalf("%s --apply: exit %d\n%s%s", tc.name, r.code, r.stdout, r.stderr)
			}
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				switch {
				case err != nil:
					return err
				case d.IsDir() && d.Name() == ".git":
					return filepath.SkipDir
				case d.IsDir():
					return nil
				}
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				for _, part := range person {
					if strings.Contains(string(b), part) {
						rel, _ := filepath.Rel(dir, p)
						t.Errorf("%s holds %q, a part of the runner's git identity", filepath.ToSlash(rel), part)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			hist := strings.TrimSpace(read(t, dir, "history.jsonl"))
			if hist == "" {
				t.Fatal("history.jsonl holds no line")
			}
			for line := range strings.SplitSeq(hist, "\n") {
				if got := whoField.FindString(line); got != `"who":"`+tc.who+`"` {
					t.Errorf("a history line names %s, want who %q:\n%s", got, tc.who, line)
				}
			}
		})
	}
}

// TestInitWithNoOwnerEndsUsageAndReadsNoOwnerFromGit: with git naming a
// person in the home config and in the repository's, init with no --owner
// does not take git's user.name for the book's owners: it ends on its usage
// line, exit 2, and writes nothing. Planted failure: an init
// that fills --owner from git.
func TestInitWithNoOwnerEndsUsageAndReadsNoOwnerFromGit(t *testing.T) {
	t.Parallel()
	dir, env := newFolder(t), ruleEnv(t)
	env["COLUMNS"], env["GRAGAS_NAMES_FILE"] = "80", ""
	r := run(t, dir, env, "init", "--kind", "rulebook", "--name", "demo", "--apply")
	if r.code != 2 || !strings.Contains(flat(r.stdout+r.stderr), "a rulebook has at least one owner: name it with --owner") {
		t.Errorf("init with no --owner: exit %d, want usage (2) naming --owner\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, name := range []string{"rulebook.yaml", "history.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("init with no --owner wrote %s", name)
		}
	}
}
