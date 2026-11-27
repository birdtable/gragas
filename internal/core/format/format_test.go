package format

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

const identityYAML = `format: 3
name: example
version: 1.0.0
kind: overlay
licences:
  rules: CC-BY-4.0
  presets: MIT
language: en
docs_url: https://docs.example.test/example
extends:
  rulebook: base
  version: 2.1.0
imports:
  - rulebook: other
    version: 0.3.0
    parts: [go, cli]
owners: [team-a]
governance:
  contribution: proposals
  decision_required: true
  trial_releases: 2
  review_days: 90
tested:
  tools:
    - name: golangci-lint
      version: 2.1.0
  min_engine: 1.0.0
budgets:
  statement_words: 30
`

const structureYAML = `format: 3
layers:
  - slug: language
    name: Language
    parts:
      - slug: go
        name: Go
        owners: [team-a]
        concerns:
          - slug: errors
            name: Errors
          - slug: layout
            name: Layout
            aliases: [folders]
kinds:
  - slug: service
    name: Service
    description: runs as a process
`

// world is a fake world holding a repository with files under dir, with both
// schemas compiled and every document valid unless planted.
func world(t *testing.T, files map[string]string) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	withFixture := maps.Clone(files)
	for p, c := range files {
		if path.Base(p) == StructureFile && c == structureYAML {
			for fp, fc := range fixtureFiles(t) {
				// a file the test planted is kept, not the fixture's
				at := strings.TrimPrefix(join(path.Dir(p), fp), "./")
				if _, ok := withFixture[at]; !ok {
					withFixture[at] = fc
				}
			}
		}
	}
	return w, w.Seed("repo", withFixture)
}

func schemaNames() []string {
	return []string{identitySchema, structureSchema, rulesSchema, termsSchema, sourcesSchema, decisionsSchema, exclusionsSchema, historySchema, proposalsSchema, blockSchema, translationsSchema}
}

// fixtureFiles is every file of testdata/book, by path below it: the YAML form
// of a whole book but for its identity and structure. It fails the test when
// it found none, so a loop over it never passes by running nothing.
func fixtureFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	// fs.WalkDir over os.DirFS: its paths are slash paths below the folder on
	// every system.
	base := os.DirFS(filepath.FromSlash("testdata/book"))
	err := fs.WalkDir(base, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(base, p)
		if err != nil {
			return err
		}
		out[p] = string(b)
		return nil
	})
	if err != nil || len(out) != 10 {
		t.Fatalf("the fixture has %d files (%v); it should have all ten", len(out), err)
	}
	return out
}

func load(t *testing.T, w *porttest.World, root model.Root) (Loaded, error) {
	t.Helper()
	p := w.Ports()
	return Load(p.Tree, p.Schema, root)
}

