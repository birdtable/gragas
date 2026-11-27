package coreapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const projStructure = `format: 3
layers:
  - slug: language
    name: Language
    parts:
      - slug: go
        name: Go
        concerns:
          - slug: errors
            name: Errors
      - slug: ts
        name: TypeScript
        concerns:
          - slug: style
            name: Style
kinds: []
`

const projSources = `format: 3
sources:
  - slug: effective-go
    kind: guide
    title: Effective Go
    locator: https://go.dev/doc/effective_go
    attribution: The Go authors
    verified: "2026-11-01"
`

// rule is one rule of the fixture book with a rung 0 binding of tool, key.
func rule(id, status, tool, key string) string {
	r := "  - id: " + id + "\n    statement: Hold " + key + ".\n    strength: must\n    status: " + status + "\n    origin: authored\n"
	if status == "retired" {
		r += "    replaced_by: go.errors.1\n"
	}
	if status != "proposed" {
		r += "    sources:\n      - {ref: effective-go, backs: rule}\n"
	}
	return r + "    bindings:\n      - {tool: " + tool + ", target: config, key: " + key + ", severity: error, rung: 0, run_point: push}\n"
}

func goRules(rules ...string) string {
	return "format: 3\npart: go\nconcern: errors\nrules:\n" + strings.Join(rules, "")
}

type projWorld struct {
	*porttest.World
	book, root model.Root
}

// projectOf seeds a book with the go rules and the project with files, and
// answers the loaded project that pins the parts.
func projectOf(t *testing.T, goFile string, files map[string]string, parts ...string) (projWorld, project.Loaded) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/sources.schema.json")
	book := w.Seed("book", map[string]string{
		"rulebook.yaml": bookIdentity, "structure.yaml": projStructure, "sources.yaml": projSources,
		"language/go/rules/errors.yaml": goFile,
		"language/ts/rules/style.yaml":  "format: 3\npart: ts\nconcern: style\nrules: []\n",
	})
	root := w.Seed("proj", files)
	return projWorld{w, book, root}, project.Loaded{Root: root, Rulebook: book, PinPath: ".gragas/pin.yaml", Pin: model.Pin{Rulebook: "example", Source: "book", Parts: parts, Format: 1}}
}

func readersBoth() coreapi.Readers {
	return coreapi.Readers{Rows: []coreapi.Reader{
		{Tool: "golangci-lint", Files: []string{".golangci.yml"}, Kind: "entries", Exclusions: linterCounting()[0].Exclusions, Directive: "nolint"},
		{Tool: "task", Files: []string{"Taskfile.yml"}, Kind: "task-names"},
	}}
}

func today() string { return "2026-11-22" }

func asked() coreapi.ProjectAsk {
	return coreapi.ProjectAsk{Readers: readersBoth(), Today: today, NamesFile: "/names"}
}

func checkIn(t *testing.T, w projWorld, l project.Loaded, a coreapi.ProjectAsk) (model.ProjectCheck, error) {
	t.Helper()
	return coreapi.CheckProjectLoaded(context.Background(), w.Ports(), l, a)
}

func ofKind(p model.ProjectCheck, k model.GateKind) []model.GateFinding {
	var out []model.GateFinding
	for _, f := range p.Findings {
		if f.Kind == k {
			out = append(out, f)
		}
	}
	return out
}

func rowOf(p model.ProjectCheck, tool string) (model.NotChecked, bool) {
	for _, n := range p.NotChecked {
		if n.Tool == tool {
			return n, true
		}
	}
	return model.NotChecked{}, false
}

const taskfileWith = "version: '3'\ntasks:\n  lint: {cmds: [echo]}\n"

// Planted failure: an absent entry is passed over.
func TestCheckProjectFindsAnAbsentEntry(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	got, err := checkIn(t, w, l, asked())
	if err != nil {
		t.Fatal(err)
	}
	f := ofKind(got, model.GateToolConfig)
	if len(f) != 1 || !strings.Contains(f[0].Msg, "go.errors.1") || !strings.Contains(f[0].Msg, "no task gate") {
		t.Fatalf("findings = %+v", got.Findings)
	}
}

