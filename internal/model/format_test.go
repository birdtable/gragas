package model_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// The format schemas of v3 (docs/architecture.md section 7.1) describe
// the files the rulebook types of this package are read from. These tests hold
// the two to each other in both directions, hold the schemas to their
// hand-written examples' fields and enum members, and show how format v3 differs
// from the format v2 it replaces. They read the schema files where they stand,
// beside host/assets, which embeds them; cmd/gragas holds the embedded copy to
// the examples through the Schema port.

// schemaDir is the folder of the format schemas, from this package's folder.
const schemaDir = "../host/assets/format/schema"

// repoRoot is the module's root, from this package's folder.
const repoRoot = "../.."

// examples is the folder of the goldens: for each file kind, <kind>.valid.json
// holds hand-authored valid documents.
const examples = "../testdata/format-v3"

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

// ---------------------------------------------------------------------------
// Walking the schemas.

// set is the format schemas as decoded JSON, by name below the schema folder.
type set map[string]map[string]any

func loadSet(t *testing.T) set {
	t.Helper()
	s := set{}
	versions, err := os.ReadDir(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		if !v.IsDir() {
			continue
		}
		entries, err := os.ReadDir(path.Join(schemaDir, v.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			name := path.Join(v.Name(), e.Name())
			m, ok := readJSON(t, path.Join(schemaDir, name)).(map[string]any)
			if !ok {
				t.Fatalf("%s is not an object", name)
			}
			s[name] = m
		}
	}
	return s
}

// pointer follows a JSON pointer in file, following a $ref at every step, and
// answers the file the node is in and the node.
func (s set) pointer(t *testing.T, file, ptr string) (string, map[string]any) {
	t.Helper()
	node := any(s[file])
	for step := range strings.SplitSeq(strings.Trim(ptr, "/"), "/") {
		if step == "" {
			continue
		}
		file, node = s.deref(t, file, node)
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("%s %s: no object at %q", file, ptr, step)
		}
		node = m[step]
		if node == nil {
			t.Fatalf("%s %s: no %q", file, ptr, step)
		}
	}
	file, node = s.deref(t, file, node)
	m, _ := node.(map[string]any)
	return file, m
}

// deref follows the $ref of node, if it has one, until it has none.
func (s set) deref(t *testing.T, file string, node any) (string, any) {
	t.Helper()
	for range 10 {
		m, ok := node.(map[string]any)
		if !ok {
			return file, node
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return file, node
		}
		name, ptr, _ := strings.Cut(ref, "#")
		if name != "" {
			file = path.Join(path.Dir(file), name)
		}
		if _, ok := s[file]; !ok {
			t.Fatalf("%s: $ref %q names a schema that is not embedded", file, ref)
		}
		node = any(s[file])
		for step := range strings.SplitSeq(strings.Trim(ptr, "/"), "/") {
			if step != "" {
				node = node.(map[string]any)[step]
			}
		}
	}
	t.Fatalf("%s: $ref loop", file)
	return file, nil
}

func join(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}

// schemaPaths are the field paths below node: a property is its name under
// its parent's, and a list's items are "[]" after the list's.
func (s set) schemaPaths(t *testing.T, file string, node any, prefix string, out map[string]bool) {
	t.Helper()
	file, node = s.deref(t, file, node)
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for k, v := range props {
			p := join(prefix, k)
			out[p] = true
			s.schemaPaths(t, file, v, p, out)
		}
	}
	if items, ok := m["items"]; ok {
		s.schemaPaths(t, file, items, prefix+"[]", out)
	}
}

// goPaths are the field paths of a Go type, in the same form.
func goPaths(typ reflect.Type, prefix string, out map[string]bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	for f := range typ.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		p := join(prefix, name)
		out[p] = true
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Slice {
			p += "[]"
			ft = ft.Elem()
		}
		goPaths(ft, p, out)
	}
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// typeMap says which Go type each schema, or part of one, describes. Book is
// not here: it is assembled from the others, and a test below holds its
// fields to them.
type typed struct {
	file, ptr string
	typ       reflect.Type
}

