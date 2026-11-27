package journeys

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The owner's gate is make rules (gragas check) and make gen-check (gragas
// gen --check). What
// they depend on is the shape of rulebook/rules.json, bindings.json and
// coverage.json: the keys below, as literals (tests.admission.4), never read
// back from the code that writes them. A key that goes, is renamed or is
// added turns this red, so the move is seen and decided by a person. A new
// optional key is added here in the same commit that adds it to the writer.
//
// Which transcript each later move task must re-run (arch.structure.8):
//   - a move of the check path (reading, schemas, cross checks, baselines):
//     TestTheOwnersTranscripts for check, check --json and not-a-repository,
//     TestTheGoldensHoldWhatAPersonReads and TestJSONIsOneDocumentWithASchemaNumber.
//   - a move of the gen path (renderers, layers, depguard, stale files):
//     TestTheOwnersTranscripts for gen, gen --check, gen --apply and every
//     --json document of them, TestGenWritesOnlyWithApplyRecordsTheRunAndIsSafeTwice,
//     and this file, since the three JSON files are gen's output.
//   - a move of the command tree or the flags: TestEveryCommandAndFlagHasATranscript
//     and every help transcript.
//   - a move of anything that prints: all of them, at 80, 60 and 40 columns.
//
// A transcript is rewritten only by a person who reads the diff:
//
//	go test ./internal/journeys -run 'TestTheOwnersTranscripts|TestTheGoldens|TestJSONIs' -update
//
// No make target, hook or workflow passes -update (TestNoGateRewritesAGolden).

// shape is what one JSON file must hold: its top-level keys, and for each
// list or map of records the keys every record has and the ones some have.
type shape struct {
	file   string
	top    []string
	groups []group
}

// group is a set of records inside a file.
type group struct {
	name   string
	pick   func(doc map[string]any) []map[string]any
	always []string
	some   []string
}

