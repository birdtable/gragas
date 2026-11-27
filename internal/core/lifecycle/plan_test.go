package lifecycle_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func book() model.Book {
	return model.Book{
		Structure: model.Structure{Layers: []model.Layer{{Slug: "language", Parts: []model.Part{
			{Slug: "go", Concerns: []model.Concern{{Slug: "errors"}, {Slug: "style"}}},
		}}}, Kinds: []model.ProgramKind{{Slug: "service"}}},
		Rules: []model.Rule{
			{ID: "go.errors.1", Aliases: []model.RuleID{"go.errors.4"}, Statement: "a", Strength: model.Must, Status: model.Active, Origin: model.Authored},
			{ID: "go.errors.2", Statement: "b", Strength: model.Must, Status: model.Retired, Origin: model.Authored},
			{ID: "go.style.1", Statement: "c", Strength: model.Should, Status: model.Active, Origin: model.Authored},
		},
		Sources:   []model.Source{{Slug: "effective-go", Kind: model.SourceGuide, Title: "Effective Go", Locator: "https://go.dev/doc/effective_go"}},
		Decisions: []model.Decision{{Slug: "taken", Title: "t", Text: "x", Who: "w", Date: "2026-01-01"}},
	}
}

func moment() lifecycle.Moment {
	return lifecycle.Moment{Who: "Ada <ada@example.org>", Date: "2026-11-08"}
}

func files() model.PlanFiles {
	return model.PlanFiles{Rules: "rulebook/language/go/rules/errors.yaml", Sources: "rulebook/sources.yaml", Decisions: "rulebook/decisions.yaml", History: "rulebook/history.jsonl"}
}

func newReq() model.NewRule {
	return model.NewRule{Part: "go", Concern: "errors", Statement: "Wrap an error with the call that failed.", Strength: model.Should, Files: files()}
}

func changeFor(t *testing.T, p lifecycle.Plan, entity, field string) model.Change {
	t.Helper()
	for _, c := range p.Delta.Changes {
		if c.Entity == entity && c.Field == field {
			return c
		}
	}
	t.Fatalf("no change of %s %q in %+v", entity, field, p.Delta.Changes)
	return model.Change{}
}

