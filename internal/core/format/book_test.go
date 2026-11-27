package format

import (
	"encoding/json"
	"errors"
	"fmt"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/ordered"
	"maps"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// fullBook is the identity and structure with every file of the fixture.
func fullBook(t *testing.T) map[string]string {
	t.Helper()
	files := fixtureBook()
	maps.Copy(files, fixtureFiles(t))
	return files
}

func fixtureBook() map[string]string {
	return map[string]string{IdentityFile: identityYAML, StructureFile: structureYAML}
}

func TestTheFixtureLoadsEveryEntityOfTheYAMLFormAndDumpsAsAGolden(t *testing.T) {
	t.Parallel()
	w, root := world(t, fullBook(t))
	got, err := load(t, w, root)
	if err != nil || len(got.Findings) != 0 {
		t.Fatalf("Load = %v, %v", got.Findings, err)
	}
	b := got.Book
	counts := map[string][2]int{
		"rules": {len(b.Rules), 3}, "terms": {len(b.Terms), 1}, "sources": {len(b.Sources), 1}, "decisions": {len(b.Decisions), 1},
		"exclusions": {len(b.Exclusions), 1}, "history": {len(b.History), 2}, "proposals": {len(b.Proposals), 1}, "translations": {len(b.Translations), 2},
	}
	for name, c := range counts {
		if c[0] != c[1] {
			t.Errorf("%s: %d loaded, want %d", name, c[0], c[1])
		}
	}
	dump, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configtest.Golden(t, "book.golden", []byte(string(dump)+"\n"))
}

func TestARuleCarriesEveryFieldOfTheRuleRow(t *testing.T) {
	t.Parallel()
	w, root := world(t, fullBook(t))
	got, _ := load(t, w, root)
	r := got.Book.Rules[0]
	switch {
	case r.ID != "go.errors.1", len(r.Aliases) != 1, r.Statement == "", r.Strength != model.Must, r.Status != model.Active,
		r.Scope == nil, len(r.Exemptions) != 1, r.Reasoning == nil, len(r.Pairs) != 1, r.Owner == "", r.ReviewBy != "2027-01-31",
		r.Relations == nil, r.Origin != model.Authored, len(r.Sources) != 1, len(r.Bindings) != 1:
		t.Fatalf("a field of the rule row is missing: %+v", r)
	}
	if b := r.Bindings[0]; b.Value != true || b.Options == nil || b.Rung != 1 || b.RunPoint != model.RunPush || b.Tightens != "base.errors.1" {
		t.Errorf("the binding was not kept as given: %+v", b)
	}
	if got.Book.Rules[2].ReplacedBy != "go.layout.1" {
		t.Errorf("replaced_by = %q", got.Book.Rules[2].ReplacedBy)
	}
}

func TestAConcernFileMissingOrStrayIsAFinding(t *testing.T) {
	t.Parallel()
	// the concern file is taken away and a stranger put beside the other
	files := map[string]string{IdentityFile: identityYAML, StructureFile: structureYAML + "# not the shared fixture structure\n"}
	maps.Copy(files, fixtureFiles(t))
	delete(files, "language/go/rules/errors.yaml")
	files["language/go/rules/extra.yaml"] = "format: 3\n"
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, err := load(t, w, w.Seed("repo", files))
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, f := range got.Findings {
		text = append(text, f.String())
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{"language/go/rules/errors.yaml: the file is missing", "language/go/rules/extra.yaml: not a concern of part go"} {
		if !strings.Contains(all, want) {
			t.Errorf("findings do not hold %q:\n%s", want, all)
		}
	}
}

func TestAPartOrConcernThatDisagreesWithItsPlaceIsAFinding(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["language/go/rules/layout.yaml"] = strings.Replace(files["language/go/rules/layout.yaml"], "part: go", "part: cli", 1)
	files["language/go/rules/errors.yaml"] = strings.Replace(files["language/go/rules/errors.yaml"], "concern: errors", "concern: layout", 1)
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, err := load(t, w, w.Seed("repo", files))
	if err != nil || len(got.Findings) != 2 {
		t.Fatalf("Load = %v, %v", got.Findings, err)
	}
	if got.Findings[0].Line != 3 || got.Findings[1].Line != 2 {
		t.Errorf("the findings are not on the lines of part and concern: %v", got.Findings)
	}
}

func TestAHistoryLineThatCannotBeReadIsAFindingAtItsLine(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	good, _, _ := strings.Cut(files["history.jsonl"], "\n")
	files["history.jsonl"] = good + "\n\n{\"format\":3,\n" + good + "\n"
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, err := load(t, w, w.Seed("repo", files))
	if err != nil || len(got.Findings) != 1 || got.Findings[0].File != "history.jsonl" || got.Findings[0].Line != 3 {
		t.Fatalf("Load = %v, %v; want one finding at history.jsonl:3", got.Findings, err)
	}
	if len(got.Book.History) != 2 {
		t.Errorf("the lines on either side were not kept: %d", len(got.Book.History))
	}
}

func TestANewerFormatInAnyFileEndsNotReady(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"terms.yaml", "history.jsonl", "language/go/rules/errors.yaml", "proposals/tighten-wrap.yaml"} {
		files := fullBook(t)
		files[file] = strings.Replace(files[file], "format: 3", "format: 4", 1)
		files[file] = strings.Replace(files[file], `"format":3`, `"format":4`, 1)
		w := porttest.NewWorld(t)
		w.SeedSchemas(schemaNames()...)
		_, err := load(t, w, w.Seed("repo", files))
		if nr, ok := errors.AsType[*NotReady](err); !ok || nr.File != file || nr.Found != 4 {
			t.Errorf("%s: err = %v; want *NotReady naming it and format 4", file, err)
		}
	}
}

func TestAProposalNamedForAnotherSlugIsAFinding(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["proposals/other.yaml"] = files["proposals/tighten-wrap.yaml"]
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, _ := load(t, w, w.Seed("repo", files))
	if len(got.Findings) != 1 || got.Findings[0].File != "proposals/other.yaml" || got.Findings[0].Line != 2 {
		t.Fatalf("findings = %v", got.Findings)
	}
}

// TestAProposalWhoseDocumentSaysOtherwiseIsAFinding: the file's
// slug, target, state, link and opened are what the engine reads, and the
// document is what a target's owner reads, so each field the document holds
// differently is a finding at its line in the document, and a document with
// no proposal is one at the document. The fixture's own file is the control.
// Planted failure: agrees not called, or one field left out of it.
func TestAProposalWhoseDocumentSaysOtherwiseIsAFinding(t *testing.T) {
	t.Parallel()
	const file = "proposals/tighten-wrap.yaml"
	cases := []struct {
		name, from, to string
		line           int
		says           string
	}{
		{"the slug", "    slug: tighten-wrap\n", "    slug: tighten-more\n", 10, `document.proposal.slug is "tighten-more" and the file's slug is "tighten-wrap"`},
		{"the target", "    target: base\n", "    target: other\n", 11, `document.proposal.target is "other"`},
		{"the state", "    state: open\n", "    state: accepted\n", 14, `document.proposal.state is "accepted" and the file's state is "open"`},
		{"the link", "    link: https://example.test/pull/1\n", "    link: https://example.test/pull/2\n", 15, `document.proposal.link is "https://example.test/pull/2"`},
		{"the opened day", "    opened: \"2026-11-04\"\n", "    opened: \"2026-11-05\"\n", 16, `document.proposal.opened is "2026-11-05"`},
		{"no link in the document", "    link: https://example.test/pull/1\n", "", 9, `document.proposal.link is "" and the file's link is "https://example.test/pull/1"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			files := fullBook(t)
			if !strings.Contains(files[file], c.from) {
				t.Fatalf("the fixture holds no %q", c.from)
			}
			files[file] = strings.Replace(files[file], c.from, c.to, 1)
			w := porttest.NewWorld(t)
			w.SeedSchemas(schemaNames()...)
			got, _ := load(t, w, w.Seed("repo", files))
			if len(got.Findings) != 1 || got.Findings[0].File != file || got.Findings[0].Line != c.line || !strings.Contains(got.Findings[0].Msg, c.says) {
				t.Errorf("findings = %v, want one at %s:%d saying %q", got.Findings, file, c.line, c.says)
			}
		})
	}
	t.Run("a document with no proposal", func(t *testing.T) {
		t.Parallel()
		files := fullBook(t)
		i := strings.Index(files[file], "  proposal:\n")
		files[file] = files[file][:i]
		w := porttest.NewWorld(t)
		w.SeedSchemas(schemaNames()...)
		got, _ := load(t, w, w.Seed("repo", files))
		if len(got.Findings) != 1 || got.Findings[0].Line != 7 || !strings.Contains(got.Findings[0].Msg, "document holds no proposal") {
			t.Errorf("findings = %v, want one at line 7", got.Findings)
		}
	})
}

// counter counts the reads of each path.
type counter struct {
	port.Tree
	reads map[string]int
}

func (c counter) Read(r model.Root, p string) ([]byte, error) {
	c.reads[p]++
	return c.Tree.Read(r, p)
}

func TestEachFileIsReadOnce(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	p := w.Ports()
	c := counter{Tree: p.Tree, reads: map[string]int{}}
	if _, err := Load(c, p.Schema, root); err != nil {
		t.Fatal(err)
	}
	if len(c.reads) != len(files) {
		t.Errorf("read %d of %d files", len(c.reads), len(files))
	}
	for p, n := range c.reads {
		if n != 1 {
			t.Errorf("%s was read %d times", p, n)
		}
	}
}

func rulesFile(n int) string {
	var b strings.Builder
	b.WriteString("format: 3\npart: go\nconcern: errors\nrules:\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "  - id: go.errors.%d\n    statement: Wrap error %d with what was being done.\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: authored\n", i, i)
	}
	return b.String()
}

// BenchmarkLoadTenThousandRules records the time of one pass over ten thousand
// rules of one file; it is not gated.
func BenchmarkLoadTenThousandRules(b *testing.B) {
	w := porttest.NewWorld(b)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", map[string]string{
		IdentityFile: identityYAML, StructureFile: strings.Replace(structureYAML, "          - slug: layout\n            name: Layout\n            aliases: [folders]\n", "", 1),
		"language/go/rules/errors.yaml": rulesFile(10000),
	})
	p := w.Ports()
	b.ResetTimer()
	for range b.N {
		got, err := Load(p.Tree, p.Schema, root)
		if err != nil || len(got.Findings) != 0 || len(got.Book.Rules) != 10000 {
			b.Fatalf("Load = %d rules, %v, %v", len(got.Book.Rules), got.Findings, err)
		}
	}
}

func FuzzParseRules(f *testing.F) {
	f.Add([]byte(rulesFile(2)))
	f.Add([]byte("format: 9\n"))
	f.Add([]byte("format: 3\nrules: [a]\n"))
	f.Add([]byte("format: 3\nrules:\n  - id: [\n"))
	w := porttest.NewWorld(f)
	w.SeedSchemas(schemaNames()...)
	schema := w.Ports().Schema
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, bad, err := parse(schema, "language/go/rules/errors.yaml", data, rulesSchema, migrations())
		if doc != nil && (len(bad) > 0 || err != nil) {
			t.Fatalf("a document with problems was answered: %v, %v", bad, err)
		}
		if doc != nil {
			for _, m := range ordered.Maps(doc.List("rules")) {
				_ = ruleOf(m)
			}
		}
	})
}

func TestABindingsRecordIsKeptAsPathDigestAndBytesAndNeverRead(t *testing.T) {
	t.Parallel()
	w, root := world(t, fullBook(t))
	got, err := load(t, w, root)
	if err != nil || len(got.Findings) != 0 {
		t.Fatalf("Load = %v, %v: a record that is not valid YAML must not be a finding", got.Findings, err)
	}
	if len(got.Book.BindingRecords) != 1 {
		t.Fatalf("records = %d", len(got.Book.BindingRecords))
	}
	rec := got.Book.BindingRecords[0]
	// sha256 of those exact bytes, taken with shasum -a 256
	const want = "sha256:f0cbdf2a08a36336c24d9f4bc96a78af67d8a3d8abf499ce2333679c82be5b46"
	if rec.Path != "bindings/tighten-errors.yaml" || rec.Digest != want || !strings.HasPrefix(string(rec.Data), "subject: base.errors.1\n") {
		t.Errorf("record = %q %q", rec.Path, rec.Digest)
	}
}

// TestABindingWithNoOptionsLoads: options are optional in the schema, and a
// binding without them loads with none. Planted failure: the nil check taken
// out of bindingOf, which then panics.
func TestABindingWithNoOptionsLoads(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["language/go/rules/errors.yaml"] = strings.Replace(files["language/go/rules/errors.yaml"], "        options: {ignore-sigs: [\".Errorf(\"]}\n", "", 1)
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil || len(got.Findings) != 0 {
		t.Fatalf("Load = %v, %v", got.Findings, err)
	}
	if b := got.Book.Rules[0].Bindings; len(b) != 1 || b[0].Options != nil {
		t.Errorf("the binding is %+v; want one with no options", b)
	}
}
