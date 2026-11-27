package model_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// unmarked is every field of a struct in src whose doc comment says neither
// what it decides nor that it is display only (arch.ports.19).
func unmarked(t *testing.T, name, src string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, fl := range st.Fields.List {
			doc := strings.ToLower(fl.Doc.Text())
			if strings.Contains(doc, "decides:") || strings.Contains(doc, "display only") {
				continue
			}
			for _, id := range fl.Names {
				out = append(out, ts.Name.Name+"."+id.Name)
			}
		}
		return false
	})
	return out
}

// TestEveryFieldOfTheAdapterSaysWhatItDecides: an adapter may come from a
// rulebook or a project, so each field of its types says the one decision it
// drives or that it is display only (arch.ports.19, section 4.3).
func TestEveryFieldOfTheAdapterSaysWhatItDecides(t *testing.T) {
	t.Parallel()
	// What the adapter reads and what Observe answers is data from a program
	// or a rulebook the engine does not control too.
	for _, name := range []string{"adapter.go", "observed.go", "observation.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := unmarked(t, name, string(src)); len(got) > 0 {
			t.Errorf("fields of %s with no decides or display-only mark: %v", name, got)
		}
	}
	planted := "package model\n\ntype A struct {\n\t// B decides: c.\n\tB int\n\t// D is a field.\n\tD int\n\t// E is display only.\n\tE string\n}\n"
	if got := unmarked(t, "planted.go", planted); !slices.Equal(got, []string{"A.D"}) {
		t.Errorf("the planted struct gave %v, want [A.D]", got)
	}
}
