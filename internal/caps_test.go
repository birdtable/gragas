package internal

// Every cap of the code, in every package, is held at its boundary by a test
// (tests.generated.7): one over, one at. A test says which cap it holds with a
// "boundary: <Name>" in the doc comment of a test function, and a cap with no
// such line in a test of its package is red, unless it is on the owed list,
// which only shrinks. A marker anywhere else proves a comment and no test, and
// is a finding of its own.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var capName = regexp.MustCompile(`^(Max|max)[A-Z]\w*$`)

// owed are the caps with no boundary test yet, each with what is missing, read
// from gate/caps-owed.txt. The file is protected and gate/baselines.yaml
// counts its lines, so the list only falls; a cap leaves it in the change that
// proves it, and the list is checked both ways, so a cap on it that has a
// test fails too.
func owed(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "gate", "caps-owed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return owedIn(string(b))
}

// owedIn is the caps a caps-owed.txt names: each line that is not blank or a
// comment, its name before two spaces and a # and its reason after.
func owedIn(text string) map[string]string {
	out := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, why, _ := strings.Cut(line, "  # ")
		out[strings.TrimSpace(name)] = why
	}
	return out
}

// capsOf are the caps declared in the non-test files of every package, as
// "pkg.Name".
func capsOf(all []source) []string {
	var out []string
	for _, s := range all {
		if s.test {
			continue
		}
		for _, d := range s.file.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				for _, n := range spec.(*ast.ValueSpec).Names {
					if capName.MatchString(n.Name) {
						out = append(out, s.pkg+"."+n.Name)
					}
				}
			}
		}
	}
	return out
}

var marker = regexp.MustCompile(`boundary: (\w+)`)

// testDocs are the comments that are the doc of a test function of s: a func
// whose name starts with Test, in a test file.
func testDocs(s source) map[*ast.CommentGroup]bool {
	out := map[*ast.CommentGroup]bool{}
	for _, d := range s.file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Doc != nil && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
			out[fn.Doc] = true
		}
	}
	return out
}

// held is the caps that a test function of their package names with a
// "boundary: <Name>" line in its doc comment.
func held(all []source) map[string]bool {
	out := map[string]bool{}
	for _, s := range all {
		if !s.test {
			continue
		}
		for g := range testDocs(s) {
			for _, m := range marker.FindAllStringSubmatch(g.Text(), -1) {
				out[s.pkg+"."+m[1]] = true
			}
		}
	}
	return out
}

// strays are the "boundary: <Name>" lines that are no test's: in a comment that
// is not the doc of a test function, so a marker proves a comment and no test.
func strays(all []source) []string {
	var out []string
	for _, s := range all {
		if !s.test {
			continue
		}
		docs := testDocs(s)
		for _, g := range s.file.Comments {
			if docs[g] {
				continue
			}
			for _, m := range marker.FindAllStringSubmatch(g.Text(), -1) {
				if !capName.MatchString(m[1]) {
					continue // prose such as "the boundary: a ...", and a marker for something that is no cap
				}
				out = append(out, s.path+": \"boundary: "+m[1]+"\" is not in the comment of a test function")
			}
		}
	}
	return out
}

// unproved are the findings: a cap with no boundary test and not owed, and a
// cap that is owed and has a test.
func unproved(all []source, owe map[string]string) []string {
	var out []string
	have := held(all)
	for _, c := range capsOf(all) {
		_, owes := owe[c]
		switch {
		case !have[c] && !owes:
			out = append(out, c+": no test holds it at its boundary (a comment \"boundary: Name\" in a test of its package)")
		case have[c] && owes:
			out = append(out, c+": has a boundary test and is still on the owed list")
		}
	}
	return out
}

func TestEveryCapIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	_, w := tree(t)
	all := sources(t, w)
	for _, f := range unproved(all, owed(t)) {
		t.Error(f)
	}
	for _, f := range strays(all) {
		t.Error(f)
	}
	known := map[string]bool{}
	for _, c := range capsOf(all) {
		known[c] = true
	}
	for c := range owed(t) {
		if !known[c] {
			t.Errorf("%s: owed but no such cap", c)
		}
	}
}

func TestACapConstantWithNoBoundaryTestFails(t *testing.T) {
	t.Parallel()
	cap := parse(t, "internal/host/tool", "plant.go", "package tool\n\nconst maxPlanted = 3\n")
	test := parse(t, "internal/host/tool", "plant_test.go", "package tool\n\n// boundary: maxOther\nfunc TestX() {}\n")
	if got := unproved([]source{cap, test}, nil); len(got) != 1 || !strings.Contains(got[0], "internal/host/tool.maxPlanted") {
		t.Fatalf("a planted cap with no test: %q", got)
	}
	test = parse(t, "internal/host/tool", "plant_test.go", "package tool\n\n// boundary: maxPlanted\nfunc TestX() {}\n")
	if got := unproved([]source{cap, test}, nil); len(got) != 0 {
		t.Fatalf("a planted cap with its test: %q", got)
	}
	if got := unproved([]source{cap, test}, map[string]string{"internal/host/tool.maxPlanted": "x"}); len(got) != 1 {
		t.Fatalf("a planted cap with a test and on the owed list: %q", got)
	}
}

func TestTheOwedListIsReadLineByLine(t *testing.T) {
	t.Parallel()
	got := owedIn("# a comment\n\ninternal/host/tool.maxValue  # has no test at its cap\n  internal/host/git.maxRev  # x\n")
	if len(got) != 2 || got["internal/host/tool.maxValue"] != "has no test at its cap" || got["internal/host/git.maxRev"] != "x" {
		t.Fatalf("owedIn = %v", got)
	}
}

func TestAMarkerThatIsNoTestsIsAFindingAndHoldsNoCap(t *testing.T) {
	t.Parallel()
	cap := parse(t, "internal/ordered", "plant.go", "package ordered\n\nconst maxPlanted = 3\n")
	// a marker in a loose comment, in a helper's doc and in a method's doc is no test's
	stray := parse(t, "internal/ordered", "plant_test.go", "package ordered\n\n// boundary: maxPlanted\n\n// helper holds boundary: maxPlanted\nfunc helper() {}\n\ntype T struct{}\n\n// TestX boundary: maxPlanted\nfunc (T) TestX() {}\n")
	if got := strays([]source{cap, stray}); len(got) != 3 {
		t.Errorf("markers that are no test's: %q, want 3", got)
	}
	if got := unproved([]source{cap, stray}, nil); len(got) != 1 || !strings.Contains(got[0], "internal/ordered.maxPlanted") {
		t.Errorf("a cap with only markers that are no test's: %q", got)
	}
	test := parse(t, "internal/ordered", "plant_test.go", "package ordered\n\n// TestX (boundary: maxPlanted): one over and one at.\nfunc TestX(t *testing.T) {}\n")
	if got := strays([]source{cap, test}); len(got) != 0 {
		t.Errorf("a marker in a test's doc: %q", got)
	}
	if got := unproved([]source{cap, test}, nil); len(got) != 0 {
		t.Errorf("a cap with a test: %q", got)
	}
}

func TestACapOutsideCoreAndHostIsFoundAndAHeldOneIsNot(t *testing.T) {
	t.Parallel()
	cap := parse(t, "internal/dev/scan", "plant.go", "package scan\n\nconst MaxPlanted = 3\n")
	if got := capsOf([]source{cap}); len(got) != 1 || got[0] != "internal/dev/scan.MaxPlanted" {
		t.Fatalf("capsOf = %q, want the cap of a package outside core and host", got)
	}
	if got := unproved([]source{cap}, nil); len(got) != 1 {
		t.Errorf("a planted cap outside core and host with no test: %q", got)
	}
}