func TestANewRuleTakesTheNextNumberAboveEveryIDAndAliasOfItsConcern(t *testing.T) {
	t.Parallel()
	p, err := lifecycle.PlanRuleNew(book(), newReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	// ids 1 and 2, and the alias 4: 5 is the next, so 3 and 4 are never handed out again.
	c := changeFor(t, p, "rule:go.errors.5", "")
	if c.Before != "" || !strings.Contains(c.After, "id: go.errors.5\n") {
		t.Errorf("change = %+v", c)
	}
	if p.Record.Entity != "rule:go.errors.5" || p.Record.Kind != "new" || p.Record.Before != nil {
		t.Errorf("entry = %+v", p.Record)
	}
}

func TestTheFirstRuleOfAConcernIsNumberOneAndAConcernNamesNoOtherNumber(t *testing.T) {
	t.Parallel()
	b := book()
	b.Structure.Layers[0].Parts[0].Concerns = append(b.Structure.Layers[0].Parts[0].Concerns, model.Concern{Slug: "naming"})
	r := newReq()
	r.Concern = "naming"
	p, err := lifecycle.PlanRuleNew(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	changeFor(t, p, "rule:go.naming.1", "")
}

func TestANewRuleIsProposedAuthoredAndLeavesEveryLifecycleFieldItWasNotGiven(t *testing.T) {
	t.Parallel()
	p, _ := lifecycle.PlanRuleNew(book(), newReq(), moment())
	body := changeFor(t, p, "rule:go.errors.5", "").After
	for _, want := range []string{"status: proposed\n", "origin: authored\n", "strength: should\n", `statement: "Wrap an error with the call that failed."` + "\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("the rule lacks %q:\n%s", want, body)
		}
	}
	for _, not := range []string{"owner:", "review_by:", "zero_by:", "sources:", "scope:", "reasoning:", "bindings:"} {
		if strings.Contains(body, not) {
			t.Errorf("the rule holds %q although it was not given:\n%s", not, body)
		}
	}
	if got := p.Record.After; got != (model.Lifecycle{Status: model.Proposed}) {
		t.Errorf("after = %+v; want status proposed alone", got)
	}
}

func TestAGivenOwnerReviewDateScopeDefinitionAndSourceAreWritten(t *testing.T) {
	t.Parallel()
	r := newReq()
	r.Owner, r.ReviewBy = "platform", "2027-01-31"
	r.Scope = &model.RuleScope{Parts: []string{"go"}, Paths: []string{"cmd/**"}, Kinds: []string{"service"}}
	r.Definition = "An error names what failed."
	r.Cite = []string{"effective-go"}
	p, err := lifecycle.PlanRuleNew(book(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	body := changeFor(t, p, "rule:go.errors.5", "").After
	for _, want := range []string{"owner: platform\n", "review_by: \"2027-01-31\"\n", "  parts:\n    - go\n", "  paths:\n    - \"cmd/**\"\n", "  kinds:\n    - service\n", "  definition: An error names what failed.\n", "  - ref: effective-go\n    backs: rule\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("the rule lacks %q:\n%s", want, body)
		}
	}
	if got := p.Record.After; got.Owner != "platform" || got.ReviewBy != "2027-01-31" {
		t.Errorf("after = %+v", got)
	}
	for _, c := range p.Delta.Changes {
		if strings.HasPrefix(c.Entity, "source:") || strings.HasPrefix(c.Entity, "decision:") {
			t.Errorf("a cited source adds an entry: %+v", c)
		}
	}
}

func TestAnOwnDecisionAddsASourceOfThatKindAndTheDecisionItPointsAt(t *testing.T) {
	t.Parallel()
	r := newReq()
	r.Own = true
	r.Decision = &model.NewDecision{Title: "Wrap errors", Text: "We wrap every error once, at the edge."}
	p, err := lifecycle.PlanRuleNew(book(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	src := changeFor(t, p, "source:own-go-errors-5", "")
	for _, want := range []string{"slug: own-go-errors-5\n", "kind: own-decision\n", "locator: decide-go-errors-5\n"} {
		if !strings.Contains(src.After, want) {
			t.Errorf("source lacks %q:\n%s", want, src.After)
		}
	}
	dec := changeFor(t, p, "decision:decide-go-errors-5", "")
	for _, want := range []string{"who: \"Ada <ada@example.org>\"\n", "date: \"2026-11-08\"\n", "title: Wrap errors\n"} {
		if !strings.Contains(dec.After, want) {
			t.Errorf("decision lacks %q:\n%s", want, dec.After)
		}
	}
	body := changeFor(t, p, "rule:go.errors.5", "").After
	if !strings.Contains(body, "  - ref: own-go-errors-5\n    backs: rule\n") || !strings.Contains(body, "  decision: decide-go-errors-5\n") {
		t.Errorf("the rule does not cite them:\n%s", body)
	}
	if p.Record.Decision != "decide-go-errors-5" {
		t.Errorf("entry decision = %q", p.Record.Decision)
	}
}

func TestADecisionWithoutOwnIsRecordedAndNoSourceIsMade(t *testing.T) {
	t.Parallel()
	r := newReq()
	r.Decision = &model.NewDecision{Slug: "wrap-once", Title: "T", Text: "x"}
	p, err := lifecycle.PlanRuleNew(book(), r, moment())
	if err != nil {
		t.Fatal(err)
	}
	changeFor(t, p, "decision:wrap-once", "")
	for _, c := range p.Delta.Changes {
		if strings.HasPrefix(c.Entity, "source:") {
			t.Errorf("a source was added: %+v", c)
		}
	}
}

func TestAPlanLeavesOutASectionWithNothingToSayAndNamesTheFilesItWrites(t *testing.T) {
	t.Parallel()
	p, _ := lifecycle.PlanRuleNew(book(), newReq(), moment())
	var titles []string
	for _, s := range p.Sections {
		titles = append(titles, s.Title)
	}
	want := []string{"What changes", "What is written where"}
	if !slices.Equal(titles, want) {
		t.Errorf("sections = %v; want %v", titles, want)
	}
	written := strings.Join(p.Sections[1].Lines, "\n")
	for _, f := range []string{files().Rules, files().History} {
		if !strings.Contains(written, f) {
			t.Errorf("written-where lacks %s:\n%s", f, written)
		}
	}
	if strings.Contains(written, files().Sources) || strings.Contains(written, files().Decisions) {
		t.Errorf("written-where names a file nothing is added to:\n%s", written)
	}
	if !strings.Contains(strings.Join(p.Sections[0].Lines, "\n"), "proposed") {
		t.Errorf("what-changes does not give the standing: %v", p.Sections[0].Lines)
	}
}

func TestANewRuleIsRefusedWithAHintForEveryBadRequest(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		edit func(*model.NewRule)
		want string
	}{
		"no statement":    {func(r *model.NewRule) { r.Statement = "  " }, "statement"},
		"a control char":  {func(r *model.NewRule) { r.Statement = "a\nb" }, "statement"},
		"bad strength":    {func(r *model.NewRule) { r.Strength = "always" }, "strength"},
		"unknown part":    {func(r *model.NewRule) { r.Part = "rust" }, "part"},
		"unknown concern": {func(r *model.NewRule) { r.Concern = "nope" }, "concern"},
		"unknown source":  {func(r *model.NewRule) { r.Cite = []string{"missing"} }, "missing"},
		"unknown kind":    {func(r *model.NewRule) { r.Scope = &model.RuleScope{Kinds: []string{"batch"}} }, "batch"},
		"bad date":        {func(r *model.NewRule) { r.ReviewBy = "next week" }, "review"},
		"own, no text":    {func(r *model.NewRule) { r.Own = true }, "decision"},
		"taken decision": {func(r *model.NewRule) {
			r.Decision = &model.NewDecision{Slug: "taken", Title: "t", Text: "x"}
		}, "taken"},
		"bad slug": {func(r *model.NewRule) {
			r.Decision = &model.NewDecision{Slug: "Not A Slug", Title: "t", Text: "x"}
		}, "slug"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newReq()
			tc.edit(&r)
			_, err := lifecycle.PlanRuleNew(book(), r, moment())
			var ref *lifecycle.Refusal
			if !errors.As(err, &ref) {
				t.Fatalf("err = %v; want a *Refusal", err)
			}
			if !strings.Contains(strings.ToLower(ref.Msg), tc.want) || ref.Hint == "" {
				t.Errorf("refusal = %+v; want it to name %q and carry a hint", ref, tc.want)
			}
		})
	}
}

func TestTheSamePlanIsMadeTwiceFromTheSameRequest(t *testing.T) {
	t.Parallel()
	a, _ := lifecycle.PlanRuleNew(book(), newReq(), moment())
	b, _ := lifecycle.PlanRuleNew(book(), newReq(), moment())
	if len(a.Delta.Changes) != len(b.Delta.Changes) || a.Delta.Changes[0] != b.Delta.Changes[0] {
		t.Error("two plans of one request differ")
	}
}

func TestAnIdenticalSecondRunPlansNothing(t *testing.T) {
	t.Parallel()
	b := book()
	first, _ := lifecycle.PlanRuleNew(b, newReq(), moment())
	b.Rules = append(b.Rules, model.Rule{ID: "go.errors.5", Statement: newReq().Statement, Strength: model.Should, Status: model.Proposed, Origin: model.Authored})
	_ = first
	again, err := lifecycle.PlanRuleNew(b, newReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Delta.Changes) != 0 {
		t.Errorf("a rule with this statement is in the book as proposed, and the plan adds %+v", again.Delta.Changes)
	}
}

// ---- rule edit

func editReq(id string) model.EditRule { return model.EditRule{ID: id, Files: files()} }

func TestAnEditSetsOwnerAndDatesWithTheirBeforeAndAfterAndWritesOneEntry(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules[0].Owner = "old"
	r := editReq("go.errors.1")
	r.Owner, r.ReviewBy, r.ZeroBy = new("new"), new("2027-02-01"), new("2027-03-01")
	p, err := lifecycle.PlanRuleEdit(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.errors.1", "owner"); c.Before != "old" || c.After != "new" {
		t.Errorf("owner = %+v", c)
	}
	if c := changeFor(t, p, "rule:go.errors.1", "review_by"); c.Before != "" || c.After != "2027-02-01" {
		t.Errorf("review_by = %+v", c)
	}
	changeFor(t, p, "rule:go.errors.1", "zero_by")
	e := p.Record
	if e.Kind != "edit" || e.Entity != "rule:go.errors.1" || e.Before == nil || e.Before.Owner != "old" || e.After.Owner != "new" || e.After.Status != model.Active {
		t.Errorf("entry = %+v", e)
	}
}

func TestAnEditThatChangesNothingPlansNothing(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules[0].Owner = "same"
	r := editReq("go.errors.1")
	r.Owner = new("same")
	p, err := lifecycle.PlanRuleEdit(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Delta.Changes) != 0 {
		t.Errorf("changes = %+v", p.Delta.Changes)
	}
}

func TestAnEmptyValueTakesTheFieldAway(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules[0].ReviewBy = "2027-01-01"
	r := editReq("go.errors.1")
	r.ReviewBy = new("")
	p, _ := lifecycle.PlanRuleEdit(b, r, moment())
	if c := changeFor(t, p, "rule:go.errors.1", "review_by"); c.Before != "2027-01-01" || c.After != "" {
		t.Errorf("review_by = %+v", c)
	}
}

func TestAnEditAddsScopeEntriesAndRelationsThatAreNotThereYet(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules[0].Scope = &model.RuleScope{Paths: []string{"cmd/**"}}
	r := editReq("go.errors.1")
	r.AddPaths = []string{"cmd/**", "internal/**"}
	r.AddKinds = []string{"service"}
	r.Relates = []model.RuleID{"go.style.1"}
	r.Conflicts = []model.RuleID{"go.errors.2"}
	p, err := lifecycle.PlanRuleEdit(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "rule:go.errors.1", "scope.paths[]"); c.After != "internal/**" || c.Before != "" {
		t.Errorf("paths = %+v", c)
	}
	changeFor(t, p, "rule:go.errors.1", "scope.kinds[]")
	changeFor(t, p, "rule:go.errors.1", "relations.relates[]")
	changeFor(t, p, "rule:go.errors.1", "relations.conflicts[]")
	n := 0
	for _, c := range p.Delta.Changes {
		if c.Field == "scope.paths[]" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d path changes; the path already there is added once more", n)
	}
}

func TestAnEditCitesASourceOnceAndLeavesAnExistingCitationAlone(t *testing.T) {
	t.Parallel()
	b := book()
	r := editReq("go.errors.1")
	r.Cite = []string{"effective-go", "effective-go"}
	p, err := lifecycle.PlanRuleEdit(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Delta.Changes) != 1 {
		t.Fatalf("changes = %+v; want one record for the source asked for twice", p.Delta.Changes)
	}
	c := p.Delta.Changes[0]
	if c.Entity != "rule:go.errors.1" || c.Field != "sources{}" || c.Before != "" || c.After != "ref: effective-go\nbacks: rule" {
		t.Errorf("change = %+v; want the record that cites effective-go as backing the rule", c)
	}
	if p.Record.Kind != "edit" {
		t.Errorf("entry = %+v; want an edit entry", p.Record)
	}
	b.Rules[0].Sources = []model.Citation{{Ref: "effective-go", Backs: model.BacksRule}}
	p, err = lifecycle.PlanRuleEdit(b, r, moment())
	if err != nil || len(p.Delta.Changes) != 0 {
		t.Errorf("a source the rule cites already: changes = %+v, err = %v; want nothing to do", p.Delta.Changes, err)
	}
	b.Rules[0].Sources = []model.Citation{{Ref: "effective-go", Backs: model.BacksCheck}}
	if p, _ = lifecycle.PlanRuleEdit(b, r, moment()); len(p.Delta.Changes) != 1 {
		t.Errorf("a source that backs only the check: changes = %+v; want the record that backs the rule", p.Delta.Changes)
	}
}

func TestAnEditRefusesToCiteASourceTheBookLacks(t *testing.T) {
	t.Parallel()
	r := editReq("go.errors.1")
	r.Cite = []string{"missing"}
	_, err := lifecycle.PlanRuleEdit(book(), r, moment())
	var ref *lifecycle.Refusal
	if !errors.As(err, &ref) || !strings.Contains(ref.Msg, "missing") {
		t.Errorf("err = %v; want a refusal naming the source", err)
	}
}

func TestAnEditRefusesWhatItCannotDo(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		id   string
		edit func(*model.EditRule)
		want string
	}{
		"unknown rule":        {"go.errors.9", func(r *model.EditRule) { r.Owner = new("x") }, "go.errors.9"},
		"an alias":            {"go.errors.4", func(r *model.EditRule) { r.Owner = new("x") }, "go.errors.1"},
		"a retired rule":      {"go.errors.2", func(r *model.EditRule) { r.Owner = new("x") }, "retired"},
		"bad date":            {"go.errors.1", func(r *model.EditRule) { r.ZeroBy = new("soon") }, "zero"},
		"unknown related":     {"go.errors.1", func(r *model.EditRule) { r.Relates = []model.RuleID{"go.nope.1"} }, "go.nope.1"},
		"a rule to itself":    {"go.errors.1", func(r *model.EditRule) { r.Depends = []model.RuleID{"go.errors.1"} }, "itself"},
		"unknown kind":        {"go.errors.1", func(r *model.EditRule) { r.AddKinds = []string{"batch"} }, "batch"},
		"nothing asked":       {"go.errors.1", func(*model.EditRule) {}, "nothing"},
		"a status in an edit": {"go.errors.1", func(r *model.EditRule) { r.Owner = new("a\nb") }, "owner"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := editReq(tc.id)
			tc.edit(&r)
			_, err := lifecycle.PlanRuleEdit(book(), r, moment())
			var ref *lifecycle.Refusal
			if !errors.As(err, &ref) {
				t.Fatalf("err = %v; want a *Refusal", err)
			}
			if !strings.Contains(strings.ToLower(ref.Msg+" "+ref.Hint), tc.want) || ref.Hint == "" {
				t.Errorf("refusal = %+v; want it to name %q and carry a hint", ref, tc.want)
			}
		})
	}
}

