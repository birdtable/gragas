package pluginwire

import (
	"bytes"
	"fmt"
	"go/format"
	"slices"
	"strconv"
	"strings"
)

// Marker is what every generated file says in its first line; it is the
// marker of internal/dev/generate's files.
const Marker = "Written by gragas-generate"

// emitter writes the Go files.
type emitter struct {
	pats []string
}

// emit writes a file for each schema and the codec's.
func emit(r *reader, byFile map[string][]*typ) (map[string][]byte, error) {
	e := &emitter{}
	out := map[string][]byte{}
	for _, f := range Files() {
		var b bytes.Buffer
		fmt.Fprintf(&b, "// %s from %s%s; edits are lost, so edit the source and run task gen.\n\npackage plugin\n\nimport \"github.com/birdtable/gragas/internal/canon\"\n", Marker, SchemaDir, f)
		for _, t := range byFile[f] {
			switch t.kind {
			case kEnum:
				e.enum(&b, t)
			case kStruct:
				e.structure(&b, t)
			case kUnion:
				e.union(&b, t)
			case kString, kInt, kBool, kList, kAny, kObject:
				// An inline type has no declaration of its own.
			}
		}
		out[GoDir+strings.TrimSuffix(f, ".schema.json")+".gen.go"] = pretty(b.Bytes())
	}
	out[GoDir+"codec.gen.go"] = pretty([]byte(e.codec()))
	return out, nil
}

// pretty is src as gofmt writes it; source that does not parse is left as it
// is, and fails the build that reads it.
func pretty(src []byte) []byte {
	if out, err := format.Source(src); err == nil {
		return out
	}
	return src
}

// goType is the Go type of t.
func goType(t *typ) string {
	switch t.kind {
	case kString:
		return "string"
	case kInt:
		return "int"
	case kBool:
		return "bool"
	case kEnum, kStruct, kUnion:
		return t.name
	case kList:
		return "[]" + goType(t.elem)
	case kAny:
		return "canon.Value"
	case kObject:
		return "canon.Object"
	}
	return "?"
}

// pointed says whether an optional member of type t is held by a pointer: the
// kinds a nil slice, map or interface does not already say is absent.
func pointed(t *typ) bool {
	return t.kind != kList && t.kind != kAny && t.kind != kObject
}

// sentence is text as a sentence of a comment, with a full stop.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// enum writes an enum: its type, its values and Valid.
func (e *emitter) enum(b *bytes.Buffer, t *typ) {
	fmt.Fprintf(b, "\n// %s is one of the strings %s lists.\ntype %s string\n\nconst (\n", t.name, strings.Join(t.values, ", "), t.name)
	for _, v := range t.values {
		fmt.Fprintf(b, "\t// %s is %q.\n\t%s %s = %q\n", t.name+fieldName(v), v, t.name+fieldName(v), t.name, v)
	}
	fmt.Fprintf(b, ")\n\n// Valid says whether %s is one of the values.\nfunc (e %s) Valid() bool {\n\tswitch e {\n\tcase ", t.name, t.name)
	for i, v := range t.values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(t.name + fieldName(v))
	}
	b.WriteString(":\n\t\treturn true\n\t}\n\treturn false\n}\n")
}

// structure writes a struct: its type, ToCanon and FromCanon.
func (e *emitter) structure(b *bytes.Buffer, t *typ) {
	doc := sentence(t.doc)
	if doc == "" {
		doc = "is a message or part of one of plugin contract 1."
	} else {
		doc = "is " + doc
	}
	fmt.Fprintf(b, "\n// %s %s\ntype %s struct {\n", t.name, doc, t.name)
	for _, f := range t.fields {
		if f.constant != "" {
			continue
		}
		if d := sentence(f.doc); d != "" {
			fmt.Fprintf(b, "\t// %s is %s\n", f.name, d)
		}
		ty := goType(f.t)
		if !f.required && pointed(f.t) {
			ty = "*" + ty
		}
		fmt.Fprintf(b, "\t%s %s `json:%q`\n", f.name, ty, f.json)
	}
	b.WriteString("}\n")
	e.toCanon(b, t)
	e.fromCanon(b, t)
}

