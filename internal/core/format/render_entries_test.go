package format

import (
	"errors"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

const newSource = "slug: rfc-9110\nkind: standard\ntitle: HTTP Semantics\nlocator: https://www.rfc-editor.org/rfc/rfc9110\n"

const newDecision = "slug: keep-it-small\ntitle: Keep rules small\ntext: One rule says one thing.\nwho: team-a\ndate: \"2026-11-08\"\n"

func entryWorld(t *testing.T, drop ...string) (*Loaded, func(model.Delta) ([]model.Target, error)) {
	t.Helper()
	files := fullBook(t)
	delete(files, "translations/fr/errors.yaml")
	for _, d := range drop {
		delete(files, d)
	}
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	return &l, func(d model.Delta) ([]model.Target, error) {
		return Render(p.Tree, refusing{p.Schema, sourcesSchema}, root, l, d)
	}
}

func add(entity, body string) model.Delta {
	return model.Delta{Changes: []model.Change{{Entity: entity, After: body}}}
}

// TestAddingASourceOrADecisionAppendsOneEntryAndTouchesNothingElse: a literal
// diff of each file, the lines a person wrote as they were.
func TestAddingASourceOrADecisionAppendsOneEntryAndTouchesNothingElse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, entity, body, file, want string }{
		{"source", "source:rfc-9110", newSource, SourcesFile,
			"+  - slug: rfc-9110\n+    kind: standard\n+    title: HTTP Semantics\n+    locator: https://www.rfc-editor.org/rfc/rfc9110\n"},
		{"decision", "decision:keep-it-small", newDecision, DecisionsFile,
			"+  - slug: keep-it-small\n+    title: Keep rules small\n+    text: One rule says one thing.\n+    who: team-a\n+    date: \"2026-11-08\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l, render := entryWorld(t)
			ts, err := render(add(tc.entity, tc.body))
			if err != nil || len(ts) != 1 || ts[0].Path != tc.file {
				t.Fatalf("targets %v, err %v", ts, err)
			}
			old := fileOf(t, tc.file)
			if got := diff(old, string(ts[0].Data)); got != tc.want {
				t.Fatalf("diff\n%s\nwant\n%s", got, tc.want)
			}
			if ts[0].Before == "" || ts[0].Append {
				t.Errorf("target %+v: a replaced file holds the digest of what it replaces", ts[0])
			}
			_ = l
		})
	}
}

func fileOf(t *testing.T, name string) string {
	t.Helper()
	return fullBook(t)[name]
}

func TestASourceOrDecisionFileThatIsNotThereIsMadeFromItsHead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ entity, body, file, head string }{
		{"source:rfc-9110", newSource, SourcesFile, "format: 3\nsources:\n  - slug: rfc-9110\n"},
		{"decision:keep-it-small", newDecision, DecisionsFile, "format: 3\ndecisions:\n  - slug: keep-it-small\n"},
	} {
		_, render := entryWorld(t, tc.file)
		ts, err := render(add(tc.entity, tc.body))
		if err != nil || len(ts) != 1 || !strings.HasPrefix(string(ts[0].Data), tc.head) || ts[0].Before != "" {
			t.Fatalf("%s: %v, %v", tc.file, ts, err)
		}
	}
}

func TestAnEntryThatIsAlreadyThereOrIsNotAnAddIsRefused(t *testing.T) {
	t.Parallel()
	_, render := entryWorld(t)
	for name, tc := range map[string]struct {
		d    model.Delta
		want string
	}{
		"slug taken":    {add("source:effective-go", strings.Replace(newSource, "rfc-9110", "effective-go", 1)), "is in the file already"},
		"edit a field":  {model.Delta{Changes: []model.Change{{Entity: "source:effective-go", Field: "kind", Before: "guide", After: "study"}}}, "adds an entry only"},
		"supersede":     {model.Delta{Changes: []model.Change{{Entity: "decision:wrap-errors", Before: "old", After: newDecision}}}, "adds an entry only"},
		"slug mismatch": {add("source:other", newSource), "not a mapping with that slug"},
		"invalid entry": {add("source:rfc-9110", "slug: rfc-9110\nkind: gossip\ntitle: T\nlocator: l\n"), "invalid"},
		"not a mapping": {add("decision:keep-it-small", "- just\n- a list\n"), "not a mapping with that slug"},
	} {
		_, err := render(tc.d)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want %q", name, err, tc.want)
		}
	}
	_, err := render(add("source:effective-go", strings.Replace(newSource, "rfc-9110", "effective-go", 1)))
	if !errors.Is(err, ErrMoved) {
		t.Errorf("a taken slug is a moved tree: %v", err)
	}
}

