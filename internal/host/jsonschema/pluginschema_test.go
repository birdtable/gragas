package jsonschema_test

import (
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/jsonschema"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The schemas of plugin contract 1 against their hand-written examples,
// through the real adapter. The two files stand at contract/plugin/v1:
// wire.schema.json, every message a $def of it, and manifest.schema.json, the
// strict schema of plugin.yaml. Nothing embeds them yet (the codec and the
// generated types come later).

// pluginDir is the folder of the plugin schemas, from this package's folder.
const pluginDir = "../../../contract/plugin/v1"

// pluginExamples is the folder of the examples: for each message and the
// manifest, <kind>.valid.json and <kind>.invalid.json, as the format's are.
const pluginExamples = "../../testdata/plugin-v1"

// pluginFiles are the two schema files, each named by its own $id.
func pluginFiles() []string { return []string{"wire.schema.json", "manifest.schema.json"} }

// wireKinds are the messages of wire.schema.json that have examples: the
// envelope of a request and of a response, each operation's request body and
// answer, and an error. The other $defs are the parts these are made of.
func wireKinds() []string {
	out := []string{"request", "response", "error"}
	for _, op := range []string{"plan", "entries", "findings", "rank", "preset"} {
		out = append(out, op+".request", op+".response")
	}
	return out
}

// pluginKinds is each kind with examples and the schema it is checked
// against: a message is a $def of the wire, reached by a one-line schema of
// the test's own that refers to it, since the Schema port checks a document
// against a whole file; the manifest is its own file.
func pluginKinds() map[string]string {
	out := map[string]string{"manifest": "contract/plugin/v1/manifest.schema.json"}
	for _, k := range wireKinds() {
		out[k] = "contract/plugin/v1/check-" + k + ".json"
	}
	return out
}

// pluginSchema is the Schema port over the two files and a check schema for
// each message.
func pluginSchema(t *testing.T) port.Schema {
	t.Helper()
	var files []model.File
	for _, n := range pluginFiles() {
		b, err := os.ReadFile(path.Join(pluginDir, n))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, model.File{Path: "contract/plugin/v1/" + n, Data: b})
	}
	for _, k := range wireKinds() {
		files = append(files, model.File{Path: "contract/plugin/v1/check-" + k + ".json", Data: []byte(`{"$ref": "wire.schema.json#/$defs/` + k + `"}`)})
	}
	s, err := jsonschema.New(files)
	if err != nil {
		t.Fatalf("the plugin schemas do not compile: %v", err)
	}
	return s
}

// pluginSchemaFiles is the two schema files, by name, read as JSON.
func pluginSchemaFiles(t *testing.T) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, n := range pluginFiles() {
		out[n] = readJSON(t, path.Join(pluginDir, n))
	}
	return out
}

// TestThePluginSchemasHoldTheirExamples: each valid example of each message
// and of the manifest has no problem, and each planted break has the problem
// its case names: every schema has valid and
// invalid examples planted, each invalid one red.
func TestThePluginSchemasHoldTheirExamples(t *testing.T) {
	t.Parallel()
	kinds := slices.Sorted(maps.Keys(pluginKinds()))
	checkExamplesIn(t, pluginSchema(t), pluginExamples, kinds, pluginKinds())
}

// pluginEntry is one entry of the plugin folder: a plain file and what it
// holds, or, with notFile, anything else (a folder, a link).
type pluginEntry struct {
	notFile bool
	data    []byte
}

// pluginFolderFaults is what the plugin folder holds that is not its own,
// each fault naming its entry, in the order of the names. The folder holds
// exactly the two schema files: any other file, a Go file and a data file
// above all, a folder or a link is a fault. The read-glob
// matcher that stood here is internal/readglob's.
func pluginFolderFaults(folder map[string]pluginEntry) []string {
	var faults, schemas []string
	for _, name := range slices.Sorted(maps.Keys(folder)) {
		switch e := folder[name]; {
		case e.notFile:
			faults = append(faults, name+" is not a plain file")
		case strings.HasSuffix(name, ".schema.json"):
			schemas = append(schemas, name)
		default:
			faults = append(faults, name+" is not a schema of the contract")
		}
	}
	if want := slices.Sorted(slices.Values(pluginFiles())); !slices.Equal(schemas, want) {
		faults = append(faults, "the schemas are "+strconv.Quote(strings.Join(schemas, " "))+", want "+strconv.Quote(strings.Join(want, " ")))
	}
	return faults
}

