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

const guardDecisionsAt = "/repo/book/decisions.yaml"

// guardDecisionWorld is the decisions' book at the guard's root, and git's
// who, so decision supersede can run in it too.
func guardDecisionWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC))
	root := w.Seed(guardRoot, map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure, "book/sources.yaml": promoteSources,
		"book/language/go/rules/errors.yaml": decisionRules, "book/language/go/rules/layout.yaml": decisionLayout,
		"book/decisions.yaml": decisionList,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

// TestAHandEditOfDecisionsIsRefusedNamingDecisionNewOrSupersede: a hand edit
// of decisions.yaml that adds a decision, marks one superseded or changes what
// it cites ends refused, names the field and decision new or supersede, is
// journalled once and leaves the file as it was.
// Risk: a decision is added, or a chain rewritten, by hand with no history
// entry, now that decision new and supersede write one.
func TestAHandEditOfDecisionsIsRefusedNamingDecisionNewOrSupersede(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, entity, field, command string
		input                        []byte
	}{
		{"a new decision", "decision:flat", "slug", "gragas decision new or gragas decision supersede",
			hook(t, "Write", map[string]any{"file_path": guardDecisionsAt, "content": decisionList +
				"  - slug: flat\n    title: Flat\n    text: Keep it flat.\n    who: Ada\n    date: \"2026-11-17\"\n"})},
		{"superseded by hand", "decision:wrap", "superseded_by", "gragas decision supersede wrap",
			edit(t, guardDecisionsAt, "    date: \"2026-01-01\"\n", "    date: \"2026-01-01\"\n    superseded_by: wrap-again\n")},
		{"a citation by hand", "decision:wrap", "cites", "gragas decision supersede wrap",
			edit(t, guardDecisionsAt, "    date: \"2026-01-01\"\n", "    date: \"2026-01-01\"\n    cites:\n      rules:\n        - go.errors.2\n")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, root := guardDecisionWorld(t)
			g, ref := judge(t, w.Ports(), root, c.input)
			if ref == nil {
				t.Fatalf("the edit was allowed: %+v", g)
			}
			if ref.Word() != model.Refused {
				t.Fatalf("the word is %s: %s", ref.Word(), ref.Observed())
			}
			i := slices.IndexFunc(g.Changes, func(x model.Guarded) bool { return x.Entity == c.entity && x.Field == c.field })
			if i < 0 {
				t.Fatalf("no change of %s of %s among %+v: %s", c.field, c.entity, g.Changes, ref.Observed())
			}
			if g.Changes[i].Command != c.command || !strings.Contains(ref.Observed(), c.field) || !strings.Contains(ref.Hint(), "gragas decision") {
				t.Fatalf("change %+v, why %q, hint %q", g.Changes[i], ref.Observed(), ref.Hint())
			}
			if got := refusals(t, w, root); len(got) != 1 || got[0]["outcome"] != "refused" || got[0]["path"] != g.Path {
				t.Fatalf("the journal holds %+v", got)
			}
			if after := read(t, w, root, "book/decisions.yaml"); after != decisionList {
				t.Fatal("the guard wrote the file it judged")
			}
		})
	}
}

// TestADecisionThroughItsCommandIsRecordedAndItsProsePasses is the control:
// supersede writes decisions.yaml and its history entries, and a hand edit of
// a decision's prose, its title, text or rejected options, ends done and
// journals nothing.
func TestADecisionThroughItsCommandIsRecordedAndItsProsePasses(t *testing.T) {
	t.Parallel()
	w, root := guardDecisionWorld(t)
	ctx := context.Background()
	c, ref, err := coreapi.DecisionSupersede(ctx, w.Ports(), changeAsk(root, "gragas decision supersede wrap"), supersedeReq())
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if hist := read(t, w, root, "book/history.jsonl"); !strings.Contains(hist, `"entity":"decision:wrap"`) {
		t.Fatalf("history:\n%s", hist)
	}
	for _, in := range [][]byte{
		edit(t, guardDecisionsAt, "text: Wrap errors.", "text: Wrap each error."),
		edit(t, guardDecisionsAt, "title: Wrap\n", "title: Wrap them\n"),
		edit(t, guardDecisionsAt, "      - bare\n", "      - bare\n      - panic\n"),
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
