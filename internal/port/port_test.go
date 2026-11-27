package port_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/port"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// parse reads one Go source file, comments kept.
func parse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "port.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// interfaces is the exported interface types a file declares, by name.
func interfaces(f *ast.File) map[string]*ast.InterfaceType {
	out := map[string]*ast.InterfaceType{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range g.Specs {
			if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.IsExported() {
				if it, ok := ts.Type.(*ast.InterfaceType); ok {
					out[ts.Name.Name] = it
				}
			}
		}
	}
	return out
}

// theSixteen is section 4 of the architecture, in its order, with Owned,
// FileNames, Trust, Steps and Bundles, and without Tool, whose
// finds and runs went to Steps.
func theSixteen() []string {
	return []string{"Tree", "Writer", "Lock", "Journal", "History", "Remote",
		"Sandbox", "Schema", "Clock", "Assets", "Terminal", "Owned", "FileNames", "Trust", "Steps", "Bundles"}
}

// theSeventeen is the fields of Ports: the sixteen of port.go, then Plugins
// of plugin.go, which the core reaches the plugins
// through.
func theSeventeen() []string { return append(theSixteen(), "Plugins") }

// source is the package's one file, read from disk.
func source(t *testing.T) *ast.File {
	t.Helper()
	b, err := os.ReadFile("port.go")
	if err != nil {
		t.Fatal(err)
	}
	return parse(t, string(b))
}

// extra is the interfaces of f that section 4 does not list, and the listed
// ones f lacks.
func extra(f *ast.File) (more, lacks []string) {
	got := interfaces(f)
	for name := range got {
		if !slices.Contains(theSixteen(), name) {
			more = append(more, name)
		}
	}
	for _, name := range theSixteen() {
		if _, ok := got[name]; !ok {
			lacks = append(lacks, name)
		}
	}
	slices.Sort(more)
	return more, lacks
}

// TestThePackageDeclaresExactlySixteenInterfaces: the interfaces are the
// sixteen of section 4 and nothing else, and Ports has a field for each,
// and for Plugins of plugin.go, each of the interface of the same name
// (arch.ports.15; twelve since release-pin-by-hash, thirteen with Owned,
// which took ReadOwned out of Tree, fourteen with FileNames, fifteen with
// Trust, sixteen with Steps, and seventeen with Bundles, and sixteen again
// when Tool went).
// Planted: a seventeenth interface, and one of the sixteen gone.
func TestThePackageDeclaresExactlySixteenInterfaces(t *testing.T) {
	t.Parallel()
	more, lacks := extra(source(t))
	if len(more) > 0 || len(lacks) > 0 {
		t.Errorf("port declares %v that section 4 does not list, and lacks %v", more, lacks)
	}
	if n := len(interfaces(source(t))); n != 16 {
		t.Errorf("port declares %d interfaces, want 16", n)
	}
	ty := reflect.TypeFor[port.Ports]()
	var fields []string
	for f := range ty.Fields() {
		fields = append(fields, f.Name)
		if f.Type.Kind() != reflect.Interface || f.Type.Name() != f.Name {
			t.Errorf("Ports.%s is %v, want the interface %s", f.Name, f.Type, f.Name)
		}
	}
	if want := theSeventeen(); !slices.Equal(fields, want) {
		t.Errorf("Ports has the fields %v, want %v", fields, want)
	}

	t.Run("a seventeenth is seen", func(t *testing.T) {
		t.Parallel()
		f := parse(t, "package port\ntype Tree interface{}\ntype Pigeon interface{}\n")
		more, _ := extra(f)
		if !slices.Equal(more, []string{"Pigeon"}) {
			t.Errorf("a seventeenth interface was not seen: %v", more)
		}
	})
	t.Run("a missing one is seen", func(t *testing.T) {
		t.Parallel()
		_, lacks := extra(parse(t, "package port\ntype Tree interface{}\n"))
		if len(lacks) != 15 {
			t.Errorf("lacks = %v, want the other fifteen", lacks)
		}
	})
}

// A contract says three things of a method: a unit or a form for what it
// takes and gives, what nil or empty or absent means, and the failure.
var (
	unitWord    = regexp.MustCompile(`(?i)\b(bytes?|slash-separated|absolute|columns?|bits|UTC|seconds|mebibytes|lines?|JSON|object name|sorted|in order|whole content|text form|relative to)\b`)
	absentWord  = regexp.MustCompile(`(?i)\b(nil|empty|none|no |absent|never|not an error|zero)\b`)
	failureWord = regexp.MustCompile(`(?i)\b(error|errors|fails|failure|refused|matches|never fails)\b`)
)

// lacking is what a method's comment does not say, "" when it says it all.
func lacking(doc string) string {
	var out []string
	if !unitWord.MatchString(doc) {
		out = append(out, "a unit or form")
	}
	if !absentWord.MatchString(doc) {
		out = append(out, "what nil or empty means")
	}
	if !failureWord.MatchString(doc) {
		out = append(out, "its failure")
	}
	return strings.Join(out, ", ")
}

