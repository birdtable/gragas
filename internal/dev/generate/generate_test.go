package generate_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/generate"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

const (
	schemas   = "../../host/assets/contract/schema/v1"
	consumers = "../../../contract/consumers"
)

func readDir(t *testing.T, dir, suffix string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out[e.Name()] = b
		}
	}
	return out
}

func realInput(t *testing.T) generate.Input {
	t.Helper()
	return generate.Input{Schemas: readDir(t, schemas, ".schema.json"), Consumers: readDir(t, consumers, ".json")}
}

// Every schema makes a Go file and a TypeScript file, each marked as
// generated, and the index exports every TypeScript file.
func TestEverySchemaMakesAMarkedGoFileAndAZodFile(t *testing.T) {
	t.Parallel()
	in := realInput(t)
	made, err := generate.Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	for name := range in.Schemas {
		stem := strings.TrimSuffix(name, ".schema.json")
		goFile := generate.GoDir + strings.ReplaceAll(stem, "-", "_") + ".gen.go"
		tsFile := generate.TSDir + stem + ".gen.ts"
		for _, p := range []string{goFile, tsFile} {
			b, ok := made[p]
			if !ok {
				t.Errorf("%s makes no %s", name, p)
				continue
			}
			if !strings.Contains(strings.SplitN(string(b), "\n", 2)[0], generate.Marker) {
				t.Errorf("%s does not start with the generated marker", p)
			}
		}
		if idx := string(made[generate.TSDir+"index.ts"]); !strings.Contains(idx, `"./`+stem+`.gen.js"`) {
			t.Errorf("the index does not export %s", stem)
		}
	}
}

// codecImports are all the codec file may import: formatting and canon, so
// nothing it holds reaches the file system, the network or a process.
func codecImports() []string {
	return []string{"fmt", "slices", "strconv", "strings", "github.com/birdtable/gragas/internal/canon"}
}

// codecMethods are the methods a schema's file may give a type it declares.
func codecMethods() []string { return []string{"ToCanon", "FromCanon", "fromAt"} }

// The generated Go is types, constants and the codec (gragas#563): a schema's
// file declares types and constants, imports canon alone, and gives only the
// types it declares their ToCanon, FromCanon and fromAt; the codec file holds
// functions and the one generic constraint, and imports formatting and canon
// alone. No file declares a variable, a func of its own or an init. Planted
// failures: a var, a method on a type of another file, a method of another
// name, an import of os, a function in a schema's file.
func TestTheGeneratedGoDeclaresTypesAndConstantsOnly(t *testing.T) {
	t.Parallel()
	made, err := generate.Generate(realInput(t))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for p, b := range made {
		if !strings.HasSuffix(p, ".gen.go") {
			continue
		}
		seen++
		f, err := parser.ParseFile(token.NewFileSet(), p, b, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s does not parse: %v", p, err)
		}
		for _, why := range outsideTheModel(p, f) {
			t.Errorf("%s %s", p, why)
		}
	}
	if seen == 0 || made[generate.CodecFile] == nil {
		t.Fatal("no Go file or no codec file was made")
	}
}

// outsideTheModel lists what the generated file p declares or imports past
// types, constants and the codec.
func outsideTheModel(p string, f *ast.File) []string {
	codec := p == generate.CodecFile
	allowed := []string{"github.com/birdtable/gragas/internal/canon"}
	if codec {
		allowed = codecImports()
	}
	var out []string
	for _, im := range f.Imports {
		if path := strings.Trim(im.Path.Value, `"`); !slices.Contains(allowed, path) {
			out = append(out, "imports "+path)
		}
	}
	declared := map[string]bool{}
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
			for _, s := range g.Specs {
				declared[s.(*ast.TypeSpec).Name.Name] = true
			}
		}
	}
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if why := badFunc(codec, x, declared); why != "" {
				out = append(out, why)
			}
		case *ast.GenDecl:
			if x.Tok == token.VAR || (codec && x.Tok == token.TYPE && !onlyReader(x)) {
				out = append(out, "declares a "+x.Tok.String())
			}
		}
	}
	return out
}

