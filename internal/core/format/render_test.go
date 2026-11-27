package format

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const errorsFile = "language/go/rules/errors.yaml"

// commented is a concern file with what a person leaves in one: comments, a
// flow list, an anchor, a multi-line scalar, a trailing comment and a blank
// line between rules.
const commented = `# the errors of go
format: 3
part: go
concern: errors
rules:
  # the first rule
  - id: go.errors.1
    aliases: [go.errs.1]   # the old id
    statement: Wrap an error with what was being done.
    strength: must
    status: active
    scope: &scope
      parts: [go]
    reasoning:
      definition: |-
        An error says what failed
        and what was being done.
      pros:
        - the cause is found
        # a note in the list
        - the message is short
      decision: wrap-errors
    owner: team-a

  - id: go.errors.2
    statement: Return an error and never panic in a library.
    strength: should
    status: proposed
    origin: migrated
`

func renderWorld(t *testing.T, body string) (*porttest.World, model.Root, Loaded) {
	t.Helper()
	files := fullBook(t)
	files[errorsFile] = body
	delete(files, "translations/fr/errors.yaml")
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	return w, root, got
}

func render(t *testing.T, body string, d model.Delta) ([]model.Target, error) {
	t.Helper()
	w, root, got := renderWorld(t, body)
	p := w.Ports()
	return Render(p.Tree, p.Schema, root, got, d)
}

// diff lists the lines that differ between two texts, as - and + lines under
// the first line number that differs: the edit is one run of lines.
func diff(old, next string) string {
	a, b := strings.Split(strings.TrimSuffix(old, "\n"), "\n"), strings.Split(strings.TrimSuffix(next, "\n"), "\n")
	lo := 0
	for lo < len(a) && lo < len(b) && a[lo] == b[lo] {
		lo++
	}
	ea, eb := len(a), len(b)
	for ea > lo && eb > lo && a[ea-1] == b[eb-1] {
		ea--
		eb--
	}
	var sb strings.Builder
	for _, l := range a[lo:ea] {
		sb.WriteString("-" + l + "\n")
	}
	for _, l := range b[lo:eb] {
		sb.WriteString("+" + l + "\n")
	}
	return sb.String()
}

func one(t *testing.T, body string, d model.Delta) string {
	t.Helper()
	ts, err := render(t, body, d)
	if err != nil || len(ts) != 1 || ts[0].Path != errorsFile {
		t.Fatalf("targets = %v, err = %v", ts, err)
	}
	return string(ts[0].Data)
}

func setRule(id, field, before, after string) model.Delta {
	return model.Delta{Changes: []model.Change{{Entity: "rule:" + id, Field: field, Before: before, After: after}}}
}

// TestEachKindOfDeltaTouchesOnlyItsOwnedLines holds each kind against a
// literal diff: every other byte of the commented file is as it was.
func TestEachKindOfDeltaTouchesOnlyItsOwnedLines(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, entity, field, before, after, want string }{
		{"set a field", "go.errors.1", "owner", "team-a", "team-b", "-    owner: team-a\n+    owner: team-b\n"},
		{"retire a status", "go.errors.1", "status", "active", "retired", "-    status: active\n+    status: retired\n"},
		{"set a field of a second rule", "go.errors.2", "status", "proposed", "active", "-    status: proposed\n+    status: active\n"},
		{"set a dotted field", "go.errors.1", "reasoning.decision", "wrap-errors", "wrap", "-      decision: wrap-errors\n+      decision: wrap\n"},
		{"set a field that is not there", "go.errors.2", "owner", "", "team-c", "+    owner: team-c\n"},
		{"take a field out", "go.errors.1", "owner", "team-a", "", "-    owner: team-a\n"},
		{"add to a flow list", "go.errors.1", "aliases[]", "", "go.errx.1", "-    aliases: [go.errs.1]   # the old id\n+    aliases: [go.errs.1, go.errx.1]   # the old id\n"},
		{"add to a block list", "go.errors.1", "reasoning.pros[]", "", "it is plain", "+        - it is plain\n"},
		{"set a multi-line scalar", "go.errors.1", "reasoning.definition", "An error says what failed\nand what was being done.", "An error says why.",
			"-      definition: |-\n-        An error says what failed\n-        and what was being done.\n+      definition: An error says why.\n"},
		{"set a scalar through its alias id", "go.errs.1", "owner", "team-a", "team-d", "-    owner: team-a\n+    owner: team-d\n"},
	}
	ran := 0
	for _, c := range cases {
		out := one(t, commented, setRule(c.entity, c.field, c.before, c.after))
		if got := diff(commented, out); got != c.want {
			t.Errorf("%s: diff\n%s\nwant\n%s", c.name, got, c.want)
		}
		ran++
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d", ran, len(cases))
	}
}

