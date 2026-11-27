package journeys

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	"github.com/birdtable/go-proc/gitenv/gittest"
)

// The journeys of guard staged run the built binary over a real git
// repository (tests.doubles.5): the promote rulebook, committed once, and a
// change staged on top of it. The control is a rule promote staged whole, its
// data and its history line; each plant is a change a guard that accepted a
// lone change, an entry with no change, a forged digest or a rewritten line
// would let through.

// stagedRepo is the promote rulebook in a repository of its own with one
// commit, and the environment its rule commands and git run in.
func stagedRepo(t *testing.T) (dir string, env map[string]string) {
	t.Helper()
	dir, env = promoteRepo(t), ruleEnv(t)
	env["GRAGAS_NAMES_FILE"] = ""
	gitIn(t, dir, env, "init", "-q")
	commitAll(t, dir, env)
	if who := strings.TrimSpace(gitIn(t, dir, env, "log", "-1", "--format=%an <%ae>")); who != "Test Person <test@example.test>" {
		t.Fatalf("git committed as %q, not ruleEnv's person: the home's .gitconfig was not read", who)
	}
	return dir, env
}

// gitEnv is the environment a journey's git runs in: gittest.Git of the test's
// home, which reads no settings of the machine, with the fixed person of ruleEnv
// as the identity, so a commit of a journey is by the person the binary's own
// git reads from the home's .gitconfig. What is appended wins.
func gitEnv(env map[string]string) []string {
	return append(gittest.Git(env[homeKey]),
		"GIT_AUTHOR_NAME=Test Person", "GIT_AUTHOR_EMAIL=test@example.test",
		"GIT_COMMITTER_NAME=Test Person", "GIT_COMMITTER_EMAIL=test@example.test")
}