// TestThePluginFolderFaultsNameEachStrayEntry: the folder as it stands on
// release-1 has no fault, and each entry that is not the contract's own is
// named, a stray data file first.
func TestThePluginFolderFaultsNameEachStrayEntry(t *testing.T) {
	t.Parallel()
	clean := func() map[string]pluginEntry {
		return map[string]pluginEntry{
			"wire.schema.json":     {data: []byte("{}")},
			"manifest.schema.json": {data: []byte("{}")},
		}
	}
	if got := pluginFolderFaults(clean()); len(got) != 0 {
		t.Fatalf("the clean folder has faults %q, want none", got)
	}
	for _, c := range []struct {
		name  string
		entry pluginEntry
		fault string
		at    string
	}{
		{"a stray data file", pluginEntry{data: []byte(`{"a":1}`)}, "notes.json is not a schema of the contract", "notes.json"},
		{"a stray text file", pluginEntry{data: []byte("x")}, "README.md is not a schema of the contract", "README.md"},
		{"a folder", pluginEntry{notFile: true}, "testdata is not a plain file", "testdata"},
		{"the matcher's Go file back", pluginEntry{data: []byte("package v1\n")}, "glob.go is not a schema of the contract", "glob.go"},
		{"a Go test file", pluginEntry{data: []byte("package v1_test\n")}, "glob_test.go is not a schema of the contract", "glob_test.go"},
		{"a third schema", pluginEntry{data: []byte("{}")}, `the schemas are "extra.schema.json manifest.schema.json wire.schema.json", want "manifest.schema.json wire.schema.json"`, "extra.schema.json"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			folder := clean()
			folder[c.at] = c.entry
			got := pluginFolderFaults(folder)
			if len(got) != 1 || !strings.HasPrefix(got[0], c.fault) {
				t.Errorf("faults of the folder with %s = %q, want one starting %q", c.at, got, c.fault)
			}
		})
	}
	t.Run("a schema missing", func(t *testing.T) {
		t.Parallel()
		folder := clean()
		delete(folder, "wire.schema.json")
		want := `the schemas are "manifest.schema.json", want "manifest.schema.json wire.schema.json"`
		if got := pluginFolderFaults(folder); !slices.Equal(got, []string{want}) {
			t.Errorf("faults of the folder with no wire.schema.json = %q, want %q", got, []string{want})
		}
	})
}

// TestThePluginFolderHoldsTheTwoSchemasAndEveryMessageHasExamples: the
// folder is the two schema files and nothing else (pluginFolderFaults), each
// $id is its place, and every message of the
// wire, a request or a response, is one with examples, so a message added
// with none is red.
func TestThePluginFolderHoldsTheTwoSchemasAndEveryMessageHasExamples(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	folder := map[string]pluginEntry{}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			folder[e.Name()] = pluginEntry{notFile: true}
			continue
		}
		b, err := os.ReadFile(path.Join(pluginDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		folder[e.Name()] = pluginEntry{data: b}
	}
	for _, f := range pluginFolderFaults(folder) {
		t.Errorf("contract/plugin/v1: %s", f)
	}
	for n, doc := range pluginSchemaFiles(t) {
		if id, _ := doc.(map[string]any)["$id"].(string); id != "contract/plugin/v1/"+n {
			t.Errorf("%s has the $id %q, want its own place", n, id)
		}
	}
	defs, _ := pluginSchemaFiles(t)["wire.schema.json"].(map[string]any)["$defs"].(map[string]any)
	for name := range defs {
		if (strings.HasSuffix(name, ".request") || strings.HasSuffix(name, ".response") || name == "request" || name == "response" || name == "error") && !slices.Contains(wireKinds(), name) {
			t.Errorf("the message %s has no examples", name)
		}
	}
	for _, k := range wireKinds() {
		if _, ok := defs[k]; !ok {
			t.Errorf("the wire has no message %s", k)
		}
	}
}