// refusing is a Schema port that finds a problem in any file checked against
// the named schema whose text holds "gossip", the way the real schema refuses
// a kind it does not list.
type refusing struct {
	port.Schema
	name string
}

func (r refusing) Check(name string, doc []byte) ([]model.Problem, error) {
	if name == r.name && strings.Contains(string(doc), "gossip") {
		return []model.Problem{{Line: 1, Msg: "schema at sources/1/kind: not one of the kinds"}}, nil
	}
	return r.Schema.Check(name, doc)
}

const newTerm = "slug: header\ndefinition: The name a request carries.\nliteral: X-Request-Id\n"

// TestAddingATermAppendsOneEntryAndTouchesNothingElse: a literal diff of
// terms.yaml, the lines a person wrote as they were.
// Planted failure: term is not an entry kind, or the entry lands above the
// terms already there.
func TestAddingATermAppendsOneEntryAndTouchesNothingElse(t *testing.T) {
	t.Parallel()
	_, render := entryWorld(t)
	ts, err := render(add("term:header", newTerm))
	if err != nil || len(ts) != 1 || ts[0].Path != TermsFile {
		t.Fatalf("targets %v, err %v", ts, err)
	}
	want := "+  - slug: header\n+    definition: The name a request carries.\n+    literal: X-Request-Id\n"
	if got := diff(fileOf(t, TermsFile), string(ts[0].Data)); got != want {
		t.Fatalf("diff\n%s\nwant\n%s", got, want)
	}
	if ts[0].Before == "" {
		t.Errorf("a replaced file holds the digest of what it replaces: %+v", ts[0])
	}
}

