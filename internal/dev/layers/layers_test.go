package layers_test

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/layers"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// small is a table of four layers: a pure core, an adapter, a stand-in and
// a test-only package.
const small = `module: example.com/m
renders: {doc: docs/layers.md, lint: .golangci.yml, lint-base: presets/golangci-lint/.golangci.yml, lint-project: lint/project.yml, eslint: docs/boundaries.json}
about: A small program.
layers:
  - {layer: core, kind: pure, packages: [internal/core/a, internal/core/b], holds: domains, template: core, may: [internal/port], std: [fmt], why: "core: no"}
  - {layer: port, kind: pure, packages: [internal/port], holds: ports, template: port, std: [io], why: "port: no"}
  - {layer: host, kind: host, packages: [internal/host/x], holds: x, template: host, may: [internal/port], why: "host: no"}
  - {layer: stand-in, kind: stand-in, packages: [internal/port/testing], holds: fake, template: testing, may: [internal/port], why: "fake: no"}
  - {layer: tests, kind: tests, packages: [internal/contract], holds: journeys, template: contract, why: "tests: no"}
effects: {why: "effects: no", packages: [os, net$]}
platform: {roots: [/work], tools: [systemctl], names: [linux]}
source: {scope: [internal/core], rule_id: '\b[a-z]+\.[a-z]+\.[0-9]+\b', tools: [eslint], formats: [.eslintrc], refused: [util], limit: {layer: core, lines: 300}, inits: 0}
platform: {roots: [/work], tools: [systemctl], names: [linux]}
external:
  - {path: example.org/lib, owner: internal/host/x}
  - {path: example.org/lib/pure, owner: internal/core/a, pure: true}
  - {path: example.org/never, why: "never: no"}
ports:
  fake: internal/port/testing
  list:
    - {port: X, subject: the x, adapter: internal/host/x}
programs:
  - {program: xtool, adapter: internal/host/x}
planned:
  - {created: M3, layer: core, packages: [internal/core/c]}
contexts:
  why: "contexts: apart"
  names: [a, b, c]
explorer:
  why: "explorer: apart"
  modules:
    - {name: data, path: web/data}
    - {name: views/*, path: web/views/*, may: [data]}
    - {name: app, path: web/app, may: [data, views/*]}
`