// Planted failure: the clean control is a finding.
func TestCheckProjectCleanControlHasNoFinding(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	got, err := checkIn(t, w, l, asked())
	if err != nil || len(ofKind(got, model.GateToolConfig)) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// Planted failure: a trial rule is not judged.
func TestCheckProjectJudgesATrialRule(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "trial", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	got, _ := checkIn(t, w, l, asked())
	if len(ofKind(got, model.GateToolConfig)) != 1 {
		t.Fatalf("findings = %+v", got.Findings)
	}
}

// Planted failure: a proposed or retired rule is judged.
func TestCheckProjectLeavesAProposedAndARetiredRuleAlone(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint"), rule("go.errors.2", "proposed", "task", "gate"), rule("go.errors.3", "retired", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	got, err := checkIn(t, w, l, asked())
	if err != nil || len(got.Findings) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// Planted failure: .gragas/parts names the parts.
func TestCheckProjectIgnoresAPartsFile(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith, ".gragas/parts": "ts\n"}, "go")
	got, _ := checkIn(t, w, l, asked())
	if len(ofKind(got, model.GateToolConfig)) != 1 {
		t.Fatalf("the pin's part go was not judged: %+v", got.Findings)
	}
	w, l = projectOf(t, goRules(rule("go.errors.1", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith, ".gragas/parts": "go\n"}, "ts")
	got, _ = checkIn(t, w, l, asked())
	if len(ofKind(got, model.GateToolConfig)) != 0 {
		t.Fatalf("a part the pin leaves out was judged: %+v", got.Findings)
	}
}

// Planted failure: no config file is a pass.
func TestCheckProjectWithNoConfigFileFindsTheEntryAbsent(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"README.md": "x"}, "go")
	got, _ := checkIn(t, w, l, asked())
	f := ofKind(got, model.GateToolConfig)
	if len(f) != 1 || !strings.Contains(f[0].Msg, "there is no Taskfile.yml") {
		t.Fatalf("findings = %+v", got.Findings)
	}
}

// Planted failure: a binding of a tool with no reader is dropped silently.
func TestCheckProjectNeverDropsABinding(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "vale", "x"), rule("go.errors.2", "active", "vale", "y")), map[string]string{"a": "b"}, "go")
	got, _ := checkIn(t, w, l, asked())
	n, ok := rowOf(got, "vale")
	if !ok || n.Bindings != 2 {
		t.Fatalf("not checked = %+v", got.NotChecked)
	}
	if got.Word() != model.NotReady {
		t.Errorf("word = %s, want not ready", got.Word())
	}
}

// Planted failure: a refused readers table is read as none to read.
func TestCheckProjectANotCheckedRowCarriesItsToolAndCount(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint"), rule("go.errors.2", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	a := asked()
	a.Readers = coreapi.Readers{Refused: "audit/readers.yaml: bad row"}
	got, _ := checkIn(t, w, l, a)
	n, ok := rowOf(got, "task")
	if !ok || n.Bindings != 2 || !strings.Contains(n.Msg, "bad row") {
		t.Fatalf("not checked = %+v", got.NotChecked)
	}
}

// Planted failure: a book with no rule is a pass.
func TestCheckProjectRefusesAnEmptyBook(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	if _, err := checkIn(t, w, l, asked()); !errors.Is(err, coreapi.ErrNotReady) {
		t.Fatalf("err = %v, want not ready", err)
	}
}

// Planted failure: a part the book lacks is skipped.
func TestCheckProjectRefusesAPartTheBookLacks(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "go", "rust")
	_, err := checkIn(t, w, l, asked())
	if !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), "rust") {
		t.Fatalf("err = %v", err)
	}
}

// Planted failure: a pin whose parts bind nothing says nothing.
func TestCheckProjectSaysWhenNothingBinds(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "ts")
	got, err := checkIn(t, w, l, asked())
	if err != nil {
		t.Fatal(err)
	}
	n, ok := rowOf(got, "")
	if !ok || !strings.Contains(n.Msg, "binds no tool") && !strings.Contains(n.Msg, "binds a tool") {
		t.Fatalf("not checked = %+v", got.NotChecked)
	}
	if got.Word() != model.Done {
		t.Errorf("a row with no tool changed the ending: %s", got.Word())
	}
}

// Planted failure: a tier table that was not read says nothing.
func TestCheckProjectSaysWhenTheTiersWereNotRead(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	got, _ := checkIn(t, w, l, asked())
	var said bool
	for _, n := range got.NotChecked {
		said = said || n.Tool == "" && strings.Contains(n.Msg, "tier contract was not checked")
	}
	if !said {
		t.Fatalf("not checked = %+v", got.NotChecked)
	}
}

// Planted failure: no names list named is no row.
func TestCheckProjectSaysWhenNoNamesListIsNamed(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	a := asked()
	a.NamesFile = ""
	got, _ := checkIn(t, w, l, a)
	var said bool
	for _, n := range got.NotChecked {
		said = said || n.Tool == "" && strings.HasPrefix(n.Msg, "names: no list named")
	}
	if !said {
		t.Fatalf("not checked = %+v", got.NotChecked)
	}
}

const exemptYAML = `exemptions:
  - {id: a, rule: go.errors.1, path: ., expires: "2027-03-31", reason: r, exit: "o/r#1", owner: o}
`

// Planted failure: a live row does not excuse its rule.
func TestCheckProjectAnExemptionRowExcusesAFinding(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith, "gate/exemptions.yaml": exemptYAML}, "go")
	got, _ := checkIn(t, w, l, asked())
	if len(got.Findings) != 0 || len(got.Exempt) != 1 || !strings.Contains(got.Exempt[0], "exempt until 2027-03-31") {
		t.Fatalf("got %+v", got)
	}
}

