package lifecycle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func sourceFiles() model.PlanFiles {
	return model.PlanFiles{Sources: "rulebook/sources.yaml", History: "rulebook/history.jsonl"}
}

// sourceBook is the book of the rule tests with effective-go cited by two rules
// (one retired, and one that cites it twice) and by no other.
func sourceBook() model.Book {
	b := book()
	cite := []model.Citation{{Ref: "effective-go", Backs: model.BacksRule}}
	b.Rules[0].Sources = append(cite, cite[0])
	b.Rules[1].Sources = cite
	b.Rules[2].Sources = []model.Citation{{Ref: "other", Backs: model.BacksRule}}
	return b
}

func screenOf(p lifecycle.Plan) string {
	var screen strings.Builder
	for _, s := range p.Sections {
		screen.WriteString(s.Title + "\n" + strings.Join(s.Lines, "\n") + "\n")
	}
	return screen.String()
}

// TestANewSourceIsAddedWithItsKindAndRecordedByItsLocator: the entry, the record
// and the plan screen of a new source; every kind of the schema is taken.
// Planted failure: the attribution is written when none is given, or the record
// names no status.
func TestANewSourceIsAddedWithItsKindAndRecordedByItsLocator(t *testing.T) {
	t.Parallel()
	for _, k := range model.SourceKinds() {
		p, err := lifecycle.PlanSourceNew(sourceBook(), model.NewSource{Slug: "rfc-9110", Kind: k, Title: "HTTP Semantics", Locator: "RFC 9110", Files: sourceFiles()}, moment())
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		c := changeFor(t, p, "source:rfc-9110", "")
		if c.Before != "" || c.After != "slug: rfc-9110\nkind: "+string(k)+"\ntitle: HTTP Semantics\nlocator: RFC 9110\n" {
			t.Errorf("%s: change = %+v", k, c)
		}
		if p.Record.Kind != "new" || p.Record.Entity != "source:rfc-9110" || p.Record.Before != nil || len(p.Also) != 0 ||
			p.Record.After != (model.Lifecycle{Status: model.Active, Literal: "RFC 9110"}) {
			t.Errorf("%s: record = %+v, also %+v", k, p.Record, p.Also)
		}
	}
	p, err := lifecycle.PlanSourceNew(sourceBook(), model.NewSource{Slug: "rfc-9110", Kind: model.SourceStandard, Title: "HTTP Semantics", Locator: "RFC 9110", Attribution: "IETF", Files: sourceFiles()}, moment())
	if err != nil || !strings.HasSuffix(changeFor(t, p, "source:rfc-9110", "").After, "locator: RFC 9110\nattribution: IETF\n") {
		t.Fatalf("attribution: %+v, %v", p.Delta, err)
	}
	for _, want := range []string{"new source rfc-9110", "rulebook/sources.yaml: adds source rfc-9110", "rulebook/history.jsonl: appends one entry (new)"} {
		if !strings.Contains(screenOf(p), want) {
			t.Errorf("the plan screen lacks %q:\n%s", want, screenOf(p))
		}
	}
}