func TestAddingARuleAppendsItAndLeavesTheRestAlone(t *testing.T) {
	t.Parallel()
	body := "id: go.errors.3\nstatement: Log an error once.\nstrength: may\nstatus: proposed\norigin: authored"
	out := one(t, commented, setRule("go.errors.3", "", "", body))
	want := "+\n+  - id: go.errors.3\n+    statement: Log an error once.\n+    strength: may\n+    status: proposed\n+    origin: authored\n"
	if got := diff(commented, out); got != want {
		t.Errorf("diff\n%s\nwant\n%s", got, want)
	}
}

func TestAStaleBeforeIsRefusedWithTheLineAndAMatchingOneIsNot(t *testing.T) {
	t.Parallel()
	_, err := render(t, commented, setRule("go.errors.1", "owner", "team-z", "team-b"))
	if !errors.Is(err, ErrMoved) || !strings.Contains(err.Error(), errorsFile+":23") {
		t.Errorf("err = %v; want ErrMoved at %s:23", err, errorsFile)
	}
	if _, err := render(t, commented, setRule("go.errors.1", "owner", "team-a", "team-b")); err != nil {
		t.Errorf("the clean control was refused: %v", err)
	}
}

func TestADeltaThatCannotBeRenderedIsRefusedByName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		d    model.Delta
		is   error
		want string
	}{
		{"an entity kind not rendered", model.Delta{Changes: []model.Change{{Entity: "binding:x", Field: "rung", Before: "a", After: "b"}}}, ErrUnsupported, `"binding"`},
		{"a rule not in the book", setRule("go.nope.1", "owner", "a", "b"), ErrMoved, "not in the book"},
		{"an anchored value", setRule("go.errors.1", "scope", "", "x"), ErrUnsupported, ""},
	}
	for _, c := range cases {
		_, err := render(t, commented, c.d)
		if !errors.Is(err, c.is) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want %v holding %q", c.name, err, c.is, c.want)
		}
	}
}

func TestAnEmptyDeltaRendersNothingAndIsDeterministic(t *testing.T) {
	t.Parallel()
	ts, err := render(t, commented, model.Delta{})
	if err != nil || len(ts) != 0 {
		t.Fatalf("empty delta: %v, %v", ts, err)
	}
	d := model.Delta{Changes: []model.Change{
		{Entity: "rule:go.errors.1", Field: "owner", Before: "team-a", After: "team-b"},
		{Entity: "rule:go.errors.2", Field: "status", Before: "proposed", After: "active"},
		{Entity: "rule:go.errors.1", Field: "aliases[]", After: "go.errx.1"},
	}}
	first := one(t, commented, d)
	for range 20 {
		if again := one(t, commented, d); again != first {
			t.Fatal("two renders of one delta differ")
		}
	}
}

func TestATargetHoldsTheDigestOfTheBytesItReplaces(t *testing.T) {
	t.Parallel()
	ts, err := render(t, commented, setRule("go.errors.1", "owner", "team-a", "team-b"))
	if err != nil || ts[0].Before != digestOf([]byte(commented)) || ts[0].Append {
		t.Errorf("target = %+v, %v", ts, err)
	}
}

