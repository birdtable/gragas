package format

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

func migrateForm() model.InitForm {
	return model.InitForm{
		Name: "demo", Kind: model.KindBase, Version: "1.0.0", Language: "en",
		Licences: model.Licences{Rules: "CC-BY-4.0", Presets: "MIT"}, Owners: []string{"team-a"},
		Governance: model.Governance{Contribution: model.ContributionProposals, DecisionRequired: true, TrialReleases: 3, ReviewDays: 90},
	}
}

const v2Parts = `layers:
  - key: base
    name: Base
parts:
  - short: arch
    layer: base
    name: Architecture
    facets:
      - capability-role
    owner: operator
`

const v2Products = `kinds:
  - key: cli
    name: Command-line program
    description: A program run from a terminal.
`

const v2Sources = `sources:
  one-book:
    kind: book
    title: A Book
    locator: isbn:9780321293534
  a-poster:
    kind: poster
    title: A Poster
    locator: url:https://example.test/poster
`

const v2Rules = `part: arch
concern: layers
order: 1
title: Layers
rules:
  - id: arch.layers.1
    slug: no-cycles
    since: 0.1.0
    rule: Packages form no cycle.
    detail: A cycle is a finding.
    why: A cycle hides a boundary.
    strength: Hard
    status: active
    sources:
      - ref: one-book
        where: chapter 2
        backs: rule
  - id: arch.layers.2
    slug: read-the-record
    since: 0.1.0
    rule: A reader reads the record.
    strength: Record
    status: proposed
`

func v2Fixture() V2 {
	return V2{
		"parts.yaml": []byte(v2Parts), "products.yaml": []byte(v2Products), "sources.yaml": []byte(v2Sources),
		"ids.lock":                    []byte("arch.layers.1\narch.layers.2\narch.gone.1\n"),
		"base/arch/rules/layers.yaml": []byte(v2Rules),
		"base/arch/guide.md":          []byte("# Architecture\n"),
	}
}

func TestMigratingAV2RulebookCountsWhatItCarriesAndWhatItLeaves(t *testing.T) {
	t.Parallel()
	m, err := MigrateV2(v2Fixture(), migrateForm())
	if err != nil {
		t.Fatalf("MigrateV2: %v", err)
	}
	want := Carried{Rules: 2, Sources: 1, Guides: 1, Kinds: 2}
	if m.Carried.Rules != want.Rules || m.Carried.Sources != want.Sources || m.Carried.Guides != want.Guides || m.Carried.Kinds != want.Kinds {
		t.Errorf("Carried = %+v, want rules 2, sources 1, guides 1, kinds 2", m.Carried)
	}
	if len(m.Converted) != 1 || m.Converted[0] != "capability-role" {
		t.Errorf("Converted = %v, want the one facet", m.Converted)
	}
	var left []string
	for _, n := range m.NotCarried {
		left = append(left, n.Item)
	}
	if got := strings.Join(left, ","); got != "source a-poster,id arch.gone.1" {
		t.Errorf("NotCarried = %q, want the poster source and the id no rule holds: the fields with no home are parked", got)
	}
	var paths []string
	for _, tg := range m.Targets {
		paths = append(paths, tg.Path)
	}
	if got := strings.Join(paths, ","); got != "base/arch/rules/layers.yaml,sources.yaml,"+IdentityFile+","+StructureFile+","+ParkedFile {
		t.Errorf("targets = %q", got)
	}
}