// toCanon writes ToCanon of a struct.
func (e *emitter) toCanon(b *bytes.Buffer, t *typ) {
	fmt.Fprintf(b, "\n// ToCanon is x as canon's value tree: a member that is optional and not set is\n// left out.\nfunc (x %s) ToCanon() canon.Value {\n\to := canon.Object{}\n", t.name)
	for _, f := range t.fields {
		switch {
		case f.constant != "":
			fmt.Fprintf(b, "\to[%q] = %s\n", f.json, f.constant)
		case f.required:
			fmt.Fprintf(b, "\to[%q] = %s\n", f.json, writeExpr(f.t, "x."+f.name, 0))
		case pointed(f.t):
			fmt.Fprintf(b, "\tif x.%s != nil {\n\t\to[%q] = %s\n\t}\n", f.name, f.json, writeExpr(f.t, "(*x."+f.name+")", 0))
		default:
			fmt.Fprintf(b, "\tif x.%s != nil {\n\t\to[%q] = %s\n\t}\n", f.name, f.json, writeExpr(f.t, "x."+f.name, 0))
		}
	}
	b.WriteString("\treturn o\n}\n")
}

// fromCanon writes FromCanon of a struct.
func (e *emitter) fromCanon(b *bytes.Buffer, t *typ) {
	fmt.Fprintf(b, "\n// FromCanon reads v, a document that passed canon.Decode, into x, and refuses\n// what the schema refuses that a type can hold: a member of another type, a\n// null, a member the object does not have when it is closed, a member it\n// requires and lacks, a value outside an enum, a pattern or a bound. The\n// *Fault names where, by JSON Pointer.\nfunc (x *%s) FromCanon(v canon.Value) error { return x.read(v, \"\") }\n", t.name)
	fmt.Fprintf(b, "\nfunc (x *%s) read(v canon.Value, p string) error {\n\to, err := object(v, p)\n\tif err != nil {\n\t\treturn err\n\t}\n", t.name)
	if t.closed {
		var ks []string
		for _, f := range t.fields {
			ks = append(ks, strconv.Quote(f.json))
		}
		fmt.Fprintf(b, "\tif err := known(o, p, %s); err != nil {\n\t\treturn err\n\t}\n", strings.Join(ks, ", "))
	}
	for _, g := range t.exactlyOne {
		var ks []string
		for _, k := range g {
			ks = append(ks, strconv.Quote(k))
		}
		fmt.Fprintf(b, "\tif err := exactlyOne(o, p, %s); err != nil {\n\t\treturn err\n\t}\n", strings.Join(ks, ", "))
	}
	if t.minProps > 0 {
		fmt.Fprintf(b, "\tif len(o) < %d {\n\t\treturn fault(p, \"has fewer than %d members\")\n\t}\n", t.minProps, t.minProps)
	}
	for _, w := range t.whens {
		var ks []string
		for _, k := range w.require {
			ks = append(ks, strconv.Quote(k))
		}
		fmt.Fprintf(b, "\tif err := needs(o, p, %q, canon.String(%q), %s); err != nil {\n\t\treturn err\n\t}\n", w.prop, w.is, strings.Join(ks, ", "))
	}
	for _, f := range t.fields {
		e.member(b, f)
	}
	b.WriteString("\treturn nil\n}\n")
}

// member writes the read of one member of a struct.
func (e *emitter) member(b *bytes.Buffer, f *field) {
	fmt.Fprintf(b, "\t{\n\t\tm, ok := o[%q]\n", f.json)
	if f.required {
		fmt.Fprintf(b, "\t\tif !ok {\n\t\t\treturn fault(p, \"has no member %s\")\n\t\t}\n", f.json)
	} else {
		b.WriteString("\t\tif ok {\n")
	}
	pe := fmt.Sprintf("pointer(p, %q)", f.json)
	switch {
	case f.constant != "":
		fmt.Fprintf(b, "\t\tif m != %s {\n\t\t\treturn fault(%s, \"is not %s\")\n\t\t}\n", f.constant, pe, strings.NewReplacer("canon.String(", "", "canon.Int(", "", ")", "", `"`, "").Replace(f.constant))
	case !f.required && pointed(f.t):
		fmt.Fprintf(b, "\t\t\tvar s %s\n", goType(f.t))
		e.read(b, f.t, "m", "s", pe, 1)
		fmt.Fprintf(b, "\t\t\tx.%s = &s\n", f.name)
	default:
		e.read(b, f.t, "m", "x."+f.name, pe, 1)
	}
	if !f.required {
		b.WriteString("\t\t}\n")
	}
	b.WriteString("\t}\n")
}