// docOf is the JSON the loader hands the Schema port for a YAML document, so
// a test can plant a problem for exactly it.
func docOf(t *testing.T, y string) []byte {
	t.Helper()
	v, err := ordered.ParseYAML([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(ordered.JSON(plain(v), -1))
}

func TestACleanRulebookLoadsIdentityAndStructure(t *testing.T) {
	t.Parallel()
	w, root := world(t, map[string]string{IdentityFile: identityYAML, StructureFile: structureYAML})
	got, err := load(t, w, root)
	if err != nil || len(got.Findings) != 0 {
		t.Fatalf("Load = %v, %v; want a clean book", got.Findings, err)
	}
	id := got.Book.Identity
	if id.Name != "example" || id.Kind != model.KindOverlay || id.Version != "1.0.0" || id.Language != "en" {
		t.Errorf("identity = %+v", id)
	}
	if id.Extends == nil || id.Extends.Rulebook != "base" {
		t.Errorf("extends = %+v", id.Extends)
	}
	if len(id.Imports) != 1 || strings.Join(id.Imports[0].Parts, ",") != "go,cli" {
		t.Errorf("imports = %+v", id.Imports)
	}
	if g := id.Governance; g.Contribution != model.ContributionProposals || !g.DecisionRequired || g.TrialReleases != 2 || g.ReviewDays != 90 {
		t.Errorf("governance = %+v", g)
	}
	if id.Tested == nil || id.Tested.MinEngine != "1.0.0" || id.Tested.Tools[0].Name != "golangci-lint" {
		t.Errorf("tested = %+v", id.Tested)
	}
	if id.Budgets == nil || id.Budgets.StatementWords != 30 || id.DocsURL != "https://docs.example.test/example" {
		t.Errorf("budgets, docs = %+v, %q", id.Budgets, id.DocsURL)
	}
	s := got.Book.Structure
	if len(s.Layers) != 1 || s.Layers[0].Parts[0].Concerns[1].Aliases[0] != "folders" || s.Kinds[0].Slug != "service" {
		t.Errorf("structure = %+v", s)
	}
	if s.Layers[0].Parts[0].Concerns[0].Slug != "errors" {
		t.Errorf("the order of the file was not kept: %+v", s.Layers[0].Parts[0].Concerns)
	}
}

func TestARulebookBelowASubfolderIsReadFromThere(t *testing.T) {
	t.Parallel()
	w, root := world(t, map[string]string{
		"README.md":                  "x",
		"docs/book/" + IdentityFile:  identityYAML,
		"docs/book/" + StructureFile: structureYAML,
	})
	got, err := load(t, w, root)
	if err != nil || len(got.Findings) != 0 || got.Dir != "docs/book" || got.Book.Identity.Name != "example" {
		t.Fatalf("Load = dir %q, %v, %v", got.Dir, got.Findings, err)
	}
}

func TestTwoRulebooksInOneTreeAreRefused(t *testing.T) {
	t.Parallel()
	w, root := world(t, map[string]string{
		"a/" + IdentityFile: identityYAML, "a/" + StructureFile: structureYAML,
		"b/" + IdentityFile: identityYAML, "b/" + StructureFile: structureYAML,
	})
	_, err := load(t, w, root)
	if !errors.Is(err, ErrTwoBooks) || !strings.Contains(err.Error(), "a/rulebook.yaml") || !strings.Contains(err.Error(), "b/rulebook.yaml") {
		t.Fatalf("err = %v; want ErrTwoBooks naming both files", err)
	}
}

// TestTheShallowestRulebookWins: deeper ones are embedded fixtures.
func TestTheShallowestRulebookWins(t *testing.T) {
	t.Parallel()
	deep := "internal/host/assets/fixture/"
	w, root := world(t, map[string]string{
		"rulebook/" + IdentityFile:     identityYAML,
		"rulebook/" + StructureFile:    structureYAML,
		deep + "base/" + IdentityFile:  identityYAML,
		deep + "other/" + IdentityFile: identityYAML,
	})
	if dir, err := Find(w.Ports().Tree, root); err != nil || dir != "rulebook" {
		t.Fatalf("(a) Find = %q, %v; want rulebook", dir, err)
	}
	w, root = world(t, map[string]string{
		"a/" + IdentityFile: identityYAML, "b/" + IdentityFile: identityYAML,
		"a/x/y/" + IdentityFile: identityYAML,
	})
	if _, err := Find(w.Ports().Tree, root); !errors.Is(err, ErrTwoBooks) || !strings.Contains(err.Error(), "a/rulebook.yaml") || !strings.Contains(err.Error(), "b/rulebook.yaml") {
		t.Fatalf("(b) err = %v; want ErrTwoBooks naming both", err)
	}
	w, root = world(t, map[string]string{
		IdentityFile: identityYAML, StructureFile: structureYAML,
		"rulebook/x/" + IdentityFile: identityYAML,
	})
	if dir, err := Find(w.Ports().Tree, root); err != nil || dir != "" {
		t.Fatalf("(c) Find = %q, %v; want the root's own", dir, err)
	}
}

// TestARulebookBelowTestdataOrADotOrUnderscoreFolderIsNoBook: a
// rulebook.yaml in a fixture or a hidden folder sits beside the real book and
// the finder returns the real one alone; a lone one there is no book.
func TestARulebookBelowTestdataOrADotOrUnderscoreFolderIsNoBook(t *testing.T) {
	t.Parallel()
	w, root := world(t, map[string]string{
		IdentityFile:                         identityYAML,
		StructureFile:                        structureYAML,
		"pkg/testdata/x/" + IdentityFile:     identityYAML,
		"testdata/" + IdentityFile:           identityYAML,
		".hidden/" + IdentityFile:            identityYAML,
		"a/_old/" + IdentityFile:             identityYAML,
		"a/.git-copy/b/" + IdentityFile:      identityYAML,
		"testdatabase/" + IdentityFile + "x": "x",
	})
	got, err := load(t, w, root)
	if err != nil || got.Dir != "" {
		t.Fatalf("Load = dir %q, %v; want the root's own book", got.Dir, err)
	}
	only, root2 := world(t, map[string]string{"testdata/x/" + IdentityFile: identityYAML, ".hidden/" + IdentityFile: identityYAML})
	if _, err := load(t, only, root2); !errors.Is(err, ErrNoBook) {
		t.Fatalf("err = %v; want ErrNoBook for books below testdata and hidden folders alone", err)
	}
}

func TestNoRulebookIsNamedAsSuch(t *testing.T) {
	t.Parallel()
	w, root := world(t, map[string]string{"README.md": "x"})
	if _, err := load(t, w, root); !errors.Is(err, ErrNoBook) {
		t.Fatalf("err = %v; want ErrNoBook", err)
	}
}

func TestANewerFormatEndsNotReadyNamingBothVersions(t *testing.T) {
	t.Parallel()
	w, root := world(t, map[string]string{IdentityFile: strings.Replace(identityYAML, "format: 3", "format: 4", 1), StructureFile: structureYAML})
	_, err := load(t, w, root)
	nr, ok := errors.AsType[*NotReady](err)
	if !ok || nr.Found != 4 || nr.Reads != model.FormatVersion {
		t.Fatalf("err = %v; want *NotReady 4 vs %d", err, model.FormatVersion)
	}
	for _, want := range []string{"rulebook.yaml", "format 4", "format 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not name %q", err, want)
		}
	}
}