func TestExemptionsAnExpiredRowIsAFinding(t *testing.T) {
	t.Parallel()
	rows := strings.Replace(exemptYAML, "2027-03-31", "2026-01-01", 1)
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith, "gate/exemptions.yaml": rows}, "go")
	got, _ := checkIn(t, w, l, asked())
	e := ofKind(got, model.GateExemption)
	if len(e) != 1 || !strings.Contains(e[0].Msg, "expired on 2026-01-01") || len(ofKind(got, model.GateToolConfig)) != 1 {
		t.Fatalf("got %+v", got.Findings)
	}
}

func TestExemptionsARowForARetiredRuleIsAFinding(t *testing.T) {
	t.Parallel()
	rows := strings.Replace(exemptYAML, "go.errors.1", "go.errors.2", 1)
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint"), rule("go.errors.2", "retired", "task", "x")), map[string]string{"Taskfile.yml": taskfileWith, "gate/exemptions.yaml": rows}, "go")
	got, _ := checkIn(t, w, l, asked())
	e := ofKind(got, model.GateExemption)
	if len(e) != 1 || !strings.Contains(e[0].Msg, "not an active rule") {
		t.Fatalf("got %+v", got.Findings)
	}
}

func TestExemptionsRefuseARowForNoRule(t *testing.T) {
	t.Parallel()
	rows := strings.Replace(exemptYAML, "go.errors.1", "go.nothing.9", 1)
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith, "gate/exemptions.yaml": rows}, "go")
	got, _ := checkIn(t, w, l, asked())
	if e := ofKind(got, model.GateExemption); len(e) != 1 || !strings.Contains(e[0].Msg, "go.nothing.9") {
		t.Fatalf("got %+v", got.Findings)
	}
}

func TestExemptionsNoFileMeansNothingExempt(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	got, _ := checkIn(t, w, l, asked())
	if len(ofKind(got, model.GateExemption)) != 0 || len(got.Exempt) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestExemptionsWithNoDateMakeNoRowLive(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "task", "gate")), map[string]string{"Taskfile.yml": taskfileWith, "gate/exemptions.yaml": exemptYAML}, "go")
	a := asked()
	a.Today = nil
	got, _ := checkIn(t, w, l, a)
	if len(got.Exempt) != 0 || len(ofKind(got, model.GateToolConfig)) != 1 || len(ofKind(got, model.GateExemption)) != 1 {
		t.Fatalf("got %+v", got)
	}
}

// Planted failure: a Taskfile the tier contract cannot read as a mapping ends
// the run done, because the tier row dropped its tool and so left nothing
// unjudged. No active rule binds a tool, so only the tier contract reads the
// Taskfile and its row is the one that must end the run.
func TestCheckProjectATaskfileTheTiersCannotReadEndsNotReady(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, goRules(rule("go.errors.1", "proposed", "task", "lint")), map[string]string{"Taskfile.yml": "- not a mapping\n"}, "go")
	tt := coreapi.LoadTiers(embedded(t).Ports().Assets)
	if tt.Refused != "" {
		t.Fatalf("the embedded tier table is refused: %q", tt.Refused)
	}
	a := asked()
	a.Tiers = tt
	got, _ := checkIn(t, w, l, a)
	var row bool
	for _, n := range got.NotChecked {
		row = row || n.Tool == "task" && strings.Contains(n.Msg, "cannot be read as a mapping")
	}
	if !row || got.Word() != model.NotReady {
		t.Fatalf("word %v, not checked %+v; want not ready on a task row that says it cannot be read", got.Word(), got.NotChecked)
	}
}

// TestCheckProjectAtEndsEachRefusalOfTheLoadNotReady: project.Load's real
// refusals (no repository, a source not there, a pin that is a folder, a pin
// newer than this gragas reads) each come back ErrNotReady, in the project's
// words, so the run ends not ready and not failed.
func TestCheckProjectAtEndsEachRefusalOfTheLoadNotReady(t *testing.T) {
	t.Parallel()
	const pin = ".gragas/pin.yaml"
	good := "rulebook: example\nsource: book\nparts: [go]\nformat: 1\n"
	for _, c := range []struct {
		name  string
		files map[string]string
		says  string
	}{
		{"no repository around", map[string]string{pin: good}, "no .git"},
		{"a source that is not there", map[string]string{".git": "gitdir: x\n", pin: good}, "book"},
		{"a pin that is a folder", map[string]string{".git": "gitdir: x\n", pin + "/inside": "a\n"}, "folder"},
		{"a pin newer than this gragas reads", map[string]string{".git": "gitdir: x\n", pin: "rulebook: example\nsource: book\nparts: [go]\nformat: 99\n"}, "format 99"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			root := w.Seed("/w/repo", c.files)
			_, err := coreapi.CheckProjectAt(t.Context(), w.Ports(), string(root), pin, asked())
			if !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("CheckProjectAt = %v; want ErrNotReady in words that name %q", err, c.says)
			}
		})
	}
}
