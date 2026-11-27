package docs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The format spec of docs/format is prose beside the schemas of format v3
// (S25, M1-22). Two checks hold it to them. Every field a schema declares has
// a row in the table that follows the line "Schema: [name](...)" in a document
// of docs/format, and every row of such a table names a field of that schema
// (docs.freshness.4: a current fact is stated only where a check holds it);
// a row also says "yes" in its Required column exactly when the schema
// requires the field in its object. The fences of those documents are held in
// internal/host/jsonschema, which may import the schema adapter.

// formatSchemaDir is the folder of the v3 schemas, from the root.
const formatSchemaDir = "internal/host/assets/format/schema/v3"

// formatDocsDir is the folder of the prose.
const formatDocsDir = "docs/format/"

// formatLater are the schemas this spec does not yet describe, each with the
// milestone that adds its section. Anything else in the folder must have a
// table, so a new schema cannot be added without its prose.
func formatLater() map[string]string {
	return map[string]string{}
}

// formatShared are the schemas that are no file kind: shapes the others share.
func formatShared() []string { return []string{"common.schema.json"} }

// schemaField is one field of a schema: where it stands in the document and
// whether its object requires it.
type schemaField struct {
	path     string
	required bool
}

// schemaSet is the schemas of the folder, parsed.
type schemaSet struct {
	byName map[string]map[string]any
}

// loadSchemas reads every schema of formatSchemaDir.
func loadSchemas() (schemaSet, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(formatSchemaDir)))
	if err != nil {
		return schemaSet{}, err
	}
	s := schemaSet{byName: map[string]map[string]any{}}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(formatSchemaDir), e.Name()))
		if err != nil {
			return schemaSet{}, err
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			return schemaSet{}, fmt.Errorf("%s: %w", e.Name(), err)
		}
		s.byName[e.Name()] = doc
	}
	return s, nil
}