// contracts is every problem with the doc comments of f: a type, method,
// variable or field of Ports without one, and a method whose comment does not
// state its contract (code.comments.4, store.seams.2).
func contracts(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range g.Specs {
			switch x := s.(type) {
			case *ast.TypeSpec:
				if !x.Name.IsExported() {
					continue
				}
				if g.Doc == nil && x.Doc == nil {
					out = append(out, x.Name.Name+" has no doc comment")
				}
				if it, ok := x.Type.(*ast.InterfaceType); ok {
					for _, m := range it.Methods.List {
						if m.Doc == nil {
							out = append(out, x.Name.Name+"."+m.Names[0].Name+" has no doc comment")
						} else if l := lacking(m.Doc.Text()); l != "" {
							out = append(out, x.Name.Name+"."+m.Names[0].Name+"'s comment does not say "+l)
						}
					}
				}
			case *ast.ValueSpec:
				if g.Doc == nil && x.Doc == nil {
					out = append(out, x.Names[0].Name+" has no doc comment")
				}
			}
		}
	}
	return out
}

// TestEveryExportedNameStatesItsContract: every type, every method and every
// typed failure of the package has a doc comment, and a method's says a unit
// or form, what nil or empty means, and the failure. Planted: a method with a
// bare comment, and one with none.
func TestEveryExportedNameStatesItsContract(t *testing.T) {
	t.Parallel()
	f := source(t)
	if len(interfaces(f)) == 0 {
		t.Fatal("no interface was read")
	}
	for _, p := range contracts(f) {
		t.Error(p)
	}

	t.Run("a bare comment is seen", func(t *testing.T) {
		t.Parallel()
		got := contracts(parse(t, "package port\n// T is one.\ntype T interface {\n\t// Do does it.\n\tDo()\n}\n"))
		if len(got) != 1 || !strings.Contains(got[0], "T.Do's comment does not say") {
			t.Errorf("contracts = %v", got)
		}
	})
	t.Run("a missing comment is seen", func(t *testing.T) {
		t.Parallel()
		got := contracts(parse(t, "package port\n// T is one.\ntype T interface {\n\tDo()\n}\n"))
		if len(got) != 1 || !strings.Contains(got[0], "T.Do has no doc comment") {
			t.Errorf("contracts = %v", got)
		}
	})
	t.Run("a whole contract is not flagged", func(t *testing.T) {
		t.Parallel()
		src := "package port\n// T is one.\ntype T interface {\n\t// Do answers the file's bytes, nil when empty, and an error when it fails.\n\tDo()\n}\n"
		if got := contracts(parse(t, src)); len(got) != 0 {
			t.Errorf("contracts = %v", got)
		}
	})
}

// reaching is each interface method of f that names another port in its
// parameters or results.
func reaching(f *ast.File) []string {
	var out []string
	for name, it := range interfaces(f) {
		for _, m := range it.Methods.List {
			ast.Inspect(m.Type, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && id.Name != name && slices.Contains(theSixteen(), id.Name) {
					out = append(out, name+"."+m.Names[0].Name+" names the port "+id.Name)
				}
				return true
			})
		}
	}
	slices.Sort(out)
	return out
}

// TestNoPortReadsAnother: a method takes and gives values of the model, never
// another port, so History and Remote are two interfaces that share nothing
// but an adapter package (arch.ports.6). Planted: a method that returns
// another port.
func TestNoPortReadsAnother(t *testing.T) {
	t.Parallel()
	if got := reaching(source(t)); len(got) > 0 {
		t.Errorf("ports reach one another: %v", got)
	}
	planted := parse(t, "package port\ntype Tree interface{}\ntype Remote interface {\n\tOpen() Tree\n}\n")
	if got := reaching(planted); !slices.Equal(got, []string{"Remote.Open names the port Tree"}) {
		t.Errorf("a port that returns another was not seen: %v", got)
	}
}

// TestEveryPortSaysIfAPlatformMustFillIt: each field of Ports is tagged
// required or optional, and only the sandbox, which a machine may lack, is
// optional (arch.ports.17).
func TestEveryPortSaysIfAPlatformMustFillIt(t *testing.T) {
	t.Parallel()
	var optional []string
	for f := range reflect.TypeFor[port.Ports]().Fields() {
		switch v, ok := f.Tag.Lookup("port"); {
		case !ok || (v != "required" && v != "optional"):
			t.Errorf("Ports.%s: tag port is %q, want required or optional", f.Name, v)
		case v == "optional":
			optional = append(optional, f.Name)
		}
	}
	if !slices.Equal(optional, []string{"Sandbox"}) {
		t.Errorf("the optional ports are %v, want [Sandbox]", optional)
	}
}
