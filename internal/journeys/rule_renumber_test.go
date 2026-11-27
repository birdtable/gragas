package journeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mergedErrors = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.2
    aliases: [go.errors.1]
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
  - id: go.errors.2
    statement: Name an error.
    strength: should
    status: proposed
    origin: authored
`

// TestRenumberResolvesAnIdTwoMergedBranchesTook: two branches each added a
// rule as go.errors.2 and were merged; validate lists the clash and ends
// refused; rule renumber on the id gives the later rule go.errors.3 and keeps
// go.errors.2 on the first; validate ends done; the old id still names the first rule
// (it names the rule that kept it); the same command again is
// refused naming the rule's id now, and writes nothing; no "#" is in a file,
// the history or any output. Risk: the rule that kept the id is renumbered,
// the clash stays, or a place in a file leaks into what is written.
func TestRenumberResolvesAnIdTwoMergedBranchesTook(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	file := filepath.Join(dir, "language/go/rules/errors.yaml")
	if err := os.WriteFile(file, []byte(mergedErrors), 0o600); err != nil {
		t.Fatal(err)
	}
	before := run(t, dir, env, "validate")
	if before.code == 0 || !strings.Contains(before.stdout+before.stderr, "go.errors.2") {
		t.Fatalf("validate on a clash: exit %d, want a finding naming go.errors.2\n%s%s", before.code, before.stdout, before.stderr)
	}
	out := run(t, dir, env, with([]string{"rule", "renumber", "go.errors.2"}, append(why(), "--apply")...)...)
	if out.code != 0 || !strings.Contains(out.stdout, "done") {
		t.Fatalf("renumber: exit %d\n%s%s", out.code, out.stdout, out.stderr)
	}
	rules := read(t, dir, "language/go/rules/errors.yaml")
	first, later, _ := strings.Cut(rules, "id: go.errors.3")
	if strings.Contains(first, "go.errors.3") || !strings.Contains(first, "go.errors.1") || !strings.Contains(rules, "id: go.errors.3") || strings.Contains(later, "go.errors.2") {
		t.Errorf("the rule that kept the id was renumbered, or the later one claims the id as an alias:\n%s", rules)
	}
	if v := run(t, dir, env, "validate"); v.code != 0 {
		t.Fatalf("validate after the renumber: exit %d\n%s%s", v.code, v.stdout, v.stderr)
	}
	if h := run(t, dir, env, "history", "go.errors.3"); h.code != 0 || !strings.Contains(h.stdout, "renumber") {
		t.Errorf("the history of the new id lacks the renumber: exit %d\n%s%s", h.code, h.stdout, h.stderr)
	}
	for name, s := range map[string]string{
		"rules": rules, "history": read(t, dir, "history.jsonl"), "decisions": read(t, dir, "decisions.yaml"),
		"stdout": out.stdout, "stderr": out.stderr,
	} {
		if strings.Contains(s, "#") {
			t.Errorf("%s holds a #:\n%s", name, s)
		}
	}
	again := run(t, dir, env, with([]string{"rule", "renumber", "go.errors.2"}, append(why(), "--apply")...)...)
	if again.code == 0 && !strings.Contains(again.stdout, "nothing to do") {
		t.Errorf("renumber twice: exit %d\n%s%s", again.code, again.stdout, again.stderr)
	}
	if read(t, dir, "language/go/rules/errors.yaml") != rules {
		t.Error("the second run wrote")
	}
}
