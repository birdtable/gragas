package format

import (
	"fmt"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// rule is one minimal rule row at two-space indent, with extra lines after it.
func rule(id, extra string) string {
	return "  - id: " + id + "\n    statement: Wrap the error.\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: authored\n" + extra
}

func loadWith(t *testing.T, errors, layout string) Loaded {
	t.Helper()
	files := fullBook(t)
	files["language/go/rules/errors.yaml"] = "format: 3\npart: go\nconcern: errors\nrules:\n" + errors
	files["language/go/rules/layout.yaml"] = "format: 3\npart: go\nconcern: layout\nrules:\n" + layout
	delete(files, "exclusions.yaml")
	delete(files, "translations/fr/errors.yaml")
	delete(files, "decisions.yaml")
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, err := load(t, w, w.Seed("repo", files))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAClashOfIDOrAliasIsOneFindingNamingBothPlaces(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, errors, layout, want string }{
		{"clean control", rule("go.errors.1", "") + rule("go.errors.2", ""), rule("go.layout.1", "    aliases: [go.layout.9]\n"), ""},
		{"id twice", rule("go.errors.1", ""), rule("go.errors.1", ""), "language/go/rules/layout.yaml:5: the id go.errors.1 is also at language/go/rules/errors.yaml:5;"},
		{"alias at two rules", rule("go.errors.1", "    aliases: [go.errs.1]\n"), rule("go.layout.1", "    aliases: [go.errs.1]\n"), "language/go/rules/layout.yaml:5: the id go.errs.1 is also at language/go/rules/errors.yaml:5;"},
		{"alias equals a live id", rule("go.errors.1", ""), rule("go.layout.1", "    aliases: [go.errors.1]\n"), "language/go/rules/layout.yaml:5: the id go.errors.1 is also at language/go/rules/errors.yaml:5;"},
		{"alias equals its own id", rule("go.errors.1", "    aliases: [go.errors.1]\n"), rule("go.layout.1", ""), "language/go/rules/errors.yaml:5: the id go.errors.1 is listed twice in this rule;"},
	}
	ran := 0
	var listing strings.Builder
	for _, c := range cases {
		got := loadWith(t, c.errors, c.layout)
		ran++
		fmt.Fprintf(&listing, "## %s\n", c.name)
		for _, f := range got.Findings {
			listing.WriteString(f.String() + "\n")
		}
		if c.want == "" {
			if len(got.Findings) != 0 {
				t.Errorf("%s: findings = %v", c.name, got.Findings)
			}
			continue
		}
		if len(got.Findings) != 1 || !strings.HasPrefix(got.Findings[0].String(), c.want) {
			t.Errorf("%s: findings = %v; want one starting %q", c.name, got.Findings, c.want)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d", ran, len(cases))
	}
	configtest.Golden(t, "clashes.golden", []byte(listing.String()))
}

func TestAnAliasNeverPointsAtTwoRulesAcrossAMergeOfTwoBranches(t *testing.T) {
	t.Parallel()
	// each branch renumbered a different rule onto the alias go.errs.1
	left := rule("go.errors.1", "    aliases: [go.errs.1]\n")
	right := rule("go.errors.2", "    aliases: [go.errs.1]\n")
	got := loadWith(t, left+right, rule("go.layout.1", ""))
	if len(got.Findings) != 1 {
		t.Fatalf("findings = %v", got.Findings)
	}
	r, ok := NewTable(got.Book.Rules).Resolve("go.errs.1")
	if !ok || r.ID != "go.errors.1" || len(got.Book.Rules) != 3-1 {
		t.Errorf("the alias resolves to %v (%v) with %d rules; the clashing rule must not be taken in", r.ID, ok, len(got.Book.Rules))
	}
}

func TestAReferenceToNoRuleIsAFindingAtItsLine(t *testing.T) {
	t.Parallel()
	errs := rule("go.errors.1", "    relations:\n      relates: [go.layout.1, go.nope.1]\n      covers: [go.errs.1]\n    reasoning:\n      decision: missing-decision\n") +
		"  - id: go.errors.2\n    statement: Wrap the error.\n    strength: must\n    status: retired\n    origin: authored\n    replaced_by: go.errors.7\n"
	got := loadWith(t, errs, rule("go.layout.1", "    aliases: [go.errs.1]\n"))
	want := []string{
		"language/go/rules/errors.yaml:12: relates names go.nope.1, which is no rule's id or alias",
		"language/go/rules/errors.yaml:21: replaced_by names go.errors.7, which is no rule's id or alias",
		"language/go/rules/errors.yaml:15: decision names missing-decision, which decisions.yaml does not hold",
	}
	var text []string
	for _, f := range got.Findings {
		text = append(text, f.String())
	}
	if strings.Join(text, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(text, "\n"), strings.Join(want, "\n"))
	}
}

func TestEveryAliasResolvesToExactlyOneRule(t *testing.T) {
	t.Parallel()
	var rules []model.Rule
	for i := range 200 {
		r := model.Rule{ID: model.RuleID(fmt.Sprintf("go.errors.%d", i+1))}
		for j := range i % 4 {
			r.Aliases = append(r.Aliases, model.RuleID(fmt.Sprintf("go.old%d.%d", j, i+1)))
		}
		rules = append(rules, r)
	}
	tab := NewTable(rules)
	checked := 0
	for _, r := range rules {
		for _, id := range append([]model.RuleID{r.ID}, r.Aliases...) {
			got, ok := tab.Resolve(id)
			if !ok || got.ID != r.ID {
				t.Fatalf("%s resolved to %v, %v; want %s", id, got.ID, ok, r.ID)
			}
			checked++
		}
	}
	if _, ok := tab.Resolve("go.errors.999"); ok || checked != 200+300 {
		t.Errorf("checked %d names; an unknown id resolved: %v", checked, ok)
	}
}

// BenchmarkResolveTenThousandRules records one build of the table and a
// lookup of every name; a scan per lookup would be quadratic and show here.
func BenchmarkResolveTenThousandRules(b *testing.B) {
	rules := make([]model.Rule, 10000)
	for i := range rules {
		rules[i] = model.Rule{ID: model.RuleID(fmt.Sprintf("go.errors.%d", i+1)), Aliases: []model.RuleID{model.RuleID(fmt.Sprintf("go.old.%d", i+1))}}
	}
	b.ResetTimer()
	for range b.N {
		t := NewTable(rules)
		for _, r := range rules {
			if _, ok := t.Resolve(r.Aliases[0]); !ok {
				b.Fatal("alias did not resolve")
			}
		}
	}
}