func typeMap() []typed {
	return []typed{
		{"v3/checker.schema.json", "", reflect.TypeFor[model.Checker]()},
		{"v3/identity.schema.json", "", reflect.TypeFor[model.RulebookIdentity]()},
		{"v3/structure.schema.json", "", reflect.TypeFor[model.Structure]()},
		{"v3/terms.schema.json", "/properties/terms/items", reflect.TypeFor[model.Term]()},
		{"v3/rule.schema.json", "", reflect.TypeFor[model.Rule]()},
		{"v3/block.schema.json", "", reflect.TypeFor[model.RuleBlock]()},
		{"v3/sources.schema.json", "/properties/sources/items", reflect.TypeFor[model.Source]()},
		{"v3/decisions.schema.json", "/properties/decisions/items", reflect.TypeFor[model.Decision]()},
		{"v3/exclusions.schema.json", "/properties/exclusions/items", reflect.TypeFor[model.Exclusion]()},
		{"v3/proposals.schema.json", "", reflect.TypeFor[model.Proposal]()},
		{"v3/translations.schema.json", "/properties/translations/items", reflect.TypeFor[model.Translation]()},
		{"v3/history.schema.json", "", reflect.TypeFor[model.HistoryEntry]()},
	}
}

// envelopes are the root properties of a file schema that no type holds: the
// format version and the list the entities stand in.
func envelopes() map[string][]string {
	return map[string][]string{
		"v3/terms.schema.json":        {"format", "terms"},
		"v3/sources.schema.json":      {"format", "sources"},
		"v3/decisions.schema.json":    {"format", "decisions"},
		"v3/exclusions.schema.json":   {"format", "exclusions"},
		"v3/translations.schema.json": {"format", "translations"},
		"v3/rules.schema.json":        {"format", "part", "concern", "rules"},
	}
}

func TestEveryFieldOfTheModelIsInASchemaAndEveryFieldOfASchemaIsInTheModel(t *testing.T) {
	t.Parallel()
	s := loadSet(t)
	for _, c := range typeMap() {
		t.Run(c.typ.Name(), func(t *testing.T) {
			t.Parallel()
			file, node := s.pointer(t, c.file, c.ptr)
			want := map[string]bool{}
			s.schemaPaths(t, file, node, "", want)
			got := map[string]bool{}
			goPaths(c.typ, "", got)
			for _, p := range sorted(got) {
				if !want[p] {
					t.Errorf("model.%s has the field %s, which %s does not have", c.typ.Name(), p, c.file)
				}
			}
			for _, p := range sorted(want) {
				if !got[p] {
					t.Errorf("%s has the field %s, which model.%s does not have", c.file, p, c.typ.Name())
				}
			}
		})
	}
	for file, want := range envelopes() {
		_, root := s.pointer(t, file, "")
		have := map[string]bool{}
		for k := range root["properties"].(map[string]any) {
			have[k] = true
		}
		if !slices.Equal(sorted(have), sortedList(want)) {
			t.Errorf("%s has the root fields %v, want %v", file, sorted(have), sortedList(want))
		}
	}
}

func sortedList(l []string) []string {
	out := slices.Clone(l)
	sort.Strings(out)
	return out
}

func TestEveryFieldOfBookIsOneOfTheTypesTheSchemasDescribe(t *testing.T) {
	t.Parallel()
	described := map[reflect.Type]bool{}
	for _, c := range typeMap() {
		described[c.typ] = true
	}
	book := reflect.TypeFor[model.Book]()
	for f := range book.Fields() {
		ft := f.Type
		if ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Map {
			if f.Name != "Anchors" {
				t.Errorf("Book.%s is a map the schemas do not describe", f.Name)
			}
			continue
		}
		if ft == reflect.TypeFor[model.Opaque]() {
			continue // an opaque file, which no schema describes by design
		}
		if !described[ft] {
			t.Errorf("Book.%s is %s, which no schema describes", f.Name, ft)
		}
	}
}

// ---------------------------------------------------------------------------
// The examples are held to the type, in both directions.

func goldenPaths(v any, prefix string, out map[string]bool) {
	switch n := v.(type) {
	case map[string]any:
		for k, c := range n {
			p := join(prefix, k)
			out[p] = true
			goldenPaths(c, p, out)
		}
	case []any:
		for _, c := range n {
			goldenPaths(c, prefix+"[]", out)
		}
	}
}

