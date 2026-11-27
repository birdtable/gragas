package generate

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The TypeScript side. The zod schema is what is generated; the type of each
// declaration is z.infer of it, so the type and the schema cannot differ
// (ts.schema.1). One file per schema file, and an index that exports them.

const (
	// TSDir is where the contract package's generated source lives.
	TSDir = "explorer/packages/contract/src/"
	// SchemaDir is where the schemas the generated files name are.
	SchemaDir = "internal/host/assets/contract/schema/v1/"
)

func tsFile(file string) string { return TSDir + stem(file) + ".gen.ts" }

// tsFiles writes the zod file of every schema file and the index.
func tsFiles(t *tree, consumers []string) map[string][]byte {
	out := map[string][]byte{}
	files := t.files()
	for _, file := range files {
		out[tsFile(file)] = tsFileOf(t, file)
	}
	var idx bytes.Buffer
	idx.WriteString(header("//", SchemaDir+"*.schema.json") + "\n\n")
	for _, file := range files {
		fmt.Fprintf(&idx, "export * from \"./%s.gen.js\";\n", stem(file))
	}
	for _, c := range consumers {
		fmt.Fprintf(&idx, "export * from \"./%s-reads.gen.js\";\n", c)
	}
	out[TSDir+"index.ts"] = idx.Bytes()
	return out
}

func tsFileOf(t *tree, file string) []byte {
	var b bytes.Buffer
	b.WriteString(header("//", SchemaDir+file) + "\n\nimport { z } from \"zod\";\n")
	decls := t.inOrder(file)
	imports := map[string][]string{}
	for _, d := range decls {
		for _, r := range refsOf(d.T) {
			if o := t.index[r]; o.File != file && !slices.Contains(imports[o.File], r) {
				imports[o.File] = append(imports[o.File], r)
			}
		}
	}
	for _, f := range t.files() {
		if names := imports[f]; len(names) > 0 {
			slices.Sort(names)
			fmt.Fprintf(&b, "import { %s } from \"./%s.gen.js\";\n", strings.Join(names, ", "), stem(f))
		}
	}
	for _, d := range decls {
		b.WriteString("\n" + tsDoc(d))
		fmt.Fprintf(&b, "export const %s = %s;\nexport type %s = z.infer<typeof %s>;\n", d.Name, t.zod(d.T, ""), d.Name, d.Name)
	}
	return b.Bytes()
}

func tsDoc(d *decl) string {
	say := firstLine(d.T.Desc)
	if say == "" {
		say = firstLine(d.Title)
	}
	if say == "" {
		return ""
	}
	return "/** " + strings.ReplaceAll(say, "*/", "* /") + ". */\n"
}

// inOrder lists a file's declarations with each after the ones it names.
func (t *tree) inOrder(file string) []*decl {
	var out []*decl
	seen := map[string]bool{}
	var add func(d *decl)
	add = func(d *decl) {
		if seen[d.Name] || d.File != file {
			return
		}
		seen[d.Name] = true
		for _, r := range refsOf(d.T) {
			add(t.index[r])
		}
		out = append(out, d)
	}
	for _, d := range t.Decls {
		add(d)
	}
	return out
}

var reIdent = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func tsKey(k string) string {
	if reIdent.MatchString(k) {
		return k
	}
	return jsonOf(k)
}

// zod writes the zod expression of a type; indent is the indent of the line
// it starts on.
func (t *tree) zod(ty *typ, indent string) string {
	s := t.zodBase(ty, indent)
	if ty.Nullable {
		s += ".nullable()"
	}
	return s
}

func (t *tree) zodBase(ty *typ, indent string) string {
	switch ty.Kind {
	case kString:
		s := "z.string()"
		if ty.MinLen != nil {
			s += fmt.Sprintf(".min(%g)", *ty.MinLen)
		}
		if ty.Pattern != "" {
			s += ".regex(new RegExp(" + jsonOf(ty.Pattern) + "))"
		}
		return s
	case kConst:
		return "z.literal(" + jsonOf(ty.Const) + ")"
	case kInteger, kNumber:
		s := "z.number()"
		if ty.Kind == kInteger {
			s += ".int()"
		}
		if ty.Min != nil {
			s += fmt.Sprintf(".min(%g)", *ty.Min)
		}
		if ty.Max != nil {
			s += fmt.Sprintf(".max(%g)", *ty.Max)
		}
		return s
	case kBool:
		return "z.boolean()"
	case kArray:
		s := "z.array(" + t.zod(ty.Elem, indent) + ")"
		if ty.MinItems != nil {
			s += fmt.Sprintf(".min(%g)", *ty.MinItems)
		}
		if ty.MaxItems != nil {
			s += fmt.Sprintf(".max(%g)", *ty.MaxItems)
		}
		return s
	case kRef:
		return ty.Ref
	case kAny:
		return "z.unknown()"
	case kMap:
		return "z.record(z.string(), z.unknown())"
	case kEnum:
		vals := make([]string, len(ty.Enum))
		for i, v := range ty.Enum {
			vals[i] = jsonOf(v)
		}
		return "z.enum([" + strings.Join(vals, ", ") + "])"
	}
	return t.zodObject(ty, indent)
}

func (t *tree) zodObject(ty *typ, indent string) string {
	var b strings.Builder
	b.WriteString("z.strictObject({\n")
	for _, f := range ty.Fields {
		e := t.zod(f.T, indent+"  ")
		switch {
		case f.Required:
		case f.HasDefault:
			e += t.withDefault(f)
		default:
			e += ".optional()"
		}
		b.WriteString(indent + "  " + tsKey(f.Key) + ": " + e + ",\n")
	}
	b.WriteString(indent + "})")
	return b.String()
}

// withDefault is how a field with a default is written. An object's default
// is the empty object, which zod must parse so the object's own defaults are
// filled; where the object has a required field the empty object is no valid
// value, so the field is left optional and absent stays absent.
func (t *tree) withDefault(f field) string {
	if m, ok := f.Default.(map[string]any); ok && len(m) == 0 {
		inner := t.resolve(f.T)
		switch inner.Kind {
		case kMap, kAny:
			return ".default({})"
		case kObject:
		default:
			// a default that is no value of the type (the schema says {} for an enum):
			// absent stays absent rather than inventing a value
			return ".optional()"
		}
		for _, x := range inner.Fields {
			if x.Required {
				return ".optional()"
			}
		}
		return ".prefault({})"
	}
	return ".default(" + jsonOf(f.Default) + ")"
}