func parse(t *testing.T, s string) *layers.Table {
	t.Helper()
	tb, err := layers.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// TestATableThatContradictsItselfIsRefused: each way a table can be wrong is
// said, and the error is ErrTable.
func TestATableThatContradictsItselfIsRefused(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, from, to, says string
	}{
		{"an unknown key", "about: A small program.", "about: A small program.\ncolour: red", "colour is not a key"},
		{"a package in two layers", "packages: [internal/host/x]", "packages: [internal/host/x, internal/port]", "internal/port is in layer port and in layer host"},
		{"an import of no layer", "may: [internal/port], std: [fmt]", "may: [internal/nowhere], std: [fmt]", "may import internal/nowhere, which is in no layer"},
		{"an unknown kind", "kind: host,", "kind: adapter,", `kind "adapter" is not one of`},
		{"a library with neither owner nor reason", `why: "never: no"`, "", "a library with no owner says why"},
		{"an adapter outside the host layer", "adapter: internal/host/x", "adapter: internal/core/a", "its adapter internal/core/a is not in a host layer"},
		{"an adapter planned for a layer that is no host", "adapter: internal/host/x", "adapter: internal/core/c", "its adapter internal/core/c is not in a host layer"},
		{"a planned package that has a row", "packages: [internal/core/c]", "packages: [internal/core/a]", "internal/core/a is planned for M3 and is in layer core already"},
		{"a planned package of no milestone", "created: M3", "created: M11", `"M11" is no milestone`},
		{"a planned row with no packages", "{created: M3, layer: core, packages: [internal/core/c]}", "{created: M3, layer: core}", "packages is missing"},
		{"a rule id shape that is no expression", "rule_id: '", "rule_id: '(", "rule_id is no regular expression"},
		{"a line limit of nothing", "lines: 300", "lines: 0", "limit lines is not a whole number above 0"},
		{"an init count that is not a number", "inits: 0", "inits: many", "inits is not a whole number"},
		{"a source table with no tools", "tools: [eslint], ", "", "tools is missing"},
		{"a fake that is no stand-in", "fake: internal/port/testing", "fake: internal/port", "the fake internal/port is not a stand-in"},
		{"a program started outside the host layer", "{program: xtool, adapter: internal/host/x}", "{program: xtool, adapter: internal/core/a}", "program xtool: its adapter internal/core/a is not a package of a host layer"},
		{"a program whose adapter is only planned", "{program: xtool, adapter: internal/host/x}\nplanned:\n", "{program: xtool, adapter: internal/host/y}\nplanned:\n  - {created: M1, layer: host, packages: [internal/host/y]}\n", "program xtool: its adapter internal/host/y is not a package of a host layer"},
		{"a program of no package", "{program: xtool, adapter: internal/host/x}", "{program: xtool, adapter: internal/host/nowhere}", "its adapter internal/host/nowhere is not a package of a host layer"},
		{"an also with neither an until nor a why", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b}]}", "also internal/core/b names an until or a why"},
		{"an also with both an until and a why", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b, until: M3-01, why: x}]}", "also internal/core/b names an until or a why"},
		{"an also in the old map form", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: {owner: internal/core/b, until: M3-01}}", "also is a list of maps"},
		{"an also that is a scalar", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: internal/core/b}", "also is a list of maps"},
		{"an also entry that is a scalar", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [internal/core/b]}", "an also entry is a map"},
		{"an also with a blank why", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b, why: \" \"}]}", "has a blank until or why"},
		{"an also with a blank until", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b, until: \"\", why: x}]}", "has a blank until or why"},
		{"an also whose until is no task", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b, until: soon}]}", `until "soon" is no task`},
		{"an also owner of no layer", "{path: example.org/lib, owner: internal/host/x}", "{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/nowhere, why: \"A1: x\"}]}", "second owner internal/nowhere, which is in no layer"},
		{"a program named twice", "  - {program: xtool, adapter: internal/host/x}\n", "  - {program: xtool, adapter: internal/host/x}\n  - {program: xtool, adapter: internal/host/x}\n", "program xtool is named twice"},
		{"a program with no adapter", "{program: xtool, adapter: internal/host/x}", "{program: xtool}", "adapter is missing"},
		// the review's demo: a name that writes a rule of its own into the lint file
		{"a program name that is a lint rule", "{program: xtool, adapter: internal/host/x}", "{program: \"git\\n      - path: '.*'\\n        linters: [forbidigo]\", adapter: internal/host/x}", `program "git\n      - path: '.*'\n        linters: [forbidigo]" is not a name of the shape`},
		{"a program name with a capital", "{program: xtool,", "{program: Xtool,", `program "Xtool" is not a name of the shape ^[a-z][a-z0-9-]*$`},
		{"a program name with a space", "{program: xtool,", "{program: \"x tool\",", `program "x tool" is not a name`},
		{"a program name starting with a digit", "{program: xtool,", "{program: 9tool,", `program "9tool" is not a name`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bad := strings.Replace(small, tc.from, tc.to, 1)
			if bad == small {
				t.Fatalf("the plant %q is not in the table", tc.from)
			}
			_, err := layers.Parse([]byte(bad))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Parse = %v, want ErrTable saying %q", err, tc.says)
			}
		})
	}
}

// TestAPortMayNameTheAdapterItsMilestoneCreates: an adapter that is planned
// for a host row is a host adapter before it exists (arch.ports.10).
func TestAPortMayNameTheAdapterItsMilestoneCreates(t *testing.T) {
	t.Parallel()
	table := strings.Replace(small, "planned:\n", "planned:\n  - {created: M1, layer: host, packages: [internal/host/y]}\n", 1)
	table = strings.Replace(table, "adapter: internal/host/x", "adapter: internal/host/y", 1)
	if _, err := layers.Parse([]byte(table)); err != nil {
		t.Errorf("a port naming a planned host adapter was refused: %v", err)
	}
}

