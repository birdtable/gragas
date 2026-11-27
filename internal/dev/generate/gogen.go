package generate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"slices"
	"strings"
)

// The Go side. Every type is named for its schema, in package contract, so
// the hand-written types of internal/model (Finding, Rule, ...) keep theirs,
// and every file is internal/model/contract/<schema>.gen.go. The generated code is types,
// constants and each type's ToCanon and FromCanon (gocodec.go): no function with an
// effect, and nothing that decides.

// GoDir is the package the generated Go types live in, apart from model so
// the types of every schema are not on the closure of the program that judges.
const GoDir = "internal/model/contract/"

// goFile is the path of the Go file made from a schema file.
func goFile(file string) string {
	return GoDir + strings.ReplaceAll(stem(file), "-", "_") + ".gen.go"
}

func goName(name string) string { return goIdent(name) }

// initialism is the capitals a Go name writes for a word, when it has some.
func initialism(word string) string {
	switch word {
	case "Id", "Url", "Api", "Uri", "Sha", "Html", "Json":
		return strings.ToUpper(word)
	}
	return word
}

// goIdent writes the initialisms inside an UpperCamelCase name in capitals:
// RuleId is RuleID, ApiSpec is APISpec.
func goIdent(name string) string {
	var words []string
	start := 0
	rs := []rune(name)
	for i := 1; i < len(rs); i++ {
		if isUpper(rs[i]) && !isUpper(rs[i-1]) {
			words = append(words, string(rs[start:i]))
			start = i
		}
	}
	words = append(words, string(rs[start:]))
	for i, w := range words {
		words[i] = initialism(w)
	}
	return strings.Join(words, "")
}

// goField turns a property key into a Go field name. A key is words joined by
// underscores or dashes, or camel-cased; each word is capitalised and an
// initialism is written in capitals.
func goField(key string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(key, func(r rune) bool { return r == '_' || r == '-' || r == '$' }) {
		b.WriteString(goIdent(camel(w)))
	}
	if b.Len() == 0 {
		return "Field"
	}
	return b.String()
}

// goFiles writes the Go file of every schema file, and CodecFile.
func goFiles(t *tree) (map[string][]byte, error) {
	out := map[string][]byte{CodecFile: codecFile()}
	var errs []error
	for _, file := range t.files() {
		var b bytes.Buffer
		fmt.Fprintf(&b, "%s\n\npackage contract\n\nimport \"github.com/birdtable/gragas/internal/canon\"\n", header("//", SchemaDir+file))
		for _, d := range t.Decls {
			if d.File != file {
				continue
			}
			codec, err := goCodec(t, goName(d.Name), d)
			if err != nil {
				errs = append(errs, err)
			}
			b.WriteString("\n" + goDecl(t, d) + codec)
		}
		out[goFile(file)] = formatted(b.Bytes())
	}
	return out, errors.Join(errs...)
}

// formatted is src as gofmt writes it, so the gate that holds the tree to gofmt
// passes on a generated file; source that does not parse is left as it is and
// fails the build that reads it.
func formatted(src []byte) []byte {
	if out, err := format.Source(src); err == nil {
		return out
	}
	return src
}

// header is the marker line every generated file starts with, naming what it
// is written from.
func header(comment, from string) string {
	return fmt.Sprintf("%s %s from %s; edits are lost, so edit the source and run task gen.", comment, Marker, from)
}

func goDecl(t *tree, d *decl) string {
	name := goName(d.Name)
	doc := goDoc(name, d)
	switch d.T.Kind {
	case kEnum:
		return goEnum(name, doc, d.T)
	case kObject:
		return goStruct(t, name, doc, d.T)
	case kArray:
		return doc + "type " + name + " []" + goType(d.T.Elem) + "\n"
	}
	return doc + "type " + name + " " + goType(d.T) + "\n"
}

// goDoc is the doc comment of a type: the schema's title or description, and
// the constraints a Go reader cannot see in the type.
func goDoc(name string, d *decl) string {
	say := firstLine(d.T.Desc)
	if say == "" {
		say = firstLine(d.Title)
	}
	if say == "" {
		say = "has the shape the schema gives it"
	}
	lines := []string{name + " is " + say + "."}
	if c := constraints(d.T); c != "" {
		lines = append(lines, "Constraints: "+c+".")
	}
	return comment("", lines)
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSuffix(s, ".")
}