func TestAnOlderFormatIsMovedForwardByItsSteps(t *testing.T) {
	t.Parallel()
	old := strings.Replace(identityYAML, "format: 3", "format: 2", 1)
	steps := map[int]step{2: func(doc *ordered.Map) (*ordered.Map, []string) {
		doc.Set("format", int64(3), doc.KeyLine("format"))
		return doc, nil
	}}
	w, root := world(t, nil)
	_ = root
	p := w.Ports()
	doc, bad, err := parse(p.Schema, IdentityFile, []byte(old), identitySchema, steps)
	if err != nil || len(bad) != 0 || doc.Str("name") != "example" {
		t.Fatalf("parse = %v, %v", bad, err)
	}
	if n, _ := doc.Get("format"); n != int64(3) {
		t.Errorf("format = %v after the step", n)
	}
	_, bad, err = parse(p.Schema, IdentityFile, []byte(old), identitySchema, map[int]step{})
	if err != nil || len(bad) != 1 || bad[0].Line != 1 || !strings.Contains(bad[0].Msg, "not ready: format 2 is moved forward by gragas migrate") {
		t.Fatalf("with no step: %v, %v", bad, err)
	}
	_, bad, err = parse(p.Schema, IdentityFile, []byte(strings.Replace(identityYAML, "format: 3", "format: 1", 1)), identitySchema, steps)
	if err != nil || len(bad) != 1 || !strings.Contains(bad[0].Msg, "older than the previous major (2)") {
		t.Fatalf("format 1: %v, %v", bad, err)
	}
}

