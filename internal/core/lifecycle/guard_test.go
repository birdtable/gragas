package lifecycle_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// TestEveryPartOfTheBookIsGuardedOrSaysWhyNot: each field of model.Book is in
// the guard's list or in the unguarded list with why, never both, and neither
// names a field the book does not have.
// Risk: a part of the book that only a command may write, sources.yaml once,
// is added and the guard lets a hand edit of it through unrecorded.
func TestEveryPartOfTheBookIsGuardedOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	guarded, free := lifecycle.GuardedParts(), lifecycle.UnguardedParts()
	var fields []string
	for f := range reflect.TypeFor[model.Book]().Fields() {
		fields = append(fields, f.Name)
		_, isFree := free[f.Name]
		switch isGuarded := slices.Contains(guarded, f.Name); {
		case isGuarded && isFree:
			t.Errorf("%s is both guarded and unguarded", f.Name)
		case !isGuarded && !isFree:
			t.Errorf("model.Book.%s is neither guarded nor unguarded with why: put it in guardedParts or UnguardedParts", f.Name)
		case isFree && strings.TrimSpace(free[f.Name]) == "":
			t.Errorf("%s is unguarded with no why", f.Name)
		}
	}
	for _, name := range guarded {
		if !slices.Contains(fields, name) {
			t.Errorf("the guard names %s, which model.Book does not have", name)
		}
	}
	for name := range free {
		if !slices.Contains(fields, name) {
			t.Errorf("the unguarded list names %s, which model.Book does not have", name)
		}
	}
}

// show is the api's shown, in brief: compact JSON, a string bare, "" for empty.
func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "unreadable"
	}
	var s string
	switch err := json.Unmarshal(b, &s); {
	case string(b) == "null" || string(b) == "[]" || string(b) == "{}":
		return ""
	case err == nil:
		return s
	}
	return string(b)
}

func guardSourceBook() model.Book {
	return model.Book{
		Sources: []model.Source{
			{Slug: "effective-go", Kind: model.SourceGuide, Title: "Effective Go", Locator: "https://go.dev/doc/effective_go"},
			{Slug: "go-faq", Kind: model.SourceGuide, Title: "Go FAQ", Locator: "https://go.dev/doc/faq"},
		},
		Terms: []model.Term{
			{Slug: "wrap", Definition: "Add what was being done to an error."},
			{Slug: "spare", Definition: "Nothing uses it.", Literal: "spare-part"},
		},
	}
}

// TestAHandChangeOfASourceOrATermIsALifecycleChange: each change of a
// source's or a term's lifecycle fields, and each one added, gone or held
// twice, is one change naming the entity, the field and the command; a
// change of a definition or of verified is none.
// Risk: a source's locator, title or slug changes by hand with no history
// entry.
func TestAHandChangeOfASourceOrATermIsALifecycleChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		edit                   func(*model.Book)
		entity, field, command string
	}{
		{"locator", func(b *model.Book) { b.Sources[0].Locator = "https://example.org/go" }, "source:effective-go", "locator", "gragas source edit effective-go --locator"},
		{"title", func(b *model.Book) { b.Sources[1].Title = "The Go FAQ" }, "source:go-faq", "title", "gragas source edit go-faq --title"},
		{"attribution", func(b *model.Book) { b.Sources[0].Attribution = "The Go Authors" }, "source:effective-go", "attribution", "gragas source edit effective-go --attribution"},
		{"kind", func(b *model.Book) { b.Sources[0].Kind = model.SourceStandard }, "source:effective-go", "kind", ""},
		{"a source renamed is one gone", func(b *model.Book) { b.Sources[1].Slug = "faq" }, "source:go-faq", "slug", ""},
		{"a source renamed is one added", func(b *model.Book) { b.Sources[1].Slug = "faq" }, "source:faq", "slug", "gragas source new"},
		{"a new source", func(b *model.Book) {
			b.Sources = append(b.Sources, model.Source{Slug: "rfc-9110", Kind: model.SourceStandard, Title: "HTTP", Locator: "RFC 9110"})
		}, "source:rfc-9110", "slug", "gragas source new"},
		{"a source held twice", func(b *model.Book) { b.Sources = append(b.Sources, b.Sources[0]) }, "source:effective-go", "slug", "gragas source new"},
		{"a source gone", func(b *model.Book) { b.Sources = b.Sources[:1] }, "source:go-faq", "slug", ""},
		{"literal", func(b *model.Book) { b.Terms[1].Literal = "spare-tyre" }, "term:spare", "literal", "gragas term edit spare --literal"},
		{"retired by hand", func(b *model.Book) { b.Terms[1].Status = model.Retired }, "term:spare", "status", "gragas term retire spare"},
		{"a new term", func(b *model.Book) { b.Terms = append(b.Terms, model.Term{Slug: "span", Definition: "A timed step."}) }, "term:span", "slug", "gragas term new"},
		{"a term gone", func(b *model.Book) { b.Terms = b.Terms[1:] }, "term:wrap", "slug", "gragas term retire wrap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			after := guardSourceBook()
			c.edit(&after)
			got := lifecycle.GuardEdit(guardSourceBook(), after, show)
			i := slices.IndexFunc(got, func(g model.Guarded) bool { return g.Entity == c.entity && g.Field == c.field })
			if i < 0 {
				t.Fatalf("no change of %s of %s among %+v", c.field, c.entity, got)
			}
			if got[i].Command != c.command {
				t.Fatalf("the command is %q, want %q", got[i].Command, c.command)
			}
		})
	}
	t.Run("a field of the second of two of one slug", func(t *testing.T) {
		t.Parallel()
		before := guardSourceBook()
		before.Sources = append(before.Sources, before.Sources[0])
		after := guardSourceBook()
		after.Sources = append(after.Sources, before.Sources[0])
		after.Sources[2].Locator = "https://example.org/go"
		got := lifecycle.GuardEdit(before, after, show)
		if len(got) != 1 || got[0].Field != "locator" || got[0].Before != "https://go.dev/doc/effective_go" {
			t.Fatalf("changes %+v, want the second entry's locator", got)
		}
	})
}

