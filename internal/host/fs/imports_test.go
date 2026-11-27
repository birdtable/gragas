package fs_test

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestNoFileCallsTheSystem: host/fs reaches the
// system through go-files and the os package alone, so no file of the
// package imports syscall or golang.org/x/sys; the calls go-files lacked
// lived in os_unix.go until go-files gave them, and it went. The control:
// writer.go is seen to import go-files' trust, so the scan reads the imports
// it means to.
func TestNoFileCallsTheSystem(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "writer.go" && path == "github.com/birdtable/go-files/trust" {
				seen = true
			}
			if path == "syscall" || strings.HasPrefix(path, "golang.org/x/sys") {
				t.Errorf("%s imports %s: host/fs calls the system through go-files alone", name, path)
			}
		}
	}
	if !seen {
		t.Error("the control: writer.go is not seen to import go-files/trust, so the scan reads no import")
	}
}
