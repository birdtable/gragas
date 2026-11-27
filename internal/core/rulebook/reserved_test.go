package rulebook_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/core/rulebook"
	"github.com/birdtable/gragas/internal/model"
)

// TestAPartNamedLikeTheEngineIsOneFindingAtItsLine: parts.yaml may not name
// a part model.EnginePart, the word the engine's own check ids start with;
// a rule's id spelled so is refused on its own, below. It replaces
// TestNoRulebookPartIsNamedLikeTheValidateCheck of core/api, which read this
// repository's own parts.yaml only: the loader now refuses the part in any
// rulebook, this one's included. Planted failure: the refusal in
// partsFromDoc removed.
func TestAPartNamedLikeTheEngineIsOneFindingAtItsLine(t *testing.T) {
	t.Parallel()
	tree := fixture(t, "valid")
	parts := string(tree["rulebook/parts.yaml"].Data)
	line := strings.Count(parts, "\n") + 1
	parts += "  - short: " + model.EnginePart + "\n" +
		"    split: base-" + model.EnginePart + "\n" +
		"    layer: base\n" +
		"    name: The engine\n" +
		"    description: A part spelled like the engine's checks.\n" +
		"    builds_on: []\n" +
		"    bindings: [any]\n" +
		"    owner: operator\n"
	tree["rulebook/parts.yaml"] = &fstest.MapFile{Data: []byte(parts)}
	b, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"rulebook/parts.yaml:" + strconv.Itoa(line) + ": part " + model.EnginePart + " is reserved: it is the first word of the engine's own check ids, such as " + model.ValidateCheck + ", so a rule of that part could not be told from the engine's check"}
	if got := findings(b); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %q\n want %q", got, want)
	}
}

// TestARetiredRuleIDSpelledLikeTheEngineIsOneFindingAtItsLine: checkRule
// holds an id to its part only while the rule is not retired, so a retired
// rule under part arch could take the id gragas.validate.1. The reserved
// word is refused for every status, as one finding at the rule's line.
// Planted failure: the refusal in checkRule removed.
func TestARetiredRuleIDSpelledLikeTheEngineIsOneFindingAtItsLine(t *testing.T) {
	t.Parallel()
	const layers = "rulebook/base/arch/rules/layers.yaml"
	tree := fixture(t, "valid")
	rules := string(tree[layers].Data)
	line := strings.Count(rules, "\n") + 1
	rules += "  - id: " + model.ValidateCheck + "\n" +
		"    slug: spelled-like-the-engine\n" +
		"    since: 0.1.0\n" +
		"    rule: Imports run one way through named layers.\n" +
		"    why: A reason long enough.\n" +
		"    kind: rule\n" +
		"    level: file\n" +
		"    strength: Review\n" +
		"    review_question: Do imports run one way here?\n" +
		"    status: retired\n" +
		"    note: planted with the id of the engine's validate check\n"
	tree[layers] = &fstest.MapFile{Data: []byte(rules)}
	b, err := rulebook.Load(context.Background(), tree, valid{}, rulebook.Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{layers + ":" + strconv.Itoa(line) + ": " + model.ValidateCheck + ": a rule's id never starts with " + model.EnginePart + ", the word the engine's own check ids start with, such as " + model.ValidateCheck}
	if got := findings(b); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %q\n want %q", got, want)
	}
}