func TestANewConcernFileIsCreatedFromItsHeader(t *testing.T) {
	t.Parallel()
	files := fixtureBook()
	files[StructureFile] = strings.Replace(structureYAML, "          - slug: errors\n", "          - slug: fresh\n            name: Fresh\n          - slug: errors\n", 1)
	w, root := world(t, files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	body := "id: go.fresh.1\nstatement: Log an error once.\nstrength: may\nstatus: proposed\norigin: authored"
	ts, err := Render(p.Tree, p.Schema, root, got, setRule("go.fresh.1", "", "", body))
	if err != nil || len(ts) != 1 || ts[0].Before != "" || ts[0].Path != "language/go/rules/fresh.yaml" {
		t.Fatalf("targets = %+v, %v", ts, err)
	}
	want := "format: 3\npart: go\nconcern: fresh\nrules:\n  - id: go.fresh.1\n    statement: Log an error once.\n    strength: may\n    status: proposed\n    origin: authored\n"
	if string(ts[0].Data) != want {
		t.Errorf("data = %q; want %q", ts[0].Data, want)
	}
}

func TestAGeneratedFileIsWrittenWholeAsCanonicalJSON(t *testing.T) {
	t.Parallel()
	w, root, got := renderWorld(t, commented)
	p := w.Ports()
	d := model.Delta{Changes: []model.Change{{Entity: "file:rulebook/rules.json", Before: "", After: `{"b": 1,  "a": [true]}`}}}
	ts, err := Render(p.Tree, p.Schema, root, got, d)
	if err != nil || len(ts) != 1 || ts[0].Path != "rulebook/rules.json" || ts[0].Before != "" {
		t.Fatalf("targets = %+v, %v", ts, err)
	}
	if want := `{"a":[true],"b":1}`; strings.TrimSpace(string(ts[0].Data)) != want {
		t.Errorf("data = %q; want canonical %s", ts[0].Data, want)
	}
	if again, _ := Render(p.Tree, p.Schema, root, got, d); !bytes.Equal(again[0].Data, ts[0].Data) {
		t.Error("two renders differ")
	}
	stale := model.Delta{Changes: []model.Change{{Entity: "file:" + errorsFile, Before: "sha256:00", After: "{}"}}}
	if _, err := Render(p.Tree, p.Schema, root, got, stale); !errors.Is(err, ErrMoved) {
		t.Errorf("a stale digest: err = %v", err)
	}
}

func FuzzRenderEdits(f *testing.F) {
	f.Add(commented, "owner", "team-a", "x")
	f.Add("rules:\n  - id: a.b.1\n    owner: &x y\n    z: [1,\n      2]\n", "owner", "y", "w")
	f.Add("rules:\n  - id: a.b.1\n    list:\n      - a\n      # c\n      - b\n", "list[]", "", "z")
	f.Fuzz(func(t *testing.T, body, field, before, after string) {
		tx, err := newText("f.yaml", []byte(body))
		if err != nil {
			return
		}
		if !bytes.Equal(tx.bytes(), []byte(body)) {
			t.Fatal("an unedited text does not give back its bytes")
		}
		_ = tx.setField("a.b.1", field, before, after)
		_ = tx.appendItem("a.b.1", field, after)
	})
}

func TestAFieldThatIsANumberInTheSchemaIsRefusedByName(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"pairs.number", "bindings.rung", "bindings.pair"} {
		_, err := render(t, commented, setRule("go.errors.1", field, "", "2"))
		if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), field) {
			t.Errorf("%s: err = %v; want ErrUnsupported naming it", field, err)
		}
	}
}

// A bare carriage return is a line break to the parser and not to the text,
// so the line it reports for a rule can lie past the last line the text
// holds. The edit refuses it by name and does not index past the end.
func TestARuleOnALineBreakTheTextDoesNotSplitAtIsRefusedNotIndexed(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"rules:\r- id: a.b.1", "rules:\r- id: a.b.1\r  owner: x\r"} {
		tx, err := newText("f.yaml", []byte(body))
		if err != nil {
			t.Fatalf("%q: newText: %v", body, err)
		}
		for name, run := range map[string]func() error{
			"setField":   func() error { return tx.setField("a.b.1", "owner", "", "y") },
			"appendItem": func() error { return tx.appendItem("a.b.1", "list", "y") },
			"ruleItem":   func() error { _, err := tx.ruleItem("a.b.1"); return err },
		} {
			err := run()
			if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "f.yaml:") {
				t.Errorf("%s of %q: err = %v; want ErrUnsupported naming file:line", name, body, err)
			}
		}
	}
}