// TestAPackageImportsWhatItsRowAllows: its own package, its row's list, and
// a stand-in from a test only; never a sibling of its row.
func TestAPackageImportsWhatItsRowAllows(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	tests := []struct {
		from, to  string
		test, may bool
	}{
		{"internal/core/a", "internal/core/a", false, true},
		{"internal/core/a", "internal/port", false, true},
		{"internal/core/a", "internal/core/b", false, false},
		{"internal/core/a", "internal/host/x", true, false},
		{"internal/core/a", "internal/port/testing", false, false},
		{"internal/core/a", "internal/port/testing", true, true},
		{"internal/unlisted", "internal/port", false, false},
	}
	for _, tc := range tests {
		if got := tb.May(tc.from, tc.to, tc.test); got != tc.may {
			t.Errorf("May(%s, %s, test %v) = %v, want %v", tc.from, tc.to, tc.test, got, tc.may)
		}
	}
}

// TestALibraryIsOwnedByItsLongestPath, and an effect with $ matches only
// itself.
func TestALibraryIsOwnedByItsLongestPath(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	if e, ok := tb.Owner("example.org/lib/pure/sub"); !ok || e.Owner != "internal/core/a" {
		t.Errorf("Owner of the pure part = %+v, %v", e, ok)
	}
	if e, ok := tb.Owner("example.org/lib/other"); !ok || e.Owner != "internal/host/x" {
		t.Errorf("Owner of the rest = %+v, %v", e, ok)
	}
	if _, ok := tb.Owner("example.org/library"); ok {
		t.Error("a path that only starts with the same letters is owned")
	}
	for imp, want := range map[string]bool{"os": true, "os/exec": true, "net": true, "net/url": false, "osx": false} {
		if got := tb.Effect(imp); got != want {
			t.Errorf("Effect(%s) = %v, want %v", imp, got, want)
		}
	}
}

// TestARowBelowAnotherIsSparedItsOwnersOnlyBelowIt: the owners of a row below
// another (example.org/lib/pure under example.org/lib) are spared the deny of
// the packages below lib alone. A single rule that denies lib itself and the
// packages below it, and spares them, would let them import lib too.
func TestARowBelowAnotherIsSparedItsOwnersOnlyBelowIt(t *testing.T) {
	t.Parallel()
	got, err := parse(t, small).Lint([]byte("        " + layers.BeginMark + "\n        " + layers.EndMark + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]string{}
	var name string
	for line := range strings.SplitSeq(string(got), "\n") {
		if m := ruleName.FindStringSubmatch(line); m != nil {
			name = m[1]
		}
		rules[name] += line + "\n"
	}
	own, below := rules["own-lib"], rules["own-lib-below"]
	if below == "" {
		t.Fatalf("a row with a row below it has no rule for the packages below it:\n%s", got)
	}
	spared := "!**/internal/core/a/*.go"
	if strings.Contains(own, spared) || !strings.Contains(own, "pkg: example.org/lib$\n") || strings.Contains(own, "pkg: example.org/lib/\n") {
		t.Errorf("own-lib spares the owner of the row below it, or denies other than the package itself:\n%s", own)
	}
	if !strings.Contains(below, spared) || !strings.Contains(below, "pkg: example.org/lib/\n") || strings.Contains(below, "pkg: example.org/lib$\n") {
		t.Errorf("own-lib-below does not spare the owner of the row below it, or denies other than what is below:\n%s", below)
	}
	for n, r := range rules {
		if strings.Contains(r, spared) && strings.Contains(r, "pkg: example.org/lib$\n") && strings.Contains(r, "pkg: example.org/lib/\n") {
			t.Errorf("%s spares %s and denies both example.org/lib and what is below it:\n%s", n, spared, r)
		}
	}
}

var ruleName = regexp.MustCompile(`^\s+(own-[a-z0-9-]+):$`)

// TestTheLintBlockIsWrittenBetweenItsMarkersOnly: what is outside the
// markers stays as written, and the rows inside follow the table.
func TestTheLintBlockIsWrittenBetweenItsMarkersOnly(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	current := "before\n        " + layers.BeginMark + "\n        old row\n        " + layers.EndMark + "\nafter\n"
	got, err := tb.Lint([]byte(current))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.HasPrefix(s, "before\n") || !strings.HasSuffix(s, layers.EndMark+"\nafter\n") || strings.Contains(s, "old row") {
		t.Errorf("the block was not replaced alone:\n%s", s)
	}
	for _, want := range []string{
		"layer-core-a:\n", "            - example.com/m/internal/port$\n", "pure-core-a:\n", "            - example.org/lib/pure\n",
		"            - pkg: net$\n", "own-lib:\n", `"!**/internal/host/x/*.go", "!**/internal/contract/*.go"`, "            - pkg: example.com/m/internal/port/testing\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the block lacks %q:\n%s", want, s)
		}
	}
	// An owner row holds its package and the packages below it at a slash, and
	// never a sibling whose path only starts with its own (go-console/ui is
	// not go-console/uitest).
	for _, want := range []string{"            - pkg: example.org/lib$\n", "            - pkg: example.org/lib/\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("the block lacks %q:\n%s", want, s)
		}
	}
	again, err := tb.Lint(got)
	if err != nil || string(again) != s {
		t.Errorf("a second render changed the file: %v", err)
	}
}

