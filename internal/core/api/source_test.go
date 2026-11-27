package coreapi_test

import (
	"context"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestSourceNewWritesTheSourceAndOneHistoryLineAndASecondRunHasNothingToDo:
// the dry run writes nothing, the apply writes sources.yaml and one line of
// history, and the same run again is done with nothing to do (cli.commands.2).
// Planted failure: the dry run writes the source, or the second run appends a
// history line.
func TestSourceNewWritesTheSourceAndOneHistoryLineAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	w, root := promoteWorld(t)
	ctx := context.Background()
	req := model.NewSource{Slug: "house-rule", Kind: model.SourceOwnDecision, Title: "House rule", Locator: "decisions.yaml#house-rule"}
	c, ref, err := coreapi.SourceNew(ctx, w.Ports(), changeAsk(root, "gragas source new"), req)
	if err != nil || ref != nil {
		t.Fatalf("ref = %v, err = %v", ref, err)
	}
	if shown, ok := c.Preview(ctx); !ok || shown.Plan.Digest == "" {
		t.Fatalf("preview ok=%v word=%q", ok, shown.Word)
	}
	if strings.Contains(read(t, w, root, "book/sources.yaml"), "house-rule") {
		t.Fatal("the dry run wrote the source")
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	src := read(t, w, root, "book/sources.yaml")
	if !strings.HasPrefix(src, promoteSources) || !strings.HasSuffix(src, "  - slug: house-rule\n    kind: own-decision\n    title: House rule\n    locator: \"decisions.yaml#house-rule\"\n") {
		t.Errorf("sources file:\n%s", src)
	}
	hist := read(t, w, root, "book/history.jsonl")
	if strings.Count(hist, "\n") != 1 || !strings.Contains(hist, `"entity":"source:house-rule"`) || !strings.Contains(hist, `"kind":"new"`) {
		t.Errorf("history:\n%s", hist)
	}
	again, ref, err := coreapi.SourceNew(ctx, w.Ports(), runTwo(changeAsk(root, "gragas source new")), req)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if second := again.Apply(ctx, ""); second.Word != model.Done || !second.Nothing {
		t.Errorf("a second identical run ended %q nothing=%v (%v)", second.Word, second.Nothing, second.Cause)
	}
	if read(t, w, root, "book/history.jsonl") != hist {
		t.Error("the second run wrote history")
	}
}

// TestSourceEditSetsTheFieldAndNotesEachCitingRuleInOnePlan: the file moves by
// the one field, and history holds the source's line and one note for each of
// the three rules that cite it, all with the plan digest of the one plan.
// Planted failure: a rule is left without its note, or the notes carry
// another digest.
func TestSourceEditSetsTheFieldAndNotesEachCitingRuleInOnePlan(t *testing.T) {
	t.Parallel()
	w, root := promoteWorld(t)
	ctx := context.Background()
	title := "Effective Go, 2nd"
	c, ref, err := coreapi.SourceEdit(ctx, w.Ports(), changeAsk(root, "gragas source edit effective-go"), model.EditSource{Slug: "effective-go", Title: &title})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if got := read(t, w, root, "book/sources.yaml"); got != strings.Replace(promoteSources, "Effective Go", `"Effective Go, 2nd"`, 1) {
		t.Errorf("sources file:\n%s", got)
	}
	lines := strings.Split(strings.TrimSuffix(read(t, w, root, "book/history.jsonl"), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("history has %d lines, want 4:\n%v", len(lines), lines)
	}
	digest := func(l string) string {
		_, after, _ := strings.Cut(l, `"plan":"`)
		d, _, _ := strings.Cut(after, `"`)
		return d
	}
	for i, want := range []string{`"entity":"source:effective-go"`, `"entity":"rule:go.errors.1"`, `"entity":"rule:go.errors.2"`, `"entity":"rule:go.errors.3"`} {
		if !strings.Contains(lines[i], want) || digest(lines[i]) == "" || digest(lines[i]) != digest(lines[0]) {
			t.Errorf("line %d = %s, want %s and the digest of line 0", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[1], `"kind":"source-edit"`) {
		t.Errorf("a note is not a source-edit: %s", lines[1])
	}
}

func TestSourceEditToWhatItHoldsIsNothingToDoAndAnUnknownSourceIsRefused(t *testing.T) {
	t.Parallel()
	w, root := promoteWorld(t)
	ctx := context.Background()
	same := "Effective Go"
	c, ref, err := coreapi.SourceEdit(ctx, w.Ports(), changeAsk(root, "gragas source edit effective-go"), model.EditSource{Slug: "effective-go", Title: &same})
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || !out.Nothing {
		t.Errorf("ended %q nothing=%v (%v)", out.Word, out.Nothing, out.Cause)
	}
	_, ref, err = coreapi.SourceEdit(ctx, w.Ports(), changeAsk(root, "gragas source edit nope"), model.EditSource{Slug: "nope", Title: &same})
	if err == nil && (ref == nil || ref.Word() != model.Refused) {
		t.Fatalf("ref = %+v, err = %v", ref, err)
	}
	if read(t, w, root, "book/history.jsonl") != "" || read(t, w, root, "book/sources.yaml") != promoteSources {
		t.Error("a run with nothing to do wrote")
	}
}