// malformed is the planted files of the golden: each is one finding listing.
func TestFindingsCarryFileAndLineAndAreAGolden(t *testing.T) {
	t.Parallel()
	dupName := strings.Replace(identityYAML, "name: example\n", "name: example\nname: again\n", 1)
	var out strings.Builder
	list := func(title string, files map[string]string, plant func(w *porttest.World)) {
		w, root := world(t, files)
		if plant != nil {
			plant(w)
		}
		got, err := load(t, w, root)
		out.WriteString("## " + title + "\n")
		if err != nil {
			out.WriteString("error: " + err.Error() + "\n")
		}
		for _, f := range got.Findings {
			out.WriteString(f.String() + "\n")
		}
		if err == nil && len(got.Findings) == 0 {
			out.WriteString("clean\n")
		}
	}
	list("a clean control", map[string]string{IdentityFile: identityYAML, StructureFile: structureYAML}, nil)
	list("a field written twice", map[string]string{IdentityFile: dupName, StructureFile: structureYAML}, nil)
	list("yaml that does not parse", map[string]string{IdentityFile: "format: 3\nname: [unclosed\n", StructureFile: structureYAML}, nil)
	list("not a mapping", map[string]string{IdentityFile: "- a\n- b\n", StructureFile: structureYAML}, nil)
	list("the structure file missing", map[string]string{IdentityFile: identityYAML}, nil)
	list("a schema problem lands on its line", map[string]string{IdentityFile: identityYAML, StructureFile: structureYAML}, func(w *porttest.World) {
		w.Plant(identitySchema, docOf(t, identityYAML), model.Problem{Line: 1, Msg: "schema at governance/review_days: must be at least 1"})
	})
	list("a slug used twice", map[string]string{IdentityFile: identityYAML, StructureFile: strings.Replace(structureYAML, "slug: layout", "slug: errors", 1)}, nil)
	list("a part alias that takes a slug in use", map[string]string{IdentityFile: identityYAML, StructureFile: strings.Replace(structureYAML, "name: Go\n", "name: Go\n        aliases: [go]\n", 1)}, nil)
	list("a part named like the engine's checks", map[string]string{IdentityFile: identityYAML, StructureFile: strings.Replace(structureYAML, "slug: go\n", "slug: gragas\n", 1)}, nil)

	configtest.Golden(t, "findings.golden", []byte(out.String()))
}

func FuzzParseIdentity(f *testing.F) {
	f.Add([]byte(identityYAML))
	f.Add([]byte("format: 4\n"))
	f.Add([]byte("- a\n"))
	f.Add([]byte("name: [\n"))
	w := porttest.NewWorld(f)
	w.SeedSchemas(identitySchema, structureSchema)
	schema := w.Ports().Schema
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, bad, err := parse(schema, IdentityFile, data, identitySchema, migrations())
		if doc != nil && (len(bad) > 0 || err != nil) {
			t.Fatalf("a document with problems was answered: %v, %v", bad, err)
		}
		if doc != nil {
			_ = identityOf(doc)
		}
	})
}

func FuzzParseStructure(f *testing.F) {
	f.Add([]byte(structureYAML))
	f.Add([]byte("format: 9\n"))
	f.Add([]byte("layers: {}\n"))
	w := porttest.NewWorld(f)
	w.SeedSchemas(identitySchema, structureSchema)
	schema := w.Ports().Schema
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, bad, err := parse(schema, StructureFile, data, structureSchema, migrations())
		if doc != nil && (len(bad) > 0 || err != nil) {
			t.Fatalf("a document with problems was answered: %v, %v", bad, err)
		}
		if doc != nil {
			_ = structureOf(doc)
			_ = slugFindings(StructureFile, doc)
		}
	})
}

func TestTheSchemaNamesAreOfTheCurrentFormat(t *testing.T) {
	t.Parallel()
	prefix := fmt.Sprintf("v%d/", model.FormatVersion)
	for _, n := range schemaNames() {
		if !strings.HasPrefix(n, prefix) {
			t.Errorf("%s is not under %s: the schemas are one folder a format version", n, prefix)
		}
	}
}