// witKeywords are the keywords a plugin schema may use. A record is an
// object with named fields; a list, an enum, a string, a number and a flag
// are what they say; a reference, and the bounds of each. A variant is oneOf
// of cases each a reference or a set of required fields, and if and then say
// which fields one case of a kind requires; allOf extends a record by
// another. No keyword makes a map, a union of types or a negation, so
// contract 1 stays in the shapes WIT can say.
func witKeywords() []string {
	return []string{
		"$schema", "$id", "$ref", "$defs", "title", "description",
		"type", "properties", "required", "additionalProperties", "minProperties", "maxProperties",
		"items", "minItems", "maxItems", "uniqueItems",
		"enum", "const", "pattern", "minLength", "maxLength", "minimum", "maximum",
		"oneOf", "allOf", "if", "then",
	}
}

// witProblems is what in the schema node at, under the name at, is not a
// shape WIT can say: a keyword outside witKeywords, a type that is a list or
// null, additionalProperties that is a schema (a map), or a null anywhere a
// value is fixed. An object that does not close its fields is a request,
// which a plugin reads ignoring what it does not know (4.7); the schema says
// nothing of that, and neither does WIT.
func witProblems(node any, at string) []string {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		v := m[k]
		switch {
		case !slices.Contains(witKeywords(), k):
			out = append(out, at+": the keyword "+k+" is not one of a record, a list, an enum, a string, a number or a flag")
		case k == "type":
			if s, ok := v.(string); !ok || s == "null" {
				out = append(out, at+": the type is not one named type")
			}
		case k == "enum":
			if l, _ := v.([]any); slices.Contains(l, nil) {
				out = append(out, at+": the enum holds a null")
			}
		case k == "const" && v == nil:
			out = append(out, at+": the const is null")
		}
	}
	if a, has := m["additionalProperties"]; has && a != false {
		out = append(out, at+": additionalProperties is a schema, which makes a map")
	}
	for _, k := range []string{"properties", "$defs"} {
		if sub, ok := m[k].(map[string]any); ok {
			for _, n := range slices.Sorted(maps.Keys(sub)) {
				out = append(out, witProblems(sub[n], at+"/"+k+"/"+n)...)
			}
		}
	}
	out = append(out, witProblems(m["items"], at+"/items")...)
	for _, k := range []string{"oneOf", "allOf"} {
		l, _ := m[k].([]any)
		for i, v := range l {
			out = append(out, witProblems(v, at+"/"+k+"/"+strconv.Itoa(i))...)
		}
	}
	for _, k := range []string{"if", "then"} {
		out = append(out, witProblems(m[k], at+"/"+k)...)
	}
	return out
}