func TestANewSourceTwiceIsNothingToDoAndAClashOrABadWordIsRefused(t *testing.T) {
	t.Parallel()
	same := model.NewSource{Slug: "effective-go", Kind: model.SourceGuide, Title: "Effective Go", Locator: "https://go.dev/doc/effective_go", Files: sourceFiles()}
	p, err := lifecycle.PlanSourceNew(sourceBook(), same, moment())
	if err != nil || len(p.Delta.Changes) != 0 || p.Record.Entity != "source:effective-go" || !strings.Contains(screenOf(p), "nothing") {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	for name, mut := range map[string]func(*model.NewSource){
		"other words":    func(r *model.NewSource) { r.Title = "Another title" },
		"not a slug":     func(r *model.NewSource) { r.Slug = "Not A Slug" },
		"no kind":        func(r *model.NewSource) { r.Kind = "" },
		"a made-up kind": func(r *model.NewSource) { r.Kind = "gossip" },
		"no title":       func(r *model.NewSource) { r.Title = " " },
		"a long title":   func(r *model.NewSource) { r.Title = "a\nb" },
		"no locator":     func(r *model.NewSource) { r.Locator = "" },
		"a long who":     func(r *model.NewSource) { r.Attribution = "a\nb" },
	} {
		r := same
		mut(&r)
		if name == "not a slug" || name == "no kind" || name == "a made-up kind" {
			r.Slug = map[bool]string{true: r.Slug, false: "rfc-9110"}[name == "not a slug"]
		}
		if p, err := lifecycle.PlanSourceNew(sourceBook(), r, moment()); err == nil || len(p.Delta.Changes) != 0 {
			t.Errorf("%s: planned %+v", name, p)
		}
	}
}

// TestASourceEditChangesEachFieldAndNotesItselfOnEachRuleThatCitesIt: one
// change per field that differs, the source's own record, and one note on each
// citing rule, once even where a rule cites the source twice, with the rule's
// own fields before and after; a rule that does not cite it is not named.
// Planted failure: a note on a rule that cites another source, a rule noted
// twice, or a note that changes the rule's lifecycle.
func TestASourceEditChangesEachFieldAndNotesItselfOnEachRuleThatCitesIt(t *testing.T) {
	t.Parallel()
	title, where, who := "Effective Go, 2nd", "https://go.dev/doc/effective_go#errors", "the Go team"
	p, err := lifecycle.PlanSourceEdit(sourceBook(), model.EditSource{Slug: "effective-go", Title: &title, Locator: &where, Attribution: &who, Files: sourceFiles()}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "source:effective-go", "title"); c.Before != "Effective Go" || c.After != title {
		t.Errorf("title: %+v", c)
	}
	if c := changeFor(t, p, "source:effective-go", "locator"); c.Before != "https://go.dev/doc/effective_go" || c.After != where {
		t.Errorf("locator: %+v", c)
	}
	if c := changeFor(t, p, "source:effective-go", "attribution"); c.Before != "" || c.After != who {
		t.Errorf("attribution: %+v", c)
	}
	if p.Record.Kind != "edit" || p.Record.Entity != "source:effective-go" ||
		p.Record.Before == nil || p.Record.Before.Literal != "https://go.dev/doc/effective_go" || p.Record.After.Literal != where {
		t.Errorf("record = %+v", p.Record)
	}
	if len(p.Also) != 2 {
		t.Fatalf("also = %+v, want a note on go.errors.1 and go.errors.2", p.Also)
	}
	for i, id := range []string{"go.errors.1", "go.errors.2"} {
		a := p.Also[i]
		if a.Kind != "source-edit" || a.Entity != "rule:"+id || a.Before == nil || *a.Before != a.After || a.After.Status == "" {
			t.Errorf("note %d = %+v", i, a)
		}
	}
	if p.Also[1].After.Status != model.Retired {
		t.Errorf("a retired rule is noted with its own status: %+v", p.Also[1])
	}
	for _, want := range []string{"go.errors.1, go.errors.2", "appends 3 entries", "sets title, locator, attribution of effective-go"} {
		if !strings.Contains(screenOf(p), want) {
			t.Errorf("the plan screen lacks %q:\n%s", want, screenOf(p))
		}
	}
}

func TestASourceEditNoRuleCitesIsOneEntryAndAnUnchangedFieldIsLeftOut(t *testing.T) {
	t.Parallel()
	b := sourceBook()
	b.Sources = append(b.Sources, model.Source{Slug: "lonely", Kind: model.SourceStudy, Title: "T", Locator: "doi:1", Attribution: "A"})
	title, same := "T2", "doi:1"
	p, err := lifecycle.PlanSourceEdit(b, model.EditSource{Slug: "lonely", Title: &title, Locator: &same, Files: sourceFiles()}, moment())
	if err != nil || len(p.Delta.Changes) != 1 || len(p.Also) != 0 || !strings.Contains(screenOf(p), "appends one entry (edit)") {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	none := ""
	p, err = lifecycle.PlanSourceEdit(b, model.EditSource{Slug: "lonely", Attribution: &none, Files: sourceFiles()}, moment())
	if c := changeFor(t, p, "source:lonely", "attribution"); err != nil || c.Before != "A" || c.After != "" {
		t.Errorf("an empty attribution takes it away: %+v, %v", c, err)
	}
}

func TestASourceEditToTheWordsItHasIsNothingToDoAndWhatItCannotPlanIsRefused(t *testing.T) {
	t.Parallel()
	title, empty, long := "Effective Go", "", "a\nb"
	p, err := lifecycle.PlanSourceEdit(sourceBook(), model.EditSource{Slug: "effective-go", Title: &title, Files: sourceFiles()}, moment())
	if err != nil || len(p.Delta.Changes) != 0 || len(p.Also) != 0 || p.Record.Entity != "source:effective-go" || !strings.Contains(screenOf(p), "nothing") {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	for name, r := range map[string]model.EditSource{
		"an unknown source": {Slug: "nope", Title: &title},
		"no field":          {Slug: "effective-go"},
		"an empty title":    {Slug: "effective-go", Title: &empty},
		"an empty locator":  {Slug: "effective-go", Locator: &empty},
		"a long title":      {Slug: "effective-go", Title: &long},
		"a long locator":    {Slug: "effective-go", Locator: &long},
		"a long who":        {Slug: "effective-go", Attribution: &long},
	} {
		r.Files = sourceFiles()
		if p, err := lifecycle.PlanSourceEdit(sourceBook(), r, moment()); err == nil || len(p.Delta.Changes) != 0 {
			t.Errorf("%s: planned %+v", name, p)
		}
	}
}

// TestTheRefusalsOfASourceNameTheFlagsThereAre: a bad slug and a bad kind are
// each refused with a hint that names the flag of `source new` that sets it, as
// the command spells it. Planted failure: a hint that names --slug or --kind,
// which the command does not take.
func TestTheRefusalsOfASourceNameTheFlagsThereAre(t *testing.T) {
	t.Parallel()
	base := model.NewSource{Slug: "rfc-9110", Kind: model.SourceGuide, Title: "HTTP", Locator: "https://example.org/rfc9110", Files: sourceFiles()}
	for name, c := range map[string]struct {
		mut  func(*model.NewSource)
		flag string
	}{
		"a bad slug": {func(r *model.NewSource) { r.Slug = "Not A Slug" }, "--source-slug"},
		"a bad kind": {func(r *model.NewSource) { r.Kind = "gossip" }, "--source-kind"},
	} {
		r := base
		c.mut(&r)
		_, err := lifecycle.PlanSourceNew(sourceBook(), r, moment())
		var ref *lifecycle.Refusal
		if !errors.As(err, &ref) {
			t.Errorf("%s: err = %v, want a refusal", name, err)
			continue
		}
		if !strings.Contains(ref.Hint, c.flag) {
			t.Errorf("%s: hint %q does not name %s", name, ref.Hint, c.flag)
		}
	}
}
