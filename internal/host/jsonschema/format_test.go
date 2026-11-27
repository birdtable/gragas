package jsonschema_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/port"
)

// The format schemas of v3 (M1-01, docs/architecture.md section 7.1) against
// their hand-written examples, through the real adapter. The schemas are read
// where they stand beside host/assets, which embeds them; cmd/gragas holds the
// embedded copy to these files and to the valid examples.

// assetsDir is the folder host/assets embeds from, from this package's folder.
const assetsDir = "../assets"

// schemaRoot is the folder of the Assets port that holds the format schemas.
const schemaRoot = "format/schema"

// examples is the folder of the goldens: for each file kind, <kind>.valid.json
// (hand-authored valid documents) and <kind>.invalid.json (planted breaks,
// each a patch of the valid documents and the problem it must raise).
const examples = "../../testdata/format-v3"

// disk is an Assets port over the folder host/assets embeds.
type disk struct{}

func (disk) Read(p string) ([]byte, error) { return os.ReadFile(path.Join(assetsDir, p)) }

func (disk) List(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(path.Join(assetsDir, dir), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, strings.TrimPrefix(filepath.ToSlash(p), assetsDir+"/"))
		}
		return err
	})
	sort.Strings(out)
	return out, err
}

// fileKinds are the file kinds of section 7.1 and the adapter of section 4.1, and the schema of each.
func fileKinds() map[string]string {
	return map[string]string{
		"checker":      "v3/checker.schema.json",
		"identity":     "v3/identity.schema.json",
		"structure":    "v3/structure.schema.json",
		"terms":        "v3/terms.schema.json",
		"rules":        "v3/rules.schema.json",
		"block":        "v3/block.schema.json",
		"sources":      "v3/sources.schema.json",
		"decisions":    "v3/decisions.schema.json",
		"exclusions":   "v3/exclusions.schema.json",
		"proposals":    "v3/proposals.schema.json",
		"translations": "v3/translations.schema.json",
		"history":      "v3/history.schema.json",
	}
}