func records(v any) []map[string]any {
	var out []map[string]any
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
	case map[string]any:
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

// bindingRows are the rows under bindings.json's "bindings": each key maps
// to a list of rows.
func bindingRows(doc map[string]any) []map[string]any {
	byRule, _ := doc["bindings"].(map[string]any)
	var out []map[string]any
	for _, v := range byRule {
		out = append(out, records(v)...)
	}
	return out
}

func field(name string) func(map[string]any) []map[string]any {
	return func(doc map[string]any) []map[string]any { return records(doc[name]) }
}

func coverageCounts() []string {
	return []string{"rules", "enforced", "planned", "advisory", "later", "retired", "proposed", "unsourced", "checks", "unresolved", "external_equal", "external_stricter"}
}

// shapes are the three files as the owner's rulebook has them today
// (rules.json in schema 3, which gen writes; the other two as the old gen wrote them).
func shapes() []shape {
	return []shape{
		{
			file: "rules.json",
			top:  []string{"generated_by", "kinds", "parts", "rules", "schema", "sources"},
			groups: []group{
				{"parts", field("parts"),
					[]string{"slug", "layer", "name", "concerns"},
					[]string{"aliases", "owners"}},
				{"kinds", field("kinds"),
					[]string{"slug", "name"},
					[]string{"description"}},
				{"rules", field("rules"),
					[]string{"id", "part", "concern", "statement", "strength", "status", "origin"},
					[]string{"aliases", "owner", "review_by", "zero_by", "scope", "exemptions", "reasoning", "relations", "sources", "bindings", "pairs", "replaced_by", "standing"}},
				{"sources", field("sources"),
					[]string{"slug", "kind", "title", "locator"},
					[]string{"attribution", "verified"}},
			},
		},
		{
			file: "bindings.json",
			top:  []string{"bindings", "generated_by"},
			groups: []group{
				{"binding rows", bindingRows,
					[]string{"binding", "class", "tool", "key", "tier", "fix"}, nil},
			},
		},
		{
			file: "coverage.json",
			top:  []string{"generated_by", "parts", "total"},
			groups: []group{
				{"parts", field("parts"), coverageCounts(), nil},
				{"total", func(doc map[string]any) []map[string]any {
					t, _ := doc["total"].(map[string]any)
					return []map[string]any{t}
				}, coverageCounts(), nil},
			},
		},
	}
}

// problems is every way data differs from s, in a stable order, or none.
func problems(s shape, data []byte) []string {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return []string{s.file + " is not a JSON object: " + err.Error()}
	}
	var out []string
	got := make([]string, 0, len(doc))
	for k := range doc {
		got = append(got, k)
	}
	sort.Strings(got)
	if !slices.Equal(got, s.top) {
		out = append(out, fmt.Sprintf("%s: top-level keys are %v, want %v", s.file, got, s.top))
	}
	if g, _ := doc["generated_by"].(string); g != "gragas gen" {
		out = append(out, fmt.Sprintf("%s: generated_by is %q, want %q", s.file, doc["generated_by"], "gragas gen"))
	}
	for _, g := range s.groups {
		recs := g.pick(doc)
		if len(recs) == 0 {
			out = append(out, fmt.Sprintf("%s: %s holds no records, so nothing was checked", s.file, g.name))
			continue
		}
		allowed := append(slices.Clone(g.always), g.some...)
		missing, unknown := map[string]bool{}, map[string]bool{}
		for _, r := range recs {
			for _, k := range g.always {
				if _, ok := r[k]; !ok {
					missing[k] = true
				}
			}
			for k := range r {
				if !slices.Contains(allowed, k) {
					unknown[k] = true
				}
			}
		}
		for _, k := range sortedKeys(missing) {
			out = append(out, fmt.Sprintf("%s: a record of %s has no %q", s.file, g.name, k))
		}
		for _, k := range sortedKeys(unknown) {
			out = append(out, fmt.Sprintf("%s: a record of %s has the key %q, which the contract does not know", s.file, g.name, k))
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func readShape(t *testing.T, s shape) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot(), "rulebook", s.file))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestTheGeneratedJSONHoldsTheKeysTheGateReads: the owner's own three files
// pass. Risk: gen drifting a key make rules depends on.
func TestTheGeneratedJSONHoldsTheKeysTheGateReads(t *testing.T) {
	t.Parallel()
	if len(shapes()) != 3 {
		t.Fatalf("%d files in the contract, want rules.json, bindings.json and coverage.json", len(shapes()))
	}
	for _, s := range shapes() {
		t.Run(s.file, func(t *testing.T) {
			if p := problems(s, readShape(t, s)); len(p) != 0 {
				t.Errorf("%s", strings.Join(p, "\n"))
			}
		})
	}
}

// TestARenamedOrLostKeyTurnsTheContractRed: a planted rename of a key in
// each file is named in the problems, a clean control passes, and a file that
// is not an object or holds no records is refused (gates.plants.1).
func TestARenamedOrLostKeyTurnsTheContractRed(t *testing.T) {
	t.Parallel()
	renames := map[string]struct {
		pick     func(doc map[string]any) []map[string]any
		from, to string
		want     string
	}{
		"rules.json":    {field("rules"), "statement", "text", `has no "statement"`},
		"bindings.json": {bindingRows, "tool", "program", `has no "tool"`},
		"coverage.json": {field("parts"), "enforced", "enforcing", `has no "enforced"`},
	}
	for _, s := range shapes() {
		rn, ok := renames[s.file]
		if !ok {
			t.Fatalf("no planted rename for %s", s.file)
		}
		t.Run(s.file, func(t *testing.T) {
			data := readShape(t, s)
			if p := problems(s, data); len(p) != 0 {
				t.Fatalf("the clean control is red:\n%s", strings.Join(p, "\n"))
			}
			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			recs := rn.pick(doc)
			if len(recs) == 0 {
				t.Fatal("no record to plant the rename in")
			}
			recs[0][rn.to] = recs[0][rn.from]
			delete(recs[0], rn.from)
			planted, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			p := strings.Join(problems(s, planted), "\n")
			if !strings.Contains(p, rn.want) || !strings.Contains(p, `"`+rn.to+`"`) {
				t.Errorf("the planted rename of %s to %s was not named:\n%s", rn.from, rn.to, p)
			}
			// A top-level key lost.
			var top map[string]any
			_ = json.Unmarshal(data, &top)
			delete(top, s.top[len(s.top)-1])
			lost, _ := json.Marshal(top)
			if len(problems(s, lost)) == 0 {
				t.Error("a lost top-level key passed")
			}
		})
	}
	s := shapes()[0]
	for name, data := range map[string]string{
		"not JSON":       "nope",
		"an empty file":  "",
		"an array":       "[]",
		"no records":     `{"generated_by":"gragas gen","kinds":[],"parts":[],"rules":[],"schema":2,"sources":{}}`,
		"wrong producer": `{"generated_by":"someone","kinds":[],"parts":[],"rules":[],"schema":2,"sources":{}}`,
	} {
		if len(problems(s, []byte(data))) == 0 {
			t.Errorf("%s passed the contract", name)
		}
	}
}