// names are the schemas that are file kinds, in order.
func (s schemaSet) names() []string {
	var out []string
	for n := range s.byName {
		if !slices.Contains(formatShared(), n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// resolve follows a $ref of schema file from to the node it names. A ref to
// a whole other schema file is not followed: it is the rule, which has a
// table of its own, and the caller stops there.
func (s schemaSet) resolve(from string, node map[string]any) (file string, out map[string]any, whole bool) {
	ref, ok := node["$ref"].(string)
	if !ok {
		return from, node, false
	}
	file, frag, _ := strings.Cut(ref, "#")
	if file == "" {
		file = from
	}
	if frag == "" {
		return file, s.byName[file], true
	}
	cur := any(s.byName[file])
	for k := range strings.SplitSeq(strings.TrimPrefix(frag, "/"), "/") {
		m, _ := cur.(map[string]any)
		cur = m[k]
	}
	m, _ := cur.(map[string]any)
	return file, m, false
}

// fields are the fields of schema file name, in path order: an object's
// property is its path, an array's item is its path followed by [], and a
// shape another schema file owns as a whole (the rule) is a field and is not
// walked into.
func (s schemaSet) fields(name string) []schemaField {
	var out []schemaField
	s.walk(name, s.byName[name], "", &out)
	return out
}

func (s schemaSet) walk(file string, node map[string]any, prefix string, out *[]schemaField) {
	req := map[string]bool{}
	if rs, ok := node["required"].([]any); ok {
		for _, r := range rs {
			req[r.(string)] = true
		}
	}
	props, _ := node["properties"].(map[string]any)
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		p := prefix + k
		*out = append(*out, schemaField{path: p, required: req[k]})
		child, _ := props[k].(map[string]any)
		s.into(file, child, p, out)
	}
}

// into walks below a field: through its $ref, and into the items of an array.
func (s schemaSet) into(file string, node map[string]any, path string, out *[]schemaField) {
	file, node, whole := s.resolve(file, node)
	if whole || node == nil {
		return
	}
	if items, ok := node["items"].(map[string]any); ok {
		s.into(file, items, path+"[]", out)
		return
	}
	if _, ok := node["properties"]; ok {
		s.walk(file, node, path+".", out)
	}
}

var (
	schemaLine = regexp.MustCompile(`^Schema: \[([a-z]+\.schema\.json)\]\(`)
)

// proseRow is one row of a field table: the field it names and whether its
// Required column says yes.
type proseRow struct {
	field string
	yes   bool
	line  int
}

// proseTables are the field tables of one document, by the schema file the
// line before each one links.
func proseTables(text string) (map[string][]proseRow, []string) {
	out := map[string][]proseRow{}
	var problems []string
	var cur string
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if m := schemaLine.FindStringSubmatch(l); m != nil {
			cur = m[1]
			if _, dup := out[cur]; dup {
				problems = append(problems, fmt.Sprintf("line %d: a second table for %s in one document", i+1, cur))
			}
			out[cur] = []proseRow{}
			continue
		}
		if cur == "" {
			continue
		}
		if !strings.HasPrefix(l, "|") {
			if strings.TrimSpace(l) != "" && len(out[cur]) > 0 {
				cur = ""
			}
			continue
		}
		cells := strings.Split(strings.Trim(l, "| "), "|")
		first := strings.TrimSpace(cells[0])
		if first == "Field" || strings.HasPrefix(first, "---") {
			continue
		}
		name := strings.Trim(first, "`")
		if name == first || len(cells) < 3 {
			problems = append(problems, fmt.Sprintf("line %d: a row of the %s table is not a `field` | Required | Meaning row", i+1, cur))
			continue
		}
		out[cur] = append(out[cur], proseRow{field: name, yes: strings.HasPrefix(strings.TrimSpace(cells[1]), "yes"), line: i + 1})
	}
	return out, problems
}

// formatFields is the two-way check of the schemas and the field tables.
func formatFields(w docWorld, s schemaSet) []string {
	var out []string
	claimed := map[string]string{}
	for _, name := range w.names() {
		if !strings.HasPrefix(name, formatDocsDir) {
			continue
		}
		tables, problems := proseTables(w.docs[name])
		for _, p := range problems {
			out = append(out, name+": "+p)
		}
		for schema, rows := range tables {
			if _, ok := s.byName[schema]; !ok || slices.Contains(formatShared(), schema) {
				out = append(out, name+": the table for "+schema+" names no schema of the format")
				continue
			}
			if other, dup := claimed[schema]; dup {
				out = append(out, name+": "+schema+" has a table in "+other+" already")
			}
			claimed[schema] = name
			want := map[string]bool{}
			for _, f := range s.fields(schema) {
				want[f.path] = f.required
			}
			seen := map[string]bool{}
			for _, r := range rows {
				req, ok := want[r.field]
				switch {
				case seen[r.field]:
					out = append(out, fmt.Sprintf("%s: line %d: %s is a row of the %s table twice", name, r.line, r.field, schema))
				case !ok:
					out = append(out, fmt.Sprintf("%s: line %d: the row %s names no field of %s", name, r.line, r.field, schema))
				case req && !r.yes:
					out = append(out, fmt.Sprintf("%s: line %d: %s is required by %s and the row does not say yes", name, r.line, r.field, schema))
				case !req && r.yes:
					out = append(out, fmt.Sprintf("%s: line %d: %s is not required by %s and the row says yes", name, r.line, r.field, schema))
				}
				seen[r.field] = true
			}
			for _, f := range s.fields(schema) {
				if !seen[f.path] {
					out = append(out, fmt.Sprintf("%s: the field %s of %s has no row", name, f.path, schema))
				}
			}
		}
	}
	for _, n := range s.names() {
		if _, later := formatLater()[n]; later {
			continue
		}
		if _, ok := claimed[n]; !ok {
			out = append(out, "docs/format: "+n+" has no field table")
		}
	}
	return out
}

// formatDocs is the check, on the repository's schemas. The fences of the
// same documents are held in internal/host/jsonschema, which may import the
// schema adapter.
func formatDocs(w docWorld) []string {
	s, err := loadSchemas()
	if err != nil {
		return []string{"the format schemas cannot be read: " + err.Error()}
	}
	return formatFields(w, s)
}

// TestTheFormatDocsChecksFireOnAPlantedBreach: the check is run on the
// repository's docs with one breach planted at a time, and says it; the
// repository's own docs are the clean control.
func TestTheFormatDocsChecksFireOnAPlantedBreach(t *testing.T) {
	t.Parallel()
	w := readDocWorld(t)
	if got := formatDocs(w); len(got) != 0 {
		t.Fatalf("the clean control has findings: %q", got)
	}
	terms := w.docs["docs/format/terms.md"]
	if terms == "" {
		t.Fatal("no docs/format/terms.md to plant a breach in")
	}
	drop := func(text, row string) string {
		out := strings.Replace(text, row, "", 1)
		if out == text {
			t.Fatalf("the plant found nothing to remove: %q", row)
		}
		return out
	}
	swap := func(text, from, to string) string {
		out := strings.Replace(text, from, to, 1)
		if out == text {
			t.Fatalf("the plant found nothing to change: %q", from)
		}
		return out
	}
	const literalRow = "| `terms[].literal` | no |"
	tests := []struct {
		name  string
		world docWorld
		says  string
	}{
		{"a schema field with no prose row", w.planted("docs/format/terms.md", drop(terms, "| `terms[].literal` | no | The name the term stands for in the world, such as a header name or a package, so a statement can stay neutral. The literal is the one name a rulebook may use bare |\n")), "the field terms[].literal of terms.schema.json has no row"},
		{"a prose row naming no field", w.planted("docs/format/terms.md", swap(terms, literalRow, "| `terms[].colour` | no |")), "the row terms[].colour names no field of terms.schema.json"},
		{"a required field the row does not say yes to", w.planted("docs/format/terms.md", swap(terms, "| `terms[].slug` | yes |", "| `terms[].slug` | no |")), "terms[].slug is required by terms.schema.json and the row does not say yes"},
		{"an optional field the row says yes to", w.planted("docs/format/terms.md", swap(terms, literalRow, "| `terms[].literal` | yes |")), "terms[].literal is not required by terms.schema.json and the row says yes"},
		{"a field listed twice", w.planted("docs/format/terms.md", swap(terms, "| `terms[].slug` | yes |", "| `terms[].slug` | yes | x |\n| `terms[].slug` | yes |")), "terms[].slug is a row of the terms.schema.json table twice"},
		{"a schema with no table", w.without("docs/format/terms.md"), "docs/format: terms.schema.json has no field table"},
		{"a table for no schema", w.planted("docs/format/x.md", "Schema: [nothing.schema.json](x)\n\n| Field | Required | Meaning |\n|---|---|---|\n| `a` | no | b |\n"), "names no schema of the format"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatDocs(tc.world)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("said %q, want a problem saying %q", got, tc.says)
			}
		})
	}
}
