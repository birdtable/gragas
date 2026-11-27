package gocode

import (
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// lineDirective is a comment the go tool reads as a line directive: //line
// or /*line, then a space and the place it names.
var lineDirective = regexp.MustCompile(`^(//|/\*)line\s`)

// lineDirectives are the line directives of Go sources, by slash path from
// the module's root with their text, each as one finding naming its file and
// its line as the file holds it. A directive moves every finding below it to
// the name it gives: below the package clause, one naming an x_test.go puts
// a product file's findings under the test files' exclusions, and the
// push's changed-files filter drops a finding moved to an unchanged name;
// above it, it changes which depguard lists the file comes under. None
// is allowed: a directive needs a
// decision record first, and there is none. The scanner reads comments
// only, so a string that holds the text is no directive.
func lineDirectives(sources map[string]string) []string {
	var out []string
	for _, name := range sortedKeys(sources) {
		fset := token.NewFileSet()
		src := []byte(sources[name])
		file := fset.AddFile(name, -1, len(src))
		var bad []string
		var s scanner.Scanner
		s.Init(file, src, func(pos token.Position, msg string) {
			bad = append(bad, name+": cannot be scanned at line "+strconv.Itoa(pos.Line)+": "+msg)
		}, scanner.ScanComments)
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok == token.COMMENT && lineDirective.MatchString(lit) {
				// PositionFor with no adjustment: the line as the file
				// holds it, not the one a directive above made up.
				at := fset.PositionFor(pos, false)
				out = append(out, name+":"+strconv.Itoa(at.Line)+": holds a line directive, which moves every lint finding below it to another file's name and past its bans: none is allowed without a decision record")
			}
		}
		out = append(out, bad...)
	}
	return out
}

// everyGoFile reads every Go file of the tree, tests and testdata included,
// where allGoSources leaves testdata out: a directive in any of them reaches
// the lint. Folders whose name starts with a dot (.git, .tools) and
// node_modules folders hold no tracked Go file.
func everyGoFile(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNoGoFileHoldsALineDirective(t *testing.T) {
	t.Parallel()
	src := everyGoFile(t)
	if len(src) < 1000 {
		t.Fatalf("%d Go files read: the tree holds over a thousand, so the walk cannot be trusted", len(src))
	}
	for _, want := range []string{"cmd/gragas/main.go", "internal/repo/gocode/linedirective_test.go"} {
		if _, ok := src[want]; !ok {
			t.Fatalf("%s was not read, so the walk cannot be trusted (%d files read)", want, len(src))
		}
	}
	if !slices.ContainsFunc(sortedKeys(src), func(n string) bool { return strings.Contains(n, "/testdata/") }) {
		t.Fatal("no Go file under a testdata folder was read: the walk leaves them out")
	}
	for _, f := range lineDirectives(src) {
		t.Error(f)
	}
}

func TestALineDirectiveFailsAboveAndBelowThePackageClause(t *testing.T) {
	t.Parallel()
	// The directives are joined at run time, so this file holds none.
	slash, block := "//"+"line", "/*"+"line"
	for _, tc := range []struct{ name, src, want string }{
		{"below the clause, naming a test file", "package p\n\n" + slash + " x_test.go:1\nfunc F() {}\n", "a.go:3: holds a line directive"},
		{"above the clause", slash + " other/b.go:1\npackage p\n", "a.go:1: holds a line directive"},
		{"a block one inside a line", "package p\n\nvar v = 1 " + block + " x_test.go:9:1*/\n", "a.go:3: holds a line directive"},
		{"the second of two, at its own line", "package p\n" + slash + " z.go:90\n\n" + slash + " y.go:1\n", "a.go:4: holds a line directive"},
		{"a file that does not scan", "package p\n\nvar s = \"open\n", "a.go: cannot be scanned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fires(t, lineDirectives(map[string]string{"a.go": tc.src}), tc.want)
		})
	}
	for _, src := range []string{
		"package p\n\nvar s = \"" + slash + " x_test.go:1\"\n",
		"package p\n\n// a comment about the " + slash + " directive\n",
		"package p\n\n" + slash + "s of code\n",
	} {
		if got := lineDirectives(map[string]string{"a.go": src}); len(got) != 0 {
			t.Errorf("%q is no directive and was refused: %v", src, got)
		}
	}
}
