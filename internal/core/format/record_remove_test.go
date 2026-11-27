package format

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// cited is a concern file whose rules cite more than one source, in each style
// a person writes a list of records in: a block list with comments and blank
// lines between its items, a flow list, a flow list with quotes and commas in
// its texts, and a block list of one.
const cited = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error with what was being done.
    strength: must
    status: active
    origin: authored
    sources:
      - ref: wrap-errors   # the first
        where: section 2
        backs: rule

      # the standard
      - ref: rfc-9110
        backs: rule
      - {ref: go-faq, backs: check}
    owner: team-a

  - id: go.errors.2
    statement: Return an error and never panic in a library.
    strength: should
    status: active
    origin: authored
    sources: [{ref: wrap-errors, backs: rule}, {ref: rfc-9110, backs: rule}]   # flow

  - id: go.errors.3
    statement: Log an error once.
    strength: may
    status: active
    origin: authored
    sources: [{ref: go-faq, says: "a, b and [c]", backs: rule},{ref: rfc-9110, where: 'it''s 4, a', backs: rule}]

  - id: go.errors.4
    statement: Wrap with the verb w.
    strength: should
    status: proposed
    origin: authored
    sources:
      - ref: wrap-errors
        backs: rule
    owner: team-b
`

func dropRecord(id, field, before string) model.Delta { return setRule(id, field+"{}", before, "") }

func TestARecordIsTakenOutOfEachListInEachStyleAndTouchesNothingElse(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, id, record, want string }{
		{"the first of a block list, with its blank line", "go.errors.1", "ref: wrap-errors\nwhere: section 2\nbacks: rule",
			"-      - ref: wrap-errors   # the first\n-        where: section 2\n-        backs: rule\n-\n"},
		{"the middle of a block list, with the comment above it", "go.errors.1", "ref: rfc-9110\nbacks: rule",
			"-      # the standard\n-      - ref: rfc-9110\n-        backs: rule\n"},
		{"the last of a block list, written in flow style", "go.errors.1", "ref: go-faq\nbacks: check",
			"-      - {ref: go-faq, backs: check}\n"},
		{"the first of a flow list", "go.errors.2", "ref: wrap-errors\nbacks: rule",
			"-    sources: [{ref: wrap-errors, backs: rule}, {ref: rfc-9110, backs: rule}]   # flow\n+    sources: [{ref: rfc-9110, backs: rule}]   # flow\n"},
		{"the last of a flow list", "go.errors.2", "ref: rfc-9110\nbacks: rule",
			"-    sources: [{ref: wrap-errors, backs: rule}, {ref: rfc-9110, backs: rule}]   # flow\n+    sources: [{ref: wrap-errors, backs: rule}]   # flow\n"},
		{"a flow item with commas and brackets in quotes", "go.errors.3", "ref: go-faq\nsays: a, b and [c]\nbacks: rule",
			"-    sources: [{ref: go-faq, says: \"a, b and [c]\", backs: rule},{ref: rfc-9110, where: 'it''s 4, a', backs: rule}]\n+    sources: [{ref: rfc-9110, where: 'it''s 4, a', backs: rule}]\n"},
		{"the second flow item, quoted with a doubled quote", "go.errors.3", "ref: rfc-9110\nwhere: it's 4, a\nbacks: rule",
			"-    sources: [{ref: go-faq, says: \"a, b and [c]\", backs: rule},{ref: rfc-9110, where: 'it''s 4, a', backs: rule}]\n+    sources: [{ref: go-faq, says: \"a, b and [c]\", backs: rule}]\n"},
		{"the only one, with its key", "go.errors.4", "ref: wrap-errors\nbacks: rule",
			"-    sources:\n-      - ref: wrap-errors\n-        backs: rule\n"},
	}
	for _, c := range cases {
		out := one(t, cited, dropRecord(c.id, "sources", c.record))
		if got := diff(cited, out); got != c.want {
			t.Errorf("%s: diff\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

func TestTwoRecordsAreTakenOutOfOneListInOneDelta(t *testing.T) {
	t.Parallel()
	d := dropRecord("go.errors.1", "sources", "ref: wrap-errors\nwhere: section 2\nbacks: rule")
	d.Changes = append(d.Changes, dropRecord("go.errors.1", "sources", "ref: go-faq\nbacks: check").Changes...)
	const was = "      - ref: wrap-errors   # the first\n        where: section 2\n        backs: rule\n\n" +
		"      # the standard\n      - ref: rfc-9110\n        backs: rule\n      - {ref: go-faq, backs: check}\n"
	want := strings.Replace(cited, was, "      # the standard\n      - ref: rfc-9110\n        backs: rule\n", 1)
	if got := one(t, cited, d); got != want {
		t.Errorf("diff\n%s", diff(want, got))
	}
}

func TestARecordIsTakenOutOfAListInAGuideBlock(t *testing.T) {
	t.Parallel()
	added, err := guideRender(t, proseGuide, addRecord("go.layout.9", "sources", "ref: rfc-9110\nbacks: rule"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := guideRender(t, string(added), dropRecord("go.layout.9", "sources", "ref: rfc-9110\nbacks: rule"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != proseGuide {
		t.Errorf("the record and its key were not all taken out:\n%s", diff(proseGuide, string(out)))
	}
}

func TestARecordTheListDoesNotHoldIsNotTakenOut(t *testing.T) {
	t.Parallel()
	const flowInFlow = "rules:\n  - id: a.b.1\n    reasoning: {sources: [{ref: a, backs: rule}]}\n"
	cases := []struct{ name, body, id, field, record, says string }{
		{"a record the list does not hold", cited, "go.errors.1", "sources", "ref: go-faq\nbacks: rule", "does not hold"},
		{"a record with a place the list holds without it", cited, "go.errors.2", "sources", "ref: rfc-9110\nwhere: x\nbacks: rule", "does not hold"},
		{"a rule with no list", recorded, "go.errors.3", "sources", "ref: wrap-errors\nbacks: rule", "does not hold"},
		{"a field that is not a list", cited, "go.errors.1", "owner", "ref: a\nbacks: rule", "owner is not a list"},
		{"a record that is not a mapping", cited, "go.errors.1", "sources", "- ref: x", "not a mapping"},
		{"a list inside a flow mapping", flowInFlow, "a.b.1", "reasoning.sources", "ref: a\nbacks: rule", "written in flow style"},
		{"a rule not in the file", cited, "go.errors.9", "sources", "ref: a\nbacks: rule", "not in the file"},
	}
	for _, c := range cases {
		tx, err := newText("f.yaml", []byte(c.body))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		err = tx.apply(model.Change{Entity: "rule:" + c.id, Field: c.field + "{}", Before: c.record})
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v; want one saying %q", c.name, err, c.says)
		}
		if string(tx.bytes()) != c.body {
			t.Errorf("%s: a refused removal changed the text", c.name)
		}
	}
}

// TestARecordChangeWithABeforeAndAnAfterIsMadeInPlace: a change
// that names both changes the record that holds the before keys on the lines
// they stand on, and leaves the record's other keys and comments as written; a
// change that adds a key, or whose before the list does not hold, moves nothing.
func TestARecordChangeWithABeforeAndAnAfterIsMadeInPlace(t *testing.T) {
	t.Parallel()
	tx, err := newText("f.yaml", []byte(cited))
	if err != nil {
		t.Fatal(err)
	}
	change := func(before, after string) error {
		return tx.apply(model.Change{Entity: "rule:go.errors.1", Field: "sources{}", Before: before, After: after})
	}
	if err := change("ref: wrap-errors\nbacks: rule", "ref: wrap-errors\nbacks: check"); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(cited, "        backs: rule\n\n      # the standard", "        backs: check\n\n      # the standard", 1)
	if want == cited || string(tx.bytes()) != want {
		t.Errorf("the text after the change:\n%s\nwant:\n%s", tx.bytes(), want)
	}
	for name, c := range map[string][2]string{
		"a key added":       {"ref: wrap-errors\nbacks: check", "ref: wrap-errors\nbacks: check\nwhere: here"},
		"a record not held": {"ref: nobody\nbacks: rule", "ref: nobody\nbacks: check"},
	} {
		kept := string(tx.bytes())
		if err := change(c[0], c[1]); err == nil || string(tx.bytes()) != kept {
			t.Errorf("%s: err = %v, or the text changed; want a refusal that moves nothing", name, err)
		}
	}
}

func TestFlowItemsPartsAListOnlyAtItsOwnCommas(t *testing.T) {
	t.Parallel()
	cases := []struct {
		list string
		want []string
		ok   bool
	}{
		{"[a, b]", []string{"a", "b"}, true},
		{"[ a ,b ]", []string{"a", "b"}, true},
		{`[{x: "a, b"}, 'c, ''d''', [e, f]]`, []string{`{x: "a, b"}`, `'c, ''d'''`, "[e, f]"}, true},
		{`["a\", b", c]`, []string{`"a\", b"`, "c"}, true},
		{"[]", nil, true},
		{"[a, , b]", nil, false},
		{"[a, b", nil, false},
		{"[a] x", nil, false},
		{"[{a: b]", nil, false},
	}
	for _, c := range cases {
		spans, ok := flowItems(c.list)
		var got []string
		for _, s := range spans {
			got = append(got, c.list[s[0]:s[1]])
		}
		if ok != c.ok || (ok && strings.Join(got, "|") != strings.Join(c.want, "|")) {
			t.Errorf("flowItems(%q) = %q, %v; want %q, %v", c.list, got, ok, c.want, c.ok)
		}
	}
}

func FuzzRemoveRecord(f *testing.F) {
	f.Add(cited, "go.errors.1", "sources", "ref: rfc-9110\nbacks: rule")
	f.Add(cited, "go.errors.3", "sources", "ref: go-faq\nsays: a, b and [c]\nbacks: rule")
	f.Add(cited, "go.errors.4", "sources", "ref: wrap-errors\nbacks: rule")
	f.Fuzz(func(t *testing.T, body, id, field, before string) {
		tx, err := newText("f.yaml", []byte(body))
		if err != nil || before == "" {
			return
		}
		err = tx.apply(model.Change{Entity: "rule:" + id, Field: field + "{}", Before: before})
		if err != nil {
			if string(tx.bytes()) != body {
				t.Fatalf("a refused removal changed the text:\n%q\n%q", body, tx.bytes())
			}
			return
		}
		if _, err := tx.doc(); err != nil {
			t.Fatalf("a removal left text that does not parse: %v\n%q", err, tx.bytes())
		}
	})
}

// TestABindingWithOptionsIsTakenOutWholeAndANestedMappingMustMatch: the record
// goes with its option lines, a comment above the next stays, and a record whose
// options differ is not the one asked for.
func TestABindingWithOptionsIsTakenOutWholeAndANestedMappingMustMatch(t *testing.T) {
	t.Parallel()
	const body = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error with what was being done.
    strength: must
    status: active
    origin: authored
    bindings:
      - tool: lint-a
        target: config
        key: wrap
        options:
          max: 3
          skip: [a, b]
        severity: error
        rung: 1
        run_point: commit
      # the second
      - tool: lint-b
        target: config
        key: other
        severity: error
        rung: 1
        run_point: commit
`
	const rec = "tool: lint-a\ntarget: config\nkey: wrap\noptions:\n  max: 3\n  skip: [a, b]\nseverity: error\nrung: 1\nrun_point: commit"
	out := one(t, body, dropRecord("go.errors.1", "bindings", rec))
	want := "-      - tool: lint-a\n-        target: config\n-        key: wrap\n-        options:\n-          max: 3\n-          skip: [a, b]\n-        severity: error\n-        rung: 1\n-        run_point: commit\n"
	if got := diff(body, out); got != want {
		t.Errorf("diff\n%s\nwant\n%s", got, want)
	}
	tx, err := newText("f.yaml", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Replace(rec, "max: 3", "max: 4", 1)
	err = tx.apply(model.Change{Entity: "rule:go.errors.1", Field: "bindings{}", Before: other})
	if err == nil || !strings.Contains(err.Error(), "does not hold") || string(tx.bytes()) != body {
		t.Errorf("a record with other options: err = %v, text changed = %v", err, string(tx.bytes()) != body)
	}
}