// TestThePluginSchemasUseOnlyShapesWITCanSay: no plugin schema
// makes a map, a union of types or a negation, or holds a null, so a later
// move to the component model rewrites no message; and the planted shapes are
// each refused by the check.
func TestThePluginSchemasUseOnlyShapesWITCanSay(t *testing.T) {
	t.Parallel()
	for name, doc := range pluginSchemaFiles(t) {
		if got := witProblems(doc, name); len(got) > 0 {
			t.Errorf("%s: %q", name, got)
		}
	}
	// the manifest is a file a person writes and an approval reads: no
	// branch on a value, as gates.registry.8 holds of the adapter format;
	// the grant's oneOf of three closed
	// records stays
	if got := branches(pluginSchemaFiles(t)["manifest.schema.json"], "manifest.schema.json"); len(got) > 0 {
		t.Errorf("the manifest branches on a value: %q", got)
	}
	if got := branches(map[string]any{"$defs": map[string]any{"g": map[string]any{"if": map[string]any{}, "then": map[string]any{}}}}, "plant"); len(got) != 2 {
		t.Errorf("a planted if and then gave %q, want both named", got)
	}
	// a oneOf whose branches are told apart by a const is an if and then by
	// another name; the grant's three records, told apart by their keys, are
	// not
	if got := constTagged(pluginSchemaFiles(t)["manifest.schema.json"], "manifest.schema.json"); len(got) > 0 {
		t.Errorf("the manifest tells a variant's cases apart by a const: %q", got)
	}
	tagged := map[string]any{"oneOf": []any{
		map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"const": "a"}, "x": map[string]any{"type": "string"}}},
		map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"const": "b"}, "y": map[string]any{"type": "string"}}},
	}}
	if got := constTagged(map[string]any{"properties": map[string]any{"g": map[string]any{"type": "array", "items": tagged}}}, "plant"); len(got) != 1 {
		t.Errorf("a planted const-tagged oneOf gave %q, want it named", got)
	}
	// the same oneOf with a one-value enum for the tag: the generator reads that as a const, so it is one
	enumTagged := map[string]any{"oneOf": []any{
		map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"enum": []any{"a"}}}},
		map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"enum": []any{"b"}}}},
	}}
	if got := constTagged(enumTagged, "plant"); len(got) != 1 {
		t.Errorf("a planted oneOf tagged by a one-value enum gave %q, want it named", got)
	}
	for _, c := range []struct {
		name  string
		node  map[string]any
		words string
	}{
		{"an anyOf", map[string]any{"anyOf": []any{}}, "the keyword anyOf"},
		{"a not", map[string]any{"not": map[string]any{}}, "the keyword not"},
		{"a pattern of keys", map[string]any{"type": "object", "patternProperties": map[string]any{}}, "the keyword patternProperties"},
		{"a map", map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}, "makes a map"},
		{"a null type inside a case", map[string]any{"oneOf": []any{map[string]any{"type": "null"}}}, "/oneOf/0: the type is not one named type"},
		{"a type list", map[string]any{"type": []any{"string", "null"}}, "not one named type"},
		{"a null type", map[string]any{"type": "null"}, "not one named type"},
		{"a null in an enum", map[string]any{"enum": []any{"a", nil}}, "the enum holds a null"},
		{"a deep map", map[string]any{"$defs": map[string]any{"x": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": true}}}}, "/$defs/x/items: additionalProperties is a schema"},
	} {
		if got := witProblems(c.node, "plant"); !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, c.words) }) {
			t.Errorf("%s: problems %q lack %q", c.name, got, c.words)
		}
	}
}

// fieldNames is every field a schema node declares, at any depth.
func fieldNames(node any, out map[string]bool) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for k, v := range props {
			out[k] = true
			fieldNames(v, out)
		}
	}
	if defs, ok := m["$defs"].(map[string]any); ok {
		for _, v := range defs {
			fieldNames(v, out)
		}
	}
	fieldNames(m["items"], out)
}

// keysOf is every key of every object in a document, at any depth.
func keysOf(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			out[k] = true
			keysOf(e, out)
		}
	case []any:
		for _, e := range x {
			keysOf(e, out)
		}
	}
}

// TestEveryPluginFieldIsInAValidExample: each field any plugin schema
// declares is a key of some valid example, so no field stands untried by the
// examples that are to hold it.
func TestEveryPluginFieldIsInAValidExample(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, doc := range pluginSchemaFiles(t) {
		fieldNames(doc, declared)
	}
	used := map[string]bool{}
	for kind := range pluginKinds() {
		keysOf(readJSON(t, path.Join(pluginExamples, kind+".valid.json")), used)
	}
	for _, f := range slices.Sorted(maps.Keys(declared)) {
		if !used[f] {
			t.Errorf("no valid example has the field %s", f)
		}
	}
}

