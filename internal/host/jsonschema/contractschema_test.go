package jsonschema_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The contract schemas of v1 (M1-16, docs/architecture.md section 7.3): one
// per emitted document and one per entity, each field with the contract
// version it came in and its default. They are read where they stand beside
// host/assets, which embeds them, and compiled through the same adapter the
// format schemas are.

// contractRoot is the folder of the Assets port that holds the contract schemas.
const contractRoot = "contract/schema"

// contractExamples is the folder of the contract's goldens: for each document,
// <name>.valid.json and <name>.invalid.json, the same form as the format's.
const contractExamples = "../../testdata/contract-v1"

// contractDocuments are the emitted documents, the schema of each.
func contractDocuments() map[string]string {
	out := map[string]string{}
	for _, n := range []string{"bundle", "report", "changelog", "sarif", "plan", "proposal", "grid"} {
		out[n] = "v1/document-" + n + ".schema.json"
	}
	return out
}

func contractSchema(t *testing.T) port.Schema {
	t.Helper()
	s, err := jsonschema.FromAssets(disk{}, contractRoot)
	if err != nil {
		t.Fatalf("the contract schemas do not compile: %v", err)
	}
	return s
}

func contractNames(t *testing.T) []string {
	t.Helper()
	l, err := disk{}.List(contractRoot)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range l {
		if strings.HasSuffix(p, ".json") {
			out = append(out, strings.TrimPrefix(p, contractRoot+"/"))
		}
	}
	return out
}

func loadContractSchemas(t *testing.T) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, n := range contractNames(t) {
		m, _ := readJSON(t, path.Join(assetsDir, contractRoot, n)).(map[string]any)
		out[n] = m
	}
	return out
}

