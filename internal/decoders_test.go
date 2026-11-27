package internal

import (
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// decoderCalls are the calls that decode input the program did not write
// (tests.generated.1): the YAML and JSON decoders of the two libraries the
// engine reads with, by import path and function name.
func decoderCalls() map[string][]string {
	return map[string][]string{
		"encoding/json":      {"Unmarshal", "NewDecoder"},
		"go.yaml.in/yaml/v3": {"Unmarshal", "NewDecoder"},
	}
}

// unfuzzed is the packages that decode and have no fuzz target, each with why
// it is not a parser of foreign input: the rows of gate/exemptions.yaml for
// tests.generated.1, whose path is the package's folder. That file is protected
// and the gate fails a row past its expiry, so the list is held and the date
// is not read here. A package leaves it when it gets a target.
func unfuzzed(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "gate", "exemptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return exemptPackages(t, b)
}

// exemptPackages is the folders, and why, of the exemption rows for
// tests.generated.1 in the file b.
func exemptPackages(t *testing.T, b []byte) map[string]string {
	t.Helper()
	v, err := ordered.ParseYAML(b)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := v.(*ordered.Map).Val("exemptions").([]any)
	out := map[string]string{}
	for _, r := range rows {
		m, ok := r.(*ordered.Map)
		if !ok || m.Val("rule") != "tests.generated.1" {
			continue
		}
		p, _ := m.Val("path").(string)
		why, _ := m.Val("reason").(string)
		out[strings.TrimSuffix(p, "/")] = why
	}
	return out
}

// siteFile is the list of decode call sites, each with the fuzz target that
// reaches it (a test fixture, not a gate file).
const siteFile = "internal/testdata/decoders.txt"

// siteRow is one row of siteFile: a file, the function that holds a decode
// call in it, and the fuzz target of the file's package that reaches it, or
// "-" with why there is none.
type siteRow struct {
	file, fn, target, why string
}

// siteRows is the rows of the list b; a row of fewer than three fields is
// refused.
func siteRows(t *testing.T, b []byte) []siteRow {
	t.Helper()
	var out []siteRow
	for n, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.SplitN(line, "\t", 4)
		if len(f) < 3 {
			t.Fatalf("%s:%d: %q is not a file, a function and a target, by tabs", siteFile, n+1, line)
		}
		r := siteRow{file: f[0], fn: f[1], target: f[2]}
		if len(f) == 4 {
			r.why = f[3]
		}
		out = append(out, r)
	}
	return out
}

// listedSites is the rows of siteFile in the tree.
func listedSites(t *testing.T) []siteRow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(siteFile)))
	if err != nil {
		t.Fatal(err)
	}
	return siteRows(t, b)
}

// holder is the name of the function d as the list writes it: a method as
// Type.name, the receiver's star and type arguments dropped.
func holder(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	x := d.Recv.List[0].Type
	if s, ok := x.(*ast.StarExpr); ok {
		x = s.X
	}
	switch g := x.(type) {
	case *ast.IndexExpr:
		x = g.X
	case *ast.IndexListExpr:
		x = g.X
	}
	if id, ok := x.(*ast.Ident); ok {
		return id.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

// decodeCalls is each decode call of s, as its function, a tab and the call
// (json.Unmarshal), in source order; a call outside any function is held by
// "(package)".
func decodeCalls(s source) []string {
	local := map[string]string{}
	for _, im := range s.file.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil || decoderCalls()[p] == nil {
			continue
		}
		name := "json"
		if strings.HasPrefix(p, "go.yaml.in/yaml") {
			name = "yaml"
		}
		if im.Name != nil {
			name = im.Name.Name
		}
		local[name] = p
	}
	var out []string
	for _, d := range s.file.Decls {
		fn := "(package)"
		if f, ok := d.(*ast.FuncDecl); ok {
			fn = holder(f)
		}
		ast.Inspect(d, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && slices.Contains(decoderCalls()[local[id.Name]], sel.Sel.Name) {
				out = append(out, fn+"\t"+id.Name+"."+sel.Sel.Name)
			}
			return true
		})
	}
	return out
}