func comment(indent string, lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(indent + "// " + l + "\n")
	}
	return b.String()
}

// constraints says in words what a Go type cannot hold of a schema.
func constraints(ty *typ) string {
	var out []string
	if ty.Pattern != "" {
		out = append(out, "matches "+ty.Pattern)
	}
	if ty.MinLen != nil {
		out = append(out, fmt.Sprintf("at least %g characters", *ty.MinLen))
	}
	if ty.Min != nil {
		out = append(out, fmt.Sprintf("at least %g", *ty.Min))
	}
	if ty.Max != nil {
		out = append(out, fmt.Sprintf("at most %g", *ty.Max))
	}
	if ty.MinItems != nil {
		out = append(out, fmt.Sprintf("at least %g items", *ty.MinItems))
	}
	if ty.MaxItems != nil {
		out = append(out, fmt.Sprintf("at most %g items", *ty.MaxItems))
	}
	if ty.Const != "" {
		out = append(out, "always "+ty.Const)
	}
	return strings.Join(out, ", ")
}

func goEnum(name, doc string, ty *typ) string {
	var b strings.Builder
	b.WriteString(doc + "type " + name + " string\n\n")
	b.WriteString("// The values of " + name + ".\nconst (\n")
	for _, v := range ty.Enum {
		fmt.Fprintf(&b, "\t%s%s %s = %q\n", name, goIdent(camel(v)), name, v)
	}
	b.WriteString(")\n")
	return b.String()
}

func goStruct(t *tree, name, doc string, ty *typ) string {
	var b strings.Builder
	b.WriteString(doc + "type " + name + " struct {\n")
	for _, f := range ty.Fields {
		var lines []string
		if f.Desc != "" {
			lines = append(lines, f.Desc+".")
		}
		if f.Decides != "" {
			lines = append(lines, "Decides: "+f.Decides+".")
		} else {
			lines = append(lines, "Display only.")
		}
		if f.Since != "" {
			lines = append(lines, "Since contract "+f.Since+".")
		}
		if f.HasDefault {
			lines = append(lines, "Absent means "+jsonOf(f.Default)+".")
		}
		if c := constraints(f.T); c != "" {
			lines = append(lines, "Constraints: "+c+".")
		}
		b.WriteString(comment("\t", lines))
		fmt.Fprintf(&b, "\t%s %s `json:\"%s%s\"`\n", goField(f.Key), goFieldType(t, f), f.Key, goTag(f))
	}
	b.WriteString("}\n")
	return b.String()
}

func jsonOf(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%q", fmt.Sprint(v))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// goPointer says whether an optional field is a pointer: an object, or a
// value whose default is not the zero value, since a zero value there would
// read as an answer.
func goPointer(t *tree, f field) bool {
	switch {
	case f.T.Nullable:
		return f.T.Kind != kAny
	case f.Required, f.T.Kind == kArray, f.T.Kind == kMap, f.T.Kind == kAny:
		return false
	case f.T.Kind == kRef && t.index[f.T.Ref].T.Kind == kObject:
		return true
	}
	return f.HasDefault && !zero(f.Default)
}

func zero(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case bool:
		return !x
	case int, int64, float64:
		return fmt.Sprint(x) == "0"
	case []any:
		return len(x) == 0
	}
	return false
}

func goTag(f field) string {
	if f.Required {
		return ""
	}
	return ",omitempty"
}

func goFieldType(t *tree, f field) string {
	s := goType(f.T)
	if goPointer(t, f) {
		return "*" + s
	}
	return s
}

func goType(ty *typ) string {
	switch ty.Kind {
	case kString, kConst:
		return "string"
	case kInteger:
		return "int"
	case kNumber:
		return "float64"
	case kBool:
		return "bool"
	case kArray:
		return "[]" + goType(ty.Elem)
	case kRef:
		return goName(ty.Ref)
	case kMap:
		return "canon.Object"
	}
	return "canon.Value"
}

// files lists the schema files of the tree in order.
func (t *tree) files() []string {
	var out []string
	for _, d := range t.Decls {
		if !slices.Contains(out, d.File) {
			out = append(out, d.File)
		}
	}
	return out
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