func TestARetiredRuleWithTheSameStatementDoesNotMakeANewOneNothingToDo(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules = append(b.Rules, model.Rule{ID: "go.errors.5", Statement: newReq().Statement, Strength: model.Should, Status: model.Retired, Origin: model.Authored})
	p, err := lifecycle.PlanRuleNew(b, newReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	changeFor(t, p, "rule:go.errors.6", "")
}

func TestANewRuleOfAGuideConcernIsAddedToTheBlockItsSiblingsStandIn(t *testing.T) {
	t.Parallel()
	b := book()
	b.Anchors = map[model.RuleID]string{"go.errors.1": "errors"}
	r := newReq()
	r.Files.Rules = "rulebook/language/go/guide.md"
	p, err := lifecycle.PlanRuleNew(b, r, moment())
	if err != nil {
		t.Fatal(err)
	}
	c := changeFor(t, p, "block:rulebook/language/go/guide.md#errors", "rules[]")
	if c.Before != "" || !strings.HasPrefix(c.After, "id: go.errors.5\n") {
		t.Errorf("change = %+v", c)
	}
	if p.Record.Entity != "rule:go.errors.5" {
		t.Errorf("the history names %q; it names the rule, not the block", p.Record.Entity)
	}
}

func TestAGuideConcernWithNoBlockOfItsOwnIsRefusedNotGuessed(t *testing.T) {
	t.Parallel()
	r := newReq()
	r.Files.Rules = "rulebook/language/go/guide.md"
	_, err := lifecycle.PlanRuleNew(book(), r, moment())
	var ref *lifecycle.Refusal
	if !errors.As(err, &ref) || !strings.Contains(ref.Msg, "block") || ref.Hint == "" {
		t.Errorf("err = %v; want a refusal that names the block", err)
	}
}

func TestAPlanThatChangesNothingStillNamesTheRuleItIsAbout(t *testing.T) {
	t.Parallel()
	b := book()
	b.Rules = append(b.Rules, model.Rule{ID: "go.errors.5", Statement: newReq().Statement, Strength: model.Should, Status: model.Proposed, Owner: "o", Origin: model.Authored})
	same, _ := lifecycle.PlanRuleNew(b, newReq(), moment())
	if same.Record.Kind != "new" || same.Record.Entity != "rule:go.errors.5" || same.Record.After.Status != model.Proposed || same.Record.After.Owner != "o" {
		t.Errorf("new: record = %+v", same.Record)
	}
	b.Rules[0].Owner = "same"
	r := editReq("go.errors.1")
	r.Owner = new("same")
	none, _ := lifecycle.PlanRuleEdit(b, r, moment())
	if none.Record.Kind != "edit" || none.Record.Entity != "rule:go.errors.1" || none.Record.After.Status != model.Active {
		t.Errorf("edit: record = %+v", none.Record)
	}
}