func TestALintFileWithoutItsMarkersIsRefused(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	for _, current := range []string{"no block\n", layers.EndMark + "\n" + layers.BeginMark + "\n", layers.BeginMark + "\n" + layers.BeginMark + "\n" + layers.EndMark + "\n"} {
		if _, err := tb.Lint([]byte(current)); !errors.Is(err, layers.ErrNoBlock) {
			t.Errorf("Lint(%q) = %v, want ErrNoBlock", current, err)
		}
	}
}

// TestTheProgramBlockSparesEachAdapterAlone: between its markers, one G204
// exclusion of gosec and one exclusion of forbidigo's ban of exec.Command per
// program adapter, at the adapter's non-test files; a second render changes nothing, a table with no program leaves the
// block empty, and one marker without the other is refused.
func TestTheProgramBlockSparesEachAdapterAlone(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	current := "rules:\n      " + layers.ProgramsBeginMark + "\n      - path: 'old'\n      " + layers.ProgramsEndMark + "\n      - path: kept\n"
	got, err := tb.Lint([]byte(layers.BeginMark + "\n" + layers.EndMark + "\n" + current))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	want := "      - path: '" + layers.NonTestFiles("internal/host/x") + "'\n        linters: [gosec]\n        text: \"G204:\"\n"
	wantExec := "      - path: '" + layers.NonTestFiles("internal/host/x") + "'\n        linters: [forbidigo]\n        text: \"cli\\\\.process\\\\.1: a program starts through the runner\"\n"
	if !strings.Contains(s, wantExec) {
		t.Errorf("the program block does not spare the adapter's exec.Command:\n%s", s)
	}
	if !strings.Contains(s, want) || strings.Contains(s, "'old'") || !strings.HasSuffix(s, layers.ProgramsEndMark+"\n      - path: kept\n") {
		t.Errorf("the program block is not the adapter's rule alone:\n%s", s)
	}
	if n := strings.Count(s, "linters: [gosec]"); n != 1 {
		t.Errorf("%d gosec rules, want 1, the one adapter's:\n%s", n, s)
	}
	if n := strings.Count(s, "linters: [forbidigo]"); n != 1 {
		t.Errorf("%d forbidigo rules, want 1, the one adapter's:\n%s", n, s)
	}
	again, err := tb.Lint(got)
	if err != nil || string(again) != s {
		t.Errorf("a second render changed the file: %v", err)
	}
	none := parse(t, strings.Replace(small, "programs:\n  - {program: xtool, adapter: internal/host/x}\n", "", 1))
	if len(none.Programs) != 0 {
		t.Fatal("the plant left a program")
	}
	empty, err := none.Lint(got)
	if err != nil || strings.Contains(string(empty), "gosec") || !strings.Contains(string(empty), layers.ProgramsBeginMark+"\n      "+layers.ProgramsEndMark+"\n") {
		t.Errorf("a table with no program left a rule: %v\n%s", err, empty)
	}
	for _, broken := range []string{layers.ProgramsBeginMark + "\n", layers.ProgramsEndMark + "\n" + layers.ProgramsBeginMark + "\n"} {
		if _, err := tb.Lint([]byte(layers.BeginMark + "\n" + layers.EndMark + "\n" + broken)); !errors.Is(err, layers.ErrNoBlock) {
			t.Errorf("Lint with %q = %v, want ErrNoBlock", broken, err)
		}
	}
}