func TestMigratingKeepsTheStrengthsAsMustShouldMay(t *testing.T) {
	t.Parallel()
	m, err := MigrateV2(v2Fixture(), migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	out := string(m.Targets[0].Data)
	for _, w := range []string{"strength: must", "strength: should"} {
		if !strings.Contains(out, w) {
			t.Errorf("the rules file lacks %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "Hard") || strings.Contains(out, "Record") {
		t.Errorf("an old strength word is left:\n%s", out)
	}
}

func TestMigratingWithoutAFileTheRulebookNeedsFails(t *testing.T) {
	t.Parallel()
	v := v2Fixture()
	delete(v, "products.yaml")
	if _, err := MigrateV2(v, migrateForm()); err == nil || !strings.Contains(err.Error(), "products.yaml") {
		t.Errorf("err = %v, want it to name products.yaml", err)
	}
}

func TestMigratingCarriesAScopeAnExemptionAndABinding(t *testing.T) {
	t.Parallel()
	v := v2Fixture()
	v["base/arch/rules/layers.yaml"] = []byte(strings.Replace(v2Rules, "    status: active\n", `    status: active
    when:
      kinds: [cli]
      paths: ["cmd/**"]
    unless:
      - paths: ["gen/**"]
        reason: generated
    checks:
      - binding: go
        class: analyzer
        tool: golangci-lint
        key: depguard
        tier: "1"
      - binding: go
        class: fact
        note: no tool
`, 1))
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	out := string(m.Targets[0].Data)
	for _, w := range []string{"scope:", "- cli", "cmd/**", "exemptions:", "reason: generated", "tool: golangci-lint", "rung: 3", "run_point: commit", "target: source"} {
		if !strings.Contains(out, w) {
			t.Errorf("the rules file lacks %q:\n%s", w, out)
		}
	}
	if m.Carried.Bindings != 1 {
		t.Errorf("bindings = %d, want 1: the check with no tool is not one", m.Carried.Bindings)
	}
	if got := parkedAt(t, m, "arch.layers.1", "checks", "2"); compact(got) != `{"binding":"go","class":"fact","note":"no tool"}` {
		t.Errorf("the check with no tool is parked as %s, want it whole", compact(got))
	}
	if got := parkedAt(t, m, "arch.layers.1", "checks", "1"); got != nil {
		t.Errorf("the bound check with nothing left over is parked as %s, want nothing", compact(got))
	}
}

// oneRule is a rules file of one rule whose strength, status and check are the
// words given, so a test can put any old word through the migration.
func oneRule(strength, status, binding, class, tier string) []byte {
	return []byte(`part: arch
concern: layers
order: 1
title: Layers
rules:
  - id: arch.layers.1
    rule: A rule.
    strength: ` + strength + `
    status: ` + status + `
    replaced_by: arch.layers.2
    checks:
      - binding: ` + binding + `
        class: ` + class + `
        tool: golangci-lint
        key: depguard
        tier: "` + tier + `"
`)
}

func migrateRule(t *testing.T, rule []byte) (Migration, error) {
	t.Helper()
	v := v2Fixture()
	v["base/arch/rules/layers.yaml"] = rule
	return MigrateV2(v, migrateForm())
}

func TestMigratingMapsEveryOldWordToItsNewOne(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ claim, old, want string }{{"Hard is must", "Hard", "must"}, {"Answered is should", "Answered", "should"}, {"Session is should", "Session", "should"}, {"Record is should", "Record", "should"}, {"Lens is may", "Lens", "may"}, {"Review is may", "Review", "may"}} {
		old, want := c.old, c.want
		t.Run("strength "+c.claim, func(t *testing.T) {
			t.Parallel()
			m, err := migrateRule(t, oneRule(old, "active", "go", "fact", "1"))
			if err != nil || !strings.Contains(string(m.Targets[0].Data), "strength: "+want+"\n") {
				t.Errorf("err %v, rules:\n%s", err, m.Targets[0].Data)
			}
		})
	}
	for _, c := range []struct{ claim, old, want string }{{"preset-rule is rung 0", "preset-rule", "0"}, {"preset-config is rung 1", "preset-config", "1"}, {"analyzer is rung 3", "analyzer", "3"}, {"hook is rung 3", "hook", "3"}, {"fact is rung 4", "fact", "4"}, {"test-kit is rung 5", "test-kit", "5"}} {
		old, want := c.old, c.want
		t.Run("class "+c.claim, func(t *testing.T) {
			t.Parallel()
			m, err := migrateRule(t, oneRule("Hard", "active", "go", old, "1"))
			if err != nil || !strings.Contains(string(m.Targets[0].Data), "rung: "+want+"\n") {
				t.Errorf("err %v, rules:\n%s", err, m.Targets[0].Data)
			}
		})
	}
	for _, c := range []struct{ claim, old, want string }{{"0 is edit", "0", "edit"}, {"1 is commit", "1", "commit"}, {"2 is push", "2", "push"}, {"3 is land", "3", "land"}, {"4 is release", "4", "release"}, {"R is review", "R", "review"}, {"G is deploy", "G", "deploy"}} {
		old, want := c.old, c.want
		t.Run("tier "+c.claim, func(t *testing.T) {
			t.Parallel()
			m, err := migrateRule(t, oneRule("Hard", "active", "go", "fact", old))
			if err != nil || !strings.Contains(string(m.Targets[0].Data), "run_point: "+want+"\n") {
				t.Errorf("err %v, rules:\n%s", err, m.Targets[0].Data)
			}
		})
	}
	for _, c := range []struct{ claim, old, want string }{{"go is source", "go", "source"}, {"ts is source", "ts", "source"}, {"any is source", "any", "source"}, {"sql is migration", "sql", "migration"}} {
		old, want := c.old, c.want
		t.Run("binding "+c.claim, func(t *testing.T) {
			t.Parallel()
			m, err := migrateRule(t, oneRule("Hard", "active", old, "fact", "1"))
			if err != nil || !strings.Contains(string(m.Targets[0].Data), "target: "+want+"\n") {
				t.Errorf("err %v, rules:\n%s", err, m.Targets[0].Data)
			}
		})
	}
	t.Run("a replaced rule keeps replaced_by and every rule is migrated", func(t *testing.T) {
		t.Parallel()
		m, err := migrateRule(t, oneRule("Hard", "retired", "go", "fact", "1"))
		out := string(m.Targets[0].Data)
		if err != nil || !strings.Contains(out, "replaced_by: arch.layers.2\n") || !strings.Contains(out, "origin: migrated\n") || !strings.Contains(out, "status: retired\n") {
			t.Errorf("err %v, rules:\n%s", err, out)
		}
	})
	t.Run("a later rule is proposed and its old word parked", func(t *testing.T) {
		t.Parallel()
		m, err := migrateRule(t, oneRule("Hard", "later", "go", "fact", "1"))
		if err != nil || !strings.Contains(string(m.Targets[0].Data), "status: proposed\n") || parkedAt(t, m, "arch.layers.1", "status") != "later" {
			t.Errorf("err %v, parked %v, rules:\n%s", err, parkedAt(t, m, "arch.layers.1", "status"), m.Targets[0].Data)
		}
	})
}

func TestMigratingMapsEverySourceKind(t *testing.T) {
	t.Parallel()
	for old, want := range map[string]string{"standard": "standard", "article": "guide", "book": "guide", "paper": "study", "incident": "incident", "decision": "decision-record", "vendor-doc": "vendor-document", "target": "own-decision"} {
		t.Run(old, func(t *testing.T) {
			t.Parallel()
			v := v2Fixture()
			v["sources.yaml"] = []byte("sources:\n  s:\n    kind: " + old + "\n    title: T\n    locator: url:https://example.test\n    verified:\n      \"on\": \"2026-01-02\"\n      by: someone\n")
			m, err := MigrateV2(v, migrateForm())
			if err != nil {
				t.Fatal(err)
			}
			var out string
			for _, tg := range m.Targets {
				if tg.Path == "sources.yaml" {
					out = string(tg.Data)
				}
			}
			if !strings.Contains(out, "kind: "+want+"\n") || !strings.Contains(out, `verified: "2026-01-02"`) {
				t.Errorf("sources:\n%s", out)
			}
			if got := parkedAt(t, m, "source s", "verified", "by"); got != "someone" {
				t.Errorf("verified.by is parked as %v, want someone", got)
			}
		})
	}
}

func TestMigratingRefusesAWordWithNoCounterpart(t *testing.T) {
	t.Parallel()
	for name, rule := range map[string][]byte{
		"strength Mystery": oneRule("Mystery", "active", "go", "fact", "1"),
		"status wobbly":    oneRule("Hard", "wobbly", "go", "fact", "1"),
		"class oracle":     oneRule("Hard", "active", "go", "oracle", "1"),
		"tier 9":           oneRule("Hard", "active", "go", "fact", "9"),
		"binding cobol":    oneRule("Hard", "active", "cobol", "fact", "1"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := migrateRule(t, rule)
			word := name[strings.Index(name, " ")+1:]
			if err == nil || !strings.Contains(err.Error(), "arch.layers.1") || !strings.Contains(err.Error(), `"`+word+`"`) {
				t.Errorf("err = %v, want it to name the rule and the word %q", err, word)
			}
		})
	}
}

// TestMigrateParksEveryFieldItDoesNotReadAndListsOnlyTheRemainder: the eleven
// classes (slug, since, level, kind, the unkeyed check, open, note,
// review_question, summary, by and the part fields) and every other field
// with no home are parked at their path; the list holds only the remainder
// kept out of the parked file: the top level of the parts and
// products files, a kind's instances, and a source whose kind has none.
func TestMigrateParksEveryFieldItDoesNotReadAndListsOnlyTheRemainder(t *testing.T) {
	t.Parallel()
	v := V2{
		"parts.yaml": []byte(`layers:
  - key: base
    name: Base
    extra: x
waivers:
  Hard: native
parts:
  - short: arch
    layer: base
    name: Architecture
    description: d
    builds_on: []
    bindings: [go]
    split: s
    when: {}
    owner: operator
`),
		"products.yaml": []byte("kinds:\n  - key: cli\n    name: Cli\n    note: n\n    instances:\n      - ref: r\n"),
		"sources.yaml":  []byte(v2Sources),
		"ids.lock":      []byte("arch.layers.1\n"),
		"base/arch/rules/layers.yaml": []byte(`part: arch
concern: layers
order: 1
title: Layers
summary: s
rules:
  - id: arch.layers.1
    slug: one
    since: 0.1.0
    level: package
    kind: rule
    rule: A rule.
    strength: Hard
    status: active
    when:
      kinds: [cli]
      other: x
    unless:
      - paths: [a]
        reason: r
        more: y
    sources:
      - ref: one-book
        backs: rule
        by: someone
    checks:
      - binding: go
        class: fact
        tool: t
        key: k
        tier: "1"
        note: n
        fix: never
      - class: fact
        tier: "1"
    open:
      - o
    note: rn
    review_question: q?
`),
	}
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		at   []string
		want string
	}{
		{[]string{"layer base", "extra"}, `"x"`},
		{[]string{"part arch", "split"}, `"s"`},
		{[]string{"part arch", "description"}, `"d"`},
		{[]string{"part arch", "builds_on"}, `[]`},
		{[]string{"part arch", "bindings"}, `["go"]`},
		{[]string{"part arch", "when"}, `{}`},
		{[]string{"kind cli", "note"}, `"n"`},
		{[]string{"base/arch/rules/layers.yaml", "summary"}, `"s"`},
		{[]string{"arch.layers.1", "slug"}, `"one"`},
		{[]string{"arch.layers.1", "since"}, `"0.1.0"`},
		{[]string{"arch.layers.1", "level"}, `"package"`},
		{[]string{"arch.layers.1", "kind"}, `"rule"`},
		{[]string{"arch.layers.1", "open"}, `["o"]`},
		{[]string{"arch.layers.1", "note"}, `"rn"`},
		{[]string{"arch.layers.1", "review_question"}, `"q?"`},
		{[]string{"arch.layers.1", "when", "other"}, `"x"`},
		{[]string{"arch.layers.1", "unless", "1", "more"}, `"y"`},
		{[]string{"arch.layers.1", "sources", "1", "by"}, `"someone"`},
		{[]string{"arch.layers.1", "checks", "1"}, `{"tool":"t","key":"k","note":"n","fix":"never"}`},
		{[]string{"arch.layers.1", "checks", "2"}, `{"class":"fact","tier":"1"}`},
	} {
		if got := parkedAt(t, m, c.at[0], c.at[1:]...); compact(got) != c.want {
			t.Errorf("parked %v = %s, want %s", c.at, compact(got), c.want)
		}
	}
	var left []string
	for _, n := range m.NotCarried {
		left = append(left, n.Item)
	}
	if got := strings.Join(left, ","); got != "kind cli instances,parts.yaml waivers,source a-poster" {
		t.Errorf("NotCarried = %q, want only the remainder ruled out of the parked file", got)
	}
}

