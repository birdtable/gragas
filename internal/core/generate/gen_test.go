package generate_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/core/generate"
	"github.com/birdtable/gragas/internal/ordered"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// book is a rulebook in memory, built by the test as generate reads one: two
// parts, the second building on the first, and one concern file in each
// part that has one.
type book struct {
	parts    []generate.Part
	concerns map[string][]generate.Concern
	rules    []generate.Rule
	standing map[string]string
	sources  *ordered.Map
}

func (b book) Root() string                         { return "book" }
func (b book) Parts() []generate.Part               { return b.parts }
func (b book) Concerns(p string) []generate.Concern { return b.concerns[p] }
func (b book) Tools(string) []*ordered.Map          { return nil }
func (b book) Registry() generate.Registry          { return generate.Registry{Sources: b.sources} }
func (b book) HasGuide(string) bool                 { return false }
func (b book) Rules() []generate.Rule               { return b.rules }
func (b book) Standing(id string) string            { return b.standing[id] }
func (b book) Check(c *ordered.Map) generate.CheckState {
	return generate.CheckState{Resolves: c.Str("key") != "", Holds: c.Str("key") != ""}
}
func (b book) LockIDs() []string { return []string{"lang.errors.1", "lang.errors.2", "lang.errors.3"} }

func yaml(t *testing.T, s string) *ordered.Map {
	t.Helper()
	v, err := ordered.ParseYAML([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		t.Fatalf("not a map: %s", s)
	}
	return m
}

func sample(t *testing.T) book {
	t.Helper()
	doc := yaml(t, `title: Errors
summary: How errors go.
rules:
  - id: lang.errors.1
    slug: wrapped
    rule: Wrap them.
    why: Context.
    kind: rule
    strength: Hard
    level: function
    status: active
    sources:
      - ref: a-book
        where: chapter 2
        backs: rule
    checks:
      - binding: go
        class: preset-rule
        tool: linter
        key: wrapcheck
        tier: "1"
        fix: never
      - binding: go
        class: fact
        tier: "R"
  - id: lang.errors.2
    slug: named
    rule: Name them.
    why: Readers.
    kind: rule
    strength: Lens
    level: function
    status: active
    sources:
      - ref: other-rule
        backs: rule
        covers: equal
`)
	sources := yaml(t, `a-book:
  kind: book
  title: A book
  locator: isbn:9780000000002
other-rule:
  kind: external-rule
  title: Another rulebook's rule
  locator: rule:other/R-1
`)
	rules := ordered.Maps(doc.List("rules"))
	return book{
		parts: []generate.Part{
			{Short: "core", Layer: "base", Name: "Core", Description: "The core.", LayerName: "Base"},
			{Short: "lang", Layer: "lang", Name: "A language", Description: "One language.", LayerName: "Languages", BuildsOn: []string{"core"}},
		},
		concerns: map[string][]generate.Concern{"lang": {{Name: "errors", Order: 1, Doc: doc}}},
		rules: []generate.Rule{
			{ID: "lang.errors.1", Part: "lang", Concern: "errors", Data: rules[0]},
			{ID: "lang.errors.2", Part: "lang", Concern: "errors", Data: rules[1]},
		},
		standing: map[string]string{"lang.errors.1": "enforced", "lang.errors.2": "advisory"},
		sources:  sources,
	}
}

func file(t *testing.T, outs []generate.Output, p string) string {
	t.Helper()
	for _, o := range outs {
		if o.Path == p {
			return string(o.Data)
		}
	}
	t.Fatalf("no output %s", p)
	return ""
}

// TestTheOutputsAreTheJSONFilesAndTheLockAndNoReadme: the generated files
// are the JSON files and the lock, all under the root, and no part's README:
// the board and the docs page say what a part holds.
func TestTheOutputsAreTheJSONFilesAndTheLockAndNoReadme(t *testing.T) {
	t.Parallel()
	var paths []string
	for _, o := range generate.Outputs(sample(t)) {
		paths = append(paths, o.Path)
	}
	want := []string{"book/rules.json", "book/bindings.json", "book/coverage.json", "book/ids.lock"}
	if !slices.Equal(paths, want) {
		t.Errorf("outputs = %q, want %q", paths, want)
	}
}

// TestBindingsHoldOnlyTheEnforcedRules, with only the checks that hold
// them; rules.json gives each rule its standing, and coverage.json counts
// standings and the external rules a rule covers.
func TestBindingsHoldOnlyTheEnforcedRules(t *testing.T) {
	t.Parallel()
	outs := generate.Outputs(sample(t))
	bindings := file(t, outs, "book/bindings.json")
	if !strings.Contains(bindings, `"lang.errors.1"`) || strings.Contains(bindings, `"lang.errors.2"`) || strings.Contains(bindings, `"fact"`) {
		t.Errorf("bindings.json:\n%s", bindings)
	}
	rules := file(t, outs, "book/rules.json")
	if !strings.Contains(rules, `"standing": "enforced"`) || !strings.Contains(rules, `"standing": "advisory"`) {
		t.Errorf("rules.json:\n%s", rules)
	}
	coverage := file(t, outs, "book/coverage.json")
	for _, want := range []string{`"enforced": 1`, `"advisory": 1`, `"checks": 2`, `"unresolved": 1`, `"external_equal": 1`} {
		if !strings.Contains(coverage, want) {
			t.Errorf("coverage.json lacks %s:\n%s", want, coverage)
		}
	}
	if lock := file(t, outs, "book/ids.lock"); !strings.HasSuffix(lock, "lang.errors.2\nlang.errors.3\n") {
		t.Errorf("ids.lock:\n%s", lock)
	}
}

// TestStaleIsTheMissingAndTheChanged: a file that is current is left out, a
// missing one and a changed one are stale, in the order given.
func TestStaleIsTheMissingAndTheChanged(t *testing.T) {
	t.Parallel()
	tree := fstest.MapFS{
		"a": &fstest.MapFile{Data: []byte("same")},
		"b": &fstest.MapFile{Data: []byte("old")},
	}
	outs := []generate.Output{{Path: "c", Data: []byte("new")}, {Path: "a", Data: []byte("same")}, {Path: "b", Data: []byte("new")}}
	stale, err := generate.Stale(tree, outs)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range stale {
		got = append(got, o.Path)
	}
	if !slices.Equal(got, []string{"c", "b"}) {
		t.Errorf("stale = %v, want [c b]", got)
	}
}
