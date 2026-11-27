package format

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// openExclusions is the fixture's exclusions with an open record after the
// lifted one, and comments a person leaves.
const openExclusions = `format: 3
# what this overlay drops of its parent
exclusions:
  - overlay: example
    rule: base.errors.1
    number: 1
    decision: wrap-errors
    who: team-a
    when: "2026-11-02T10:00:00Z"
    lifted:
      decision: wrap-errors
      who: team-a
      when: "2026-11-03T10:00:00Z"

  - overlay: example
    rule: base.errors.1
    number: 2
    decision: wrap-errors   # again
    who: team-a
    when: "2026-11-04T10:00:00Z"
`

const newExclusion = "overlay: example\nrule: base.errors.2\nnumber: 1\ndecision: wrap-errors\nwho: team-a\nwhen: \"2026-11-08T10:00:00Z\"\n"

// exclusionWorld loads the fixture book with body as its exclusions.yaml, or
// with none when body is "", and renders a delta against it.
func exclusionWorld(t *testing.T, body string) (Loaded, func(model.Delta) ([]model.Target, error)) {
	t.Helper()
	files := fullBook(t)
	delete(files, "translations/fr/errors.yaml")
	delete(files, ExclusionsFile)
	if body != "" {
		files[ExclusionsFile] = body
	}
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	return l, func(d model.Delta) ([]model.Target, error) { return Render(p.Tree, p.Schema, root, l, d) }
}

func lift(number string, fields map[string]string) model.Delta {
	var d model.Delta
	for _, f := range []string{"decision", "who", "when"} {
		if v, ok := fields[f]; ok {
			d.Changes = append(d.Changes, model.Change{Entity: "exclusion:example/base.errors.1/" + number, Field: "lifted." + f, After: v})
		}
	}
	return d
}

// TestAnExclusionIsAppendedAndEveryOtherLineStays: a literal diff, the
// person's comments and blank line as they were.
func TestAnExclusionIsAppendedAndEveryOtherLineStays(t *testing.T) {
	t.Parallel()
	_, render := exclusionWorld(t, openExclusions)
	ts, err := render(add("exclusion:example/base.errors.2/1", newExclusion))
	if err != nil || len(ts) != 1 || ts[0].Path != ExclusionsFile || ts[0].Before == "" {
		t.Fatalf("targets %v, err %v", ts, err)
	}
	want := "+\n+  - overlay: example\n+    rule: base.errors.2\n+    number: 1\n+    decision: wrap-errors\n+    who: team-a\n+    when: \"2026-11-08T10:00:00Z\"\n"
	if got := diff(openExclusions, string(ts[0].Data)); got != want {
		t.Fatalf("diff\n%s\nwant\n%s", got, want)
	}
}

// TestAnOverlayWithNoExclusionsFileGetsOneFromItsHead.
func TestAnOverlayWithNoExclusionsFileGetsOneFromItsHead(t *testing.T) {
	t.Parallel()
	_, render := exclusionWorld(t, "")
	ts, err := render(add("exclusion:example/base.errors.2/1", newExclusion))
	if err != nil || len(ts) != 1 || ts[0].Before != "" {
		t.Fatalf("targets %v, err %v", ts, err)
	}
	if got := string(ts[0].Data); !strings.HasPrefix(got, "format: 3\nexclusions:\n  - overlay: example\n    rule: base.errors.2\n") {
		t.Fatalf("made\n%s", got)
	}
}

// TestLiftingWritesTheLiftedMappingAndKeepsTheRecord: the three fields land
// under lifted of the open record, and nothing of the record goes. Risk: an
// include that removes the record, so the history of the exclusion is lost.
func TestLiftingWritesTheLiftedMappingAndKeepsTheRecord(t *testing.T) {
	t.Parallel()
	_, render := exclusionWorld(t, openExclusions)
	ts, err := render(lift("2", map[string]string{"decision": "wrap-errors", "who": "team-b", "when": "2026-11-08T10:00:00Z"}))
	if err != nil || len(ts) != 1 {
		t.Fatalf("targets %v, err %v", ts, err)
	}
	want := "+    lifted:\n+      decision: wrap-errors\n+      who: team-b\n+      when: \"2026-11-08T10:00:00Z\"\n"
	if got := diff(openExclusions, string(ts[0].Data)); got != want {
		t.Fatalf("diff\n%s\nwant\n%s", got, want)
	}
}