// TestTheShownClassRefusesWhatTheReaderOnlyTakesEscaped: a C1 control, such
// as U+009B (the one-character CSI), and U+FEFF are refused by the reader
// when they stand raw in a document, so they reach the schema only written
// as escapes; the manifest's shown class refuses them there, in each field
// that refers to it.
// Planted failure: a class that stops at C0, which takes the escaped CSI.
func TestTheShownClassRefusesWhatTheReaderOnlyTakesEscaped(t *testing.T) {
	t.Parallel()
	s := pluginSchema(t)
	valid := docs(t, readJSON(t, path.Join(pluginExamples, "manifest.valid.json")))
	base := string(encode(t, valid[0]))
	for _, c := range []struct{ field, from, to string }{
		{"wiring.import.at", `"at":"/extends"`, `"at":"/ext\u009bends"`},
		{"version_pattern", `"version_pattern":"v([0-9.]+)"`, `"version_pattern":"v([0-9.]+)\u009b2K"`},
		{"script_at", `"script_at":"/`, `"script_at":"/\ufeff`},
		{"preset.path", `"path":"lint-a.config.js"`, `"path":"lint-a.config\u0085.js"`},
		{"version_pattern, DEL", `"version_pattern":"v([0-9.]+)"`, `"version_pattern":"v([0-9.]+)\u007f"`},
	} {
		doc := strings.Replace(base, c.from, c.to, 1)
		if doc == base {
			t.Fatalf("%s: the valid manifest holds no %s to plant in", c.field, c.from)
		}
		ps, err := s.Check(pluginKinds()["manifest"], []byte(doc))
		if err != nil {
			t.Fatalf("%s: %v", c.field, err)
		}
		if !slices.ContainsFunc(ps, func(p model.Problem) bool { return strings.Contains(p.Msg, "does not match") }) {
			t.Errorf("%s: an escaped %s was taken: %v", c.field, c.to, ps)
		}
	}
}

// branches is every if and then at any depth of the schema node at.
func branches(node any, at string) []string {
	var out []string
	switch x := node.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			if k == "if" || k == "then" {
				out = append(out, at+"/"+k)
			}
			out = append(out, branches(x[k], at+"/"+k)...)
		}
	case []any:
		for i, v := range x {
			out = append(out, branches(v, at+"/"+strconv.Itoa(i))...)
		}
	}
	return out
}

// uncapped is the strings and lists the schemas leave without a cap, each
// with the reason it is open: the codec's caps in bytes (4.3) hold the byte
// fields and a preset file's text; the class of shown text is a fragment
// every field that refers to it caps itself; a feature name is the
// engine's own to write; the request body and an answer's ok are checked
// against their own message.
func uncapped() map[string]string {
	return map[string]string{
		"wire.schema.json/$defs/bytes/properties/text":                                  "the codec's cap in bytes, 4.3",
		"wire.schema.json/$defs/bytes/properties/base64":                                "the codec's cap in bytes, 4.3",
		"wire.schema.json/$defs/preset.response/properties/files/items/properties/text": "the codec's cap in bytes, 4.3",
		"wire.schema.json/$defs/request/properties/features/items":                      "a feature name the engine writes; the list is capped",
		"manifest.schema.json/$defs/shown":                                              "a fragment; each field that refers to it has its own maxLength",
		"manifest.schema.json/$defs/ascii":                                              "a fragment; each field that refers to it has its own maxLength",
	}
}

