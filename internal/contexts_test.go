package internal

import (
	"debug/buildinfo"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/ordered"
)

// The contexts of layers.yaml never import one another (arch.ports.8), and the
// refusal is the boundary lint generated into .golangci.yml. gosec reads no
// suppression comment and no generated-file header (cli.process.1,
// gates.exemptions.1; gragas#397). These tests run that very file, with the
// pinned golangci-lint, over a small module in a temp folder, with and
// without a plant (gates.plants.1).

// writeModule writes files, each a slash path and its body, under a new temp
// folder and returns it.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// lintConfig is the repository's own .golangci.yml.
func lintConfig(t *testing.T) string {
	t.Helper()
	cfg, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(cfg)
}

// contextModule writes a module with the contexts link and proof, where proof
// imports what imports says, and returns its folder.
func contextModule(t *testing.T, imports string) string {
	t.Helper()
	return writeModule(t, map[string]string{
		".golangci.yml":                lintConfig(t),
		"go.mod":                       "module " + table(t).Module + "\n\ngo 1.23.0\n",
		"internal/core/link/link.go":   "package link\n\n// One is a value.\nfunc One() int { return 1 }\n",
		"internal/core/proof/proof.go": "package proof\n\n" + imports + "\n// Two is a value.\nfunc Two() int { return 2 }\n",
	})
}

// toolPin is a tool's pin in tools.yaml, doc: the line its version run
// says and the sha256 of its program on each platform tools.yaml names. It
// fails on a file that pins no such tool, pins it twice, has no line, or
// has a platform with no sum or none at all, so a missing pin never reads as
// any binary. The test holds the program to every platform's sum, not this
// one's: arch.ports.1 keeps the platform out of a test, and a program pinned
// for another platform does not run here, so its version run fails.
func toolPin(doc *ordered.Map, id string) (says string, sums []string, err error) {
	found := 0
	built := false
	for _, v := range doc.List("tools") {
		tool, ok := v.(*ordered.Map)
		if !ok || tool.Str("id") != id {
			continue
		}
		found++
		says = tool.Str("says")
		built = tool.Str("from") == "go"
		p := tool.Sub("platforms")
		if p == nil {
			continue
		}
		for _, name := range p.Keys() {
			sum := ""
			if pin := p.Sub(name); pin != nil {
				sum = pin.Str("sha256")
			}
			if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(sum) {
				return "", nil, fmt.Errorf("tools.yaml pins no sha256 of %s for %s", id, name)
			}
			sums = append(sums, sum)
		}
	}
	switch {
	case found != 1:
		return "", nil, fmt.Errorf("tools.yaml pins %s %d times, want once", id, found)
	case says == "":
		return "", nil, fmt.Errorf("tools.yaml's %s says nothing its version run must print", id)
	case len(sums) == 0 && !built:
		return "", nil, fmt.Errorf("tools.yaml pins %s on no platform", id)
	}
	return says, sums, nil
}

// builtPinned holds the program at bin, a go tool built from the tools module,
// the way task tools-check holds it (builtFromTheLock of cmd/gragas-dev): its
// build info (go version -m) names the main module and every dependency that has
// a sum, each as a line of tools/go.sum, so a program built before a pin moved,
// or against other dependencies than the lock's, is refused. A source build
// carries no release sha, so none is read (gragas#1288): a sha256 pins a
// downloaded release program, and this holds a program the hook builds itself.
func builtPinned(bin string) error {
	info, err := buildinfo.ReadFile(bin)
	if err != nil {
		return fmt.Errorf("%s has no build info: %w; task tools builds it", bin, err)
	}
	sums, err := os.ReadFile(filepath.Join(root, "tools", "go.sum"))
	if err != nil {
		return err
	}
	lines := map[string]bool{}
	for line := range strings.SplitSeq(string(sums), "\n") {
		lines[line] = true
	}
	held := func(m *debug.Module) error {
		if m.Sum == "" {
			return fmt.Errorf("%s: module %s has no sum in its build info; task tools builds it", bin, m.Path)
		}
		if !lines[m.Path+" "+m.Version+" "+m.Sum] {
			return fmt.Errorf("%s: %s %s %s is no line of tools/go.sum; task tools builds the pinned one", bin, m.Path, m.Version, m.Sum)
		}
		return nil
	}
	if err := held(&info.Main); err != nil {
		return err
	}
	for _, d := range info.Deps {
		if d.Replace != nil {
			if err := held(d.Replace); err != nil {
				return err
			}
			continue
		}
		if d.Sum == "" {
			continue // as the shell reads a dep line that has none
		}
		if err := held(d); err != nil {
			return err
		}
	}
	return nil
}