// pat is the name of the variable that holds a pattern.
func (e *emitter) pat(p string) string {
	i := slices.Index(e.pats, p)
	if i < 0 {
		e.pats = append(e.pats, p)
		i = len(e.pats) - 1
	}
	return "pattern" + strconv.Itoa(i)
}

// read writes the code that reads src, a canon.Value, as type t into dst, at
// the pointer pe, d levels down.
func (e *emitter) read(b *bytes.Buffer, t *typ, src, dst, pe string, d int) {
	sfx := strconv.Itoa(d)
	switch t.kind {
	case kString:
		re := "nil"
		if t.pattern != "" {
			re = e.pat(t.pattern)
		}
		fmt.Fprintf(b, "s%s, err := text(%s, %s, %d, %d, %s)\nif err != nil {\n\treturn err\n}\n", sfx, src, pe, t.minLen, t.maxLen, re)
		for _, p := range t.also {
			v := e.pat(p)
			fmt.Fprintf(b, "if !%s.MatchString(s%s) {\n\treturn fault(%s, \"does not match %%s\", %s)\n}\n", v, sfx, pe, v)
		}
		fmt.Fprintf(b, "%s = s%s\n", dst, sfx)
	case kInt:
		fmt.Fprintf(b, "n%s, err := whole(%s, %s, %d, %d)\nif err != nil {\n\treturn err\n}\n%s = n%s\n", sfx, src, pe, t.lo, t.hi, dst, sfx)
	case kBool:
		fmt.Fprintf(b, "t%s, err := truth(%s, %s)\nif err != nil {\n\treturn err\n}\n%s = t%s\n", sfx, src, pe, dst, sfx)
	case kEnum:
		fmt.Fprintf(b, "s%s, err := text(%s, %s, 0, -1, nil)\nif err != nil {\n\treturn err\n}\nif !%s(s%s).Valid() {\n\treturn fault(%s, \"is not one of %s\")\n}\n%s = %s(s%s)\n", sfx, src, pe, t.name, sfx, pe, strings.Join(t.values, ", "), dst, t.name, sfx)
	case kAny:
		fmt.Fprintf(b, "%s = %s\n", dst, src)
	case kObject:
		fmt.Fprintf(b, "o%s, err := object(%s, %s)\nif err != nil {\n\treturn err\n}\n%s = o%s\n", sfx, src, pe, dst, sfx)
	case kStruct, kUnion:
		fmt.Fprintf(b, "if err := %s.read(%s, %s); err != nil {\n\treturn err\n}\n", dst, src, pe)
	case kList:
		fmt.Fprintf(b, "a%s, err := array(%s, %s, %d, %d)\nif err != nil {\n\treturn err\n}\n%s = make(%s, len(a%s))\nfor i%s, e%s := range a%s {\n", sfx, src, pe, t.minItems, t.maxIts, dst, goType(t), sfx, sfx, sfx, sfx)
		e.read(b, t.elem, "e"+sfx, dst+"[i"+sfx+"]", "index("+pe+", i"+sfx+")", d+1)
		b.WriteString("}\n")
		switch {
		case t.unique && (t.elem.kind == kStruct || t.elem.kind == kUnion || t.elem.kind == kAny || t.elem.kind == kObject):
			fmt.Fprintf(b, "if err := distinctValues(%s, %s, func(e %s) canon.Value { return %s }); err != nil {\n\treturn err\n}\n", dst, pe, goType(t.elem), writeExpr(t.elem, "e", 0))
		case t.unique:
			fmt.Fprintf(b, "if err := distinct(%s, %s); err != nil {\n\treturn err\n}\n", dst, pe)
		}
	}
}

// writeExpr is the expression of the canon.Value of e, of type t.
func writeExpr(t *typ, e string, d int) string {
	switch t.kind {
	case kString, kEnum:
		return "canon.String(" + e + ")"
	case kInt:
		return "canon.Int(" + e + ")"
	case kBool:
		return "canon.Bool(" + e + ")"
	case kAny, kObject:
		return e
	case kStruct, kUnion:
		return e + ".ToCanon()"
	case kList:
		v := "x" + strconv.Itoa(d)
		return fmt.Sprintf("listOf(%s, func(%s %s) canon.Value { return %s })", e, v, goType(t.elem), writeExpr(t.elem, v, d+1))
	}
	return "nil"
}