// gitIn runs git in dir as the binary runs it, in gitEnv.
func gitIn(t *testing.T, dir string, env map[string]string, args ...string) string {
	t.Helper()
	c := exec.CommandContext(t.Context(), "git", "-c", "commit.gpgsign=false")
	c.Args = append(c.Args, args...)
	c.Dir = dir
	c.Env = gitEnv(env)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// commitAll stages every file and commits it.
func commitAll(t *testing.T, dir string, env map[string]string) {
	t.Helper()
	msg := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(msg, []byte("a step of the journey\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, env, "add", "-A")
	gitIn(t, dir, env, "commit", "-q", "-F", msg)
}

// promoteApplied runs rule promote id with a decision and --apply, which
// writes the rule, the decision and one history line.
func promoteApplied(t *testing.T, dir string, env map[string]string, id string) {
	t.Helper()
	if r := run(t, dir, env, with([]string{"rule", "promote", id}, append(decide(), "--apply")...)...); r.code != 0 {
		t.Fatalf("rule promote %s: exit %d\n%s%s", id, r.code, r.stdout, r.stderr)
	}
}

// staged runs guard staged in dir, as the git hook runs it.
func staged(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()
	return run(t, dir, env, append([]string{"guard", "staged"}, args...)...)
}

// handPromote is the may's status changed by hand, as an agent's edit makes it.
func handPromote(t *testing.T, dir string) {
	t.Helper()
	edit(t, dir, "language/go/rules/errors.yaml", "strength: may\n    status: proposed", "strength: may\n    status: active")
}

func edit(t *testing.T, dir, name, old, new string) {
	t.Helper()
	p := filepath.Join(dir, name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s does not hold %q:\n%s", name, old, b)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func refusedNaming(t *testing.T, what string, r result, words ...string) {
	t.Helper()
	if r.code != 5 {
		t.Fatalf("%s: exit %d, want refused (5)\n%s%s", what, r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "refused: ") || !strings.Contains(r.stderr, "instead: ") {
		t.Errorf("%s: the refusal is not on standard error, where the hook shows it:\n%s", what, r.stderr)
	}
	// matched with the wrapping undone (flat): a refusal wraps at 80 columns,
	// and where depends on the length of a path or a sha
	for _, w := range words {
		if !strings.Contains(flat(r.stdout+r.stderr), flat(w)) {
			t.Errorf("%s: the refusal does not name %q:\n%s%s", what, w, r.stdout, r.stderr)
		}
	}
}

// TestTheGuardStagedGoldens: a rule promote staged whole, done, and the same
// status staged by hand, refused, as a pipe at 80, 60 and 40 columns and under
// --json, each against its golden, written only with -update by a person who
// reads the diff (tests.generated.5, cli.look.8).
// Risk: the refusal the hook shows changes shape unseen, or stops naming the
// field and the command.
func TestTheGuardStagedGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(t *testing.T, dir string, env map[string]string)
		code   int
	}{
		{"guard-staged-done", func(t *testing.T, dir string, env map[string]string) {
			t.Helper()
			promoteApplied(t, dir, env, "go.errors.2")
		}, 0},
		{"guard-staged-refused", func(t *testing.T, dir string, _ map[string]string) {
			t.Helper()
			handPromote(t, dir)
		}, 5},
	}
	for _, tc := range cases {
		for _, cols := range []string{"80", "60", "40", "json"} {
			dir, env := stagedRepo(t)
			tc.change(t, dir, env)
			gitIn(t, dir, env, "add", "-A")
			var args []string
			name := tc.name + "-" + cols
			if cols == "json" {
				env["COLUMNS"], args, name = "80", []string{"--json"}, "json-"+tc.name
			} else {
				env["COLUMNS"] = cols
			}
			r := staged(t, dir, env, args...)
			if r.code != tc.code {
				t.Fatalf("%s: exit %d, want %d\n%s%s", name, r.code, tc.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, name, transcript(append([]string{"guard", "staged"}, args...), r))
		}
	}
}

// TestGuardStagedAllowsAnUndoAppliedWithTheEntryAsTyped: an undo applied with
// the entry as a number is staged whole and passes, as the replay makes the
// same plan from the command the line stores. The dry run names the entry by
// its digest, so a replay that took that form would make another command and
// another plan digest, and refuse the line.
// Risk: kitSession.Undo sets --entry to the digest on a replay.
func TestGuardStagedAllowsAnUndoAppliedWithTheEntryAsTyped(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	commitAll(t, dir, env)
	if r := run(t, dir, env, "undo", "--entry", "1", "--apply"); r.code != 0 {
		t.Fatalf("undo --entry 1 --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	gitIn(t, dir, env, "add", "-A")
	if r := staged(t, dir, env); r.code != 0 || r.stderr != "" {
		t.Fatalf("the undo with its line: exit %d, want done (0) with nothing on standard error\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestGuardStagedAllowsTheChangeWithItsLineAndRefusesEitherAlone: rule
// promote staged whole passes, and validate agrees; the same field staged by
// hand is refused naming the field and the command; the command's data staged
// without its line is refused the same way; its line staged without the data
// is refused naming the file the line's command writes. Each refusal is kept
// in the journal ('s Done-when, agents.guards.1 and .4).
// Risk: a guard that passes a lone change, or a lone entry, or refuses what the
// command writes.
func TestGuardStagedAllowsTheChangeWithItsLineAndRefusesEitherAlone(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	gitIn(t, dir, env, "add", "-A")
	if r := staged(t, dir, env); r.code != 0 || r.stderr != "" {
		t.Fatalf("the change with its line: exit %d, want done (0) with nothing on standard error\n%s%s", r.code, r.stdout, r.stderr)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate after the allowed change: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}

	dir, env = stagedRepo(t)
	handPromote(t, dir)
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "the field by hand", staged(t, dir, env), "status", "rule:go.errors.2", "gragas rule promote go.errors.2")
	if l := run(t, dir, env, "journal", "list", "--json"); !strings.Contains(l.stdout, "gragas guard staged") {
		t.Errorf("the journal does not list the refusal:\n%s%s", l.stdout, l.stderr)
	}

	dir, env = stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	gitIn(t, dir, env, "add", "language", "decisions.yaml")
	refusedNaming(t, "the change without its line", staged(t, dir, env), "status", "rule:go.errors.2", "no line of history.jsonl")

	dir, env = stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	gitIn(t, dir, env, "add", "history.jsonl")
	refusedNaming(t, "the line without its change", staged(t, dir, env), "is not what the commands of history.jsonl write")
}

// TestGuardStagedRefusesAHandEditStagedWithTheCommandsChange: a promote staged
// whole, with a prose edit of the same file made by hand beside it, is refused:
// the staged file is not what the command writes, so the commit is split.
// Risk: a hand edit rides through on a command's line.
func TestGuardStagedRefusesAHandEditStagedWithTheCommandsChange(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	edit(t, dir, "language/go/rules/errors.yaml", "statement: Wrap an error.", "statement: Wrap an error with what was being done.")
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a hand edit beside the command's", staged(t, dir, env), "language/go/rules/errors.yaml", "commit a hand edit on its own")
}

var planDigest = regexp.MustCompile(`"plan":"sha256:[0-9a-f]{64}"`)

// TestGuardStagedRefusesAForgedDigest: the plan digest of the staged line is
// one the command did not plan; guard staged makes the plan again from the
// tree HEAD holds and refuses it, naming both digests ('s Done-when).
// Risk: a line whose digest is written by hand passes as a plan's.
func TestGuardStagedRefusesAForgedDigest(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	h := read(t, dir, "history.jsonl")
	forged := `"plan":"sha256:` + strings.Repeat("ab", 32) + `"`
	if !planDigest.MatchString(h) {
		t.Fatalf("the line has no plan digest:\n%s", h)
	}
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(planDigest.ReplaceAllString(h, forged)), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a forged digest", staged(t, dir, env), "sha256:"+strings.Repeat("ab", 32), "over the tree before it")
}

// TestGuardStagedRefusesAnEarlierLineRemovedOrRewritten: over a commit that
// holds one promote, a second promote staged whole passes; the same with the
// committed line removed, or with its who rewritten, is refused: the history
// only grows ('s Done-when, store.consistency.4).
// Risk: a commit that rewrites what history already says.
func TestGuardStagedRefusesAnEarlierLineRemovedOrRewritten(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	commitAll(t, dir, env)
	promoteApplied(t, dir, env, "go.errors.1")
	gitIn(t, dir, env, "add", "-A")
	if r := staged(t, dir, env); r.code != 0 {
		t.Fatalf("a second promote over committed history: exit %d, want done\n%s%s", r.code, r.stdout, r.stderr)
	}
	whole := read(t, dir, "history.jsonl")
	lines := strings.SplitAfter(whole, "\n")
	for _, tc := range []struct{ name, history string }{
		{"the earlier line removed", strings.Join(lines[1:], "")},
		{"the earlier line rewritten", otherWho(whole)},
	} {
		if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(tc.history), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, env, "add", "-A")
		refusedNaming(t, tc.name, staged(t, dir, env), "does not keep every line HEAD has")
	}
}

// TestDuringAMergeTheOtherSidesLinesAreTheirs: a merge that brings in a
// branch's promote passes, since the line and its change are the branch's;
// the same merge with a status changed by hand on top is refused.
// Risk: every merge of a recorded change is refused, or a merge carries a
// hand edit through.
func TestDuringAMergeTheOtherSidesLinesAreTheirs(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	gitIn(t, dir, env, "branch", "-M", "main")
	gitIn(t, dir, env, "checkout", "-q", "-b", "side")
	promoteApplied(t, dir, env, "go.errors.2")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "checkout", "-q", "main")
	edit(t, dir, "language/go/rules/errors.yaml", "statement: Return an error.", "statement: Return an error to the caller.")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
	if r := staged(t, dir, env); r.code != 0 {
		t.Fatalf("the merge of a recorded change: exit %d, want done\n%s%s", r.code, r.stdout, r.stderr)
	}
	edit(t, dir, "language/go/rules/errors.yaml", "strength: must\n    status: proposed\n    origin: authored\n    sources:\n      - ref: effective-go\n        backs: rule\n    pairs:",
		"strength: must\n    status: active\n    origin: authored\n    sources:\n      - ref: effective-go\n        backs: rule\n    pairs:")
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a hand edit in a merge", staged(t, dir, env), "status", "rule:go.errors.1")
}

// TestGuardStagedJudgesTheFirstCommitAgainstNoTree: in a repository with no
// commit, init --apply staged whole passes, its line made again over a tree
// that holds nothing; rules staged with no history are refused; in a folder
// that is no rulebook the run is not ready.
// Risk: the first commit of a rulebook passes unjudged, init's own first
// commit is refused, or a project's commit is refused as if it were one.
func TestGuardStagedJudgesTheFirstCommitAgainstNoTree(t *testing.T) {
	t.Parallel()
	env := ruleEnv(t)
	env["GRAGAS_NAMES_FILE"] = ""
	first := newFolder(t)
	if r := run(t, first, env, append([]string{"init"}, append(initFlags(), "--apply")...)...); r.code != 0 {
		t.Fatalf("init --apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	gitIn(t, first, env, "add", "-A")
	if r := staged(t, first, env); r.code != 0 {
		t.Fatalf("init's first commit: exit %d, want done\n%s%s", r.code, r.stdout, r.stderr)
	}

	dir := promoteRepo(t)
	gitIn(t, dir, env, "init", "-q")
	gitIn(t, dir, env, "add", "-A")
	refusedNaming(t, "a first commit of proposed rules with no history", staged(t, dir, env), "no line of history.jsonl")

	empty := t.TempDir()
	gitIn(t, empty, env, "init", "-q")
	if r := staged(t, empty, env); r.code != 4 {
		t.Fatalf("no rulebook: exit %d, want not ready (4)\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// commandField is a history line's command, its JSON string whole.
var commandField = regexp.MustCompile(`"command":"(?:[^"\\]|\\.)*"`)

// TestGuardStagedRefusesALineWhoseCommandIsNotOneToPlan: the staged promote's
// line with its command made into one the replay must not run, or cannot,
// is refused, naming why, and nothing is applied: --apply and --apply=…, a
// read-only command, gen -C over a folder of this host, a settings file of
// this host, a program that is not gragas, a quote left open, and the field
// dropped; another rule's promote is refused by its plan.
// Risk: a line from another author's branch runs a command that writes, or
// reads a folder of this host, or one that is not gragas's, on the machine
// of whoever commits or checks.
func TestGuardStagedRefusesALineWhoseCommandIsNotOneToPlan(t *testing.T) {
	t.Parallel()
	quote := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) }
	for _, c := range []struct {
		name  string
		to    func(was string) string // the new field, from the old one whole
		words []string
	}{
		{"--apply", func(was string) string { return strings.TrimSuffix(was, `"`) + ` --apply"` }, []string{"which the line of a yes never holds"}},
		{"--apply=true", func(was string) string { return strings.TrimSuffix(was, `"`) + ` --apply=true"` }, []string{"which the line of a yes never holds"}},
		{"a read-only command", func(string) string { return `"command":"gragas validate"` }, []string{"gragas validate is not a command that plans a change to the rulebook"}},
		{"gen over a folder of this host", func(string) string { return `"command":"` + quote("gragas gen -C "+os.TempDir()) + `"` }, []string{"gragas gen is not a command that plans a change to the rulebook"}},
		{"a settings file of this host", func(was string) string {
			return strings.TrimSuffix(was, `"`) + ` --config ` + quote(filepath.Join(os.TempDir(), "settings.toml")) + `"`
		}, []string{"it sets --config"}},
		{"another program", func(string) string { return `"command":"git status"` }, []string{"it is not a gragas command"}},
		{"an open quote", func(string) string { return `"command":"` + quote(`gragas rule promote 'go.errors.2`) + `"` }, []string{"a quote is not closed"}},
		{"another rule", func(was string) string { return strings.Replace(was, "go.errors.2", "go.errors.1", 1) }, nil},
		{"no command", func(string) string { return `"command":""` }, []string{"names no command"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, env := stagedRepo(t)
			promoteApplied(t, dir, env, "go.errors.2")
			if r := staged(t, dir, env); r.code != 0 {
				t.Fatalf("control, the promote staged whole: exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			h := read(t, dir, "history.jsonl")
			was := commandField.FindString(h)
			if was == "" || !strings.Contains(was, "go.errors.2") {
				t.Fatalf("the line has no command naming the rule:\n%s", h)
			}
			if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(strings.Replace(h, was, c.to(was), 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			gitIn(t, dir, env, "add", "-A")
			before := read(t, dir, "language/go/rules/errors.yaml")
			refusedNaming(t, c.name, staged(t, dir, env), c.words...)
			if after := read(t, dir, "language/go/rules/errors.yaml"); after != before {
				t.Errorf("%s: the replay changed the working folder:\n%s", c.name, after)
			}
		})
	}
}

// whoField and whenField are a history line's who and when, each whole.
var (
	whoField  = regexp.MustCompile(`"who":"(?:[^"\\]|\\.)*"`)
	whenField = regexp.MustCompile(`"when":"(?:[^"\\]|\\.)*"`)
)

// TestGuardStagedRefusesALineItCannotMakeAgain: the staged
// promote's line, with its who or its when not one a command writes, a field
// no command writes added, the line staged twice, or a line that is not JSON
// after it, is refused, each beside the control of the promote staged whole,
// which passes. Risk: a line let through that its command did not write.
func TestGuardStagedRefusesALineItCannotMakeAgain(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		to    func(h string) string
		words []string
	}{
		{"who a person, not its command", func(h string) string { return whoField.ReplaceAllString(h, `"who":"Test Person <test@example.test>"`) }, []string{"is not the line its command writes"}},
		{"when not an instant", func(h string) string { return whenField.ReplaceAllString(h, `"when":"yesterday"`) }, []string{"names when it was made as yesterday"}},
		{"a field no command writes", func(h string) string {
			i := strings.LastIndex(strings.TrimSuffix(h, "\n"), "\n") + 1
			return h[:i] + `{"about":"a field no command writes",` + h[i+1:]
		}, []string{"is not the line its command writes over the tree before it"}},
		{"the line twice", func(h string) string {
			i := strings.LastIndex(strings.TrimSuffix(h, "\n"), "\n") + 1
			return h + h[i:]
		}, []string{"gragas rule promote go.errors.2", "over the tree before it"}},
		{"a line that is not JSON", func(h string) string { return h + "not a line of JSON\n" }, []string{"is not an entry"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, env := stagedRepo(t)
			promoteApplied(t, dir, env, "go.errors.2")
			gitIn(t, dir, env, "add", "-A")
			allowed(t, "control, the promote staged whole", staged(t, dir, env))
			h := read(t, dir, "history.jsonl")
			if got := c.to(h); got == h {
				t.Fatalf("the plant changed nothing in:\n%s", h)
			} else {
				write(t, dir, "history.jsonl", got)
			}
			gitIn(t, dir, env, "add", "-A")
			refusedNaming(t, c.name, staged(t, dir, env), c.words...)
		})
	}
}

// TestGuardStagedRefusesARulebookItCannotReadOrPlace: a HEAD
// whose rulebook this engine cannot read, and a staged change that moves the
// rulebook to another folder, are refused; a merge that brings in a commit
// removing the rulebook, or one whose rulebook cannot be read, is refused
// naming that commit, though the side branch puts the rulebook back after it.
// Each starts from a control that passes. Risk: a change judged against a
// rulebook that was not read, or not judged at all because it moved.
func TestGuardStagedRefusesARulebookItCannotReadOrPlace(t *testing.T) {
	t.Parallel()
	t.Run("HEAD not ready", func(t *testing.T) {
		t.Parallel()
		dir, env := stagedRepo(t)
		edit(t, dir, "rulebook.yaml", "format: 3", "format: 99")
		commitAll(t, dir, env)
		edit(t, dir, "rulebook.yaml", "format: 99", "format: 3")
		gitIn(t, dir, env, "add", "-A")
		refusedNaming(t, "HEAD not ready", staged(t, dir, env), "HEAD's rulebook could not be read, so the staged change cannot be judged")
	})
	t.Run("moved", func(t *testing.T) {
		t.Parallel()
		dir, env := stagedRepo(t)
		allowed(t, "control, nothing staged", staged(t, dir, env))
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
		refusedNaming(t, "moved", staged(t, dir, env), "moves the rulebook from the root to book")
	})
	for _, c := range []struct {
		name       string
		gone, back func(dir string, env map[string]string)
		words      []string
	}{
		{
			"a commit brought in that removes the rulebook",
			func(dir string, env map[string]string) { gitIn(t, dir, env, "rm", "-q", "rulebook.yaml") },
			func(dir string, env map[string]string) {
				gitIn(t, dir, env, "checkout", "-q", "main", "--", "rulebook.yaml")
			},
			[]string{"the merge brings in commit", "removes the rulebook its parent holds"},
		},
		{
			"a commit brought in that is not ready",
			func(dir string, _ map[string]string) { edit(t, dir, "rulebook.yaml", "format: 3", "format: 99") },
			func(dir string, _ map[string]string) { edit(t, dir, "rulebook.yaml", "format: 99", "format: 3") },
			[]string{"the merge brings in commit", "'s rulebook could not be read, so it cannot be judged"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, env := branched(t, func(dir string, env map[string]string) {
				c.gone(dir, env)
				commitAll(t, dir, env)
				c.back(dir, env)
			}, proseEdit(t))
			if gitTry(t, dir, env, "diff", "--quiet", "main~1", "side") != nil {
				t.Fatal("side does not end where it began, so the control is not one")
			}
			gitIn(t, dir, env, "merge", "-q", "--no-ff", "--no-commit", "side")
			refusedNaming(t, c.name, staged(t, dir, env), c.words...)
		})
	}
}

// otherWho is s with the first who it holds made someone else's: a history
// line changed after it was written.
func otherWho(s string) string {
	at := whoField.FindStringIndex(s)
	if at == nil {
		return s
	}
	return s[:at[0]] + `"who":"someone else"` + s[at[1]:]
}