func kindNames() []string {
	var out []string
	for k := range fileKinds() {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func readJSON(t *testing.T, p string) any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return v
}

func formatSchema(t *testing.T) port.Schema {
	t.Helper()
	s, err := jsonschema.FromAssets(disk{}, schemaRoot)
	if err != nil {
		t.Fatalf("the format schemas do not compile: %v", err)
	}
	return s
}

// docs is the documents of an example file: a JSON array.
func docs(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		t.Fatalf("an example file holds a non-empty array, got %T", v)
	}
	return l
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// problemsOf checks every document of l against the schema and answers the
// problems' text.
func problemsOf(t *testing.T, s port.Schema, schema string, l []any) []string {
	t.Helper()
	var out []string
	for i, d := range l {
		ps, err := s.Check(schema, encode(t, d))
		if err != nil {
			t.Fatalf("Check(%s) document %d: %v", schema, i, err)
		}
		for _, p := range ps {
			out = append(out, p.Msg)
		}
	}
	return out
}

// patchOp is one step of a patch, the add, remove and replace of RFC 6902.
type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// apply changes doc by op and answers the result; doc is not changed.
func apply(doc any, op patchOp) (any, error) {
	copyOf := func(v any) any {
		b, _ := json.Marshal(v)
		var out any
		_ = json.Unmarshal(b, &out)
		return out
	}
	doc = copyOf(doc)
	steps := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
	var walk func(node any, steps []string) (any, error)
	walk = func(node any, steps []string) (any, error) {
		step := steps[0]
		last := len(steps) == 1
		switch n := node.(type) {
		case map[string]any:
			if last {
				_, there := n[step]
				switch op.Op {
				case "add":
					n[step] = op.Value
				case "replace":
					if !there {
						return nil, fmt.Errorf("replace %s: no such field", op.Path)
					}
					n[step] = op.Value
				case "remove":
					if !there {
						return nil, fmt.Errorf("remove %s: no such field", op.Path)
					}
					delete(n, step)
				default:
					return nil, fmt.Errorf("op %q is not add, remove or replace", op.Op)
				}
				return n, nil
			}
			child, ok := n[step]
			if !ok {
				return nil, fmt.Errorf("%s: no field %q", op.Path, step)
			}
			r, err := walk(child, steps[1:])
			n[step] = r
			return n, err
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i < 0 || i >= len(n) {
				return nil, fmt.Errorf("%s: no item %q", op.Path, step)
			}
			if last {
				if op.Op != "remove" && op.Op != "replace" {
					return nil, fmt.Errorf("op %q on an item", op.Op)
				}
				if op.Op == "remove" {
					return slices.Delete(n, i, i+1), nil
				}
				n[i] = op.Value
				return n, nil
			}
			r, err := walk(n[i], steps[1:])
			n[i] = r
			return n, err
		}
		return nil, fmt.Errorf("%s: %q is inside a value that has no fields", op.Path, step)
	}
	return walk(doc, steps)
}

// invalidCase is one planted break in an <kind>.invalid.json.
type invalidCase struct {
	Name    string    `json:"name"`
	Patch   []patchOp `json:"patch"`
	Problem string    `json:"problem"`
}

func loadCasesIn(t *testing.T, dir, kind string) []invalidCase {
	t.Helper()
	b, err := os.ReadFile(path.Join(dir, kind+".invalid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out []invalidCase
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s.invalid.json: %v", kind, err)
	}
	return out
}

// checkExamples runs the examples of every kind against s through the Schema
// port: each valid document has no problem and each planted break has the
// problem its case names.
func checkExamples(t *testing.T, s port.Schema) {
	t.Helper()
	checkExamplesIn(t, s, examples, kindNames(), fileKinds())
}

// checkExamplesIn is checkExamples over the kinds named, each with its schema,
// whose example files sit in dir.
func checkExamplesIn(t *testing.T, s port.Schema, dir string, kinds []string, schemas map[string]string) {
	t.Helper()
	for _, kind := range kinds {
		schema := schemas[kind]
		valid := docs(t, readJSON(t, path.Join(dir, kind+".valid.json")))
		t.Run(kind+" valid", func(t *testing.T) {
			t.Parallel()
			if got := problemsOf(t, s, schema, valid); len(got) > 0 {
				t.Errorf("a valid %s example has problems: %v", kind, got)
			}
		})
		cases := loadCasesIn(t, dir, kind)
		if len(cases) < 3 {
			t.Errorf("%s has %d planted cases, want an unknown field, a missing field and a wrong enum at least", kind, len(cases))
		}
		for _, c := range cases {
			t.Run(kind+" "+c.Name, func(t *testing.T) {
				t.Parallel()
				var doc any = valid
				for _, op := range c.Patch {
					var err error
					if doc, err = apply(doc, op); err != nil {
						t.Fatal(err)
					}
				}
				got := problemsOf(t, s, schema, doc.([]any))
				if len(got) == 0 {
					t.Fatalf("the planted case was accepted")
				}
				if !slices.ContainsFunc(got, func(m string) bool { return strings.Contains(m, c.Problem) }) {
					t.Errorf("problems %q do not hold %q", got, c.Problem)
				}
			})
		}
	}
}

func TestTheFormatSchemasHoldTheirExamples(t *testing.T) {
	t.Parallel()
	checkExamples(t, formatSchema(t))
}

// TestEveryFormatFieldIsInItsValidExample: each field a file kind's schema
// declares is a key of that kind's valid example, so no field of the book
// stands untried by the examples that are to hold it; the sibling of
// TestEveryPluginFieldIsInAValidExample. The fields of
// common.schema.json are each kind's own where it refers to them.
func TestEveryFormatFieldIsInItsValidExample(t *testing.T) {
	t.Parallel()
	for _, kind := range kindNames() {
		declared := map[string]bool{}
		fieldNames(readJSON(t, path.Join(assetsDir, "format/schema", fileKinds()[kind])), declared)
		used := map[string]bool{}
		keysOf(readJSON(t, path.Join(examples, kind+".valid.json")), used)
		for _, f := range slices.Sorted(maps.Keys(declared)) {
			if !used[f] {
				t.Errorf("%s: no valid example has the field %s", kind, f)
			}
		}
	}
}

// The harness itself can fail: a planted case that changes nothing is
// accepted by the schema, and a patch of a missing field is an error.
func TestTheExampleHarnessFailsOnAPlantThatBreaksNothingAndOnABadPatch(t *testing.T) {
	t.Parallel()
	s := formatSchema(t)
	valid := docs(t, readJSON(t, path.Join(examples, "identity.valid.json")))
	if got := problemsOf(t, s, fileKinds()["identity"], valid); len(got) != 0 {
		t.Fatalf("setup: %v", got)
	}
	harmless, err := apply(valid, patchOp{Op: "add", Path: "/0/owners/0", Value: "alex"})
	if err == nil {
		_ = harmless
		t.Error("a patch that adds inside a list was taken")
	}
	if _, err := apply(valid, patchOp{Op: "remove", Path: "/0/nothing"}); err == nil {
		t.Error("a patch of a missing field was taken")
	}
	if _, err := apply(valid, patchOp{Op: "move", Path: "/0/name"}); err == nil {
		t.Error("an op other than add, remove and replace was taken")
	}
	same, err := apply(valid, patchOp{Op: "replace", Path: "/0/name", Value: "house"})
	if err != nil {
		t.Fatal(err)
	}
	if got := problemsOf(t, s, fileKinds()["identity"], same.([]any)); len(got) != 0 {
		t.Errorf("a patch that breaks nothing raised %v, so a case's problem would be vacuous", got)
	}
}