func TestMigratingMakesOneKindOfAFacetTwoPartsShareOrAProductHas(t *testing.T) {
	t.Parallel()
	v := v2Fixture()
	v["parts.yaml"] = []byte(`layers:
  - key: base
    name: Base
parts:
  - short: arch
    layer: base
    name: Architecture
    facets: [cli, shared]
  - short: code
    layer: base
    name: Code
    facets: [shared]
`)
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	if m.Carried.Kinds != 2 || len(m.Converted) != 1 || m.Converted[0] != "shared" {
		t.Errorf("kinds = %d, converted = %v; want the product's cli and one kind, shared, converted once", m.Carried.Kinds, m.Converted)
	}
}

func TestMigratingCarriesAFacetOfAWhenAsAKindAndTheRelationsOfARule(t *testing.T) {
	t.Parallel()
	v := v2Fixture()
	v["base/arch/rules/layers.yaml"] = []byte(strings.Replace(v2Rules, "    status: active\n", `    status: active
    when:
      facets: [capability-role]
    relates:
      - arch.layers.2
`, 1))
	m, err := MigrateV2(v, migrateForm())
	if err != nil {
		t.Fatal(err)
	}
	out := string(m.Targets[0].Data)
	for _, w := range []string{"- capability-role", "relates:", "- arch.layers.2"} {
		if !strings.Contains(out, w) {
			t.Errorf("the rules file lacks %q:\n%s", w, out)
		}
	}
}
