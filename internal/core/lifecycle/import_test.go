package lifecycle_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
)

func importBook() model.Book {
	b := book()
	b.Identity.Governance.ReviewDays = 90
	b.Identity.Owners = []string{"team-a"}
	return b
}

func importReq() model.ImportRequest {
	return model.ImportRequest{Draft: guideReq(), Tool: "golangci-lint", Target: "go-code", Entries: []model.ImportEntry{
		{Config: ".golangci.yml", Key: "errors", Severity: model.SeverityError}, // names two drafts: a to-do
		{Config: ".golangci.yml", Key: "jitter", Options: `{"max":3}`, Severity: model.SeverityWarning},
		{Config: ".golangci.yml", Key: "stutter", Severity: model.SeverityError},
		{Config: ".golangci.yml", Key: "gocyclo", Value: "15", Severity: model.SeverityError},
	}}
}

// TestAnImportEntersTheRulesThatHaveABindingActiveAndTheRestProposed: a draft
// whose name is an entry of the old config is active, imported, the guide its
// source, with a binding at the entry's own severity and no pair; the others are
// proposed; an entry no draft names is a to-do. Planted failure: a binding
// weaker than its entry, a grandfathered rule that is not active, or an entry
// lost without a to-do.
func TestAnImportEntersTheRulesThatHaveABindingActiveAndTheRestProposed(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	p, err := lifecycle.PlanImport(importBook(), docsOf(doc), importReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(screenOf(p))
	for _, c := range p.Delta.Changes {
		if strings.HasPrefix(c.Entity, "rule:") {
			fmt.Fprintf(&out, "--- %s\n%s", c.Entity, c.After)
		}
	}
	golden(t, "import.golden", out.String())
	active := 0
	for _, r := range append([]lifecycle.Record{p.Record}, p.Also...) {
		if r.After.Status == model.Active {
			active++
			if r.After.Owner == "" || r.After.ReviewBy != "2027-02-06" || r.Decision != "imported-handler-guide-v1-2-0" {
				t.Errorf("an active import records %+v", r)
			}
		}
	}
	if active != 2 {
		t.Errorf("%d rules entered active, want 2 (the jitter line and the stutter line; errors names two)", active)
	}
}

// TestAnImportBindsNoRuleWeakerThanTheConfig: every entry that a draft names is
// a binding of the same key, severity and options, so the preset a release
// writes from them holds the old config in full.
func TestAnImportBindsNoRuleWeakerThanTheConfig(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	req := importReq()
	p, err := lifecycle.PlanImport(importBook(), docsOf(doc), req, moment())
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, c := range p.Delta.Changes {
		all.WriteString(c.After)
	}
	for key, want := range map[string]string{"stutter": "severity: error", "jitter": "severity: warning"} {
		i := strings.Index(all.String(), "key: "+key+"\n")
		if i < 0 || !strings.Contains(all.String()[i:i+200], want) {
			t.Errorf("the binding of %s does not keep %q", key, want)
		}
	}
	if !strings.Contains(all.String(), `options: {"max":3}`) {
		t.Error("the options of the deadline entry are not kept")
	}
	if strings.Contains(all.String(), "pair:") {
		t.Error("a grandfathered binding names a pair it has not got")
	}
}

// TestAnImportRunTwiceIsANoOpAndAnEmptyOneIsRefused: the rules a run wrote are
// held by the concern, so the next run plans nothing; with no guide and no
// config there is nothing to plan.
func TestAnImportRunTwiceIsANoOpAndAnEmptyOneIsRefused(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	b := importBook()
	p, err := lifecycle.PlanImport(b, docsOf(doc), importReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Delta.Changes {
		if id, ok := strings.CutPrefix(c.Entity, "rule:"); ok {
			stmt := strings.TrimPrefix(strings.SplitN(c.After, "\n", 3)[1], "statement: ")
			b.Rules = append(b.Rules, model.Rule{ID: model.RuleID(id), Statement: strings.Trim(stmt, `"`), Status: model.Proposed})
		}
	}
	again, err := lifecycle.PlanImport(b, docsOf(doc), importReq(), moment())
	if err != nil || len(again.Delta.Changes) != 0 {
		t.Errorf("a second run plans %d changes (err %v), want none", len(again.Delta.Changes), err)
	}
	if _, err := lifecycle.PlanImport(importBook(), nil, model.ImportRequest{Draft: guideReq()}, moment()); err == nil {
		t.Error("an import of no guide and no config was planned")
	}
}

func docsOf(d *markdown.Doc) []*markdown.Doc { return []*markdown.Doc{d} }

// TestAnEntrysValueIsWrittenAsItsOwnTextAndNeverAsYAML: a string value that
// carries a newline and a key, or a regex such as [a-z]+, is written as one
// quoted string. The first adds no key to the binding, so no pair is forged on
// a grandfathered rule; the second imports as that text. Planted failure: a
// value written bare, which adds a `pair` key or fails the plan to render.
func TestAnEntrysValueIsWrittenAsItsOwnTextAndNeverAsYAML(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	for name, tc := range map[string]struct{ value, want string }{
		"a forged key":   {"a\n    pair: 1", `value: "a\n    pair: 1"`},
		"a regex":        {"[a-z]+", `value: "[a-z]+"`},
		"a number":       {"5", "value: 5\n"},
		"a literal":      {"true", "value: true\n"},
		"a list":         {`["a","b"]`, `value: ["a","b"]`},
		"a word":         {"error", `value: "error"`},
		"a map":          {`{"max": 3}`, `value: {"max": 3}`},
		"a float":        {"-0.5e3", "value: -0.5e3\n"},
		"a broken list":  {"[1,", `value: "[1,"`},
		"a bare dash":    {"-", `value: "-"`},
		"a leading zero": {"05", `value: "05"`},
	} {
		req := importReq()
		req.Entries = []model.ImportEntry{{Config: ".golangci.yml", Key: "stutter", Value: tc.value, Severity: model.SeverityError}}
		p, err := lifecycle.PlanImport(importBook(), docsOf(doc), req, moment())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		found := false
		for _, c := range p.Delta.Changes {
			if !strings.HasPrefix(c.Entity, "rule:") || !strings.Contains(c.After, "bindings:") {
				continue
			}
			found = true
			if strings.Contains(c.After, "\n    pair:") || !strings.Contains(c.After, tc.want) {
				t.Errorf("%s: the binding is\n%s\nwant %s and no pair", name, c.After, tc.want)
			}
		}
		if !found {
			t.Errorf("%s: no rule took the entry", name)
		}
	}
}

// TestAValueNestedPastTheDepthCapIsQuotedAndOneAtItIsWrittenAsIs: a list nested
// 65 deep is the string it came as; one nested 64 deep is the list. Risk: a
// hostile config value that recurses the scanner far. boundary: maxJSONDepth
func TestAValueNestedPastTheDepthCapIsQuotedAndOneAtItIsWrittenAsIs(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	for name, tc := range map[string]struct {
		depth  int
		quoted bool
	}{"at the cap": {65, false}, "over the cap": {66, true}} {
		v := strings.Repeat("[", tc.depth) + strings.Repeat("]", tc.depth)
		req := importReq()
		req.Entries = []model.ImportEntry{{Config: ".golangci.yml", Key: "stutter", Value: v, Severity: model.SeverityError}}
		p, err := lifecycle.PlanImport(importBook(), docsOf(doc), req, moment())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var all strings.Builder
		for _, c := range p.Delta.Changes {
			all.WriteString(c.After)
		}
		if got := strings.Contains(all.String(), `value: "[`); got != tc.quoted {
			t.Errorf("%s: quoted is %v, want %v", name, got, tc.quoted)
		}
	}
}

// TestAnImportThatGrandfathersARuleIntoABookWithNoOwnerIsRefused: a
// grandfathered rule answers to the book's first owner, and the
// moment's who is a command's name, not a person, so with no owner nobody
// answers and the import is refused naming the owners. A loaded book's schema
// keeps this from the command, so the plan is held here. Planted break: the
// refusal gone, or the rule given the moment's who as its owner.
func TestAnImportThatGrandfathersARuleIntoABookWithNoOwnerIsRefused(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	b := importBook()
	b.Identity.Owners = nil
	_, err := lifecycle.PlanImport(b, docsOf(doc), importReq(), moment())
	if err == nil || !strings.Contains(err.Error(), "the rulebook names no owner") {
		t.Fatalf("an import into a book with no owner: err = %v, want a refusal naming the missing owner", err)
	}
}