// union writes a union: a struct of one pointer to each alternative, of which
// one is set.
func (e *emitter) union(b *bytes.Buffer, t *typ) {
	fmt.Fprintf(b, "\n// %s is one of %s, by the %s it holds; exactly one of its members is set.\ntype %s struct {\n", t.name, strings.Join(altNames(t), ", "), "alternative", t.name)
	for i, a := range t.alts {
		fmt.Fprintf(b, "\t// %s is set when the member is a %s.\n\t%s *%s\n", t.altFields[i], a.name, t.altFields[i], a.name)
	}
	b.WriteString("}\n")
	fmt.Fprintf(b, "\n// ToCanon is the alternative that is set; with none set it is null, which no\n// schema accepts.\nfunc (x %s) ToCanon() canon.Value {\n\tswitch {\n", t.name)
	for i := range t.alts {
		fmt.Fprintf(b, "\tcase x.%s != nil:\n\t\treturn x.%s.ToCanon()\n", t.altFields[i], t.altFields[i])
	}
	b.WriteString("\t}\n\treturn canon.Null{}\n}\n")
	fmt.Fprintf(b, "\n// FromCanon reads v into the alternative it is.\nfunc (x *%s) FromCanon(v canon.Value) error { return x.read(v, \"\") }\n", t.name)
	fmt.Fprintf(b, "\nfunc (x *%s) read(v canon.Value, p string) error {\n", t.name)
	if t.disc != "" {
		fmt.Fprintf(b, "\to, err := object(v, p)\n\tif err != nil {\n\t\treturn err\n\t}\n\tm, ok := o[%q]\n\tif !ok {\n\t\treturn fault(p, \"has no member %s\")\n\t}\n\tswitch m {\n", t.disc, t.disc)
		var is []string
		for i, a := range t.alts {
			c := a.constOf(t.disc)
			is = append(is, strings.NewReplacer("canon.String(", "", ")", "", `"`, "").Replace(c))
			fmt.Fprintf(b, "\tcase %s:\n\t\tx.%s = new(%s)\n\t\treturn x.%s.read(v, p)\n", c, t.altFields[i], a.name, t.altFields[i])
		}
		fmt.Fprintf(b, "\t}\n\treturn fault(pointer(p, %q), \"is not one of %s\")\n}\n", t.disc, strings.Join(is, ", "))
		return
	}
	b.WriteString("\tvar n int\n")
	for i, a := range t.alts {
		fmt.Fprintf(b, "\tif a := new(%s); a.read(v, p) == nil {\n\t\tx.%s = a\n\t\tn++\n\t}\n", a.name, t.altFields[i])
	}
	fmt.Fprintf(b, "\tif n != 1 {\n\t\t*x = %s{}\n\t\treturn fault(p, \"is not exactly one of %s\")\n\t}\n\treturn nil\n}\n", t.name, strings.Join(altNames(t), ", "))
}

// altNames are the Go names of a union's alternatives.
func altNames(t *typ) []string {
	var out []string
	for _, a := range t.alts {
		out = append(out, a.name)
	}
	return out
}

// constOf is the Go expression of the constant member json of a struct.
func (t *typ) constOf(json string) string {
	for _, f := range t.fields {
		if f.json == json {
			return f.constant
		}
	}
	return ""
}

// codec is codec.gen.go: the functions every type's code reads and writes
// with, and the patterns it checks.
// escapedBackslash is the regexp s with each escaped backslash (two of them)
// as \x5c.
func escapedBackslash(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == '\\' {
				b.WriteString(`\x5c`)
			} else {
				b.WriteString(s[i : i+2])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// goString is s as a Go string literal: raw when it can be, as a pattern reads
// best.
func goString(s string) string {
	// An escaped backslash is written as its code, \x5c: the same to the
	// regexp, and no path of backslashes in the source.
	s = escapedBackslash(s)
	if strings.ContainsAny(s, "\r\n") {
		return strconv.Quote(s)
	}
	// A backtick cannot sit in a raw string, so it is joined in.
	return "`" + strings.ReplaceAll(s, "`", "`+\"`\"+`") + "`"
}

func (e *emitter) codec() string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s from %s*.schema.json; edits are lost, so edit the source and run task gen.\n\npackage plugin\n\n", Marker, SchemaDir)
	b.WriteString(codecHead)
	if len(e.pats) > 0 {
		b.WriteString("\n// The patterns of the schemas.\nvar (\n")
		for i, p := range e.pats {
			fmt.Fprintf(&b, "\tpattern%d = regexp.MustCompile(%s)\n", i, goString(p))
		}
		b.WriteString(")\n")
	}
	return b.String()
}