// badFunc says what is wrong with a func the generated file declares: in the
// codec file a method or an init, in a schema's file anything but a codec
// method of a type the file declares.
func badFunc(codec bool, x *ast.FuncDecl, declared map[string]bool) string {
	name := x.Name.Name
	if codec {
		if x.Recv != nil || name == "init" {
			return "declares the method or init " + name
		}
		return ""
	}
	if x.Recv == nil {
		return "declares the function " + name
	}
	recv := x.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	id, ok := recv.(*ast.Ident)
	if !ok || !declared[id.Name] || !slices.Contains(codecMethods(), name) {
		return "declares the method " + name + " outside the codec of its own types"
	}
	return ""
}

// onlyReader says whether a type declaration of the codec file is the
// generic constraint reader and nothing else.
func onlyReader(g *ast.GenDecl) bool {
	return len(g.Specs) == 1 && g.Specs[0].(*ast.TypeSpec).Name.Name == "reader"
}

// Running the generator twice gives the same bytes.
func TestTheGeneratorIsDeterministic(t *testing.T) {
	t.Parallel()
	a, err := generate.Generate(realInput(t))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		b, err := generate.Generate(realInput(t))
		if err != nil {
			t.Fatal(err)
		}
		for p, want := range a {
			if string(b[p]) != string(want) {
				t.Fatalf("%s differs between two runs", p)
			}
		}
	}
}

const common = `{"$id":"common.schema.json","$defs":{"slug":{"type":"string","pattern":"^[a-z]+$"},"color":{"enum":["red","blue"],"default":"red"}}}`

func one(entity string) generate.Input {
	return generate.Input{Schemas: map[string][]byte{
		"common.schema.json":       []byte(common),
		"entity-thing.schema.json": []byte(entity),
	}}
}