func TestTheValidExamplesExerciseEveryFieldOfTheirSchemaAndNoOther(t *testing.T) {
	t.Parallel()
	s := loadSet(t)
	for _, kind := range kindNames() {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			file := fileKinds()[kind]
			schema := map[string]bool{}
			s.schemaPaths(t, file, s[file], "", schema)
			open := map[string]bool{}
			for p := range schema {
				open[p] = true
			}
			for p := range schema {
				for q := range schema {
					if strings.HasPrefix(q, p+".") || strings.HasPrefix(q, p+"[]") {
						delete(open, p)
						break
					}
				}
			}
			golden := map[string]bool{}
			goldenPaths(readJSON(t, path.Join(examples, kind+".valid.json")), "", golden)
			golden = trimRoot(golden)
			for p := range golden {
				if schema[p] {
					continue
				}
				under := false
				for o := range open {
					if strings.HasPrefix(p, o+".") || strings.HasPrefix(p, o+"[]") {
						under = true
					}
				}
				if !under {
					t.Errorf("the example has the field %s, which the schema does not have", p)
				}
			}
			if kind == "block" {
				// The block's example shows the anchor and a short rule; the
				// rules example exercises every field of the rule.
				return
			}
			for _, p := range sorted(schema) {
				if !golden[p] {
					t.Errorf("no example has the field %s", p)
				}
			}
		})
	}
}

// trimRoot drops the "[]" the array of documents puts before every path.
func trimRoot(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for p := range in {
		p = strings.TrimPrefix(p, "[]")
		p = strings.TrimPrefix(p, ".")
		if p != "" {
			out[p] = true
		}
	}
	return out
}

// enumCase is one closed set: the Go values, the schema node that lists them
// and the example that must show each member.
type enumCase struct {
	name      string
	file, ptr string
	golden    string
	key       string
	members   []string
}

func strs[T ~string](l []T) []string {
	out := make([]string, len(l))
	for i, v := range l {
		out[i] = string(v)
	}
	return out
}

func enumCases() []enumCase {
	return []enumCase{
		{name: "status", file: "v3/rule.schema.json", ptr: "/properties/status", golden: "rules", key: "status", members: strs(model.Statuses())},
		{name: "strength", file: "v3/rule.schema.json", ptr: "/properties/strength", golden: "rules", key: "strength", members: strs(model.Strengths())},
		{name: "origin", file: "v3/rule.schema.json", ptr: "/properties/origin", golden: "rules", key: "origin", members: strs(model.Origins())},
		{name: "backs", file: "v3/rule.schema.json", ptr: "/properties/sources/items/properties/backs", golden: "rules", key: "backs", members: strs(model.Backings())},
		{name: "pair target", file: "v3/rule.schema.json", ptr: "/properties/pairs/items/properties/target", golden: "rules", key: "target", members: strs(model.TargetTypes())},
		{name: "binding target", file: "v3/rule.schema.json", ptr: "/$defs/binding/properties/target", golden: "rules", key: "target", members: strs(model.TargetTypes())},
		{name: "severity", file: "v3/rule.schema.json", ptr: "/$defs/binding/properties/severity", golden: "rules", key: "severity", members: strs(model.Severities())},
		{name: "run point", file: "v3/rule.schema.json", ptr: "/$defs/binding/properties/run_point", golden: "rules", key: "run_point", members: strs(model.RunPoints())},
		{name: "identity kind", file: "v3/identity.schema.json", ptr: "/properties/kind", golden: "identity", key: "kind", members: strs(model.IdentityKinds())},
		{name: "contribution", file: "v3/identity.schema.json", ptr: "/properties/governance/properties/contribution", golden: "identity", key: "contribution", members: strs(model.Contributions())},
		{name: "checker proof", file: "v3/identity.schema.json", ptr: "/properties/governance/properties/checker_proof", golden: "identity", key: "checker_proof", members: strs(model.ProofLevels())},
		{name: "source kind", file: "v3/sources.schema.json", ptr: "/properties/sources/items/properties/kind", golden: "sources", key: "kind", members: strs(model.SourceKinds())},
		{name: "proposal state", file: "v3/proposals.schema.json", ptr: "/properties/state", golden: "proposals", key: "state", members: strs(model.ProposalStates())},
	}
}