func docNames() []string {
	var out []string
	for k := range contractDocuments() {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Each schema compiles, each valid example has no problem and each planted
// invalid document of each schema has the problem its case names.
func TestTheContractSchemasHoldTheirExamples(t *testing.T) {
	t.Parallel()
	checkExamplesIn(t, contractSchema(t), contractExamples, docNames(), contractDocuments())
}

// Every document names the contract version in one top-level field.
func TestEveryContractDocumentNamesTheContractVersionAtItsTop(t *testing.T) {
	t.Parallel()
	all := loadContractSchemas(t)
	for _, name := range docNames() {
		sch := all[contractDocuments()[name]]
		props, _ := sch["properties"].(map[string]any)
		if _, ok := props["contract"]; !ok {
			t.Errorf("%s has no top-level contract field", name)
		}
		req, _ := sch["required"].([]any)
		if !slices.Contains(req, any("contract")) {
			t.Errorf("%s does not require its contract field", name)
		}
	}
	for _, n := range contractNames(t) {
		if strings.HasPrefix(path.Base(n), "document-") != (contractDocuments()[strings.TrimSuffix(strings.TrimPrefix(path.Base(n), "document-"), ".schema.json")] != "") {
			t.Errorf("%s is named like neither a document nor an entity", n)
		}
	}
}

var introduced = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// problemsOfMetadata walks every property of a schema and lists those that
// lack the contract's version metadata: the version it was introduced in, a
// default when it may be absent, a default inside its enum, and the window of
// a deprecation.
func problemsOfMetadata(name string, node any, at string) []string {
	var out []string
	switch n := node.(type) {
	case map[string]any:
		if props, ok := n["properties"].(map[string]any); ok {
			req, _ := n["required"].([]any)
			keys := make([]string, 0, len(props))
			for k := range props {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p, _ := props[k].(map[string]any)
				where := name + ": " + at + "/" + k
				if v, _ := p["x-introduced"].(string); !introduced.MatchString(v) {
					out = append(out, where+" has no introduced version")
				}
				if _, has := p["default"]; !has && !slices.Contains(req, any(k)) {
					out = append(out, where+" may be absent and has no default")
				}
				if e, ok := p["enum"].([]any); ok {
					if d, has := p["default"]; has && !slices.Contains(e, d) {
						out = append(out, where+" has a default outside its enum")
					}
				}
				if dep, ok := p["x-deprecated"].(map[string]any); ok {
					if dep["since"] == nil || dep["removed_in"] == nil {
						out = append(out, where+" is deprecated with no window")
					}
				}
			}
		}
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if m, ok := n[k].(map[string]any); ok && (k == "properties" || k == "$defs") {
				// the values are schemas; a field named properties is one of them
				for _, f := range sortedKeys(m) {
					out = append(out, problemsOfMetadata(name, m[f], at+"/"+k+"/"+f)...)
				}
				continue
			}
			out = append(out, problemsOfMetadata(name, n[k], at+"/"+k)...)
		}
	case []any:
		for i, v := range n {
			out = append(out, problemsOfMetadata(name, v, at+"/"+string(rune('0'+i%10)))...)
		}
	}
	return out
}

func TestEveryContractFieldHasAnIntroducedVersionAndADefault(t *testing.T) {
	t.Parallel()
	for n, sch := range loadContractSchemas(t) {
		if bad := problemsOfMetadata(n, sch, ""); len(bad) > 0 {
			t.Errorf("%s", strings.Join(bad, "\n"))
		}
	}
}

// The check can fail: a field without a version, one that may be absent with
// no default, an enum default outside the enum and a deprecation with no
// window are each told.
func TestTheMetadataCheckFailsOnAPlantedField(t *testing.T) {
	t.Parallel()
	var sch any
	planted := `{"required": ["a"], "properties": {
		"a": {"type": "string"},
		"b": {"type": "string", "x-introduced": "1.0"},
		"e": {"enum": ["x"], "x-introduced": "1.0", "default": "y"},
		"d": {"type": "string", "x-introduced": "1.0", "default": "", "x-deprecated": {"since": "1.1"}}}}`
	if err := json.Unmarshal([]byte(planted), &sch); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(problemsOfMetadata("planted", sch, ""), "\n")
	for _, want := range []string{"/a has no introduced version", "/b may be absent and has no default", "/e has a default outside its enum", "/d is deprecated with no window"} {
		if !strings.Contains(got, want) {
			t.Errorf("the planted field was not told: want %q in\n%s", want, got)
		}
	}
}

// The contract holds no project identifier (S48): no field is named for a
// project, a path, a remote or an author.
func TestNoContractFieldNamesAProject(t *testing.T) {
	t.Parallel()
	banned := []string{"project", "project_name", "remote", "author", "repository", "repo", "hostname", "user"}
	var walk func(name string, node any)
	walk = func(name string, node any) {
		switch n := node.(type) {
		case map[string]any:
			if props, ok := n["properties"].(map[string]any); ok {
				for k := range props {
					if slices.Contains(banned, k) {
						t.Errorf("%s has a field named %q", name, k)
					}
				}
			}
			for _, v := range n {
				walk(name, v)
			}
		case []any:
			for _, v := range n {
				walk(name, v)
			}
		}
	}
	for n, sch := range loadContractSchemas(t) {
		walk(n, sch)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// A defaultCase is a property that has a default: where it stands and the
// schema its default has to satisfy.
type defaultCase struct {
	where  string
	file   string
	prop   map[string]any
	defval any
}

// collectDefaults lists every property under node that has a default.
func collectDefaults(file string, node any, at string, out *[]defaultCase) {
	switch n := node.(type) {
	case map[string]any:
		if props, ok := n["properties"].(map[string]any); ok {
			for _, k := range sortedKeys(props) {
				p, _ := props[k].(map[string]any)
				if d, has := p["default"]; has {
					*out = append(*out, defaultCase{where: file + ": " + at + "/" + k, file: file, prop: p, defval: d})
				}
			}
		}
		for _, k := range sortedKeys(n) {
			if m, ok := n[k].(map[string]any); ok && (k == "properties" || k == "$defs") {
				for _, f := range sortedKeys(m) {
					collectDefaults(file, m[f], at+"/"+k+"/"+f, out)
				}
				continue
			}
			collectDefaults(file, n[k], at+"/"+k, out)
		}
	case []any:
		for _, v := range n {
			collectDefaults(file, v, at, out)
		}
	}
}

// localRefs points every "#/..." reference under node at file, so a copy of
// a property compiles as a schema of its own beside the others.
func localRefs(node any, file string) any {
	switch n := node.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range n {
			if s, ok := v.(string); ok && k == "$ref" && strings.HasPrefix(s, "#") {
				out[k] = path.Base(file) + s
				continue
			}
			out[k] = localRefs(v, file)
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, v := range n {
			out[i] = localRefs(v, file)
		}
		return out
	}
	return node
}

// defaultProblems validates each default against its own field's schema, with
// the Schema port, through $ref, so a pattern, a minimum length, a format and
// a type all count: a default a reader fills in must be a value the schema
// takes.
func defaultProblems(files map[string]map[string]any) []string {
	var cases []defaultCase
	var all []model.File
	for _, name := range slices.Sorted(maps.Keys(files)) {
		collectDefaults(name, files[name], "", &cases)
		b, _ := json.Marshal(files[name])
		all = append(all, model.File{Path: name, Data: b})
	}
	probe := func(i int) string { return fmt.Sprintf("probe-%d.schema.json", i) }
	for i, c := range cases {
		s, _ := localRefs(c.prop, c.file).(map[string]any)
		s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		s["$id"] = probe(i)
		delete(s, "default")
		b, _ := json.Marshal(s)
		all = append(all, model.File{Path: path.Join(path.Dir(c.file), probe(i)), Data: b})
	}
	sch, err := jsonschema.New(all)
	if err != nil {
		return []string{"the schemas and their probes do not compile: " + err.Error()}
	}
	var out []string
	for i, c := range cases {
		doc, _ := json.Marshal(c.defval)
		probs, err := sch.Check(path.Join(path.Dir(c.file), probe(i)), doc)
		if err != nil {
			out = append(out, c.where+": "+err.Error())
			continue
		}
		if len(probs) > 0 {
			out = append(out, c.where+" has the default "+string(doc)+", which its own schema refuses: "+probs[0].Msg)
		}
	}
	return out
}

// Every default holds against its field's schema, through $ref: {} is not a
// standing and "" is not an id.
func TestEveryContractDefaultHoldsAgainstItsFieldsSchema(t *testing.T) {
	t.Parallel()
	if bad := defaultProblems(loadContractSchemas(t)); len(bad) > 0 {
		t.Errorf("%d defaults the schema refuses:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// The check sees the defects it was written for: an object where an enum is
// meant, an empty string where a shape has a minimum length or a pattern, and
// takes a null where an anyOf allows it.
func TestTheDefaultCheckSeesWhatASchemaRefuses(t *testing.T) {
	t.Parallel()
	shapes := map[string]any{"$id": "c.json", "$defs": map[string]any{
		"s": map[string]any{"enum": []any{"a"}},
		"t": map[string]any{"type": "string", "minLength": float64(1)},
		"i": map[string]any{"type": "string", "pattern": "^[a-z]+$"},
	}}
	field := func(prop map[string]any) map[string]map[string]any {
		return map[string]map[string]any{"v1/c.json": shapes,
			"v1/x.json": {"properties": map[string]any{"f": prop}}}
	}
	anyOfNull := func(ref string) map[string]any {
		return map[string]any{"anyOf": []any{map[string]any{"$ref": "c.json#/$defs/" + ref}, map[string]any{"type": "null"}}}
	}
	for _, tc := range []struct {
		name string
		prop map[string]any
		bad  bool
	}{
		{"an object for an enum", map[string]any{"$ref": "c.json#/$defs/s", "default": map[string]any{}}, true},
		{"an empty text", map[string]any{"$ref": "c.json#/$defs/t", "default": ""}, true},
		{"an empty id", map[string]any{"$ref": "c.json#/$defs/i", "default": ""}, true},
		{"a null for a nullable id", with(anyOfNull("i"), "default", nil), false},
		{"an empty id for a nullable id", with(anyOfNull("i"), "default", ""), true},
		{"a value of the enum", map[string]any{"$ref": "c.json#/$defs/s", "default": "a"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := defaultProblems(field(tc.prop))
			if (len(got) > 0) != tc.bad {
				t.Errorf("want a problem: %v, got %v", tc.bad, got)
			}
		})
	}
}

// with is m with k set to v.
func with(m map[string]any, k string, v any) map[string]any {
	m[k] = v
	return m
}

// TestTheParentStandInHoldsTheBundleSchema: the parent's bundle that the rule
// exclude and include tests seed into the releases cache is a document of the
// bundle schema, and the same file with one rule's standing taken away is not
// (tests.doubles.6). Risk: a stand-in that only a fake schema accepts, so a
// test passes that the program would refuse.
func TestTheParentStandInHoldsTheBundleSchema(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../core/api/testdata/parent/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	s, name := contractSchema(t), contractDocuments()["bundle"]
	if got, err := s.Check(name, data); err != nil || len(got) > 0 {
		t.Fatalf("the parent's bundle: %v, %v; want it to hold the bundle schema", got, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc["rules"].([]any)[0].(map[string]any), "standing")
	planted, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Check(name, planted); err != nil || len(got) == 0 {
		t.Errorf("a rule with no standing: %v, %v; want the schema to refuse it", got, err)
	}
}

// contractFixture is the fixture's hand-authored documents, each with
// the document it is. The real schemas read them here: the contract package's
// own tests run a fake schema port.
const contractFixture = "../../../contract/fixture"

func contractFixtureDocuments() map[string]string {
	return map[string]string{
		"bundle.json":          "bundle",
		"changelog.json":       "changelog",
		"report-clean.json":    "report",
		"report-weakened.json": "report",
		"plan.json":            "plan",
		"sarif.json":           "sarif",
		"proposal.json":        "proposal",
		"grid.json":            "grid",
	}
}

// Every document of the fixture has no problem under the real schema of its
// kind.
func TestEveryContractFixtureDocumentHoldsItsRealSchema(t *testing.T) {
	t.Parallel()
	s := contractSchema(t)
	schemas := contractDocuments()
	for file, doc := range contractFixtureDocuments() {
		b, err := os.ReadFile(path.Join(contractFixture, file))
		if err != nil {
			t.Fatal(err)
		}
		problems, err := s.Check(schemas[doc], b)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, p := range problems {
			t.Errorf("%s: %+v", file, p)
		}
	}
}

// enumsWithoutAType lists, in order, each enum of the schema node v that carries no type, by its path in the
// document: a generic JSON Schema reader (zod 4, go-jsonschema) drops an untyped enum's neighbours and trips on
// it. An enum that holds null is typed [string, null].
func enumsWithoutAType(v any, at string) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		if _, has := x["enum"]; has && x["type"] == nil {
			out = append(out, at)
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = append(out, enumsWithoutAType(x[k], at+"/"+k)...)
		}
	case []any:
		for i, e := range x {
			out = append(out, enumsWithoutAType(e, fmt.Sprintf("%s/%d", at, i))...)
		}
	}
	return out
}

// TestEveryEnumOfTheContractSchemasHasAType: every enum in every contract schema carries its
// type, "string" or, where the enum holds null, ["string", "null"]. No document that was valid stops being
// valid, which the conformance vectors and the fixture hold; the type only says what the enum already was.
// Risk: an untyped enum makes a generic reader drop its pattern, format or bounds, and the generated types
// and zod of the explorer lose what it said.
func TestEveryEnumOfTheContractSchemasHasAType(t *testing.T) {
	t.Parallel()
	schemas := loadContractSchemas(t)
	if len(schemas) == 0 {
		t.Fatal("no contract schema was read")
	}
	for _, name := range slices.Sorted(maps.Keys(schemas)) {
		for _, at := range enumsWithoutAType(schemas[name], "") {
			t.Errorf("%s%s: an enum with no type", name, at)
		}
	}
	t.Run("an untyped enum is seen", func(t *testing.T) {
		t.Parallel()
		doc := map[string]any{"properties": map[string]any{
			"a": map[string]any{"enum": []any{"x"}},
			"b": map[string]any{"type": "string", "enum": []any{"x"}},
		}}
		if got := enumsWithoutAType(doc, ""); !slices.Equal(got, []string{"/properties/a"}) {
			t.Errorf("found %v, want only /properties/a", got)
		}
	})
}

// TestAnEnumThatHoldsNullIsTypedWithNull: an enum with null among its values is typed ["string", "null"], not
// "string" alone, or the null the schema allows would be refused by the type that is meant to describe it.
func TestAnEnumThatHoldsNullIsTypedWithNull(t *testing.T) {
	t.Parallel()
	var walk func(v any, at string)
	walk = func(v any, at string) {
		switch x := v.(type) {
		case map[string]any:
			if vals, ok := x["enum"].([]any); ok && slices.Contains(vals, nil) {
				if ty, _ := x["type"].([]any); len(ty) != 2 || ty[0] != "string" || ty[1] != "null" {
					t.Errorf("%s: an enum that holds null has the type %v, want [string null]", at, x["type"])
				}
			}
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k], at+"/"+k)
			}
		case []any:
			for i, e := range x {
				walk(e, fmt.Sprintf("%s/%d", at, i))
			}
		}
	}
	for name, doc := range loadContractSchemas(t) {
		walk(doc, name)
	}
}
