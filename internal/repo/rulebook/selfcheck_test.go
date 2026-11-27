package rulebook

// The register of docs/self-check.md is held to the rulebook (docs.freshness.4,
// gates.command.31): an active must rule of the gates, supply, agents, ts and
// cli parts is held (bound, and shipped in the generated preset: the engine's own
// shipped list) or has a row in the register, and a rule in neither is red. A row may name a whole
// part as "<part>.*".

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/repo/gatetest"
)

// cycleParts are the parts the register is held to.
func cycleParts() []string { return []string{"gates", "supply", "agents", "ts", "cli"} }

// ruleRow is a rule as the register test judges it. Held says a binding of the rule is shipped.
type ruleRow struct {
	ID, Part, Strength, Status string
	Held                       bool
}

var (
	idFirst = regexp.MustCompile(`^([a-z]+\.[a-z-]+)\.(\d+)(?: to \.(\d+))?$`)
	idMore  = regexp.MustCompile(`^\.(\d+)(?: to \.(\d+))?$`)
	idPart  = regexp.MustCompile(`^([a-z]+)\.\*$`)
)

// registered are the rule ids the first cell of each table row of a register
// names: "a.b.1, .8", "a.b.1 to .4" and "a.b.1, .2; c.d.3", and "a.*" for a
// part, kept as "a.*".
func registered(text string) map[string]bool {
	out := map[string]bool{}
	for line := range strings.SplitSeq(text, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 2 {
			continue
		}
		for group := range strings.SplitSeq(cells[1], ";") {
			prefix := ""
			for tok := range strings.SplitSeq(group, ",") {
				tok = strings.TrimSpace(strings.Trim(tok, "` "))
				if m := idPart.FindStringSubmatch(tok); m != nil {
					out[m[1]+".*"] = true
					continue
				}
				m := idFirst.FindStringSubmatch(tok)
				from, to := 0, 0
				switch {
				case m != nil:
					prefix = m[1]
					from, _ = strconv.Atoi(m[2])
					to = from
					if m[3] != "" {
						to, _ = strconv.Atoi(m[3])
					}
				case prefix != "" && idMore.MatchString(tok):
					m = idMore.FindStringSubmatch(tok)
					from, _ = strconv.Atoi(m[1])
					to = from
					if m[2] != "" {
						to, _ = strconv.Atoi(m[2])
					}
				default:
					continue
				}
				for n := from; n <= to; n++ {
					out[prefix+"."+strconv.Itoa(n)] = true
				}
			}
		}
	}
	return out
}

// unaccounted are the active must rules of the cycle's parts that no resolved
// check holds and no row of the register names.
func unaccounted(rules []ruleRow, register map[string]bool) []string {
	var out []string
	for _, r := range rules {
		if r.Strength != "must" || r.Status != "active" || !slices.Contains(cycleParts(), r.Part) {
			continue
		}
		if r.Held || register[r.ID] || register[r.Part+".*"] {
			continue
		}
		out = append(out, r.ID+": a must rule of "+r.Part+" that no binding ships and no row in docs/self-check.md")
	}
	return out
}

func readRules(t *testing.T) []ruleRow {
	t.Helper()
	book := gatetest.Of(t, root)
	rules, err := gate.ReadRules(book.Rulebook)
	if err != nil {
		t.Fatal(err)
	}
	shipped := book.Shipped(t)
	out := make([]ruleRow, 0, len(rules))
	for _, r := range rules {
		out = append(out, ruleRow{ID: r.ID, Part: r.Part, Strength: r.Strength, Status: r.Status, Held: shipped[r.ID]})
	}
	return out
}

// TestEveryMustRuleOfTheCyclePartsIsHeldOrRegistered: the rulebook and the
// register agree. Risk: a must rule that binds gragas, has no check and is on
// no list, so nothing says it is unmet.
func TestEveryMustRuleOfTheCyclePartsIsHeldOrRegistered(t *testing.T) {
	t.Parallel()
	rules := readRules(t)
	b, err := os.ReadFile(filepath.Join(root, "docs", "self-check.md"))
	if err != nil {
		t.Fatal(err)
	}
	reg := registered(string(b))
	if len(reg) < 100 {
		t.Fatalf("the register names %d rules; the reader finds almost none of its rows", len(reg))
	}
	if got := unaccounted(rules, reg); len(got) > 0 {
		t.Errorf("%d must rules are held by no check and listed nowhere, first %q", len(got), got[0])
	}
	seen := map[string]int{}
	for _, r := range rules {
		if r.Strength == "must" && r.Status == "active" {
			seen[r.Part]++
		}
	}
	for _, p := range cycleParts() {
		if seen[p] == 0 {
			t.Errorf("no active must rule of %s: the part moved or the reader of the rule files is blind", p)
		}
	}
}

// TestARuleInNeitherTheChecksNorTheRegisterIsRed: the planted cases of the
// test above. A rule in neither fails, one in the register or held by a
// check passes, and a part row covers the part.
func TestARuleInNeitherTheChecksNorTheRegisterIsRed(t *testing.T) {
	t.Parallel()
	planted := ruleRow{ID: "gates.plant.1", Part: "gates", Strength: "must", Status: "active"}
	register := registered("| gates.plant.1, .2 | unmet | not placed |\n| gates.other.3 to .5 | unmet | not placed |\n| ts.* | not applicable | none |\n")
	rows := []struct {
		claim string
		rule  ruleRow
		reg   map[string]bool
		fails bool
	}{
		{"in neither", planted, map[string]bool{}, true},
		{"in the register", planted, register, false},
		{"in a range of the register", ruleRow{ID: "gates.other.4", Part: "gates", Strength: "must", Status: "active"}, register, false},
		{"held: a binding is shipped", ruleRow{ID: "gates.plant.9", Part: "gates", Strength: "must", Status: "active", Held: true}, map[string]bool{}, false},
		{"a part row", ruleRow{ID: "ts.plant.1", Part: "ts", Strength: "must", Status: "active"}, register, false},
		{"a part with no row", ruleRow{ID: "agents.plant.1", Part: "agents", Strength: "must", Status: "active"}, register, true},
		{"not must", ruleRow{ID: "gates.plant.2", Part: "gates", Strength: "should", Status: "active"}, map[string]bool{}, false},
		{"not active", ruleRow{ID: "gates.plant.3", Part: "gates", Strength: "must", Status: "later"}, map[string]bool{}, false},
		{"of a part the register is not held to", ruleRow{ID: "go.plant.1", Part: "go", Strength: "must", Status: "active"}, map[string]bool{}, false},
	}
	for _, r := range rows {
		got := unaccounted([]ruleRow{r.rule}, r.reg)
		if r.fails {
			fires(t, got, r.rule.ID)
		} else if len(got) > 0 {
			t.Errorf("%s: %q, want no finding", r.claim, got)
		}
	}
}
