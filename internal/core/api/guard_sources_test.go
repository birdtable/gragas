package coreapi_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const (
	guardSourcesAt = "/repo/book/sources.yaml"
	guardTermsAt   = "/repo/book/terms.yaml"
)

// guardSourceWorld is the guard's book with sources.yaml and terms.yaml, and
// git's who, so source edit can run in it too.
func guardSourceWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(append(allSchemas(), "v3/terms.schema.json")...)
	w.Instant(time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC))
	root := w.Seed(guardRoot, map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": bookStructure,
		"book/language/go/rules/errors.yaml": guardRules,
		"book/sources.yaml":                  promoteSources,
		"book/terms.yaml":                    termsFile,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

// TestAHandEditOfASourceOrATermIsRefusedNamingTheFieldAndTheCommand: a hand
// edit of sources.yaml or terms.yaml that changes a lifecycle field, or adds
// or takes away an entry, ends refused, names the field and the command, and
// is journalled once.
// Risk: a source's locator, title or slug changes by hand, with no history
// entry, and the guard lets it through.
func TestAHandEditOfASourceOrATermIsRefusedNamingTheFieldAndTheCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, entity, field, command string
		input                        []byte
	}{
		{"locator", "source:effective-go", "locator", "gragas source edit effective-go --locator",
			edit(t, guardSourcesAt, "https://go.dev/doc/effective_go", "https://example.org/effective_go")},
		{"title", "source:effective-go", "title", "gragas source edit effective-go --title",
			edit(t, guardSourcesAt, "title: Effective Go", "title: Effective Go, 2nd")},
		{"slug", "source:effective-go", "slug", "",
			edit(t, guardSourcesAt, "slug: effective-go", "slug: effective")},
		{"a new source", "source:go-faq", "slug", "gragas source new",
			hook(t, "Write", map[string]any{"file_path": guardSourcesAt, "content": promoteSources +
				"  - slug: go-faq\n    kind: guide\n    title: Go FAQ\n    locator: https://go.dev/doc/faq\n"})},
		{"literal", "term:spare", "literal", "gragas term edit spare --literal",
			edit(t, guardTermsAt, "literal: spare-part", "literal: spare-tyre")},
		{"retired by hand", "term:spare", "status", "gragas term retire spare",
			edit(t, guardTermsAt, "literal: spare-part\n", "literal: spare-part\n    status: retired\n")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, root := guardSourceWorld(t)
			g, ref := judge(t, w.Ports(), root, c.input)
			if ref == nil {
				t.Fatalf("the edit was allowed: %+v", g)
			}
			if ref.Word() != model.Refused {
				t.Fatalf("the word is %s", ref.Word())
			}
			i := slices.IndexFunc(g.Changes, func(x model.Guarded) bool { return x.Entity == c.entity && x.Field == c.field })
			if i < 0 {
				t.Fatalf("no change of %s of %s among %+v: %s", c.field, c.entity, g.Changes, ref.Observed())
			}
			if g.Changes[i].Command != c.command || !strings.Contains(ref.Observed(), c.field) || ref.Hint() == "" {
				t.Fatalf("change %+v, why %q, hint %q", g.Changes[i], ref.Observed(), ref.Hint())
			}
			got := refusals(t, w, root)
			if len(got) != 1 || got[0]["outcome"] != "refused" || got[0]["path"] != g.Path {
				t.Fatalf("the journal holds %+v", got)
			}
			if after := read(t, w, root, "book/sources.yaml"); after != promoteSources {
				t.Fatal("the guard wrote the file it judged")
			}
		})
	}
}

// TestASourceEditThroughItsCommandIsRecordedAndProseOfTheFilesPasses is the
// control: the same locator change made through source edit writes
// sources.yaml and its history entry, and a hand edit of prose in either file,
// a term's definition, a comment or the day a source was read, ends done and
// journals nothing.
func TestASourceEditThroughItsCommandIsRecordedAndProseOfTheFilesPasses(t *testing.T) {
	t.Parallel()
	w, root := guardSourceWorld(t)
	ctx := context.Background()
	locator := "https://example.org/effective_go"
	c, ref, err := coreapi.SourceEdit(ctx, w.Ports(), changeAsk(root, "gragas source edit effective-go"), model.EditSource{Slug: "effective-go", Locator: &locator})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if got := read(t, w, root, "book/sources.yaml"); !strings.Contains(got, locator) {
		t.Fatalf("sources file:\n%s", got)
	}
	if hist := read(t, w, root, "book/history.jsonl"); !strings.Contains(hist, `"entity":"source:effective-go"`) {
		t.Fatalf("history:\n%s", hist)
	}
	for _, in := range [][]byte{
		edit(t, guardTermsAt, "Add what was being done to an error.", "Say what was being done when the error happened."),
		edit(t, guardSourcesAt, "sources:\n", "# the books and pages the rules cite\nsources:\n"),
		edit(t, guardSourcesAt, "    kind: guide\n", "    kind: guide\n    verified: \"2026-11-16\"\n"),
	} {
		g, ref := judge(t, w.Ports(), root, in)
		if ref != nil || len(g.Changes) != 0 {
			t.Fatalf("%s: refused %v, changes %+v", in, ref, g.Changes)
		}
	}
	if got := refusals(t, w, root); len(got) != 0 {
		t.Fatalf("an allowed edit was journalled: %+v", got)
	}
}