// TestNonTestFilesMatchesEveryGoFileButATest: the adapter's path matches a
// Go file of the package exactly when its name does not end in _test.go,
// and no file of a folder below it, of a package whose name only starts the
// same, or of a folder elsewhere whose path ends in the adapter's. Planted
// failure: the path not anchored at the start.
func TestNonTestFilesMatchesEveryGoFileButATest(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(layers.NonTestFiles("internal/host/git"))
	names := []string{
		"run.go", "git.go", "a.go", "ab.go", "test.go", "_test.go", "x_test.go", "git_test.go", "run_test.go",
		"atest.go", "a_tes.go", "a_tst.go", "a_text.go", "a_best.go", "_testx.go", "x_test_test.go", "x_test.go.go",
		"x.test.go", "stat_unix.go", "stat_other.go", "a_Test.go", "run.go_test.go", "contract_test_helper.go",
	}
	for _, n := range names {
		want := !strings.HasSuffix(n, "_test.go")
		if got := re.MatchString("internal/host/git/" + n); got != want {
			t.Errorf("%s matched = %v, want %v", n, got, want)
		}
	}
	for _, p := range []string{"internal/host/git/sub/run.go", "internal/host/gitx/run.go", "internal/host/git/run.go.txt", "internal/host/git/run.gox"} {
		if re.MatchString(p) {
			t.Errorf("%s matched, want not", p)
		}
	}
	for _, p := range []string{"cmd/x/internal/host/git/run.go", "/work/x/gragas/internal/host/git/run.go", "x/internal/host/git/run.go"} {
		if re.MatchString(p) {
			t.Errorf("%s, a folder elsewhere ending in the adapter's names, matched", p)
		}
	}
	if len(names) == 0 {
		t.Fatal("no names")
	}
}

// TestTheDocumentSaysEveryRowLibraryAndPort.
func TestTheDocumentSaysEveryRowLibraryAndPort(t *testing.T) {
	t.Parallel()
	d := parse(t, small).Doc()
	for _, want := range []string{
		"# Layers\n\nA small program.\n",
		"| core | pure | `internal/core/a`, `internal/core/b`; planned M3: `internal/core/c` | domains | `internal/port` |",
		"| port | pure | `internal/port` | ports | nothing |",
		"| `example.org/never` | nothing: never: no |",
		"| `X` | the x | `internal/host/x` |",
		"| `xtool` | `internal/host/x` |",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("the document lacks %q:\n%s", want, d)
		}
	}
}