// An optional field whose default is not the zero value is a pointer, so an
// absent field and a zero one are told apart; one whose default is the zero
// value is a plain value.
func TestAnOptionalFieldWithANonZeroDefaultIsAPointer(t *testing.T) {
	t.Parallel()
	made, err := generate.Generate(one(`{"title":"A thing","type":"object","additionalProperties":false,"required":["name"],"properties":{
	  "name":{"$ref":"common.schema.json#/$defs/slug","x-introduced":"1.0"},
	  "tint":{"$ref":"common.schema.json#/$defs/color","x-introduced":"1.0","default":"red"},
	  "note":{"type":"string","x-introduced":"1.0","default":""},
	  "live":{"type":"boolean","x-introduced":"1.0","default":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := string(made[generate.GoDir+"entity_thing.gen.go"])
	for _, want := range []string{"Tint *Color `json:\"tint,omitempty\"`", "Note string `json:\"note,omitempty\"`", "Live *bool `json:\"live,omitempty\"`", "Name Slug `json:\"name\"`"} {
		if !strings.Contains(got, want) {
			t.Errorf("the Go file lacks %s:\n%s", want, got)
		}
	}
	ts := string(made[generate.TSDir+"entity-thing.gen.ts"])
	for _, want := range []string{`tint: Color.default("red")`, `live: z.boolean().default(true)`, "z.strictObject({"} {
		if !strings.Contains(ts, want) {
			t.Errorf("the zod file lacks %s:\n%s", want, ts)
		}
	}
}

// A schema outside what the generators read is refused with the reason, and
// never skipped over.
func TestASchemaOutsideWhatTheGeneratorReadsIsRefused(t *testing.T) {
	t.Parallel()
	for name, entity := range map[string]string{
		"an open object":   `{"type":"object","properties":{"a":{"type":"string"}}}`,
		"an unknown type":  `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"null"}}}`,
		"a foreign ref":    `{"type":"object","additionalProperties":false,"properties":{"a":{"$ref":"https://example.org/x.json"}}}`,
		"an unlisted name": `{"type":"object","additionalProperties":false,"properties":{"a":{"$ref":"common.schema.json#/$defs/nope"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := generate.Generate(one(entity)); err == nil {
				t.Error("the schema was taken")
			}
		})
	}
}

// Two schemas that refer to one another cannot be written as zod, so the
// circle is told.
func TestACircleOfReferencesIsRefused(t *testing.T) {
	t.Parallel()
	in := generate.Input{Schemas: map[string][]byte{
		"common.schema.json":   []byte(common),
		"entity-a.schema.json": []byte(`{"type":"object","additionalProperties":false,"properties":{"b":{"$ref":"entity-b.schema.json"}}}`),
		"entity-b.schema.json": []byte(`{"type":"object","additionalProperties":false,"properties":{"a":{"$ref":"entity-a.schema.json"}}}`),
	}}
	if _, err := generate.Generate(in); err == nil || !strings.Contains(err.Error(), "circle") {
		t.Errorf("err = %v, want a circle", err)
	}
}

// The consumer's list is held to the schemas: a field the schema lacks, and
// one written as a list that is not one, fail generating (arch.ports.7).
func TestAConsumerFieldTheSchemaLacksFailsGenerating(t *testing.T) {
	t.Parallel()
	in := realInput(t)
	if _, err := generate.Generate(in); err != nil {
		t.Fatalf("setup: %v", err)
	}
	in.Consumers = map[string][]byte{"explorer.json": []byte(`{"consumer":"explorer","reads":[{"document":"bundle","fields":["rules[].id","rules[].colour","released[]","nothing"]}]}`)}
	_, err := generate.Generate(in)
	if err == nil {
		t.Fatal("the consumer list was taken")
	}
	for _, f := range []string{"rules[].colour", "released[]", "nothing"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("err does not name %s: %v", f, err)
		}
	}
	if strings.Contains(err.Error(), "rules[].id,") {
		t.Errorf("err names a field that is there: %v", err)
	}
}

// The explorer's list is written as a constant of the contract package.
func TestTheExplorersFieldListIsWritten(t *testing.T) {
	t.Parallel()
	made, err := generate.Generate(realInput(t))
	if err != nil {
		t.Fatal(err)
	}
	got := string(made[generate.TSDir+"explorer-reads.gen.ts"])
	if !strings.Contains(got, "export const explorerReads = {") || !strings.Contains(got, `"rules[].id"`) {
		t.Errorf("the reads file is:\n%s", got)
	}
	if !slices.Contains(strings.Split(string(made[generate.TSDir+"index.ts"]), "\n"), `export * from "./explorer-reads.gen.js";`) {
		t.Error("the index does not export the reads")
	}
}

// A field that takes a shape or null is a pointer in Go and nullable in zod,
// with null as its default: null is a value, so an absent field is never
// given a standing it does not have. A branch other than null is refused.
func TestAFieldOfAShapeOrNullIsAPointerAndNullable(t *testing.T) {
	t.Parallel()
	made, err := generate.Generate(one(`{"title":"A thing","type":"object","additionalProperties":false,"required":["name"],"properties":{
	  "name":{"$ref":"common.schema.json#/$defs/slug","x-introduced":"1.0"},
	  "tint":{"anyOf":[{"$ref":"common.schema.json#/$defs/color"},{"type":"null"}],"x-introduced":"1.0","default":null}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(made[generate.GoDir+"entity_thing.gen.go"]); !strings.Contains(got, "Tint *Color `json:\"tint,omitempty\"`") {
		t.Errorf("the Go file lacks a pointer for tint:\n%s", got)
	}
	if ts := string(made[generate.TSDir+"entity-thing.gen.ts"]); !strings.Contains(ts, "tint: Color.nullable().default(null)") {
		t.Errorf("the zod file lacks the nullable default:\n%s", ts)
	}
	_, err = generate.Generate(one(`{"title":"A thing","type":"object","additionalProperties":false,"properties":{
	  "x":{"anyOf":[{"type":"string"},{"type":"integer"}]}}}`))
	if err == nil || !strings.Contains(err.Error(), "anyOf") {
		t.Errorf("an anyOf of two shapes: err %v, want one naming anyOf", err)
	}
}

// The list of what the contract package exports is written with it, for the
// surface listing, which links no generator.
func TestTheExportsFileNamesEachDeclarationAndEachConsumersReads(t *testing.T) {
	t.Parallel()
	made, err := generate.Generate(realInput(t))
	if err != nil {
		t.Fatal(err)
	}
	got := string(made[generate.ExportsFile])
	for _, want := range []string{`"name": "Finding"`, `"name": "BundleDocument"`, `"name": "explorerReads"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %s", generate.ExportsFile, want)
		}
	}
}

// A default its own shape rejects is refused, by the name of the field: ""
// where the shape needs a character, {} where the object has a required
// field, a value outside an enum. A null default of a field that takes null,
// and a default the shape takes, pass (the control).
func TestADefaultItsShapeRejectsIsRefusedByFieldName(t *testing.T) {
	t.Parallel()
	wrap := func(field string) string {
		return `{"title":"A thing","type":"object","additionalProperties":false,"properties":{` + field + `}}`
	}
	for name, tc := range map[string]struct{ field, want string }{
		"empty string":  {`"note":{"type":"string","minLength":1,"default":""}`, "Thing.note"},
		"outside enum":  {`"tint":{"$ref":"common.schema.json#/$defs/color","default":"green"}`, "Thing.tint"},
		"empty object":  {`"inner":{"type":"object","additionalProperties":false,"required":["a"],"properties":{"a":{"type":"string"}},"default":{}}`, "Thing.inner"},
		"null not null": {`"note":{"type":"string","default":null}`, "Thing.note"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := generate.Generate(one(wrap(tc.field)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err %v, want one naming %s", err, tc.want)
			}
		})
	}
	for _, ok := range []string{
		`"note":{"type":"string","default":""}`,
		`"tint":{"$ref":"common.schema.json#/$defs/color","default":"blue"}`,
		`"live":{"anyOf":[{"$ref":"common.schema.json#/$defs/slug"},{"type":"null"}],"default":null}`,
	} {
		t.Run(ok, func(t *testing.T) {
			t.Parallel()
			if _, err := generate.Generate(one(wrap(ok))); err != nil {
				t.Errorf("%s: the control fails: %v", ok, err)
			}
		})
	}
}

// TestEveryGoFieldSaysWhatItDecidesOrThatItIsDisplayOnly: a field whose
// property names a decision in x-decides says "Decides:" and that decision,
// and every other field says "Display only.", so no field of the contract's
// types is unmarked and none decides unless its schema says so (arch.ports.19,
// A46). Planted failure: the mark left out, or written on no field.
func TestEveryGoFieldSaysWhatItDecidesOrThatItIsDisplayOnly(t *testing.T) {
	t.Parallel()
	made, err := generate.Generate(one(`{"title":"A thing","type":"object","additionalProperties":false,"required":["name","digest"],"properties":{
	  "name":{"$ref":"common.schema.json#/$defs/slug","x-introduced":"1.0"},
	  "digest":{"type":"string","x-introduced":"1.0","x-decides":"whether the thing is the one listed"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := string(made[generate.GoDir+"entity_thing.gen.go"])
	for _, want := range []string{"\t// Display only.\n\t// Since contract 1.0.\n\tName Slug", "\t// Decides: whether the thing is the one listed.\n\t// Since contract 1.0.\n\tDigest string"} {
		if !strings.Contains(got, want) {
			t.Errorf("the Go file lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Decides: whether the thing is the one listed.\n\t// Display only.") {
		t.Errorf("a deciding field is also marked display only:\n%s", got)
	}
}

// TestAnXDecidesThatSaysNothingIsRefused: a mark that names no decision is
// refused with the field's key, never written as a field that decides
// nothing in particular. Planted failure: the check of x-decides removed.
func TestAnXDecidesThatSaysNothingIsRefused(t *testing.T) {
	t.Parallel()
	for name, decides := range map[string]string{
		"an empty decision":         `""`,
		"a decision of blanks":      `"  "`,
		"a decision that is a flag": `true`,
		"a decision that is a list": `["a"]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := generate.Generate(one(`{"type":"object","additionalProperties":false,"properties":{"digest":{"type":"string","x-decides":` + decides + `}}}`))
			if err == nil || !strings.Contains(err.Error(), "digest") || !strings.Contains(err.Error(), "x-decides must name") {
				t.Errorf("err = %v, want digest refused for its x-decides", err)
			}
		})
	}
}

// kinds is a common schema of every kind the Go codec writes a declaration
// of, beside the object: a string, a whole number, an enum, a list, an object
// of any members, and a name for another declaration.
const kinds = `{"$id":"common.schema.json","$defs":{
  "slug":{"type":"string","pattern":"^[a-z]+$"},
  "count":{"type":"integer"},
  "color":{"enum":["red","blue"]},
  "slugs":{"type":"array","items":{"$ref":"#/$defs/slug"}},
  "bag":{"type":"object"},
  "name":{"$ref":"#/$defs/slug"}}}`

// TestTheCodecOfEveryKindTypeChecks: the methods and helpers the generator
// writes for every kind of declaration and field type-check as Go, with the
// codec file and canon, so a kind no real schema has yet still makes code
// that builds. Planted failure: a reader or writer named wrong for one kind.
func TestTheCodecOfEveryKindTypeChecks(t *testing.T) {
	t.Parallel()
	in := generate.Input{Schemas: map[string][]byte{
		"common.schema.json": []byte(kinds),
		"entity-thing.schema.json": []byte(`{"title":"A thing","type":"object","additionalProperties":false,"required":["name","grid","any"],"properties":{
		  "name":{"$ref":"common.schema.json#/$defs/name"},
		  "tags":{"$ref":"common.schema.json#/$defs/slugs"},
		  "bag":{"$ref":"common.schema.json#/$defs/bag"},
		  "n":{"$ref":"common.schema.json#/$defs/count"},
		  "tint":{"$ref":"common.schema.json#/$defs/color","default":"red"},
		  "grid":{"type":"array","items":{"type":"array","items":{"type":"number"}}},
		  "maybe":{"anyOf":[{"type":"array","items":{"type":"boolean"}},{"type":"null"}]},
		  "extra":{"anyOf":[{"type":"object"},{"type":"null"}]},
		  "any":{},
		  "inner":{"type":"object","additionalProperties":false,"properties":{"at":{"type":"integer","default":3}}}}}`),
	}}
	made, err := generate.Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, p := range []string{generate.CodecFile, generate.GoDir + "common.gen.go", generate.GoDir + "entity_thing.gen.go"} {
		f, err := parser.ParseFile(fset, p, made[p], 0)
		if err != nil {
			t.Fatalf("%s does not parse: %v\n%s", p, err, made[p])
		}
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	if _, err := conf.Check("github.com/birdtable/gragas/internal/model/contract", fset, files, nil); err != nil {
		t.Errorf("the generated codec does not type-check: %v\n%s", err, made[generate.GoDir+"entity_thing.gen.go"])
	}
}

// TestADeclarationOfAnyValueIsRefusedByTheCodec: a declaration that is any
// JSON value is an interface in Go, which takes no method, so the generator
// refuses it by name rather than write code that does not build.
func TestADeclarationOfAnyValueIsRefusedByTheCodec(t *testing.T) {
	t.Parallel()
	in := one(`{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string"}}}`)
	in.Schemas["common.schema.json"] = []byte(`{"$id":"common.schema.json","$defs":{"loose":{}}}`)
	if _, err := generate.Generate(in); err == nil || !strings.Contains(err.Error(), "Loose is any JSON value") {
		t.Errorf("err = %v, want Loose refused", err)
	}
}
