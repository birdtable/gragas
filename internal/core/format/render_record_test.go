package format

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// recorded is a concern file with the lists of records a person writes in each
// style: a block list with a comment, a flow list, a block list written under
// the key's own column, and a rule with neither list.
const recorded = `format: 3
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
    exemptions:
      - paths: [tools/**]
        reason: generated code
    owner: team-a

  - id: go.errors.2
    statement: Return an error and never panic in a library.
    strength: should
    status: active
    origin: authored
    sources: [{ref: wrap-errors, backs: rule}]   # flow
    exemptions:
    - kinds: [test]
      reason: tests may panic

  - id: go.errors.3
    statement: Log an error once.
    strength: may
    status: proposed
    origin: authored

  - id: go.errors.4
    statement: Wrap with the verb w.
    strength: should
    status: active
    origin: authored
    exemptions:
      -   kinds: [test]
          reason: tests print
`

func addRecord(id, field, after string) model.Delta { return setRule(id, field+"{}", "", after) }

func TestARecordIsAppendedToEachListInEachStyleAndTouchesNothingElse(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, id, field, record, want string }{
		{"a source to a block list", "go.errors.1", "sources", "ref: rfc-9110\nbacks: check",
			"+      - ref: rfc-9110\n+        backs: check\n"},
		{"a source with a place to a block list", "go.errors.1", "sources", "ref: rfc-9110\nwhere: section 4\nbacks: rule",
			"+      - ref: rfc-9110\n+        where: section 4\n+        backs: rule\n"},
		{"a source to a flow list", "go.errors.2", "sources", "ref: rfc-9110\nbacks: rule",
			"-    sources: [{ref: wrap-errors, backs: rule}]   # flow\n+    sources: [{ref: wrap-errors, backs: rule}, {ref: rfc-9110, backs: rule}]   # flow\n"},
		{"a source to a rule with no list", "go.errors.3", "sources", "ref: rfc-9110\nbacks: rule",
			"+    sources:\n+      - ref: rfc-9110\n+        backs: rule\n"},
		{"an exemption to a block list", "go.errors.1", "exemptions", "paths: [vendor/**]\nreason: not ours",
			"+      - paths: [\"vendor/**\"]\n+        reason: not ours\n"},
		{"an exemption to a list at the key's own column", "go.errors.2", "exemptions", "kinds: [bench]\nreason: benchmarks may panic",
			"+    - kinds: [bench]\n+      reason: benchmarks may panic\n"},
		{"an exemption to a list whose keys stand wide of the dash", "go.errors.4", "exemptions", "kinds: [bench]\nreason: benchmarks print",
			"+      -   kinds: [bench]\n+          reason: benchmarks print\n"},
		{"a binding whose boolean and number stay bare", "go.errors.3", "bindings", "tool: lint-a\ntarget: config\nkey: wrap\nvalue: true\nseverity: error\nrung: 1\nrun_point: commit",
			"+    bindings:\n+      - tool: lint-a\n+        target: config\n+        key: wrap\n+        value: true\n+        severity: error\n+        rung: 1\n+        run_point: commit\n"},
		{"a binding with options, written as a mapping under its key", "go.errors.3", "bindings", "tool: lint-a\ntarget: config\nkey: wrap\nvalue: true\noptions:\n  max: 3\n  skip: [a, b]\nseverity: error\nrung: 1\nrun_point: commit",
			"+    bindings:\n+      - tool: lint-a\n+        target: config\n+        key: wrap\n+        value: true\n+        options:\n+          max: 3\n+          skip: [a, b]\n+        severity: error\n+        rung: 1\n+        run_point: commit\n"},
		{"an exemption with a list written in block style", "go.errors.3", "exemptions", "paths:\n  - a/**\n  - b/**\nreason: both are vendored",
			"+    exemptions:\n+      - paths: [\"a/**\", \"b/**\"]\n+        reason: both are vendored\n"},
	}
	for _, c := range cases {
		out := one(t, recorded, addRecord(c.id, c.field, c.record))
		if got := diff(recorded, out); got != c.want {
			t.Errorf("%s: diff\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

func TestARecordIsAppendedToAListInAGuideBlock(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, id, field, record, want string }{
		{"a source to a rule with none", "go.layout.9", "sources", "ref: rfc-9110\nbacks: rule",
			"+    sources:\n+      - ref: rfc-9110\n+        backs: rule\n"},
		{"an exemption to a rule with none", "go.layout.9", "exemptions", "paths: [x/**]\nreason: scratch",
			"+    exemptions:\n+      - paths: [\"x/**\"]\n+        reason: scratch\n"},
	}
	for _, c := range cases {
		out, err := guideRender(t, proseGuide, addRecord(c.id, c.field, c.record))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := diff(proseGuide, string(out)); got != c.want {
			t.Errorf("%s: diff\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

// refusal is a record the renderer turns away, with the words of the refusal.
// An empty body is the recorded file. Each is a fuzz seed too (testdata/fuzz).
type refusal struct{ name, body, id, field, before, record, says string }

func recordRefusals() []refusal {
	const flow = "rules:\n  - id: a.b.1\n    reasoning: {pros: [x]}\n"
	return []refusal{
		{"a key the schema does not name", "", "go.errors.1", "sources", "", "ref: rfc-9110\nbacks: rule\nnote: x", "invalid"},
		{"a source with no backs", "", "go.errors.3", "sources", "", "ref: rfc-9110", "invalid"},
		{"an exemption with no reason", "", "go.errors.3", "exemptions", "", "paths: [x/**]", "invalid"},
		{"a record already in a block list", "", "go.errors.1", "sources", "", "ref: wrap-errors\nwhere: section 2\nbacks: rule", "already holds"},
		{"a record already in a flow list", "", "go.errors.2", "sources", "", "ref: wrap-errors\nbacks: rule", "already holds"},
		{"a record already in a list at the key's own column", "", "go.errors.2", "exemptions", "", "kinds: [test]\nreason: tests may panic", "already holds"},
		{"a record that is not a mapping", "", "go.errors.1", "sources", "", "- ref: x", "not a mapping"},
		{"a record with no key", "", "go.errors.1", "sources", "", "{}", "not a mapping"},
		{"a record that is not YAML", "", "go.errors.1", "sources", "", "ref: [", "it is not YAML"},
		{"a record added to a field that is not a list", "", "go.errors.1", "owner", "", "ref: a\nbacks: rule", "owner is not a list"},
		{"a record with an empty nested mapping", "", "go.errors.1", "sources", "", "ref: {}\nbacks: rule", "empty mapping"},
		{"a record with an empty value", "", "go.errors.1", "sources", "", "ref: \"\"\nbacks: rule", "not a text"},
		{"a record with an empty list", "", "go.errors.3", "exemptions", "", "paths: []\nreason: none", "holds an empty list"},
		{"a record with an empty item in a list", "", "go.errors.3", "exemptions", "", "paths: [\"\"]\nreason: none", "holds an item that is not a text"},
		{"a record with a mapping in a list", "", "go.errors.3", "exemptions", "", "paths: [{a: b}]\nreason: none", "holds an item that is not a text"},
		{"a record with a before value", "", "go.errors.1", "sources", "x", "ref: a\nbacks: rule", "not a mapping"},
		{"a record in a list inside a flow mapping", flow, "a.b.1", "reasoning.pros", "", "ref: a", "written in flow style"},
	}
}

func TestARecordTheSchemaDoesNotAllowOrTheListHoldsIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range recordRefusals() {
		var err error
		if c.says == "invalid" {
			_, err = renderChecked(t, recorded, addRecord(c.id, c.field, c.record))
		} else {
			body := cmp.Or(c.body, recorded)
			tx, nerr := newText("f.yaml", []byte(body))
			if nerr != nil {
				t.Fatalf("%s: %v", c.name, nerr)
			}
			err = tx.apply(model.Change{Entity: "rule:" + c.id, Field: c.field + "{}", Before: c.before, After: c.record})
			if string(tx.bytes()) != body {
				t.Errorf("%s: a refused record changed the text", c.name)
			}
		}
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v; want one saying %q", c.name, err, c.says)
		}
	}
}

func FuzzRenderRecord(f *testing.F) {
	for _, c := range recordRefusals() {
		f.Add(cmp.Or(c.body, recorded), c.id, c.field, c.before, c.record)
	}
	f.Add(recorded, "go.errors.1", "sources", "", "ref: rfc-9110\nbacks: rule")
	f.Fuzz(func(t *testing.T, body, id, field, before, record string) {
		tx, err := newText("f.yaml", []byte(body))
		if err != nil {
			return
		}
		c := model.Change{Entity: "rule:" + id, Field: field + "{}", Before: before, After: record}
		if err := tx.apply(c); err != nil && string(tx.bytes()) != body {
			t.Fatalf("a refused record changed the text:\n%q\n%q", body, tx.bytes())
		}
	})
}

// holdsRecords is a Schema that holds the records of sources and exemptions to
// the keys the real schema names (rule.schema.json): the double accepts any
// document, so it is asked to refuse these.
type holdsRecords struct{ port.Schema }

func recordKeys() map[string]struct{ allowed, required []string } {
	return map[string]struct{ allowed, required []string }{
		"sources":    {[]string{"ref", "where", "says", "backs"}, []string{"ref", "backs"}},
		"exemptions": {[]string{"paths", "kinds", "reason"}, []string{"reason"}},
	}
}

func (s holdsRecords) Check(name string, doc []byte) ([]model.Problem, error) {
	var d struct {
		Rules []map[string]json.RawMessage `json:"rules"`
	}
	if json.Unmarshal(doc, &d) == nil {
		for _, r := range d.Rules {
			for list, k := range recordKeys() {
				var recs []map[string]json.RawMessage
				_ = json.Unmarshal(r[list], &recs)
				for _, rec := range recs {
					for key := range rec {
						if !slices.Contains(k.allowed, key) {
							return []model.Problem{{Line: 1, Msg: list + " does not name the key " + key}}, nil
						}
					}
					for _, key := range k.required {
						if _, ok := rec[key]; !ok {
							return []model.Problem{{Line: 1, Msg: list + " lacks " + key}}, nil
						}
					}
				}
			}
		}
	}
	return s.Schema.Check(name, doc)
}

func renderChecked(t *testing.T, body string, d model.Delta) ([]model.Target, error) {
	t.Helper()
	w, root, got := renderWorld(t, body)
	p := w.Ports()
	return Render(p.Tree, holdsRecords{p.Schema}, root, got, d)
}
