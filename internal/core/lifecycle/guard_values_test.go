package lifecycle_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// showValue is a value as a test shows it, "" for one empty or absent.
func showValue(v any) string {
	s := fmt.Sprintf("%+v", v)
	switch s {
	case "", "[]", "<nil>", "map[]":
		return ""
	}
	return s
}

// changed is each key whose value differs between a and b.
func changed(a, b map[string]model.Guarded) []string {
	var out []string
	for k := range a {
		if a[k] != b[k] {
			out = append(out, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// TestValuesHoldEachLifecycleValueWhole: a list's order is no change, and a
// change inside an entry, a rule added or a record changed names the one
// value it changes, never a count.
func TestValuesHoldEachLifecycleValueWhole(t *testing.T) {
	t.Parallel()
	base := overlay()
	base.BindingRecords = []model.Opaque{{Path: "bindings/a.yaml", Digest: "d1"}, {Path: "bindings/b.yaml", Digest: "d2"}}
	base.Sources = []model.Source{
		{Slug: "guide", Kind: model.SourceGuide, Title: "A guide", Locator: "https://example.org/guide"},
		{Slug: "study", Kind: model.SourceStudy, Title: "A study", Locator: "https://example.org/study"},
	}
	base.Terms = []model.Term{{Slug: "header", Definition: "d", Literal: "X-One", Status: model.Active}}
	cases := map[string]struct {
		edit func(*model.Book)
		want []string
	}{
		// A merge once held no source or term field
		"a source's locator changed": {func(b *model.Book) {
			b.Sources[0].Locator = "https://example.org/elsewhere"
		}, []string{"source:guide\x00locator"}},
		"a source added": {func(b *model.Book) {
			b.Sources = append(b.Sources, model.Source{Slug: "incident", Kind: model.SourceIncident, Title: "t", Locator: "l"})
		}, []string{"source:incident\x00kind", "source:incident\x00locator", "source:incident\x00slug", "source:incident\x00title"}},
		"a source dropped": {func(b *model.Book) {
			b.Sources = b.Sources[1:]
		}, []string{"source:guide\x00kind", "source:guide\x00locator", "source:guide\x00slug", "source:guide\x00title"}},
		"a source's slug written twice": {func(b *model.Book) {
			again := b.Sources[0]
			again.Locator = "https://example.org/elsewhere"
			b.Sources = append(b.Sources, again)
		}, []string{"source:guide\x00attribution", "source:guide\x00kind", "source:guide\x00locator", "source:guide\x00slug", "source:guide\x00title"}},
		"a source's verified day changed": {func(b *model.Book) {
			b.Sources[0].Verified = "2026-11-17"
		}, nil},
		"a term's literal changed": {func(b *model.Book) {
			b.Terms[0].Literal = "X-Two"
		}, []string{"term:header\x00literal"}},
		"a term's status changed": {func(b *model.Book) {
			b.Terms[0].Status = model.Retired
		}, []string{"term:header\x00status"}},
		"a term's definition changed": {func(b *model.Book) {
			b.Terms[0].Definition = "prose"
		}, nil},
		// A merge holds a decision's chain as guard edit does
		"a decision superseded": {func(b *model.Book) {
			b.Decisions[0].SupersededBy = "other"
		}, []string{"decision:taken\x00superseded_by"}},
		"a decision's text reworded": {func(b *model.Book) {
			b.Decisions[0].Text = "prose"
		}, nil},
		"the lists in another order": {func(b *model.Book) {
			slices.Reverse(b.Exclusions)
			slices.Reverse(b.BindingRecords)
		}, nil},
		"an exclusion's lifted lines deleted": {func(b *model.Book) {
			b.Exclusions[1].Lifted = nil
		}, []string{"exclusions.yaml\x00the exclusion house/std.errors.3/1"}},
		"a lifted exclusion's record written again above it, not lifted": {func(b *model.Book) {
			again := b.Exclusions[1]
			again.Lifted = nil
			b.Exclusions = slices.Insert(b.Exclusions, 1, again)
		}, []string{"exclusions.yaml\x00the exclusion house/std.errors.3/1"}},
		"a lifted exclusion's record written again below it, not lifted": {func(b *model.Book) {
			again := b.Exclusions[1]
			again.Lifted = nil
			b.Exclusions = append(b.Exclusions, again)
		}, []string{"exclusions.yaml\x00the exclusion house/std.errors.3/1"}},
		"a rule added": {func(b *model.Book) {
			b.Rules = append(b.Rules, model.Rule{ID: "go.errors.10", Statement: "d", Strength: model.May, Status: model.Proposed, Origin: model.Authored})
		}, []string{"rule:go.errors.10\x00id", "rule:go.errors.10\x00origin", "rule:go.errors.10\x00status", "rule:go.errors.10\x00strength"}},
		"a binding record changed": {func(b *model.Book) {
			b.BindingRecords[1].Digest = "d3"
		}, []string{"bindings/\x00bindings/b.yaml"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			after := base
			after.Rules = slices.Clone(base.Rules)
			after.Exclusions = slices.Clone(base.Exclusions)
			after.BindingRecords = slices.Clone(base.BindingRecords)
			after.Sources = slices.Clone(base.Sources)
			after.Terms = slices.Clone(base.Terms)
			after.Decisions = slices.Clone(base.Decisions)
			c.edit(&after)
			before, now := lifecycle.Values(base, showValue), lifecycle.Values(after, showValue)
			if got := changed(before, now); !slices.Equal(got, c.want) {
				t.Errorf("the values changed are %q, want %q\nkeys: %q", got, c.want, slices.Sorted(maps.Keys(now)))
			}
		})
	}
}

// TestEveryGuardedPartIsHeldInAMerge: each part
// GuardEdit compares, but history.jsonl, which a merge holds line by line,
// has values a change to it moves. A part added to the guard and not to
// Values fails here, not in a merge that lets it through.
// Risk: a merge commit changes a guarded part by hand, unrecorded.
func TestEveryGuardedPartIsHeldInAMerge(t *testing.T) {
	t.Parallel()
	edits := map[string]func(*model.Book){
		"Rules": func(b *model.Book) { b.Rules = append(b.Rules, model.Rule{ID: "go.errors.10", Status: model.Proposed}) },
		"BindingRecords": func(b *model.Book) {
			b.BindingRecords = append(b.BindingRecords, model.Opaque{Path: "bindings/c.yaml", Digest: "d"})
		},
		"Exclusions": func(b *model.Book) {
			b.Exclusions = append(b.Exclusions, model.Exclusion{Overlay: "house", Rule: "std.errors.1", Number: 1, Decision: "taken"})
		},
		"Sources": func(b *model.Book) { b.Sources = append(b.Sources, model.Source{Slug: "s", Locator: "l"}) },
		"Terms":   func(b *model.Book) { b.Terms = append(b.Terms, model.Term{Slug: "t", Literal: "X"}) },
		"Decisions": func(b *model.Book) {
			b.Decisions = append(b.Decisions, model.Decision{Slug: "d", Title: "t", Text: "x", Who: "w", Date: "2026-01-01"})
		},
	}
	base := overlay()
	for _, part := range lifecycle.GuardedParts() {
		if part == "History" {
			continue
		}
		edit, ok := edits[part]
		if !ok {
			t.Errorf("the guarded part %s has no edit here: give it one, and a key in lifecycle.Values", part)
			continue
		}
		after := base
		after.Rules = slices.Clone(base.Rules)
		after.Exclusions = slices.Clone(base.Exclusions)
		after.BindingRecords = slices.Clone(base.BindingRecords)
		after.Sources = slices.Clone(base.Sources)
		after.Terms = slices.Clone(base.Terms)
		after.Decisions = slices.Clone(base.Decisions)
		edit(&after)
		if len(lifecycle.GuardEdit(base, after, showValue)) == 0 {
			t.Errorf("%s: the edit is no change to guard edit", part)
		}
		if got := changed(lifecycle.Values(base, showValue), lifecycle.Values(after, showValue)); len(got) == 0 {
			t.Errorf("%s: a change guard edit lists moves no value a merge holds", part)
		}
	}
}