// lintPin is golangci-lint's pin in tools.yaml.
func lintPin(doc *ordered.Map) (says string, sums []string, err error) {
	return toolPin(doc, "golangci-lint")
}

// sumPinned holds the program at bin to the sums tools.yaml pins: it is refused,
// naming bin, when its sha256 is on no platform's line. A wrapper that runs the
// real binary says what the real one says, so only this check refuses it.
func sumPinned(bin string, sums []string) error {
	prog, err := os.ReadFile(bin)
	if err != nil {
		return fmt.Errorf("%w; task tools places it", err)
	}
	if got := digest.Hex(prog); !slices.Contains(sums, got) {
		return fmt.Errorf("%s has sha256 %s, which tools.yaml pins on no platform; task tools places the pinned one", bin, got)
	}
	return nil
}

// pinnedLint puts the golangci-lint the gate runs first on this test's PATH:
// the .tools/bin copy make tools places, held to tools.yaml's pin by the
// sha256 of its program and by what its version run says. A command names
// golangci-lint, and lint checks that the name found this copy, so a verdict
// here holds for the binary make lint runs and never for one that happens to
// be on the PATH.
func pinnedLintPath(t *testing.T) string {
	t.Helper()
	return pinnedPath(t, "golangci-lint")
}

func pinnedLint(t *testing.T) string {
	t.Helper()
	says, sums, err := lintPin(lintDoc(t, "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	bin := pinnedLintPath(t)
	dir := filepath.Dir(bin)
	if err := sumPinned(bin, sums); err != nil {
		t.Fatal(err)
	}
	path := dir
	for _, kv := range configtest.Inherited() {
		if rest, ok := strings.CutPrefix(kv, "PATH="); ok && rest != "" {
			path += string(filepath.ListSeparator) + rest
		}
	}
	t.Setenv("PATH", path)
	out, ok := lint(t, bin, "", exec.CommandContext(t.Context(), "golangci-lint", "version"))
	if !ok {
		t.Fatalf("%s does not run, so the lint cannot be run:\n%s", bin, out)
	}
	if !strings.HasPrefix(out, says) {
		t.Fatalf("%s does not say tools.yaml's %q:\n%s", bin, says, out)
	}
	return bin
}

// TestTheLintPinIsTheOneToolsYamlHolds: lintPin reads golangci-lint's line
// and every platform's sum from tools.yaml's own text, and refuses a file
// whose pin is missing, doubled, has no line, a platform with no sum, or no
// platforms. Planted failures: each of those, made from the real file.
func TestTheLintPinIsTheOneToolsYamlHolds(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	parse := func(s string) *ordered.Map {
		t.Helper()
		v, err := ordered.ParseYAML([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return v.(*ordered.Map)
	}
	real := string(data)
	start := strings.Index(real, "  - id: golangci-lint\n")
	end := strings.Index(real[start+1:], "\n  - id: ")
	if start < 0 || end < 0 {
		t.Fatal("tools.yaml holds no golangci-lint entry with one after it")
	}
	entry := real[start : start+1+end+1]
	says, sums, err := lintPin(parse(real))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entry, "says: \""+says+"\"") {
		t.Errorf("lintPin read the line %q, which golangci-lint's entry does not hold", says)
	}
	if want := strings.Count(entry, "sha256: "); len(sums) != want {
		t.Errorf("lintPin read %d sums, want the entry's %d", len(sums), want)
	}
	for _, sum := range sums {
		if !strings.Contains(entry, "sha256: "+sum+"\n") {
			t.Errorf("lintPin read the sum %s, which golangci-lint's entry does not hold", sum)
		}
	}
	if len(sums) == 0 {
		t.Fatal("lintPin read no sum")
	}
	in := func(from, to string) string {
		return real[:start] + strings.Replace(entry, from, to, 1) + real[start+len(entry):]
	}
	for name, plant := range map[string]string{
		"no golangci-lint": in("id: golangci-lint", "id: golangci-lint-x"),
		"no line":          in("says: \""+says+"\"", "says: \"\""),
		"no sum":           in("sha256: "+sums[len(sums)-1], "sha256: \"\""),
		"no platforms":     in("platforms:", "platformz:"),
		"pinned twice":     real[:start] + entry + real[start:],
	} {
		if plant == real {
			t.Fatalf("%s: the plant changed nothing", name)
		}
		if _, _, err := lintPin(parse(plant)); err == nil {
			t.Errorf("%s: lintPin read the plant as a pin", name)
		}
	}
}

// lint runs cmd, made after pinnedLint and naming golangci-lint, in dir (or
// here when dir is ""), and returns its report and whether it found nothing.
// It fails the test when cmd would run any binary but bin, and when it is a
// run without --allow-parallel-runners, which task lint passes: a run without
// it waits on, or fails beside, a gate's lint on the same host (gragas#643).
func lint(t *testing.T, bin, dir string, cmd *exec.Cmd) (string, bool) {
	t.Helper()
	if cmd.Err != nil || cmd.Path != bin {
		t.Fatalf("golangci-lint is %s, not the pinned %s: %v", cmd.Path, bin, cmd.Err)
	}
	if slices.Contains(cmd.Args, "run") && !slices.Contains(cmd.Args, "--allow-parallel-runners") {
		t.Fatalf("%v runs golangci-lint without --allow-parallel-runners, unlike task lint", cmd.Args)
	}
	cmd.Dir = dir
	cmd.Env = append(configtest.Inherited(), cmd.Env...) // a caller's own variables (GOOS) follow the inherited ones
	out, err := cmd.CombinedOutput()
	// golangci-lint prints a finding's file with the host's separator, a
	// backslash on Windows; the checks name a file with forward slashes, so the
	// output is read in that one form on every system (gragas#1391). ToSlash is
	// identity on Unix.
	return filepath.ToSlash(string(out)), err == nil
}

// TestAnImportBetweenTwoContextsFailsTheLint: proof importing link is named by
// the context rule of its own (context-proof), with the table's reason, and
// not only by the layer rule that refuses the same import; the same module with no import
// passes, so the lint is running at all.
func TestAnImportBetweenTwoContextsFailsTheLint(t *testing.T) {
	tb := table(t)
	if len(tb.Contexts.Names) < 2 {
		t.Fatalf("the table lists %d contexts, so nothing is held apart", len(tb.Contexts.Names))
	}
	bin := pinnedLint(t)
	depguard := func() *exec.Cmd {
		return exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--default=none", "--enable=depguard", "./...")
	}
	if out, ok := lint(t, bin, contextModule(t, ""), depguard()); !ok {
		t.Fatalf("the clean control fails the lint:\n%s", out)
	}
	planted := contextModule(t, fmt.Sprintf("import \"%s/internal/core/link\"\n\nvar _ = link.One\n", tb.Module))
	out, ok := lint(t, bin, planted, depguard())
	if ok || !strings.Contains(out, "internal/core/link") || !strings.Contains(out, "arch.ports.8") || !strings.Contains(out, "context-proof") {
		t.Errorf("a planted import of one context from another gave ok=%v:\n%s", ok, out)
	}
}

// TestAGosecSuppressionCommentSilencesNothing runs gosec with the
// repository's .golangci.yml over a module whose program starts are each
// reported: one bare, one under each suppression comment, and one in a file
// and a test file headed as generated. A start with no variable is the clean
// control, so the lint is reading the module.
func TestAGosecSuppressionCommentSilencesNothing(t *testing.T) {
	start := func(pkg, name, comment string) string {
		return fmt.Sprintf("package %s\n\nimport (\n\t\"context\"\n\t\"os/exec\"\n)\n\n// %s runs go with args.\nfunc %s(ctx context.Context, args ...string) error {\n\treturn exec.CommandContext(ctx, \"go\", args...).Run()%s\n}\n", pkg, name, name, comment)
	}
	marks := []string{"#" + "nosec", "gosec" + ":disable"}
	header := "// Code " + "generated by hand. DO NOT " + "EDIT.\n\n" // spelled apart, so this file holds no header
	reported := map[string]string{
		"run/bare.go":           start("run", "Bare", ""),
		"run/nosec.go":          start("run", "Nosec", " // "+marks[0]+" G204"),
		"run/disable.go":        start("run", "Disable", " //"+marks[1]+" G204"),
		"run/generated.go":      header + start("run", "Generated", ""),
		"run/generated_test.go": header + start("run", "GeneratedTest", ""),
	}
	files := map[string]string{
		".golangci.yml": lintConfig(t),
		"go.mod":        "module example.com/planted\n\ngo 1.23.0\n",
		"run/doc.go":    "// Package run starts go.\npackage run\n",
		"run/fixed.go":  "package run\n\nimport (\n\t\"context\"\n\t\"os/exec\"\n)\n\n// Fixed runs go version.\nfunc Fixed(ctx context.Context) error {\n\treturn exec.CommandContext(ctx, \"go\", \"version\").Run()\n}\n",
	}
	maps.Copy(files, reported)
	bin := pinnedLint(t)
	out, ok := lint(t, bin, writeModule(t, files), exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--enable-only=gosec", "./..."))
	if ok {
		t.Fatalf("gosec found nothing:\n%s", out)
	}
	for name, body := range reported {
		line := strings.Count(body[:strings.Index(body, "exec.CommandContext")], "\n") + 1
		if !strings.Contains(out, fmt.Sprintf("%s:%d:9: G204", name, line)) {
			t.Errorf("%s is not reported at line %d:\n%s", name, line, out)
		}
	}
	if strings.Contains(out, "run/fixed.go") {
		t.Errorf("a start with no variable is reported:\n%s", out)
	}
}

// TestEveryGosecRuleRunsAndTheFourNamedFamiliesFire runs gosec with the
// repository's .golangci.yml over a module that plants each family
// supply.scans.3 names, a query built by formatting (G201) and by
// concatenation (G202) and weak randomness (G404), and a file read by a
// variable path (G304), which runs only when every rule does. The control
// is a parameterised query, which passes; the same G304 read in a test file
// is the tests' exclusion, and passes too. gosec reads a query only in an
// assignment or a bare call, never in a return, and G201 only a query held
// in a variable declared with :=, so each plant has that shape.
func TestEveryGosecRuleRunsAndTheFourNamedFamiliesFire(t *testing.T) {
	const imports = "import (\n\t\"context\"\n\t\"database/sql\"\n\t\"fmt\"\n\t\"math/rand\"\n\t\"os\"\n)\n\n"
	reported := map[string]string{
		"G201": "// Format queries a name built by formatting.\nfunc Format(ctx context.Context, db *sql.DB, name string) (*sql.Rows, error) {\n\tquery := fmt.Sprintf(\"SELECT * FROM t WHERE name = '%s'\", name)\n\trows, err := db.QueryContext(ctx, query)\n\treturn rows, err\n}\n",
		"G202": "// Concat queries a name built by concatenation.\nfunc Concat(ctx context.Context, db *sql.DB, name string) (*sql.Rows, error) {\n\trows, err := db.QueryContext(ctx, \"SELECT * FROM t WHERE name = '\"+name+\"'\")\n\treturn rows, err\n}\n",
		"G404": "// Weak is a number from the weak source.\nfunc Weak() int { return rand.Int() }\n",
		"G304": "// Read reads the file at p.\nfunc Read(p string) ([]byte, error) { return os.ReadFile(p) }\n",
	}
	body := "package q\n\n" + imports
	for _, id := range slices.Sorted(maps.Keys(reported)) {
		body += reported[id] + "\n"
	}
	files := map[string]string{
		".golangci.yml":  lintConfig(t),
		"go.mod":         "module example.com/planted\n\ngo 1.23.0\n",
		"q/doc.go":       "// Package q queries.\npackage q\n",
		"q/planted.go":   body,
		"q/param.go":     "package q\n\nimport (\n\t\"context\"\n\t\"database/sql\"\n)\n\n// Param queries a name as a parameter.\nfunc Param(ctx context.Context, db *sql.DB, name string) (*sql.Rows, error) {\n\trows, err := db.QueryContext(ctx, \"SELECT * FROM t WHERE name = ?\", name)\n\treturn rows, err\n}\n",
		"q/read_test.go": "package q\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestRead(t *testing.T) {\n\tp := t.TempDir() + \"/x\"\n\tif _, err := os.ReadFile(p); err == nil {\n\t\tt.Fatal(p)\n\t}\n}\n",
	}
	out, ok := lint(t, pinnedLint(t), writeModule(t, files), exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--enable-only=gosec", "./..."))
	if ok {
		t.Fatalf("gosec found nothing:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	for id := range reported {
		if !slices.ContainsFunc(lines, func(l string) bool {
			return strings.HasPrefix(l, "q/planted.go:") && strings.Contains(l, ": "+id+":")
		}) {
			t.Errorf("%s is not reported:\n%s", id, out)
		}
	}
	for _, clean := range []string{"q/param.go", "q/read_test.go"} {
		if strings.Contains(out, clean) {
			t.Errorf("%s is reported:\n%s", clean, out)
		}
	}
}

// TestNoExclusionDropsANamedGosecFamily (supply.scans.3): gosec's settings
// hold no includes and no excludes list, so every rule runs, and no
// exclusion rule silences G201, G202 or G404 at any path. G204 is held by
// TestOnlyAProgramsAdapterIsSparedG204. Planted failure: a rule naming G404,
// or one for gosec with no text.
func TestNoExclusionDropsANamedGosecFamily(t *testing.T) {
	repo := lintDoc(t, ".golangci.yml")
	for _, k := range []string{"includes", "excludes"} {
		if _, ok := setting(repo, "linters.settings.gosec."+k); ok {
			t.Errorf("gosec's settings hold %s, which narrows the rules that run", k)
		}
	}
	rules := exclusions(t)
	for _, r := range rules {
		if (len(r.linters) == 0 || slices.Contains(r.linters, "gosec")) && r.text == "" {
			t.Errorf("the exclusion at path %q, path-except %q, silences every gosec rule: name its rule ids in text", r.path, r.pathExcept)
		}
	}
	for _, finding := range []string{"G201: SQL string formatting", "G202: SQL string concatenation", "G404: Use of weak random number generator"} {
		for _, path := range []string{"internal/core/x/x.go", "cmd/gragas-dev/x.go", "internal/host/fs/x.go", "internal/host/git/x.go", "x_test.go"} {
			if silenced(t, rules, path, finding) {
				t.Errorf("%s: %q is silenced", path, finding)
			}
		}
	}
}

// TestEveryFileReviveGoesQuietOnFailsTheTreeCheck runs revive with the
// repository's .golangci.yml over a module where each file holds a name
// var-naming reports: one bare, and one under each of revive's directives.
// The bare one is reported, so the lint is reading the module, and each file
// revive is quiet on is one the tree check refuses: the directives silence
// revive, and suppressions.txt never counts them.
func TestEveryFileReviveGoesQuietOnFailsTheTreeCheck(t *testing.T) {
	revive := "revive" + ":" // spelled apart, so this file holds no directive
	planted := map[string]string{
		"run/bare.go": "package run\n\nvar aUrl = 1\n",
		"run/off.go":  "//" + revive + "disable\n\npackage run\n\nvar bUrl = 2\n",
		"run/rule.go": "//" + revive + "disable:var-naming\n\npackage run\n\nvar cUrl = 3\n",
		"run/line.go": "package run\n\nvar dUrl = 4 //" + revive + "disable-line\n",
		"run/next.go": "package run\n\n//" + revive + "disable-next-line:var-naming\nvar eUrl = 5\n",
	}
	files := map[string]string{
		".golangci.yml": lintConfig(t),
		"go.mod":        "module example.com/planted\n\ngo 1.23.0\n",
		"run/doc.go":    "// Package run holds names.\npackage run\n",
	}
	maps.Copy(files, planted)
	dir := writeModule(t, files)
	bin := pinnedLint(t)
	out, _ := lint(t, bin, dir, exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--enable-only=revive", "./..."))
	if !strings.Contains(out, "run/bare.go:3:5: var-naming") {
		t.Fatalf("the bare name is not reported, so revive is not reading the module:\n%s", out)
	}
	found, read, err := silencedLines(os.DirFS(dir))
	if err != nil || read != len(files)-2 {
		t.Fatalf("the tree check read %d Go files, want %d: %v", read, len(files)-2, err)
	}
	quiet := 0
	for name := range planted {
		if strings.Contains(out, name+":") {
			continue
		}
		quiet++
		if !slices.ContainsFunc(found, func(f string) bool { return strings.HasPrefix(f, name+":") }) {
			t.Errorf("revive is quiet on %s, and the tree check does not refuse it:\n%s", name, files[name])
		}
	}
	if quiet == 0 {
		t.Logf("revive reads no directive here, so nothing is silenced:\n%s", out)
	}
}

// TestAWrapperInPlaceOfTheLintBinaryIsRefusedByItsSum: a test's own
// .tools/bin holds a script that runs the real, pinned golangci-lint. It says
// what the real one says, so the version check alone passes it; the sum check
// refuses it by name, and accepts the real binary. Planted failure: a binary
// whose sha256 tools.yaml does not pin. Risk: a lint verdict from a program
// the pin never named.
func TestAWrapperInPlaceOfTheLintBinaryIsRefusedByItsSum(t *testing.T) {
	_, sums, err := lintPin(lintDoc(t, "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	real := pinnedLintPath(t)
	if err := sumPinned(real, sums); err != nil {
		t.Fatalf("the control, the pinned binary, was refused: %v", err)
	}
	wrapperDir := filepath.Join(t.TempDir(), ".tools", "bin")
	if err := os.MkdirAll(wrapperDir, 0o750); err != nil {
		t.Fatal(err)
	}
	wrapper := configtest.Fake(t, wrapperDir, "golangci-lint", configtest.Step{Exec: real})
	err = sumPinned(wrapper, sums)
	if err == nil || !strings.Contains(err.Error(), wrapper) {
		t.Errorf("sumPinned(wrapper) = %v, want a refusal naming %s", err, wrapper)
	}
}
