package format

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestAPartSpelledLikeTheEngineIsOneFindingAtItsLine: the format reserves the
// part name the engine's own check ids start with, as a slug and
// as an alias; a rule's id spelled so is refused on its own, below. Each plant
// is one finding at the line of the word, and it names the word. Planted failure:
// the refusal in slugFindings removed.
func TestAPartSpelledLikeTheEngineIsOneFindingAtItsLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, structure string
		line            int
	}{
		{"a slug", strings.Replace(structureYAML, "slug: go\n", "slug: "+model.EnginePart+"\n", 1), 6},
		{"an alias", strings.Replace(structureYAML, "name: Go\n", "name: Go\n        aliases: ["+model.EnginePart+"]\n", 1), 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, root := world(t, map[string]string{IdentityFile: identityYAML, StructureFile: tc.structure})
			got, err := load(t, w, root)
			if err != nil {
				t.Fatal(err)
			}
			var reserved []Finding
			for _, f := range got.Findings {
				if strings.Contains(f.Msg, "is reserved") {
					reserved = append(reserved, f)
				}
			}
			if len(reserved) != 1 {
				t.Fatalf("want one reserved-part finding, got %d: %v", len(reserved), got.Findings)
			}
			f := reserved[0]
			if f.File != StructureFile || f.Line != tc.line || !strings.Contains(f.Msg, `"`+model.EnginePart+`"`) {
				t.Errorf("finding %s: want %s:%d naming %q", f, StructureFile, tc.line, model.EnginePart)
			}
		})
	}
}

// TestARuleIDSpelledLikeTheEngineIsOneFindingAtItsLine: a rule's id is not
// held to its part in format 3, and an alias may carry a renamed part's old
// slug, so the part refusal alone would let a rule under part go take the id
// gragas.validate.1. An active id, a retired id and an alias spelled so are
// each one finding at the rule's line, naming the word, and the rule is not
// in the book. Planted failure: the refusal in addRule removed.
func TestARuleIDSpelledLikeTheEngineIsOneFindingAtItsLine(t *testing.T) {
	t.Parallel()
	const errorsFile, layoutFile = "language/go/rules/errors.yaml", "language/go/rules/layout.yaml"
	files := fixtureFiles(t)
	for _, tc := range []struct{ name, file, old, new string }{
		{"an active id", errorsFile, "  - id: go.errors.1\n", "  - id: " + model.ValidateCheck + "\n"},
		{"a retired id", layoutFile, "  - id: go.layout.1\n", "  - id: " + model.ValidateCheck + "\n"},
		{"an alias", layoutFile, "  - id: go.layout.1\n", "  - id: go.layout.1\n    aliases: [" + model.ValidateCheck + "]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planted := strings.Replace(files[tc.file], tc.old, tc.new, 1)
			if planted == files[tc.file] {
				t.Fatalf("%s no longer holds %q; the plant plants nothing", tc.file, tc.old)
			}
			w, root := world(t, map[string]string{IdentityFile: identityYAML, StructureFile: structureYAML, tc.file: planted})
			got, err := load(t, w, root)
			if err != nil {
				t.Fatal(err)
			}
			var reserved []Finding
			for _, f := range got.Findings {
				if strings.Contains(f.Msg, `starts with "`+model.EnginePart+`"`) {
					reserved = append(reserved, f)
				}
			}
			if len(reserved) != 1 || reserved[0].File != tc.file || reserved[0].Line != 5 || !strings.Contains(reserved[0].Msg, model.ValidateCheck) {
				t.Fatalf("want one finding at %s:5 naming %s, got %v (all: %v)", tc.file, model.ValidateCheck, reserved, got.Findings)
			}
			for _, r := range got.Book.Rules {
				if len(reservedIDs(r)) > 0 {
					t.Errorf("rule %s with a reserved id or alias is in the book", r.ID)
				}
			}
		})
	}
}

// TestARulebookNamedLikeTheEngineIsNoFinding: the rulebook's name is not
// reserved. An imported id is namespaced as name.part.concern.n,
// four words, and the validate check's id is three, so a rulebook named
// gragas loads clean and none of its namespaced ids is model.ValidateCheck.
// Planted failure: the name reserved, or an engine check id of four words.
func TestARulebookNamedLikeTheEngineIsNoFinding(t *testing.T) {
	t.Parallel()
	if n := strings.Count(model.ValidateCheck, ".") + 1; n != 3 {
		t.Fatalf("%s has %d words; a namespaced id has four, and this test holds them apart", model.ValidateCheck, n)
	}
	named := strings.Replace(identityYAML, "name: example\n", "name: "+model.EnginePart+"\n", 1)
	w, root := world(t, map[string]string{IdentityFile: named, StructureFile: structureYAML})
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 0 {
		t.Fatalf("a rulebook named %q: want no finding, got %v", model.EnginePart, got.Findings)
	}
	if got.Book.Identity.Name != model.EnginePart || len(got.Book.Rules) == 0 {
		t.Fatalf("loaded %q with %d rules; the test checks nothing", got.Book.Identity.Name, len(got.Book.Rules))
	}
	for _, r := range got.Book.Rules {
		id := got.Book.Identity.Name + "." + string(r.ID)
		if strings.Count(id, ".")+1 != 4 || id == model.ValidateCheck {
			t.Errorf("namespaced id %s: want four words, never %s", id, model.ValidateCheck)
		}
	}
}