// TestAFieldOfATermIsSetFromItsBeforeAndNothingElseMoves: the literal is
// changed, taken away and added, and the status set, each from the value the
// file holds now.
// Planted failure: the field is set without reading its before value, or
// another line of the file moves.
func TestAFieldOfATermIsSetFromItsBeforeAndNothingElseMoves(t *testing.T) {
	t.Parallel()
	set := func(field, before, after string) model.Delta {
		return model.Delta{Changes: []model.Change{{Entity: "term:gate", Field: field, Before: before, After: after}}}
	}
	for _, tc := range []struct {
		name string
		d    model.Delta
		want string
	}{
		{"change the literal", set("literal", "make check", "gate run"), "-    literal: make check\n+    literal: gate run\n"},
		{"take the literal away", set("literal", "make check", ""), "-    literal: make check\n"},
		{"retire it", set("status", "", "retired"), "+    status: retired\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, render := entryWorld(t)
			ts, err := render(tc.d)
			if err != nil || len(ts) != 1 || ts[0].Path != TermsFile {
				t.Fatalf("targets %v, err %v", ts, err)
			}
			if got := diff(fileOf(t, TermsFile), string(ts[0].Data)); got != tc.want {
				t.Fatalf("diff\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// TestATermEditThatIsNotALifecycleFieldOrNotWhatTheFileHoldsIsRefused: the
// definition is prose and a file edit; a before value that is not the file's is
// a moved tree.
func TestATermEditThatIsNotALifecycleFieldOrNotWhatTheFileHoldsIsRefused(t *testing.T) {
	t.Parallel()
	_, render := entryWorld(t)
	for name, tc := range map[string]struct {
		d    model.Delta
		is   error
		want string
	}{
		"the definition":  {model.Delta{Changes: []model.Change{{Entity: "term:gate", Field: "definition", Before: "A check that stops a push.", After: "x"}}}, ErrUnsupported, "adds an entry only"},
		"a moved literal": {model.Delta{Changes: []model.Change{{Entity: "term:gate", Field: "literal", Before: "other", After: "x"}}}, ErrMoved, "expected"},
		"an unknown term": {model.Delta{Changes: []model.Change{{Entity: "term:nope", Field: "literal", Before: "", After: "x"}}}, ErrMoved, "not in the file"},
		"a term twice":    {add("term:gate", "slug: gate\ndefinition: again\n"), ErrMoved, "in the file already"},
	} {
		_, err := render(tc.d)
		if err == nil || !strings.Contains(err.Error(), tc.want) || (tc.is != nil && !errors.Is(err, tc.is)) {
			t.Errorf("%s: err %v, want %q", name, err, tc.want)
		}
	}
}

// TestAFieldOfASourceIsSetFromItsBeforeAndNothingElseMoves: the title and the
// locator change, an attribution is added and taken away, each from the value
// the file holds now, and no other line moves.
// Planted failure: source is not an editable kind, or the field is set without
// reading its before value.
func TestAFieldOfASourceIsSetFromItsBeforeAndNothingElseMoves(t *testing.T) {
	t.Parallel()
	set := func(field, before, after string) model.Delta {
		return model.Delta{Changes: []model.Change{{Entity: "source:effective-go", Field: field, Before: before, After: after}}}
	}
	for _, tc := range []struct {
		name string
		d    model.Delta
		want string
	}{
		{"retitle", set("title", "Effective Go", "Effective Go, 2nd"), "-    title: Effective Go\n+    title: \"Effective Go, 2nd\"\n"},
		{"relocate", set("locator", "https://go.dev/doc/effective_go", "https://go.dev/doc/effective_go#errors"),
			"-    locator: https://go.dev/doc/effective_go\n+    locator: \"https://go.dev/doc/effective_go#errors\"\n"},
		{"say who else", set("attribution", "The Go authors", "the Go team"), "-    attribution: The Go authors\n+    attribution: the Go team\n"},
		{"take it away", set("attribution", "The Go authors", ""), "-    attribution: The Go authors\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, render := entryWorld(t)
			ts, err := render(tc.d)
			if err != nil || len(ts) != 1 || ts[0].Path != SourcesFile {
				t.Fatalf("targets %v, err %v", ts, err)
			}
			if got := diff(fileOf(t, SourcesFile), string(ts[0].Data)); got != tc.want {
				t.Fatalf("diff\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// TestASourceEditThatIsNotAFieldOfItOrNotWhatTheFileHoldsIsRefused: the kind is
// not edited, a before value that is not the file's is a moved tree, and an
// unknown source is not in the file.
func TestASourceEditThatIsNotAFieldOfItOrNotWhatTheFileHoldsIsRefused(t *testing.T) {
	t.Parallel()
	_, render := entryWorld(t)
	for name, tc := range map[string]struct {
		d    model.Delta
		is   error
		want string
	}{
		"the kind":        {model.Delta{Changes: []model.Change{{Entity: "source:effective-go", Field: "kind", Before: "guide", After: "study"}}}, ErrUnsupported, "adds an entry only"},
		"a moved title":   {model.Delta{Changes: []model.Change{{Entity: "source:effective-go", Field: "title", Before: "other", After: "x"}}}, ErrMoved, "expected"},
		"an unknown slug": {model.Delta{Changes: []model.Change{{Entity: "source:nope", Field: "title", Before: "T", After: "x"}}}, ErrMoved, "not in the file"},
	} {
		_, err := render(tc.d)
		if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, tc.is) {
			t.Errorf("%s: err %v, want %q", name, err, tc.want)
		}
	}
}