// capProblems is every string and list under the schema node at with no
// cap and no reason in uncapped: a string wants a maxLength, an enum or a
// const, and a list a maxItems, or uniqueItems over an enum.
func capProblems(node any, at string, open map[string]string) []string {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	if _, why := open[at]; !why {
		_, hasEnum := m["enum"]
		_, hasConst := m["const"]
		switch m["type"] {
		case "string":
			if _, ok := m["maxLength"]; !ok && !hasEnum && !hasConst {
				out = append(out, at+": a string with no maxLength")
			}
		case "array":
			items, _ := m["items"].(map[string]any)
			_, enumItems := items["enum"]
			if _, ok := m["maxItems"]; !ok && (m["uniqueItems"] != true || !enumItems) {
				out = append(out, at+": a list with no maxItems")
			}
		}
	}
	for _, k := range []string{"properties", "$defs"} {
		if sub, ok := m[k].(map[string]any); ok {
			for _, n := range slices.Sorted(maps.Keys(sub)) {
				out = append(out, capProblems(sub[n], at+"/"+k+"/"+n, open)...)
			}
		}
	}
	out = append(out, capProblems(m["items"], at+"/items", open)...)
	for _, k := range []string{"oneOf", "allOf"} {
		l, _ := m[k].([]any)
		for i, v := range l {
			out = append(out, capProblems(v, at+"/"+k+"/"+strconv.Itoa(i), open)...)
		}
	}
	for _, k := range []string{"if", "then"} {
		out = append(out, capProblems(m[k], at+"/"+k, open)...)
	}
	return out
}

// TestEveryPluginStringAndListHasItsCap:
// every string either schema names has a maxLength, an enum or a const, and
// every list a maxItems or uniqueItems over an enum, but the few uncapped
// names with their reason; so a cap taken out is red here, not in a case
// each. And every reason names a place that is still open.
func TestEveryPluginStringAndListHasItsCap(t *testing.T) {
	t.Parallel()
	open := uncapped()
	var all []string
	for name, doc := range pluginSchemaFiles(t) {
		if got := capProblems(doc, name, open); len(got) > 0 {
			t.Errorf("%q", got)
		}
		all = append(all, capProblems(doc, name, nil)...)
	}
	// each place named open is still open: a place capped since its line
	// was written is red here, so the list cannot hold a stale name
	for at, why := range open {
		if !slices.ContainsFunc(all, func(p string) bool { return strings.HasPrefix(p, at+": ") }) {
			t.Errorf("%s is named open (%s), and the schema caps it: take its line out of uncapped()", at, why)
		}
	}
	for _, c := range []struct {
		name string
		node map[string]any
	}{
		{"a string", map[string]any{"type": "string", "pattern": "^a+$"}},
		{"a list", map[string]any{"type": "array", "items": map[string]any{"type": "string", "maxLength": 1}}},
		{"a list of unique words, not an enum", map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "maxLength": 1}}},
		{"a deep string", map[string]any{"$defs": map[string]any{"x": map[string]any{"oneOf": []any{map[string]any{"type": "string"}}}}}},
	} {
		if got := capProblems(c.node, "plant", nil); len(got) == 0 {
			t.Errorf("%s with no cap passed", c.name)
		}
	}
}

// tags is whether a property fixes its value: by a const, or by an enum of one
// value, which the generator reads as the same tag.
func tags(p map[string]any) bool {
	if _, ok := p["const"]; ok {
		return true
	}
	values, ok := p["enum"].([]any)
	return ok && len(values) == 1
}

// constTagged is every oneOf at any depth of the schema node at whose cases
// are told apart by a const, or by an enum of one value: a case that fixes the
// value of a field.
func constTagged(node any, at string) []string {
	var out []string
	switch x := node.(type) {
	case map[string]any:
		if cases, ok := x["oneOf"].([]any); ok {
			for _, c := range cases {
				props, _ := c.(map[string]any)["properties"].(map[string]any)
				if slices.ContainsFunc(slices.Collect(maps.Values(props)), func(p any) bool { return tags(p.(map[string]any)) }) {
					out = append(out, at+"/oneOf")
					break
				}
			}
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = append(out, constTagged(x[k], at+"/"+k)...)
		}
	case []any:
		for i, v := range x {
			out = append(out, constTagged(v, at+"/"+strconv.Itoa(i))...)
		}
	}
	return out
}