// TestAChangeOfADefinitionOrOfVerifiedIsNone is the control: a term's
// definition is prose (S59) and verified is set by hand.
func TestAChangeOfADefinitionOrOfVerifiedIsNone(t *testing.T) {
	t.Parallel()
	after := guardSourceBook()
	after.Terms[0].Definition = "Say what was being done when an error happened."
	after.Sources[0].Verified = "2026-11-16"
	if got := lifecycle.GuardEdit(guardSourceBook(), after, show); len(got) != 0 {
		t.Fatalf("a prose change was guarded: %+v", got)
	}
}

// guardDecisionBook holds a chain of two decisions, the old one superseded
// by the new, and a third that stands alone.
func guardDecisionBook() model.Book {
	return model.Book{Decisions: []model.Decision{
		{Slug: "wrap", Title: "Wrap", Text: "Wrap errors.", Who: "w", Date: "2026-01-01", SupersededBy: "wrap-context"},
		{Slug: "wrap-context", Title: "Wrap with context", Text: "Wrap errors with what was done.", Who: "w", Date: "2026-02-01",
			Supersedes: "wrap", Cites: &model.Cites{Rules: []model.RuleID{"go.errors.1"}}},
		{Slug: "small", Title: "Small", Text: "Keep it small.", Rejected: []string{"big"}, Who: "w", Date: "2026-03-01"},
	}}
}

// TestAHandChangeOfADecisionIsALifecycleChange: a decision
// added, gone or held twice, and a change of its chain or of what it cites,
// is one change naming the decision, the field and decision new or
// supersede; its title, text, rejected options, who and date are prose,
// and a change of them is none.
// Risk: a decision is added or its chain rewritten by hand, with no history
// entry, now that decision new and supersede write one.
func TestAHandChangeOfADecisionIsALifecycleChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		edit                   func(*model.Book)
		entity, field, command string
	}{
		{"a new decision", func(b *model.Book) {
			b.Decisions = append(b.Decisions, model.Decision{Slug: "flat", Title: "Flat", Text: "Keep it flat.", Who: "w", Date: "2026-04-01"})
		}, "decision:flat", "slug", "gragas decision new or gragas decision supersede"},
		{"a decision held twice", func(b *model.Book) { b.Decisions = append(b.Decisions, b.Decisions[2]) }, "decision:small", "slug", "gragas decision new or gragas decision supersede"},
		{"a decision gone", func(b *model.Book) { b.Decisions = b.Decisions[:2] }, "decision:small", "slug", ""},
		{"superseded by hand", func(b *model.Book) { b.Decisions[2].SupersededBy = "wrap-context" }, "decision:small", "superseded_by", "gragas decision supersede small"},
		{"the chain cut by hand", func(b *model.Book) { b.Decisions[0].SupersededBy = "" }, "decision:wrap", "superseded_by", "gragas decision supersede wrap"},
		{"supersedes another by hand", func(b *model.Book) { b.Decisions[1].Supersedes = "small" }, "decision:wrap-context", "supersedes", "gragas decision supersede small"},
		{"a citation added by hand", func(b *model.Book) {
			b.Decisions[2].Cites = &model.Cites{Sources: []string{"effective-go"}}
		}, "decision:small", "cites", "gragas decision supersede small"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			after := guardDecisionBook()
			c.edit(&after)
			got := lifecycle.GuardEdit(guardDecisionBook(), after, show)
			i := slices.IndexFunc(got, func(g model.Guarded) bool { return g.Entity == c.entity && g.Field == c.field })
			if i < 0 {
				t.Fatalf("no change of %s of %s among %+v", c.field, c.entity, got)
			}
			if got[i].Command != c.command {
				t.Fatalf("the command is %q, want %q", got[i].Command, c.command)
			}
		})
	}
	t.Run("the control: prose reworded", func(t *testing.T) {
		t.Parallel()
		after := guardDecisionBook()
		after.Decisions[2].Title, after.Decisions[2].Text = "Smaller", "Keep each part small."
		after.Decisions[2].Rejected = []string{"big", "one file"}
		after.Decisions[1].Who, after.Decisions[1].Date = "x", "2026-02-02"
		if got := lifecycle.GuardEdit(guardDecisionBook(), after, show); len(got) != 0 {
			t.Fatalf("a prose change was guarded: %+v", got)
		}
	})
}
