package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestARebindToAValueThePluginRanksTighterIsShownAndWrittenWithNoDecision
// (Must on): go.errors.4 binds the
// standin tool at bad with the value 3, and the standin plugin, added and
// approved, ranks a smaller whole number tighter. A rebind to 2, a value
// whose text differs, needs no decision: its dry run shows 3 becoming 2,
// ends declined on the apply line and writes nothing, and --apply writes the value and one history line, each
// reading asking the plugin afresh. A rebind to 5, which it ranks weaker, is
// refused for a decision and writes nothing. The plant is rankers closed
// after the first reading, which fails the dry run on a loosening.
func TestARebindToAValueThePluginRanksTighterIsShownAndWrittenWithNoDecision(t *testing.T) {
	t.Parallel()
	dir, env := recordProject(t, recordOpts{})
	bound := strings.Replace(standinRule, "key: bad, severity: error,", "key: bad, value: 3, severity: error,", 1)
	put(t, filepath.Join(dir, "language", "go", "rules"), map[string]string{"errors.yaml": promoteRules + bound})
	rebind := func(value string) []string {
		return []string{"rebind", "go.errors.4", "--tool", "standin", "--target", "source", "--key", "bad", "--value", value}
	}
	rules := func() string { return read(t, dir, "language/go/rules/errors.yaml") }
	// the project has no history until its first change
	hist := func(*testing.T, string) int {
		b, err := os.ReadFile(filepath.Join(dir, "history.jsonl"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return strings.Count(string(b), "\n")
	}
	before, lines := rules(), hist(t, dir)

	r := run(t, dir, env, rebind("5")...)
	if r.code != 5 || !strings.Contains(flat(r.stdout+r.stderr), "decision") || rules() != before || hist(t, dir) != lines {
		t.Errorf("a weaker value: exit %d, want 5 asking for a decision, nothing written\n%s%s", r.code, r.stdout, r.stderr)
	}

	r = run(t, dir, env, rebind("2")...)
	if r.code != 6 || !strings.Contains(r.stdout, "value: 3 becomes 2") || !strings.Contains(r.stdout, "Add --apply") || rules() != before || hist(t, dir) != lines {
		t.Fatalf("the dry run of a tighter value: exit %d, want 6 showing 3 becoming 2 and the apply line, nothing written\n%s%s", r.code, r.stdout, r.stderr)
	}
	r = run(t, dir, env, append(rebind("2"), "--apply")...)
	if r.code != 0 || lastEnding(t, env) != "done" || !strings.Contains(rules(), "value: 2") ||
		hist(t, dir) != lines+1 {
		t.Errorf("the apply of a tighter value: exit %d, ending %q, history %d of %d\n%s%s\n%s", r.code, lastEnding(t, env), hist(t, dir), lines+1, r.stdout, r.stderr, rules())
	}
}