// untyped is every field under the schema node at, a value under properties
// or $defs or a list's items, that names no type, $ref, enum, const, oneOf or
// allOf, but those open names; such a field takes any value, which no WIT
// type says, and a reference taken out of it is red here.
func untyped(node any, at string, open []string) []string {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	field := func(v any, where string) {
		f, ok := v.(map[string]any)
		if !ok || slices.Contains(open, where) {
			return
		}
		for _, k := range []string{"type", "$ref", "enum", "const", "oneOf", "allOf"} {
			if _, has := f[k]; has {
				return
			}
		}
		out = append(out, where+": a field with no type")
	}
	for _, k := range []string{"properties", "$defs"} {
		if sub, ok := m[k].(map[string]any); ok {
			for _, n := range slices.Sorted(maps.Keys(sub)) {
				field(sub[n], at+"/"+k+"/"+n)
				out = append(out, untyped(sub[n], at+"/"+k+"/"+n, open)...)
			}
		}
	}
	if items, ok := m["items"]; ok {
		field(items, at+"/items")
		out = append(out, untyped(items, at+"/items", open)...)
	}
	for _, k := range []string{"oneOf", "allOf"} {
		l, _ := m[k].([]any)
		for i, v := range l {
			out = append(out, untyped(v, at+"/"+k+"/"+strconv.Itoa(i), open)...)
		}
	}
	for _, k := range []string{"if", "then"} {
		out = append(out, untyped(m[k], at+"/"+k, open)...)
	}
	return out
}

// TestEveryPluginFieldHasAType: each field of either schema names its type,
// a reference to one, or its values; the one field open by design is a rule's
// value, which only the plugin reads (4.3). A reference taken out of a field,
// in any form of the grant or any answer of the wire, is red here.
func TestEveryPluginFieldHasAType(t *testing.T) {
	t.Parallel()
	open := []string{"wire.schema.json/$defs/value"}
	for name, doc := range pluginSchemaFiles(t) {
		if got := untyped(doc, name, open); len(got) > 0 {
			t.Errorf("%q", got)
		}
	}
	plant := map[string]any{"properties": map[string]any{"g": map[string]any{"oneOf": []any{map[string]any{"type": "object", "properties": map[string]any{"version_argv": map[string]any{"description": "its reference taken out"}}}}}}}
	if got := untyped(plant, "plant", nil); len(got) != 1 {
		t.Errorf("a field with its reference taken out gave %q, want it named", got)
	}
}

// TestTheShownClassRefusesEveryControlFormatAndSeparator: the manifest's
// class of text a person reads or a path names, checked alone, refuses U+2028,
// U+2029 and U+007F, with U+009B, U+200B, U+202E and U+FEFF, and takes plain text.
// Against a class
// that leaves one of them out, its row is red.
func TestTheShownClassRefusesEveryControlFormatAndSeparator(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(path.Join(pluginDir, "manifest.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := jsonschema.New([]model.File{
		{Path: "contract/plugin/v1/manifest.schema.json", Data: b},
		{Path: "contract/plugin/v1/check-shown.json", Data: []byte(`{"$ref": "manifest.schema.json#/$defs/shown"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, doc string
		refused   bool
	}{
		{"plain text", `"node_modules/lint-a"`, false},
		{"U+2028, a line separator", `"a\u2028b"`, true},
		{"U+2029, a paragraph separator", `"a\u2029b"`, true},
		{"U+007F, DEL", `"a\u007fb"`, true},
		{"U+009B, the one-character CSI", `"a\u009b2K"`, true},
		{"U+200B, a zero-width space", `"a\u200bb"`, true},
		{"U+202E, a right-to-left override", `"a\u202eb"`, true},
		{"U+FEFF", `"a\ufeffb"`, true},
	} {
		ps, err := s.Check("contract/plugin/v1/check-shown.json", []byte(c.doc))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := len(ps) > 0; got != c.refused {
			t.Errorf("%s: refused %v, want %v (%v)", c.name, got, c.refused, ps)
		}
	}
}
