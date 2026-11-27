package contract

import (
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// CommonFile is the schema of the shared definitions, which has no table of its own.
const CommonFile = "common.schema.json"

// FieldReference is the skeleton of the field reference: a table per schema of
// its fields, their types, the version each came in, its default and a
// derivation rule the handbook completes. all holds each schema parsed, by file
// name; nested objects are listed by their dotted name. It reads the schemas as
// written, so a $ref shows the name it points at. dir is where the schemas sit,
// named in the lead sentence.
func FieldReference(all map[string]*ordered.Map, dir string) string {
	var b strings.Builder
	b.WriteString("# The contract's field reference, v1\n\nA skeleton written from the schemas under `" + dir + "` by `gragas-generate`; the handbook completes the derivation column. Edit the schemas and run `make gen`; a hand edit is lost.\n")
	names := make([]string, 0, len(all))
	for n := range all {
		if n != CommonFile {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	for _, n := range names {
		b.WriteString("\n## " + strings.TrimSuffix(path.Base(n), ".schema.json") + "\n\n" + fieldTable(all[n]))
	}
	return b.String()
}

// FieldRow is one field of a schema as the field reference lists it.
type FieldRow struct {
	// Name is the dotted name of the field: a[].b for a field of an array's items.
	Name string
	// Type is the schema's type, the name a $ref points at, or enum.
	Type string
	// Since is the contract version the field came in, "" when the schema names none.
	Since string
	// Default is the JSON of the default, or "required" when there is none.
	Default string
}

const tableHead = "| Field | Type | Since | Default | Derivation |\n|---|---|---|---|---|\n"

func fieldTable(node *ordered.Map) string {
	var b strings.Builder
	b.WriteString(tableHead)
	for _, r := range Rows(node) {
		def := r.Default
		if def != "required" {
			def = "`" + def + "`"
		}
		b.WriteString("| " + r.Name + " | " + r.Type + " | " + r.Since + " | " + def + " | to be written |\n")
	}
	return b.String()
}

// Rows are the fields of one parsed schema, sorted by name at each level.
func Rows(node *ordered.Map) []FieldRow {
	var out []FieldRow
	collect(&out, node, "")
	return out
}

func collect(out *[]FieldRow, node *ordered.Map, prefix string) {
	props := node.Sub("properties")
	if props == nil {
		return
	}
	keys := slices.Clone(props.Keys())
	slices.Sort(keys)
	for _, k := range keys {
		p := props.Sub(k)
		typ := p.Str("type")
		if ref := p.Str("$ref"); ref != "" {
			typ = strings.TrimSuffix(strings.TrimPrefix(ref, "common.schema.json#/$defs/"), ".schema.json")
		}
		if _, ok := p.Get("enum"); ok {
			typ = "enum"
		}
		def := "required"
		if d, has := p.Get("default"); has {
			def = ordered.JSON(d, 0)
		}
		*out = append(*out, FieldRow{Name: prefix + k, Type: typ, Since: p.Str("x-introduced"), Default: def})
		if typ == "object" {
			collect(out, p, prefix+k+".")
		}
		if items := p.Sub("items"); items != nil && items.Sub("properties") != nil {
			collect(out, items, prefix+k+"[].")
		}
	}
}