// TestAnExclusionChangeThatIsNotAnAddOrALiftIsRefused: a record is never
// removed or rewritten, a lifted one is not lifted again, a taken key is not
// added twice, and an entity that is not a key is not guessed at.
func TestAnExclusionChangeThatIsNotAnAddOrALiftIsRefused(t *testing.T) {
	t.Parallel()
	_, render := exclusionWorld(t, openExclusions)
	one := func(c model.Change) model.Delta { return model.Delta{Changes: []model.Change{c}} }
	for name, tc := range map[string]struct {
		d    model.Delta
		want error
		says string
	}{
		"key taken":      {add("exclusion:example/base.errors.1/1", strings.Replace(newExclusion, "base.errors.2", "base.errors.1", 1)), ErrMoved, "is in the file already"},
		"lifted already": {lift("1", map[string]string{"who": "team-b"}), ErrMoved, "lifted.who of the exclusion example/base.errors.1/1"},
		"no such record": {lift("9", map[string]string{"who": "team-b"}), ErrMoved, "is not in the file"},
		"remove":         {one(model.Change{Entity: "exclusion:example/base.errors.1/2", Before: "x"}), ErrUnsupported, "adds the record or sets its lifted fields"},
		"rewrite":        {one(model.Change{Entity: "exclusion:example/base.errors.1/2", Field: "decision", Before: "wrap-errors", After: "x"}), ErrUnsupported, "adds the record or sets its lifted fields"},
		"other lifted":   {one(model.Change{Entity: "exclusion:example/base.errors.1/2", Field: "lifted.note", After: "x"}), ErrUnsupported, "adds the record or sets its lifted fields"},
		"key mismatch":   {add("exclusion:example/base.errors.3/1", newExclusion), nil, "not a mapping with that overlay, rule and number"},
		"no number":      {add("exclusion:example/base.errors.2", newExclusion), ErrUnsupported, "is not exclusion:<overlay>/<rule>/<number>"},
		"number 0":       {add("exclusion:example/base.errors.2/0", newExclusion), ErrUnsupported, "is not exclusion:<overlay>/<rule>/<number>"},
		"number 01":      {add("exclusion:example/base.errors.2/01", newExclusion), ErrUnsupported, "is not exclusion:<overlay>/<rule>/<number>"},
		"a path":         {add("exclusion:example/../x/1", newExclusion), ErrUnsupported, "is not exclusion:<overlay>/<rule>/<number>"},
	} {
		_, err := render(tc.d)
		if err == nil || !strings.Contains(err.Error(), tc.says) || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s: err %v, want %v saying %q", name, err, tc.want, tc.says)
		}
	}
}

// TestTwoRecordsUnderOneKeyAreListedByLoadAndNotLifted: two branches that
// each took the next number merge into a file that holds the key twice. Load
// lists the second with the line of the first, and a lift of that key is
// refused rather than marking one of the two.
func TestTwoRecordsUnderOneKeyAreListedByLoadAndNotLifted(t *testing.T) {
	t.Parallel()
	twice := openExclusions + "  - overlay: example\n    rule: base.errors.1\n    number: 2\n    decision: wrap-errors\n    who: team-b\n    when: \"2026-11-05T10:00:00Z\"\n"
	l, render := exclusionWorld(t, twice)
	var found []Finding
	for _, f := range l.Findings {
		if strings.Contains(f.Msg, "written twice") {
			found = append(found, f)
		}
	}
	if len(found) != 1 || found[0].File != ExclusionsFile || found[0].Line != 21 ||
		!strings.Contains(found[0].Msg, "example/base.errors.1/2 is written twice (first on line 15)") {
		t.Fatalf("findings %+v", l.Findings)
	}
	// the finding names the way guard staged's refusal of the
	// merge names, never a hand renumber, which guard staged refuses too
	if f := found[0]; strings.Contains(f.Msg+f.Fix, "highest number") ||
		!strings.Contains(f.Fix, "drop one side's commit") || !strings.Contains(f.Fix, "gragas rule exclude") {
		t.Errorf("the finding names a way the guard refuses: %q, fix %q", f.Msg, f.Fix)
	}
	if _, err := render(lift("2", map[string]string{"who": "team-b"})); err == nil || !strings.Contains(err.Error(), "2 records share the key") {
		t.Errorf("lifting a doubled key: %v", err)
	}
	clean, _ := exclusionWorld(t, openExclusions)
	for _, f := range clean.Findings {
		if strings.Contains(f.Msg, "written twice") {
			t.Errorf("one key once is listed: %+v", f)
		}
	}
}

// TestAnEditThatLeavesTheExclusionsInvalidIsRefused: the file as rendered is
// held to its schema before it is a target, so a record the schema refuses is
// never written.
func TestAnEditThatLeavesTheExclusionsInvalidIsRefused(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	delete(files, "translations/fr/errors.yaml")
	files[ExclusionsFile] = openExclusions
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	_, err = Render(p.Tree, refusing{p.Schema, exclusionsSchema}, root, l, add("exclusion:example/base.errors.2/1", strings.Replace(newExclusion, "decision: wrap-errors", "decision: gossip", 1)))
	if err == nil || !strings.Contains(err.Error(), "would leave exclusions.yaml invalid") {
		t.Errorf("err %v", err)
	}
}