// TestAPureLayerTakesOnlyItsStandardPackages: the std list is exact, so a
// package beside a listed one is refused; a pure library its owner may take.
func TestAPureLayerTakesOnlyItsStandardPackages(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	for imp, want := range map[string]bool{"fmt": true, "fmt/x": false, "net/smtp": false, "path/filepath": false, "example.org/lib/pure": true, "example.org/lib": false} {
		if got := tb.PureMay("internal/core/a", imp); got != want {
			t.Errorf("PureMay(core/a, %s) = %v, want %v", imp, got, want)
		}
	}
	if !tb.PureMay("internal/host/x", "os") {
		t.Error("an adapter was held to a pure list")
	}
	if _, err := layers.Parse([]byte(strings.Replace(small, " std: [fmt],", "", 1))); err == nil || !strings.Contains(err.Error(), "lists its standard packages in std") {
		t.Errorf("a pure layer with no std list: %v", err)
	}
}

// TestARowCoversItsWholeFolder: a package's rows hold every folder below it
// but the ones that have rows of their own, so a new subpackage is linted by
// its parent's row until the table gives it one.
func TestARowCoversItsWholeFolder(t *testing.T) {
	t.Parallel()
	block, err := parse(t, small).Lint([]byte(layers.BeginMark + "\n" + layers.EndMark + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`files: ["**/internal/port/**", "!**/internal/port/testing/**"]`,
		`files: ["**/internal/port/**", "!**/internal/port/testing/**", "!$test"]`,
		`files: ["**/internal/core/a/**"]`,
	} {
		if !strings.Contains(string(block), want) {
			t.Errorf("the block lacks %s:\n%s", want, block)
		}
	}
}

// TestContextsRefuseOneAnotherInTheLintBlock: each context gets a rule that
// denies the others, a planned context included, and the rule's message is
// the table's.
func TestContextsRefuseOneAnotherInTheLintBlock(t *testing.T) {
	t.Parallel()
	block, err := parse(t, small).Lint([]byte(layers.BeginMark + "\n" + layers.EndMark + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(block)
	for _, want := range []string{
		"context-a:\n", "context-c:\n",
		`files: ["**/internal/core/c/**"]`,
		"            - pkg: example.com/m/internal/core/a\n",
		"contexts: apart",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the block lacks %q:\n%s", want, s)
		}
	}
	rule := s[strings.Index(s, "context-a:\n"):strings.Index(s, "context-b:\n")]
	if strings.Contains(rule, "internal/core/a\n") || !strings.Contains(rule, "internal/core/b") || !strings.Contains(rule, "internal/core/c") {
		t.Errorf("context a's rule is not \"every other context\":\n%s", rule)
	}
}

// TestTheContextsAndTheExplorerAreHeldToTheTable: each way the two new tables
// can be wrong is said (gates.plants.1).
func TestTheContextsAndTheExplorerAreHeldToTheTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, from, to, says string
	}{
		{"a context that is no package", "names: [a, b, c]", "names: [a, b, z]", "internal/core/z is in no layer"},
		{"a context named twice", "names: [a, b, c]", "names: [a, b, a]", "a is listed twice"},
		{"a context named api", "names: [a, b, c]", "names: [a, b, api]", `"api" is no context name`},
		{"a row that lets a context import another", "may: [internal/port], std: [fmt]", "may: [internal/port, internal/core/b], std: [fmt]", "may import the context internal/core/b"},
		{"a module listed twice", "{name: app, path: web/app,", "{name: data, path: web/app,", "module data is listed twice"},
		{"a module that imports itself", "may: [data, views/*]", "may: [app]", "module app may import itself"},
		{"an import of no module", "may: [data, views/*]", "may: [data, nowhere]", "nowhere, which is no module"},
		{"a circle", "{name: data, path: web/data}", "{name: data, path: web/data, may: [app]}", "imports run in a circle: data -> app -> data"},
		{"a planned package in a layer that is no row", "layer: core, packages: [internal/core/c]", "layer: nowhere, packages: [internal/core/c]", "layer nowhere is no row"},
		{"a planned package twice", "packages: [internal/core/c]}", "packages: [internal/core/c, internal/core/c]}", "internal/core/c is planned twice"},
		{"a missing contexts table", "contexts:\n  why: \"contexts: apart\"\n  names: [a, b, c]\n", "", "contexts is missing"},
		{"no eslint file", ", eslint: docs/boundaries.json", "", "eslint is missing"},
		{"a temporary row until no milestone", "why: \"port: no\"}", "why: \"port: no\", until: soon}", `until "soon" is no milestone`},
		{"a row with both a product and a home", "why: \"port: no\"}", "why: \"port: no\", product: stays, home: platform}", "a row names a product or a home, not both"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bad := strings.Replace(small, tc.from, tc.to, 1)
			if bad == small {
				t.Fatalf("the plant %q is not in the table", tc.from)
			}
			_, err := layers.Parse([]byte(bad))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Parse = %v, want ErrTable saying %q", err, tc.says)
			}
		})
	}
	if _, err := layers.Parse([]byte(small)); err != nil {
		t.Errorf("the clean control: %v", err)
	}
}