// decoders is what is wrong with the decode call sites of all against the list
// rows (siteFile) and the exempt packages: a call on no row, a row whose
// target is no func Fuzz of the file's package, or whose function decodes
// nothing, a row listed twice, a row of none with no reason, or in a package
// with no fuzz target at all (only gate/exemptions.yaml, protected and
// expiring, waives a whole package); and an exempt package that has a target,
// or decodes nothing, or has rows, so both lists stay true. A package that
// already has a target does not let a new decode call through: each call is
// held by its own row.
func decoders(all []source, exempt map[string]string, rows []siteRow) []string {
	calls := map[string][]string{} // by package, file: function: call
	sites := map[string]bool{}     // file, tab, function
	fuzzed := map[string]map[string]bool{}
	for _, s := range all {
		if s.test {
			for _, d := range s.file.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Fuzz") {
					if fuzzed[s.pkg] == nil {
						fuzzed[s.pkg] = map[string]bool{}
					}
					fuzzed[s.pkg][fn.Name.Name] = true
				}
			}
			continue
		}
		for _, c := range decodeCalls(s) {
			fn, call, _ := strings.Cut(c, "\t")
			calls[s.pkg] = append(calls[s.pkg], s.path+": "+fn+": "+call)
			sites[s.path+"\t"+fn] = true
		}
	}
	listed := map[string]bool{}
	var out []string
	for _, r := range rows {
		pkg := path.Dir(r.file)
		key := r.file + "\t" + r.fn
		switch {
		case listed[key]:
			out = append(out, r.file+": "+r.fn+": is on "+siteFile+" twice")
		case exempt[pkg] != "":
			out = append(out, r.file+": "+r.fn+": is on "+siteFile+", and its package is exempt from tests.generated.1")
		case !sites[key]:
			out = append(out, r.file+": "+r.fn+": is on "+siteFile+" and decodes nothing")
		case r.target == "-" && len(fuzzed[pkg]) == 0:
			out = append(out, r.file+": "+r.fn+": has no fuzz target on "+siteFile+", and "+pkg+" has none at all: fuzz it, or exempt the package in gate/exemptions.yaml")
		case r.target == "-" && strings.TrimSpace(r.why) == "":
			out = append(out, r.file+": "+r.fn+": has no fuzz target on "+siteFile+" and no reason")
		case r.target != "-" && !fuzzed[pkg][r.target]:
			out = append(out, r.file+": "+r.fn+": its target "+r.target+" is no func Fuzz of "+pkg)
		}
		listed[key] = true
	}
	for pkg, c := range calls {
		why, exempted := exempt[pkg]
		if exempted {
			if len(fuzzed[pkg]) > 0 {
				out = append(out, pkg+": has a fuzz target and is still listed as having none ("+why+")")
			}
			continue
		}
		for _, site := range c {
			file, rest, _ := strings.Cut(site, ": ")
			fn, call, _ := strings.Cut(rest, ": ")
			if !listed[file+"\t"+fn] {
				out = append(out, file+": "+fn+": "+call+" decodes input and is on no row of "+siteFile+", so no fuzz target is named for it")
			}
		}
	}
	for pkg := range exempt {
		if len(calls[pkg]) == 0 {
			out = append(out, pkg+": is listed as decoding with no fuzz target, and decodes nothing")
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// plantedExempt is the one exempt package the plants below stand on, as the rows give it.
func plantedExempt() map[string]string {
	return map[string]string{"internal/config/testing": "a harness"}
}

// plantedHarness is a decode call of the exempt package, so the plants that
// stand on plantedExempt carry no finding of their own.
func plantedHarness(t *testing.T) source {
	t.Helper()
	return parse(t, "internal/config/testing", "rerun.go", "package testing\n\nimport \"encoding/json\"\n\nfunc R(b []byte) (v any) { _ = json.Unmarshal(b, &v); return }\n")
}

// TestTheExemptionRowsNameTheirFolders: the rows of tests.generated.1 are the
// exempt packages, with the slash of the folder dropped, and a row of another
// rule is not one.
func TestTheExemptionRowsNameTheirFolders(t *testing.T) {
	t.Parallel()
	got := exemptPackages(t, []byte("exemptions:\n  - id: a\n    rule: tests.generated.1\n    path: internal/x/\n    reason: r\n  - id: b\n    rule: cli.exit.1\n    path: .\n    reason: s\n"))
	if len(got) != 1 || got["internal/x"] != "r" {
		t.Fatalf("exemptPackages = %v", got)
	}
	if _, ok := unfuzzed(t)["internal/config/testing"]; ok {
		t.Error("gate/exemptions.yaml still has a row of tests.generated.1 for internal/config/testing, which decodes nothing since testenv holds the rerun")
	}
}

// TestTheSiteRowsAreReadByTabs: a row is a file, a function, a target and an
// optional reason, comments and blank lines are skipped, and the list in the
// tree is read.
func TestTheSiteRowsAreReadByTabs(t *testing.T) {
	t.Parallel()
	got := siteRows(t, []byte("# a comment\n\ninternal/x/a.go\tT.read\tFuzzA\ncmd/y/b.go\tb\t-\ta reason\n"))
	want := []siteRow{{file: "internal/x/a.go", fn: "T.read", target: "FuzzA"}, {file: "cmd/y/b.go", fn: "b", target: "-", why: "a reason"}}
	if !slices.Equal(got, want) {
		t.Fatalf("siteRows = %+v, want %+v", got, want)
	}
	if len(listedSites(t)) == 0 {
		t.Errorf("%s has no rows", siteFile)
	}
}

func TestEveryDecodeCallHasAFuzzTarget(t *testing.T) {
	t.Parallel()
	_, w := tree(t)
	for _, f := range decoders(sources(t, w), unfuzzed(t), listedSites(t)) {
		t.Error(f)
	}
}

func TestADecodeCallInAPackageWithNoFuzzTargetFails(t *testing.T) {
	t.Parallel()
	harness := plantedHarness(t)
	json := parse(t, "internal/core/newreader", "read.go", "package newreader\n\nimport \"encoding/json\"\n\nfunc R(b []byte) (v any) { _ = json.Unmarshal(b, &v); return }\n")
	yaml := parse(t, "internal/core/other", "read.go", "package other\n\nimport yaml \"go.yaml.in/yaml/v3\"\n\ntype T struct{}\n\nfunc (*T) R(b []byte) (v any) { _ = yaml.NewDecoder(nil).Decode(&v); return }\n")
	got := decoders([]source{harness, json, yaml}, plantedExempt(), nil)
	fires(t, got, "internal/core/newreader/read.go: R: json.Unmarshal decodes input and is on no row of "+siteFile)
	fires(t, got, "internal/core/other/read.go: T.R: yaml.NewDecoder decodes input and is on no row of "+siteFile)
	rowed := []siteRow{{file: "internal/core/newreader/read.go", fn: "R", target: "FuzzR"}}
	fires(t, decoders([]source{harness, json}, plantedExempt(), rowed), "internal/core/newreader/read.go: R: its target FuzzR is no func Fuzz of internal/core/newreader")
	fuzz := parse(t, "internal/core/newreader", "read_test.go", "package newreader_test\n\nimport \"testing\"\n\nfunc FuzzR(f *testing.F) {}\n")
	if got := decoders([]source{harness, json, fuzz}, plantedExempt(), rowed); len(got) != 0 {
		t.Errorf("a call on a row whose target is in its package was refused: %q", got)
	}
	notDecode := parse(t, "internal/core/quiet", "q.go", "package quiet\n\nimport \"encoding/json\"\n\nfunc M(v any) ([]byte, error) { return json.Marshal(v) }\n")
	if got := decoders([]source{harness, notDecode}, plantedExempt(), nil); len(got) != 0 {
		t.Errorf("a package that only encodes was refused: %q", got)
	}
}

// TestANewDecodeCallInAPackageWithATargetFails is the hole a check per package
// left open (linters.go): internal/core/api has fuzz targets, and a
// new decode call in it on no row is still red, naming its file.
func TestANewDecodeCallInAPackageWithATargetFails(t *testing.T) {
	t.Parallel()
	harness := plantedHarness(t)
	old := parse(t, "internal/core/api", "cache.go", "package coreapi\n\nimport \"encoding/json\"\n\ntype tally struct{}\n\nfunc (t *tally) line(b []byte) { var v any; _ = json.Unmarshal(b, &v) }\n")
	planted := parse(t, "internal/core/api", "planted.go", "package coreapi\n\nimport \"encoding/json\"\n\nfunc readPlanted(b []byte) (v any) { _ = json.NewDecoder(nil).Decode(&v); return }\n")
	fuzz := parse(t, "internal/core/api", "fuzz_test.go", "package coreapi_test\n\nimport \"testing\"\n\nfunc FuzzCacheListJournalLine(f *testing.F) {}\n")
	rows := []siteRow{{file: "internal/core/api/cache.go", fn: "tally.line", target: "FuzzCacheListJournalLine"}}
	if got := decoders([]source{harness, old, fuzz}, plantedExempt(), rows); len(got) != 0 {
		t.Fatalf("the listed call was refused: %q", got)
	}
	got := decoders([]source{harness, old, planted, fuzz}, plantedExempt(), rows)
	if len(got) != 1 {
		t.Errorf("findings = %q, want the planted call alone", got)
	}
	fires(t, got, "internal/core/api/planted.go: readPlanted: json.NewDecoder decodes input and is on no row of "+siteFile)
}

// TestARowThatIsNotTrueFails: a row of a function that decodes nothing, a row
// of none with no reason, a row of none in a package with no fuzz target at
// all (which only gate/exemptions.yaml may waive), a row listed twice, and a
// row in an exempt package are each red.
func TestARowThatIsNotTrueFails(t *testing.T) {
	t.Parallel()
	harness := plantedHarness(t)
	read := parse(t, "cmd/x", "r.go", "package main\n\nimport \"encoding/json\"\n\nfunc r(b []byte) (v any) { _ = json.Unmarshal(b, &v); return }\n")
	fuzz := parse(t, "cmd/x", "z_test.go", "package main\n\nimport \"testing\"\n\nfunc FuzzZ(f *testing.F) {}\n")
	withTarget := []source{harness, read, fuzz}
	fires(t, decoders(withTarget, plantedExempt(), []siteRow{{file: "cmd/x/r.go", fn: "r", target: "-", why: " "}}),
		"cmd/x/r.go: r: has no fuzz target on "+siteFile+" and no reason")
	fires(t, decoders(withTarget, plantedExempt(), []siteRow{{file: "cmd/x/r.go", fn: "r", target: "-", why: "a reason"}, {file: "cmd/x/r.go", fn: "gone", target: "-", why: "a reason"}}),
		"cmd/x/r.go: gone: is on "+siteFile+" and decodes nothing")
	fires(t, decoders(withTarget, plantedExempt(), []siteRow{{file: "cmd/x/r.go", fn: "r", target: "-", why: "a reason"}, {file: "cmd/x/r.go", fn: "r", target: "FuzzZ"}}),
		"cmd/x/r.go: r: is on "+siteFile+" twice")
	if got := decoders(withTarget, plantedExempt(), []siteRow{{file: "cmd/x/r.go", fn: "r", target: "-", why: "read only from the tree"}}); len(got) != 0 {
		t.Errorf("a row of none with its reason, in a package with a target, was refused: %q", got)
	}
	got := decoders([]source{harness, read}, plantedExempt(), []siteRow{{file: "cmd/x/r.go", fn: "r", target: "-", why: "later"}})
	fires(t, got, "cmd/x/r.go: r: has no fuzz target on "+siteFile+", and cmd/x has none at all: fuzz it, or exempt the package in gate/exemptions.yaml")
	fires(t, decoders([]source{harness}, plantedExempt(), []siteRow{{file: "internal/config/testing/rerun.go", fn: "R", target: "-", why: "a reason"}}),
		"internal/config/testing/rerun.go: R: is on "+siteFile+", and its package is exempt")
}

func TestAListedPackageThatGainedATargetOrNoDecoderFails(t *testing.T) {
	t.Parallel()
	read := parse(t, "internal/config/testing", "r.go", "package main\n\nimport \"encoding/json\"\n\nfunc R(b []byte) (v any) { _ = json.Unmarshal(b, &v); return }\n")
	fuzz := parse(t, "internal/config/testing", "r_test.go", "package main\n\nimport \"testing\"\n\nfunc FuzzR(f *testing.F) {}\n")
	fires(t, decoders([]source{read, fuzz}, plantedExempt(), nil), "internal/config/testing: has a fuzz target and is still listed")
	fires(t, decoders(nil, plantedExempt(), nil), "internal/config/testing: is listed as decoding with no fuzz target, and decodes nothing")
}