func valuesOf(v any, key string, out map[string]bool) {
	switch n := v.(type) {
	case map[string]any:
		for k, c := range n {
			if k == key {
				if s, ok := c.(string); ok {
					out[s] = true
				}
			}
			valuesOf(c, key, out)
		}
	case []any:
		for _, c := range n {
			valuesOf(c, key, out)
		}
	}
}

func TestEveryEnumIsTheGoSetAndEveryMemberIsInAnExample(t *testing.T) {
	t.Parallel()
	s := loadSet(t)
	for _, c := range enumCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, node := s.pointer(t, c.file, c.ptr)
			list, ok := node["enum"].([]any)
			if !ok {
				t.Fatalf("%s %s is not an enum", c.file, c.ptr)
			}
			var schema []string
			for _, v := range list {
				schema = append(schema, v.(string))
			}
			if !slices.Equal(sortedList(schema), sortedList(c.members)) {
				t.Errorf("the schema lists %v, model lists %v", schema, c.members)
			}
			seen := map[string]bool{}
			valuesOf(readJSON(t, path.Join(examples, c.golden+".valid.json")), c.key, seen)
			for _, m := range c.members {
				if !seen[m] {
					t.Errorf("no %s example shows the %s %q", c.golden, c.name, m)
				}
			}
		})
	}
	t.Run("rung", func(t *testing.T) {
		_, node := s.pointer(t, "v3/rule.schema.json", "/$defs/binding/properties/rung")
		if node["minimum"] != float64(0) || node["maximum"] != float64(model.MaxRung) {
			t.Errorf("the rung runs %v to %v, want 0 to %d", node["minimum"], node["maximum"], model.MaxRung)
		}
		seen := map[float64]bool{}
		var walk func(v any)
		walk = func(v any) {
			switch n := v.(type) {
			case map[string]any:
				if r, ok := n["rung"].(float64); ok {
					seen[r] = true
				}
				for _, c := range n {
					walk(c)
				}
			case []any:
				for _, c := range n {
					walk(c)
				}
			}
		}
		walk(readJSON(t, path.Join(examples, "rules.valid.json")))
		for r := model.Rung(0); r <= model.MaxRung; r++ {
			if !seen[float64(r)] {
				t.Errorf("no example shows the rung %d", r)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Status, standing and the words that are gone.

func TestTheStatusSetIsExactlyProposedTrialActiveRetiredAndStandingIsNoField(t *testing.T) {
	t.Parallel()
	if got := strs(model.Statuses()); !slices.Equal(got, []string{"proposed", "trial", "active", "retired"}) {
		t.Errorf("the statuses are %v", got)
	}
	s := loadSet(t)
	for name, m := range s {
		var walk func(node any, where string)
		walk = func(node any, where string) {
			n, ok := node.(map[string]any)
			if !ok {
				return
			}
			if props, ok := n["properties"].(map[string]any); ok {
				for k, v := range props {
					if k == "standing" || k == "tier" || k == "later" {
						t.Errorf("%s defines the field %q at %s: standing is derived and the rung beside a tier is gone", name, k, where)
					}
					walk(v, where+"/"+k)
				}
			}
			for k, v := range n {
				if k == "properties" {
					continue
				}
				walk(v, where+"/"+k)
			}
		}
		walk(m, "")
	}
	for _, c := range typeMap() {
		got := map[string]bool{}
		goPaths(c.typ, "", got)
		for p := range got {
			if strings.Contains(strings.ToLower(p), "standing") {
				t.Errorf("model.%s has the field %s: standing is derived", c.typ.Name(), p)
			}
		}
	}
}

// A schema holds declarations only (gates.registry.8): no conditional, no
// dynamic reference, no format assertion.
func schemaKeywords() []string {
	return []string{"$schema", "$id", "$ref", "$defs", "title", "description", "type", "const", "enum",
		"properties", "additionalProperties", "required", "items", "minItems", "uniqueItems", "minLength", "pattern",
		"minimum", "maximum", "minProperties", "anyOf"}
}

func expressions(node any, where string, out *[]string) {
	n, ok := node.(map[string]any)
	if !ok {
		return
	}
	for k, v := range n {
		if !slices.Contains(schemaKeywords(), k) {
			*out = append(*out, where+": "+k)
			continue
		}
		switch k {
		case "properties", "$defs":
			for name, c := range v.(map[string]any) {
				expressions(c, where+"/"+name, out)
			}
		case "items", "additionalProperties":
			expressions(v, where+"/"+k, out)
		case "anyOf":
			for i, c := range v.([]any) {
				expressions(c, fmt.Sprintf("%s/anyOf/%d", where, i), out)
			}
		}
	}
}

func TestNoFormatSchemaHoldsAnExpression(t *testing.T) {
	t.Parallel()
	var found []string
	for name, m := range loadSet(t) {
		expressions(m, name, &found)
	}
	if len(found) > 0 {
		t.Errorf("the schemas hold keywords that are not declarations: %v", found)
	}
	var planted []string
	expressions(map[string]any{"properties": map[string]any{"a": map[string]any{"if": map[string]any{}}}}, "planted", &planted)
	if len(planted) != 1 {
		t.Errorf("the guard did not see a planted if: %v", planted)
	}
}

// Every file carries its format version, and a reader tells an older file from
// a newer one by it (store.schemas.2).
func TestEveryFileSchemaRequiresTheFormatVersionAndOnlyTheBlockDoesNot(t *testing.T) {
	t.Parallel()
	s := loadSet(t)
	for _, kind := range kindNames() {
		file := fileKinds()[kind]
		req, _ := s[file]["required"].([]any)
		has := slices.Contains(req, any("format"))
		if kind == "block" && has {
			t.Errorf("the block is inside a Markdown guide and carries no format of its own")
		}
		if kind != "block" && !has {
			t.Errorf("%s does not require the format version", file)
		}
	}
	_, node := s.pointer(t, "v3/common.schema.json", "/$defs/format")
	if node["const"] != float64(model.FormatVersion) {
		t.Errorf("the schemas write format %v, model says %d", node["const"], model.FormatVersion)
	}
}

// ---------------------------------------------------------------------------
// Forward-only steps: how v2 and v3 differ, in one table.

// stepRow is one line of a numbered step: what the format says about a thing
// before and after it, each read from the schemas themselves.
type stepRow struct {
	what   string
	v2, v3 string
	wantV2 string
	wantV3 string
}

func enumOf(t *testing.T, node map[string]any) string {
	t.Helper()
	list, ok := node["enum"].([]any)
	if !ok {
		t.Fatalf("not an enum: %v", node)
	}
	var out []string
	for _, v := range list {
		out = append(out, fmt.Sprint(v))
	}
	return strings.Join(out, " ")
}

func hasProp(node map[string]any, name string) string {
	if _, ok := node["properties"].(map[string]any)[name]; ok {
		return "yes"
	}
	return "no"
}

func requires(node map[string]any, name string) string {
	req, _ := node["required"].([]any)
	if slices.Contains(req, any(name)) {
		return "required"
	}
	return "not required"
}

// Step 1 is the move from format 2 to format 3. The v2 side is read from the
// schemas the owner's rulebook is held to today (rulebook/schema); the v3 side
// from the embedded ones. A change to either side changes the table, and the
// table is what a migration is written from.
func TestStepOneTheFormatV2AndV3DifferInOneTable(t *testing.T) {
	t.Parallel()
	var v2 struct{ rules, common map[string]any }
	for name, dst := range map[string]*map[string]any{"rules.schema.json": &v2.rules, "common.schema.json": &v2.common} {
		m, ok := readJSON(t, path.Join(repoRoot, "rulebook", "schema", name)).(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object", name)
		}
		*dst = m
	}
	body := v2.rules["$defs"].(map[string]any)["ruleBody"].(map[string]any)
	v2Props := body["properties"].(map[string]any)
	v2Common := v2.common["$defs"].(map[string]any)
	v2Check := v2.rules["$defs"].(map[string]any)["check"].(map[string]any)

	s := loadSet(t)
	_, rule := s.pointer(t, "v3/rule.schema.json", "")
	_, binding := s.pointer(t, "v3/rule.schema.json", "/$defs/binding")
	_, rules := s.pointer(t, "v3/rules.schema.json", "")

	rows := []stepRow{
		{what: "status values", v2: enumOf(t, v2Props["status"].(map[string]any)), v3: enumOf(t, mustPointer(t, s, "v3/rule.schema.json", "/properties/status")),
			wantV2: "proposed active later retired", wantV3: "proposed trial active retired"},
		{what: "strength values", v2: enumOf(t, v2Common["strength"].(map[string]any)), v3: enumOf(t, mustPointer(t, s, "v3/rule.schema.json", "/properties/strength")),
			wantV2: "Hard Answered Session Lens Record Review", wantV3: "must should may"},
		{what: "the rule's sentence is called rule", v2: hasProp(body, "rule"), v3: hasProp(rule, "rule"), wantV2: "yes", wantV3: "no"},
		{what: "the rule's sentence is called statement", v2: hasProp(body, "statement"), v3: hasProp(rule, "statement"), wantV2: "no", wantV3: "yes"},
		{what: "a rule carries checks", v2: hasProp(body, "checks"), v3: hasProp(rule, "checks"), wantV2: "yes", wantV3: "no"},
		{what: "a rule carries bindings", v2: hasProp(body, "bindings"), v3: hasProp(rule, "bindings"), wantV2: "no", wantV3: "yes"},
		{what: "a check or binding has a tier", v2: hasProp(v2Check, "tier"), v3: hasProp(binding, "tier"), wantV2: "yes", wantV3: "no"},
		{what: "a check or binding has a rung", v2: hasProp(v2Check, "rung"), v3: hasProp(binding, "rung"), wantV2: "no", wantV3: "yes"},
		{what: "a check or binding has a run point", v2: hasProp(v2Check, "run_point"), v3: hasProp(binding, "run_point"), wantV2: "no", wantV3: "yes"},
		{what: "a concern file carries its format version", v2: requires(v2.rules, "format"), v3: requires(rules, "format"), wantV2: "not required", wantV3: "required"},
		{what: "a rule file has a title and a summary", v2: requires(v2.rules, "title") + " and " + requires(v2.rules, "summary"), v3: requires(rules, "title") + " and " + requires(rules, "summary"),
			wantV2: "required and required", wantV3: "not required and not required"},
		{what: "a rule can carry a standing", v2: hasProp(body, "standing"), v3: hasProp(rule, "standing"), wantV2: "yes", wantV3: "no"},
	}
	var b strings.Builder
	b.WriteString("step 1: format 2 to format 3\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-44s | %-42s | %s\n", r.what, r.v2, r.v3)
		if r.v2 != r.wantV2 {
			t.Errorf("%s: v2 says %q, the table expects %q", r.what, r.v2, r.wantV2)
		}
		if r.v3 != r.wantV3 {
			t.Errorf("%s: v3 says %q, the table expects %q", r.what, r.v3, r.wantV3)
		}
	}
	t.Log("\n" + b.String())
}

func mustPointer(t *testing.T, s set, file, ptr string) map[string]any {
	t.Helper()
	_, n := s.pointer(t, file, ptr)
	return n
}

// open walks node and lists where an object (type object, so not a branch that
// only adds a constraint to an object its parent already guards) declares
// properties and does not
// forbid the others with additionalProperties: false.
func open(node any, where string, out *[]string) {
	switch n := node.(type) {
	case map[string]any:
		if _, has := n["properties"]; has && n["type"] == "object" && n["additionalProperties"] != false {
			*out = append(*out, where)
		}
		for k, v := range n {
			open(v, where+"/"+k, out)
		}
	case []any:
		for i, v := range n {
			open(v, fmt.Sprintf("%s/%d", where, i), out)
		}
	}
}

// An object a schema describes names its fields and refuses the rest, so a
// misspelt field is told and not read as absent.
func TestEveryObjectOfEverySchemaRefusesTheFieldsItDoesNotName(t *testing.T) {
	t.Parallel()
	var found []string
	for name, m := range loadSet(t) {
		open(m, name, &found)
	}
	sort.Strings(found)
	if len(found) > 0 {
		t.Errorf("objects with properties and no additionalProperties: false: %v", found)
	}
	var planted []string
	open(map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{}}}, "planted", &planted)
	if len(planted) != 1 {
		t.Errorf("the guard did not see a planted open object: %v", planted)
	}
}