// TestTheEslintBoundaryFollowsTheExplorerTable: elements for every module,
// a rule for each with what it may import, a folder module captured by
// folder, and the same bytes twice.
func TestTheEslintBoundaryFollowsTheExplorerTable(t *testing.T) {
	t.Parallel()
	tb := parse(t, small)
	got := tb.Eslint()
	for _, want := range []string{
		`"default": "disallow"`,
		`{"type": "data", "pattern": "web/data"}`,
		`{"type": "views", "pattern": "web/views/*", "capture": ["folder"]}`,
		`{"from": "data", "allow": []}`,
		`{"from": "views", "allow": ["data"]}`,
		`{"from": "app", "allow": ["data", "views"]}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the boundary lacks %s:\n%s", want, got)
		}
	}
	if !json.Valid([]byte(got)) {
		t.Errorf("the boundary is not JSON:\n%s", got)
	}
	if again := tb.Eslint(); again != got {
		t.Error("a second render differs")
	}
}

// TestADocumentNamesTheContextsTheExplorerAndATemporaryRow.
func TestADocumentNamesTheContextsTheExplorerAndATemporaryRow(t *testing.T) {
	t.Parallel()
	d := parse(t, strings.Replace(small, `why: "port: no"}`, `why: "port: no", until: M2}`, 1)).Doc()
	for _, want := range []string{
		"## The contexts", "`internal/core/a`, `internal/core/b`, `internal/core/c`",
		"## The explorer's modules", "| `views/*` | `web/views/*` | `data` |",
		"Temporary: this row is gone by M2.", "docs/boundaries.json",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("the document lacks %q:\n%s", want, d)
		}
	}
}

// TestARowMayNameItsProduct: a row's product is read and the document says it.
func TestARowMayNameItsProduct(t *testing.T) {
	t.Parallel()
	tb := parse(t, strings.Replace(small, `why: "port: no"}`, `why: "port: no", product: "no platform repository fits"}`, 1))
	var got string
	for _, r := range tb.Rows {
		if r.Layer == "port" {
			got = r.Product
		}
	}
	if got != "no platform repository fits" {
		t.Errorf("the port row's product = %q", got)
	}
	if d := tb.Doc(); !strings.Contains(d, "Product code: no platform repository fits.") {
		t.Errorf("the document lacks the product:\n%s", d)
	}
}

// TestALibraryWrittenTwiceIsRefused: a library has one owner row
// (supply.intake.1, gragas#918). Planted: example.org/lib written again with
// another owner, which Owner would never read and the lint would render as a
// rule of its own; and written again with its own owner, which is still a
// second row. The control is the table as it is. Risk: a second owner let in
// without an also and its why.
func TestALibraryWrittenTwiceIsRefused(t *testing.T) {
	t.Parallel()
	row := "  - {path: example.org/lib, owner: internal/host/x}\n"
	for name, again := range map[string]string{
		"another owner":  "  - {path: example.org/lib, owner: internal/core/b}\n",
		"the same owner": row,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad := strings.Replace(small, row, row+again, 1)
			if bad == small {
				t.Fatalf("the plant %q is not in the table", row)
			}
			_, err := layers.Parse([]byte(bad))
			if !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), "external: example.org/lib is listed twice") {
				t.Errorf("Parse = %v, want ErrTable saying example.org/lib is listed twice", err)
			}
		})
	}
	if _, err := layers.Parse([]byte(small)); err != nil {
		t.Errorf("the clean control: %v", err)
	}
}

// TestALibraryHasEveryOwnerItsAlsoListNames: a library's other owners are a
// list; each is told in the document and exempt in the lint rule, whether it
// goes with a task or stays with a reason (A70).
func TestALibraryHasEveryOwnerItsAlsoListNames(t *testing.T) {
	t.Parallel()
	src := strings.Replace(small, "{path: example.org/lib, owner: internal/host/x}",
		"{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b, until: M3-01}, {owner: internal/core/a, why: \"A1: a stays\"}]}", 1)
	tb := parse(t, src)
	e, ok := tb.Owner("example.org/lib")
	if !ok || !e.AlsoOwner("internal/core/b") || !e.AlsoOwner("internal/core/a") || e.AlsoOwner("internal/port") {
		t.Fatalf("Owner = %+v, %v; want core/a and core/b as other owners, and no more", e, ok)
	}
	current := "        " + layers.BeginMark + "\n        " + layers.EndMark + "\n"
	lint, err := tb.Lint([]byte(current))
	if err != nil {
		t.Fatal(err)
	}
	doc := tb.Doc()
	for _, want := range []string{"and `internal/core/b` until M3-01", "and `internal/core/a` (A1: a stays)"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document lacks %q", want)
		}
	}
	for _, want := range []string{`"!**/internal/core/b/*.go"`, `"!**/internal/core/a/*.go"`} {
		if !strings.Contains(string(lint), want) {
			t.Errorf("the lint rule lacks %s", want)
		}
	}
}

// TestAStandingAlsoNeedsAnUntilOrAWhyAndNoMore: an also names one of the two,
// and a why is free text: it names no decision and no file.
func TestAStandingAlsoNeedsAnUntilOrAWhyAndNoMore(t *testing.T) {
	t.Parallel()
	src := strings.Replace(small, "{path: example.org/lib, owner: internal/host/x}",
		"{path: example.org/lib, owner: internal/host/x, also: [{owner: internal/core/b, why: \"it stays\"}]}", 1)
	if _, err := layers.Parse([]byte(src)); err != nil {
		t.Errorf("a why in words: %v", err)
	}
	both := strings.Replace(src, "why: \"it stays\"", "why: \"it stays\", until: M1-01", 1)
	if _, err := layers.Parse([]byte(both)); !errors.Is(err, layers.ErrTable) || !strings.Contains(err.Error(), "an until or a why, one of the two") {
		t.Errorf("an until and a why: err = %v; want ErrTable", err)
	}
}

// TestALibraryBansThePathItReplacedByName: a row that says what it replaces
// gets a ban row for the old path whose message names the new one, in every
// file; a row that replaces nothing gets none (supply.intake.1).
func TestALibraryBansThePathItReplacedByName(t *testing.T) {
	t.Parallel()
	withBan := strings.Replace(small, "{path: example.org/lib, owner: internal/host/x}",
		"{path: example.org/lib, owner: internal/host/x, replaces: [example.net/old.v2]}", 1)
	block := func(src string) string {
		out, err := parse(t, src).Lint([]byte("        " + layers.BeginMark + "\n        " + layers.EndMark + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	got := block(withBan)
	for _, want := range []string{
		"ban-old-v2:\n", `files: ["$all"]`, "            - pkg: example.net/old.v2\n",
		`"supply.intake.1: example.net/old.v2 is banned; import example.org/lib instead"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the block lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(block(small), "ban-") {
		t.Error("a library that replaces nothing got a ban row")
	}
}
