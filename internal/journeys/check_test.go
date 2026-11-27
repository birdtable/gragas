package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// The journeys of check in a rulebook run the built binary over a real
// git repository (tests.doubles.5): guard staged's promote rulebook, committed
// once and branched as base, then commits made on top of it with no hook
// armed, as a commit made with --no-verify is. The control is a rule promote
// committed whole; each plant is a change the gate exists to find.

// gateBase is the branch the journeys name as the base ref.
const gateBase = "base"

// gateRepo is stagedRepo with its one commit branched as gateBase, and the
// base ref named in the environment.
func gateRepo(t *testing.T) (dir string, env map[string]string) {
	t.Helper()
	dir, env = stagedRepo(t)
	gitIn(t, dir, env, "branch", gateBase)
	env["GRAGAS_BASE_REF"] = gateBase
	return dir, env
}

// gateCheck runs check in dir.
func gateCheck(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()
	return run(t, dir, env, append([]string{"check"}, args...)...)
}

// headShort is HEAD's commit as the gate names it: its first twelve digits.
func headShort(t *testing.T, dir string, env map[string]string) string {
	t.Helper()
	return strings.TrimSpace(gitIn(t, dir, env, "rev-parse", "--short=12", "HEAD"))
}

// TestCheckPassesACommandsChangeAndListsAStatusCommittedByHand: a rule
// promote committed with its line ends done; the same status committed by
// hand, with no hook to refuse it, is a gate finding naming the commit, the
// field and the command that records it (agents.guards.1).
// Risk: a hand edit that bypassed the hook passes the gate, or a command's own
// change is held to be one.
func TestCheckPassesACommandsChangeAndListsAStatusCommittedByHand(t *testing.T) {
	t.Parallel()
	dir, env := gateRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	commitAll(t, dir, env)
	if r := gateCheck(t, dir, env); r.code != 0 || !strings.Contains(flat(r.stdout), "0 gate findings against base, 1 commit") {
		t.Fatalf("a promote committed whole: exit %d, want done (0) over 1 commit\n%s%s", r.code, r.stdout, r.stderr)
	}

	dir, env = gateRepo(t)
	handPromote(t, dir)
	commitAll(t, dir, env)
	r := gateCheck(t, dir, env)
	if r.code != 1 {
		t.Fatalf("a status committed by hand: exit %d, want findings (1)\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, w := range []string{"lifecycle field changed without its entry", "commit " + headShort(t, dir, env), "rule:go.errors.2", "gragas rule promote go.errors.2"} {
		if !strings.Contains(flat(r.stdout), flat(w)) {
			t.Errorf("the listing does not name %q:\n%s%s", w, r.stdout, r.stderr)
		}
	}
}

// TestCheckListsALineOfTheBaseRemovedFromTheHistory: the promote's line on
// base, then removed on the branch, is a rewritten line of the merge base.
// Risk: an entry the base shipped is taken back unseen.
func TestCheckListsALineOfTheBaseRemovedFromTheHistory(t *testing.T) {
	t.Parallel()
	dir, env := stagedRepo(t)
	promoteApplied(t, dir, env, "go.errors.2")
	commitAll(t, dir, env)
	gitIn(t, dir, env, "branch", gateBase)
	env["GRAGAS_BASE_REF"] = gateBase
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, env)
	r := gateCheck(t, dir, env)
	if r.code != 1 || !strings.Contains(flat(r.stdout), "line 1 of history.jsonl at the merge base is removed or rewritten") {
		t.Errorf("a line of the base removed: exit %d, want findings (1) naming the rewritten history\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestCheckIsNotReadyWithNoBaseToHoldTheHistoryTo: no base ref named, and a
// base ref the repository lacks, each end not ready with the line to type;
// check never guesses a base.
// Risk: a gate that passes because it had nothing to compare against.
func TestCheckIsNotReadyWithNoBaseToHoldTheHistoryTo(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		ref   string
		names []string
	}{
		"none named":   {"", []string{"base-ref", "base-ref: origin/main"}},
		"not in reach": {"origin/gone", []string{"origin/gone", "git fetch origin"}},
	} {
		dir, env := stagedRepo(t)
		env["GRAGAS_BASE_REF"] = c.ref
		r := gateCheck(t, dir, env)
		if r.code != 4 {
			t.Fatalf("%s: exit %d, want not ready (4)\n%s%s", name, r.code, r.stdout, r.stderr)
		}
		for _, w := range c.names {
			if !strings.Contains(flat(r.stdout+r.stderr), flat(w)) {
				t.Errorf("%s: the run does not name %q:\n%s%s", name, w, r.stdout, r.stderr)
			}
		}
	}
}

// TestCheckUnderJSONIsOneDocumentWithTheGateAndWhatIsNotAvailable: the
// status committed by hand, under --json, is check's document on findings:
// the merge base, the commit counted, the gate finding of its kind, and each
// value not available yet, null with why (cli.output.9).
// Risk: a document that drops the gate, or says a value it cannot give.
func TestCheckUnderJSONIsOneDocumentWithTheGateAndWhatIsNotAvailable(t *testing.T) {
	t.Parallel()
	dir, env := gateRepo(t)
	base := strings.TrimSpace(gitIn(t, dir, env, "rev-parse", gateBase))
	handPromote(t, dir)
	commitAll(t, dir, env)
	r := gateCheck(t, dir, env, "--json")
	if r.code != 1 {
		t.Fatalf("exit %d, want findings (1)\n%s%s", r.code, r.stdout, r.stderr)
	}
	var doc struct {
		Command, Word, Base string
		BaseRef             string `json:"base_ref"`
		Commits             int
		Gate                []struct{ Kind, Msg string }
		NotAvailable        []struct {
			Name   string
			Value  *json.RawMessage
			Reason string
		} `json:"not_available"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatalf("standard output is not one document: %v\n%s", err, r.stdout)
	}
	if doc.Command != "check" || doc.Word != "findings" || doc.BaseRef != gateBase || doc.Base != base || doc.Commits != 1 {
		t.Errorf("the document: %+v; want check's, on findings, at the merge base %s over 1 commit", doc, base)
	}
	if len(doc.Gate) == 0 || doc.Gate[0].Kind != "lifecycle field changed without its entry" || !strings.Contains(doc.Gate[0].Msg, headShort(t, dir, env)) {
		t.Errorf("the gate: %+v; want the hand edit, naming its commit", doc.Gate)
	}
	if len(doc.NotAvailable) == 0 {
		t.Error("nothing is said to be not available")
	}
	for _, u := range doc.NotAvailable {
		if u.Value != nil || strings.TrimSpace(u.Reason) == "" {
			t.Errorf("%s: value %v, reason %q; want null and why", u.Name, u.Value, u.Reason)
		}
	}
}

// TestTheCheckGoldens: check done over a promote committed whole, and not
// ready with no base ref, as a pipe at 80 and 40 columns, each against its
// golden, written only with -update by a person who reads the diff
// (tests.generated.5, cli.look.8). A finding names a commit, whose digits
// change run to run, so the findings are held by the journeys above.
// Risk: the gate's listing changes shape unseen, or stops saying what it
// cannot give yet.
func TestTheCheckGoldens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ref  string
		code int
	}{
		{"check-gate-done", gateBase, 0},
		{"check-gate-no-base", "", 4},
	} {
		for _, cols := range []string{"80", "40"} {
			dir, env := gateRepo(t)
			promoteApplied(t, dir, env, "go.errors.2")
			commitAll(t, dir, env)
			env["GRAGAS_BASE_REF"], env["COLUMNS"] = tc.ref, cols
			r := gateCheck(t, dir, env)
			if r.code != tc.code {
				t.Fatalf("%s at %s: exit %d, want %d\n%s%s", tc.name, cols, r.code, tc.code, r.stdout, r.stderr)
			}
			uitest.Golden(t, tc.name+"-"+cols, transcript([]string{"check"}, r))
		}
	}
}

// TestCheckTakesADeeperRulebookForNoBook: the shallowest rulebook.yaml
// is the book, so one planted below the root is not a second book and check
// ends done.
func TestCheckTakesADeeperRulebookForNoBook(t *testing.T) {
	t.Parallel()
	dir, env := gateRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray", "rulebook.yaml"), []byte(read(t, dir, "rulebook.yaml")), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, env)
	if r := gateCheck(t, dir, env); r.code != 0 || strings.Contains(flat(r.stdout), "more than one") {
		t.Errorf("exit %d, want done (0) with the deeper rulebook.yaml not a book\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestCheckListsARulebookTheBranchRemoved: where the merge base holds one
// rulebook, the branch's removing it is a gate finding; the old check never
// runs in its place.
// Risk: a branch turns the gate off by what it holds.
func TestCheckListsARulebookTheBranchRemoved(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		change func(t *testing.T, dir string, env map[string]string)
		says   string
	}{
		"removed": {func(t *testing.T, dir string, env map[string]string) {
			t.Helper()
			gitIn(t, dir, env, "rm", "-q", "rulebook.yaml")
		}, "the working folder holds no rulebook.yaml"},
	} {
		dir, env := gateRepo(t)
		c.change(t, dir, env)
		commitAll(t, dir, env)
		r := gateCheck(t, dir, env)
		if r.code != 1 || !strings.Contains(flat(r.stdout), flat(c.says+", where the merge base holds one (rulebook.yaml)")) {
			t.Errorf("%s: exit %d, want findings (1) saying %q\n%s%s", name, r.code, c.says, r.stdout, r.stderr)
		}
	}
}
