package coreapi_test

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// embedded is the audit data the program embeds, seeded into a world's
// Assets port, so the tests read the real table and not a copy of it.
func embedded(t *testing.T) *porttest.World {
	t.Helper()
	files := map[string]string{}
	for name, path := range map[string]string{
		coreapi.TiersFile:        "../../host/assets/audit/tiers.yaml",
		coreapi.SystemsTableFile: "../../host/assets/audit/systems.yaml",
		coreapi.ReadersFile:      "../../host/assets/audit/readers.yaml",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(b)
	}
	w := porttest.NewWorld(t)
	w.SeedAssets(files)
	return w
}

// tiersOf reads the tier contract of a fixture repository against the
// embedded table, the way the check does.
func tiersOf(t *testing.T, fixture, exemptions string) coreapi.Conformance {
	t.Helper()
	w := embedded(t)
	tt := coreapi.LoadTiers(w.Ports().Assets)
	rd := coreapi.LoadReaders(w.Ports().Assets)
	if tt.Refused != "" || rd.Refused != "" {
		t.Fatalf("the embedded data is refused: %q %q", tt.Refused, rd.Refused)
	}
	dir := os.DirFS(filepath.Join("testdata", "tiers", fixture)).(fs.ReadFileFS)
	if exemptions != "" {
		dir = withExemptions{dir, exemptions}
	}
	return coreapi.TierContract(dir, rd.Rows, tt, "2026-11-15")
}

// withExemptions is a repository with a gate/exemptions.yaml laid over it.
type withExemptions struct {
	fs.ReadFileFS
	data string
}

func (w withExemptions) ReadFile(name string) ([]byte, error) {
	if name == "gate/exemptions.yaml" {
		return []byte(w.data), nil
	}
	return w.ReadFileFS.ReadFile(name)
}

// findingLines is a result's findings as a person reads them.
func findingLines(r coreapi.Conformance) []string {
	out := []string{}
	for _, f := range r.Findings {
		out = append(out, f.String())
	}
	return out
}

// TestTheCleanTaskfileListsNothing: four tiers, the members in cmds with
// their timeouts, a build and a lint for each declared system, lint with
// --allow-parallel-runners, an empty-run guard in full, a landing task that
// calls pre-push and then full, and a hook that calls the pre-push task.
func TestTheCleanTaskfileListsNothing(t *testing.T) {
	t.Parallel()
	r := tiersOf(t, "clean", "")
	line := "task: a member's timeout holds only from go-task 3.53.1; this check cannot see the go-task version"
	if len(r.Findings)+len(r.Exempt) != 0 || len(r.NotChecked) != 1 || r.NotChecked[0].String() != line {
		t.Fatalf("the clean fixture reports %+v, want only the line %q", r, line)
	}
}

// TestTheBadTaskfileListsEachFindingByNameAndLine is the fixture the issue
// names: no release task, no landing task, a member with no timeout, no
// lint run per system, and the rest of what a tier holds. Each finding names
// the file and the line it is at.
func TestTheBadTaskfileListsEachFindingByNameAndLine(t *testing.T) {
	t.Parallel()
	got := findingLines(tiersOf(t, "bad", ""))
	want := []string{
		"Taskfile.yml:12: gates.command.8 (task pre-push build timeout): the build member of pre-push starts a process tree and has no timeout:",
		"Taskfile.yml:10: gates.command.1 (task pre-push build linux/amd64): the task pre-push has no build member for linux/amd64",
		"Taskfile.yml:10: gates.command.1 (task pre-push build darwin/arm64): the task pre-push has no build member for darwin/arm64",
		"Taskfile.yml:13: gates.command.22 (task pre-push lint --allow-parallel-runners): the lint member of pre-push runs without --allow-parallel-runners",
		"Taskfile.yml:10: go.cycle.2 (task pre-push lint linux/amd64): the task pre-push has no lint member for linux/amd64",
		"Taskfile.yml:10: go.cycle.2 (task pre-push lint darwin/arm64): the task pre-push has no lint member for darwin/arm64",
		"Taskfile.yml:19: gates.command.8 (task full &): a member of full runs a command in the background with &: gragas check & sleep 1",
		"Taskfile.yml:16: gates.command.1 (task full empty): the task full runs tests through gotestsum but no member reads its count line (EMPTY or DONE 0 tests or tests count) and exits 1, so a package with no tests is green",
		"Taskfile.yml:2: gates.command.1 (task release): no task release",
		"Taskfile.yml:2: gates.command.1 (task land landing): no task landing: the land job, which runs pre-push then full on the merged tree before main",
		".gragas/systems:1: gates.command.1 (task systems plan9/arm64): plan9/arm64 is not a GOOS/GOARCH pair Go builds for",
		"hooks/pre-push: gates.hooks.4 (task pre-push hook): the hook hooks/pre-push does not call the pre-push task: it lacks \"task pre-push\"",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestAnUndeclaredSystemListIsSaidAsALine: with no .gragas/systems one build
// and one lint member are asked for, and the report says so, as a partial
// not-checked line that is no finding.
func TestAnUndeclaredSystemListIsSaidAsALine(t *testing.T) {
	t.Parallel()
	r := tiersOf(t, "nosystems", "")
	if len(r.Findings) != 0 {
		t.Fatalf("findings = %v, want none", findingLines(r))
	}
	want := "systems: none declared in .gragas/systems, so one build and one lint member are asked for"
	if len(r.NotChecked) != 2 || r.NotChecked[1].String() != want {
		t.Errorf("not checked = %+v, want the timeout line and then %q", r.NotChecked, want)
	}
}

// TestARepoWithNoTaskfileGetsTheOneFindingThatSaysSo, never an error.
func TestARepoWithNoTaskfileGetsTheOneFindingThatSaysSo(t *testing.T) {
	t.Parallel()
	got := findingLines(tiersOf(t, "notaskfile", ""))
	want := "Taskfile.yml: gates.command.1 (task tiers): there is no Taskfile.yml, so there are no tiers: pre-commit, pre-push, full, release"
	if !slices.Contains(got, want) {
		t.Errorf("findings = %v, want one of them %q", got, want)
	}
	for _, g := range got {
		if strings.Contains(g, "task pre-push ") || strings.Contains(g, "task pre-commit ") {
			t.Errorf("a member finding %q for a repo with no Taskfile", g)
		}
	}
}

// TestAFindingWithALiveExemptionRowIsExempt, with its expiry, and is no
// finding.
func TestAFindingWithALiveExemptionRowIsExempt(t *testing.T) {
	t.Parallel()
	r := tiersOf(t, "bad", "exemptions:\n  - rule: gates.hooks.4\n    expires: 2026-12-31\n")
	if len(r.Exempt) != 1 || r.Exempt[0].Rule != "gates.hooks.4" || r.Exempt[0].Until != "2026-12-31" {
		t.Errorf("exempt = %+v, want the hook's rule until 2026-12-31", r.Exempt)
	}
	for _, f := range r.Findings {
		if f.Rule == "gates.hooks.4" {
			t.Errorf("finding %v is exempt and must not be listed", f)
		}
	}
}

// TestTheFindingsJSONCarriesFileAndLine: --json is the same findings, each
// with its file and line.
func TestTheFindingsJSONCarriesFileAndLine(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(tiersOf(t, "bad", "").Findings[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["file"] != "Taskfile.yml" || m["line"] == nil || m["rule"] == nil || m["missing"] == nil {
		t.Errorf("a finding as JSON = %s, want file, line, rule and missing", b)
	}
}

// TestTheTierContractRunsNothingAndKeepsNoStamp is the tree test of the tier
// contract's `git grep` line: the files that read the tier contract import no package
// that starts a program, reads a clock or seals a stamp, and name no budget.
func TestTheTierContractRunsNothingAndKeepsNoStamp(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	for _, path := range []string{"tiers.go", "../audit/tiers.go"} {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if slices.Contains([]string{"os", "os/exec", "syscall", "net", "time", "crypto/hmac", "crypto/sha256", "context"}, p) {
				t.Errorf("%s imports %s: the tier contract starts no program, reads no clock and seals no stamp", path, p)
			}
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range []string{"stamp", "Stamp", "budget", "Budget"} {
			if strings.Contains(string(src), word) {
				t.Errorf("%s mentions %q: the tier contract keeps no stamp and reads no time budget", path, word)
			}
		}
	}
}

// TestTheTierTableIsRefusedAsAWholeWhenItIsNotWhole: a missing tier, a member
// with no rule or a bad rule id refuse the table, so the contract is never
// half checked.
func TestTheTierTableIsRefusedAsAWholeWhenItIsNotWhole(t *testing.T) {
	t.Parallel()
	good, err := os.ReadFile("../../host/assets/audit/tiers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	sys, err := os.ReadFile("../../host/assets/audit/systems.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ from, to, want string }{
		"a tier gone": {"  - name: release\n    members: []\n    whole: true\n", "", "names 3 tiers"},
		// The land entry: the land job's task and the tiers it calls, the hook's among them.
		"no land entry":          {"land:\n  task: landing\n  calls: [pre-push, full]\n", "", "has no land mapping"},
		"a land with no task":    {"  task: landing\n", "  task: ''\n", "needs a task and the tiers"},
		"a land with no calls":   {"  calls: [pre-push, full]\n", "  calls: []\n", "needs a task and the tiers"},
		"a land call no tier":    {"  calls: [pre-push, full]\n", "  calls: [pre-push, nightly]\n", "\"nightly\", which is not a tier"},
		"a land call twice":      {"  calls: [pre-push, full]\n", "  calls: [pre-push, pre-push]\n", "is named twice"},
		"a land without push":    {"  calls: [pre-push, full]\n", "  calls: [full]\n", "does not call pre-push"},
		"a land rule no rule id": {"  land: gates.command.1\n", "  land: gates\n", "is not a rule id"},
		"a hook of no tier":      {"  calls: [\"task pre-push\"]\n", "  calls: [\"make check\"]\n", "its hook calls no tier of the table"},
		"a bad rule id":          {"tier: gates.command.1", "tier: gates", "is not a rule id"},
		"a member no id":         {"{id: tidy,", "{id: '',", "needs an id"},
		"a flag no rule":         {", flag_rule: gates.command.22", "", "needs an id"},
		"a tier misnamed":        {"name: pre-commit", "name: precommit", "want \"pre-commit\""},
		// Only the commit tier may carry the staged-inputs shape of skip.
		"the push tier staged": {"  - name: pre-push\n", "  - name: pre-push\n    staged: true\n", "only the commit tier may carry"},
	} {
		w := porttest.NewWorld(t)
		w.SeedAssets(map[string]string{coreapi.TiersFile: strings.Replace(string(good), tc.from, tc.to, 1), coreapi.SystemsTableFile: string(sys)})
		r := coreapi.LoadTiers(w.Ports().Assets)
		if r.Refused == "" || !strings.Contains(r.Refused, tc.want) || r.Count() != 0 {
			t.Errorf("%s: TierTable = %+v, want refused with %q and no tiers", name, r, tc.want)
		}
	}
}

// TestAMemberGoTaskDoesNotRunOrCannotFailIsAFindingAndHoldsNothing is the
// reviewer's probe: each member of the pre-push tier carries one key that makes
// go-task skip it, swallow its failure or drop the key, and the tier itself has
// a status. Each key is a finding at its line, the member counts for nothing,
// and no member gets a system from an env it does not read.
func TestAMemberGoTaskDoesNotRunOrCannotFailIsAFindingAndHoldsNothing(t *testing.T) {
	t.Parallel()
	got := findingLines(tiersOf(t, "probe", ""))
	for _, want := range []string{
		"Taskfile.yml:8: gates.command.1 (task pre-push status)",
		"Taskfile.yml:10: gates.command.1 (task pre-push member ignore_error:)",
		"Taskfile.yml:12: gates.command.1 (task pre-push member env:)",
		"Taskfile.yml:13: gates.command.1 (task pre-push member task: beside cmd:)",
		"Taskfile.yml:15: gates.command.1 (task pre-push member if:)",
		"Taskfile.yml:16: gates.command.1 (task pre-push member platforms:)",
		"Taskfile.yml:17: gates.command.1 (task pre-push member defer:)",
		"Taskfile.yml:18: gates.command.1 (task pre-push member for:)",
		"has no vet member",
		"has no tests member",
		"has no build member for darwin/arm64",
		"has no lint member",
		"has no gitleaks member",
		"has no govulncheck member",
	} {
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, want) }) {
			t.Errorf("no finding with %q in:\n%s", want, strings.Join(got, "\n"))
		}
	}
}

// TestTheTaskfileIsTheOneGoTaskRuns: go-task's names in go-task's order, each
// matched exactly against the listing, whatever the host does with case.
func TestTheTaskfileIsTheOneGoTaskRuns(t *testing.T) {
	t.Parallel()
	clean, err := os.ReadFile(filepath.Join("testdata", "tiers", "clean", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	empty := []byte("version: '3'\ntasks: {}\n")
	w := embedded(t)
	tt := coreapi.LoadTiers(w.Ports().Assets)
	rd := coreapi.LoadReaders(w.Ports().Assets)
	for _, c := range []struct {
		name  string
		files map[string][]byte
		clean bool
	}{
		{"only a lower-case name", map[string][]byte{"taskfile.yml": clean}, true},
		{"only a dist file", map[string][]byte{"Taskfile.dist.yaml": clean}, true},
		{"taskfile.yml runs before Taskfile.yaml", map[string][]byte{"taskfile.yml": clean, "Taskfile.yaml": empty}, true},
		{"Taskfile.yml runs before taskfile.yml", map[string][]byte{"Taskfile.yml": empty, "taskfile.yml": clean}, false},
	} {
		repo := fstest.MapFS{"hooks/pre-push": {Data: []byte("task pre-push\n")}, ".gragas/systems": {Data: []byte("linux/amd64 darwin/arm64\n")}}
		for n, d := range c.files {
			repo[n] = &fstest.MapFile{Data: d}
		}
		r := coreapi.TierContract(repo, rd.Rows, tt, "2026-11-15")
		if got := len(r.Findings) == 0; got != c.clean {
			t.Errorf("%s: findings = %v, want clean %v", c.name, findingLines(r), c.clean)
		}
	}
}

// TestSetNoexecAtAnyLevelIsAFindingAndAMemberUnderItHoldsNothing: go-task hands
// set: to its shell, and n or noexec makes the shell read each command and run
// none, so the tier exits 0 having checked nothing. The member, the task and the
// file each say it; pipefail is the control.
func TestSetNoexecAtAnyLevelIsAFindingAndAMemberUnderItHoldsNothing(t *testing.T) {
	t.Parallel()
	clean, err := os.ReadFile(filepath.Join("testdata", "tiers", "clean", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	w := embedded(t)
	tt := coreapi.LoadTiers(w.Ports().Assets)
	rd := coreapi.LoadReaders(w.Ports().Assets)
	text := string(clean)
	vet := "{cmd: go vet ./..., timeout: 5m}"
	for _, c := range []struct {
		name, file, finding string
	}{
		{"file noexec", strings.Replace(text, "tasks:\n", "set: [noexec]\ntasks:\n", 1), "Taskfile.yml:2: gates.command.1 (task set): the file has set: noexec"},
		{"file n", strings.Replace(text, "tasks:\n", "set: [n]\ntasks:\n", 1), "Taskfile.yml:2: gates.command.1 (task set): the file has set: noexec"},
		{"task noexec", strings.Replace(text, "  pre-push:\n", "  pre-push:\n    set: [noexec]\n", 1), "(task pre-push set): the task pre-push has set: noexec"},
		{"member noexec", strings.Replace(text, vet, "{cmd: go vet ./..., set: [noexec], timeout: 5m}", 1), "(task pre-push member set: noexec)"},
		{"member n", strings.Replace(text, vet, "{cmd: go vet ./..., set: [n], timeout: 5m}", 1), "(task pre-push member set: noexec)"},
		{"pipefail", strings.Replace(text, vet, "{cmd: go vet ./..., set: [pipefail], timeout: 5m}", 1), ""},
	} {
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte(c.file)}, "hooks/pre-push": {Data: []byte("task pre-push\n")}, ".gragas/systems": {Data: []byte("linux/amd64 darwin/arm64\n")}}
		got := findingLines(coreapi.TierContract(repo, rd.Rows, tt, "2026-11-15"))
		if c.finding == "" {
			if len(got) != 0 {
				t.Errorf("%s: findings = %v, want none", c.name, got)
			}
			continue
		}
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, c.finding) }) {
			t.Errorf("%s: no finding with %q in:\n%s", c.name, c.finding, strings.Join(got, "\n"))
		}
		if c.name != "member noexec" && c.name != "member n" && !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "has no vet member") }) {
			t.Errorf("%s: a member under noexec still counts: %v", c.name, got)
		}
	}
}

// pooled is the clean fixture with its tier's vet and builds as the deps of an internal task, the
// way gragas's own Taskfile does, and an include that holds the tests and lint.
func tiersOfFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	w := embedded(t)
	tt := coreapi.LoadTiers(w.Ports().Assets)
	rd := coreapi.LoadReaders(w.Ports().Assets)
	repo := fstest.MapFS{"hooks/pre-push": {Data: []byte("task pre-push\n")}, ".gragas/systems": {Data: []byte("linux/amd64 darwin/arm64\n")}}
	for n, d := range files {
		repo[n] = &fstest.MapFile{Data: []byte(d)}
	}
	return findingLines(coreapi.TierContract(repo, rd.Rows, tt, "2026-11-15"))
}

const poolHead = `version: '3'
tasks:
  pre-commit:
    cmds:
      - test -z "$(gofmt -l .)"
      - go mod tidy -diff
  release:
    cmds:
      - {cmd: go test -race ./..., timeout: 30m}
  pre-push:
    deps: [members]
    cmds:
      - {cmd: gragas check, timeout: 2m}
  members:
    internal: true
    deps: [vet, builds, tests, lint, scan, vuln]
  full:
    deps: [members]
    cmds:
      - {cmd: gragas check, timeout: 2m}
  landing:
    cmds:
      - task: pre-push
      - task: full
  vet:
    cmds:
      - {cmd: go vet ./..., timeout: 5m}
  builds:
    cmds:
      - {cmd: GOOS=linux GOARCH=amd64 go build ./..., timeout: 5m}
      - {cmd: GOOS=darwin GOARCH=arm64 go build ./..., timeout: 5m}
  tests:
    cmds:
      - {cmd: gotestsum --format testname -- -count=1 ./... | tee out.txt, set: [pipefail], timeout: 20m}
      - grep -q -e EMPTY -e "DONE 0 tests" out.txt && exit 1 || true
  lint:
    cmds:
      - {cmd: GOOS=linux GOARCH=amd64 golangci-lint run --allow-parallel-runners, timeout: 10m}
      - {cmd: GOOS=darwin GOARCH=arm64 golangci-lint run --allow-parallel-runners, timeout: 10m}
  scan:
    cmds:
      - {cmd: gitleaks git, timeout: 5m}
  vuln:
    cmds:
      - {cmd: govulncheck ./..., timeout: 5m}
`

// TestATierWhoseMembersAreDepsIsGreen: deps: is a pool, and each task it names is read.
func TestATierWhoseMembersAreDepsIsGreen(t *testing.T) {
	t.Parallel()
	got := tiersOfFiles(t, map[string]string{"Taskfile.yml": poolHead})
	if len(got) != 0 {
		t.Fatalf("findings:\n%s", strings.Join(got, "\n"))
	}
}

// TestTheLandTaskCallsThePushAndThenFullOnEveryHost: the land job's task calls pre-push and then
// full as cmds go-task runs on every host and with no vars, so the hook's list is a declared subset of the land job's and every
// member of both runs before main. Each plant takes one part of that away and is a finding of the land rule; a
// call that carries vars, silent and a timeout is green.
func TestTheLandTaskCallsThePushAndThenFullOnEveryHost(t *testing.T) {
	t.Parallel()
	calls := "  landing:\n    cmds:\n      - task: pre-push\n      - task: full\n"
	if !strings.Contains(poolHead, calls) {
		t.Fatal("the pool fixture has no landing task to plant in")
	}
	with := func(to string) string { return strings.Replace(poolHead, calls, to, 1) }
	for name, c := range map[string]struct{ file, want string }{
		"no landing":            {with(""), "(task land landing): no task landing: the land job, which runs pre-push then full"},
		"full left out":         {with("  landing:\n    cmds:\n      - task: pre-push\n"), "(task land landing full): the task landing does not call full"},
		"the push left out":     {with("  landing:\n    cmds:\n      - task: full\n"), "(task land landing pre-push): the task landing does not call pre-push"},
		"full before the push":  {with("  landing:\n    cmds:\n      - task: full\n      - task: pre-push\n"), "(task land landing full): the task landing calls full before pre-push"},
		"full as a dep":         {with("  landing:\n    deps: [full]\n    cmds:\n      - task: pre-push\n"), "(task land landing full): the task landing does not call full"},
		"ignore_error on full":  {with("  landing:\n    cmds:\n      - task: pre-push\n      - task: full\n        ignore_error: true\n"), "(task land landing full): the task landing does not call full"},
		"platforms on the push": {with("  landing:\n    cmds:\n      - task: pre-push\n        platforms: [linux]\n      - task: full\n"), "(task land landing pre-push): the task landing does not call pre-push"},
		"status on landing":     {with("  landing:\n    status: ['true']\n    cmds:\n      - task: pre-push\n      - task: full\n"), "(task land landing status): the task landing has status:"},
		"platforms on landing":  {with("  landing:\n    platforms: [linux]\n    cmds:\n      - task: pre-push\n      - task: full\n"), "(task land landing platforms): the task landing has platforms:"},
		// A call's vars reach every member, and the skip guard's turns the skip on.
		"the skip's vars on full":     {with("  landing:\n    cmds:\n      - task: pre-push\n      - task: full\n        vars: {SKIP_UNCHANGED: \"1\"}\n"), "(task land landing full): the task landing does not call full"},
		"the skip's vars on the push": {with("  landing:\n    cmds:\n      - task: pre-push\n        vars: {SKIP_UNCHANGED: \"1\"}\n      - task: full\n"), "(task land landing pre-push): the task landing does not call pre-push"},
		"any vars on full":            {with("  landing:\n    cmds:\n      - task: pre-push\n      - {task: full, vars: {A: b}}\n"), "(task land landing full): the task landing does not call full"},
	} {
		got := tiersOfFiles(t, map[string]string{"Taskfile.yml": c.file})
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "gates.command.1 ") && strings.Contains(l, c.want) }) {
			t.Errorf("%s: no gates.command.1 finding with %q in:\n%s", name, c.want, strings.Join(got, "\n"))
		}
	}
	green := with("  landing:\n    cmds:\n      - task: pre-push\n        silent: true\n      - {task: full, timeout: 1h}\n      - echo landed\n")
	if got := tiersOfFiles(t, map[string]string{"Taskfile.yml": green}); len(got) != 0 {
		t.Errorf("calls with silent and a timeout, and a cmd after them: findings:\n%s", strings.Join(got, "\n"))
	}
}

// TestAFailfastPoolIsRed: failfast at the file, the task or --failfast stops the other members at a red.
func TestAFailfastPoolIsRed(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ file, want string }{
		"file": {strings.Replace(poolHead, "tasks:\n", "failfast: true\ntasks:\n", 1), "(task pre-push failfast): the file has failfast:"},
		"task": {strings.Replace(poolHead, "    deps: [members]\n", "    failfast: true\n    deps: [members]\n", 1), "(task pre-push failfast): the task pre-push has failfast:"},
		"flag": {strings.Replace(poolHead, "      - {cmd: gitleaks git, timeout: 5m}", "      - {cmd: gitleaks git, timeout: 5m}\n      - task --failfast x", 1), "runs go-task with --failfast"},
	} {
		got := tiersOfFiles(t, map[string]string{"Taskfile.yml": c.file})
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "gates.command.12") && strings.Contains(l, c.want) }) {
			t.Errorf("%s: no gates.command.12 finding with %q in:\n%s", name, c.want, strings.Join(got, "\n"))
		}
	}
}

// TestAMemberOnlyInAnIncludedFileIsFound: the tests member lives in an included file; take it out
// and the tier is red for it, so the include is what holds it.
func TestAMemberOnlyInAnIncludedFileIsFound(t *testing.T) {
	t.Parallel()
	root := strings.Replace(poolHead, "  tests:\n    cmds:\n      - {cmd: gotestsum --format testname -- -count=1 ./... | tee out.txt, set: [pipefail], timeout: 20m}\n      - grep -q -e EMPTY -e \"DONE 0 tests\" out.txt && exit 1 || true\n", "", 1)
	root = strings.Replace(root, "tasks:\n", "includes:\n  t: {taskfile: tests.yml}\ntasks:\n", 1)
	root = strings.Replace(root, "deps: [vet, builds, tests,", "deps: [vet, builds, t:tests,", 1)
	inc := "version: '3'\ntasks:\n  tests:\n    cmds:\n      - {cmd: gotestsum --format testname -- -count=1 ./... | tee out.txt, set: [pipefail], timeout: 20m}\n      - grep -q -e EMPTY -e \"DONE 0 tests\" out.txt && exit 1 || true\n"
	if got := tiersOfFiles(t, map[string]string{"Taskfile.yml": root, "tests.yml": inc}); len(got) != 0 {
		t.Fatalf("with the include: findings:\n%s", strings.Join(got, "\n"))
	}
	got := tiersOfFiles(t, map[string]string{"Taskfile.yml": root})
	if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "(task include t unread)") }) || !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "has no tests member") }) {
		t.Errorf("without the file: want the unread include and the missing member in:\n%s", strings.Join(got, "\n"))
	}
}

// TestAMemberReachedOnlyByATaskCallIntoAnIncludedFileIsFound: the tier's own cmds hold nothing but a
// task: call (the shape of a repo that adopts a shared Taskfile), and the member is in the included file.
func TestAMemberReachedOnlyByATaskCallIntoAnIncludedFileIsFound(t *testing.T) {
	t.Parallel()
	root := strings.Replace(poolHead, "  tests:\n    cmds:\n      - {cmd: gotestsum --format testname -- -count=1 ./... | tee out.txt, set: [pipefail], timeout: 20m}\n      - grep -q -e EMPTY -e \"DONE 0 tests\" out.txt && exit 1 || true\n", "", 1)
	root = strings.Replace(root, "tasks:\n", "includes:\n  t: {taskfile: tests.yml}\ntasks:\n", 1)
	root = strings.Replace(root, "deps: [vet, builds, tests,", "deps: [vet, builds,", 1)
	// The push and full both take the members pool, so each calls the tests task itself.
	root = strings.ReplaceAll(root, "    deps: [members]\n    cmds:\n", "    deps: [members]\n    cmds:\n      - task: t:tests\n")
	inc := "version: '3'\ntasks:\n  tests:\n    cmds:\n      - {cmd: gotestsum --format testname -- -count=1 ./... | tee out.txt, set: [pipefail], timeout: 20m}\n      - grep -q -e EMPTY -e \"DONE 0 tests\" out.txt && exit 1 || true\n"
	if got := tiersOfFiles(t, map[string]string{"Taskfile.yml": root, "tests.yml": inc}); len(got) != 0 {
		t.Fatalf("a task: call into the included file: findings:\n%s", strings.Join(got, "\n"))
	}
	if got := tiersOfFiles(t, map[string]string{"Taskfile.yml": root}); !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "has no tests member") }) {
		t.Errorf("without the file the member is not found, and that is a finding; got:\n%s", strings.Join(got, "\n"))
	}
}

// TestARemoteIncludeNeedsAChecksumAndARefAndIsNeverASilentPass.
func TestARemoteIncludeNeedsAChecksumAndARefAndIsNeverASilentPass(t *testing.T) {
	t.Parallel()
	root := strings.Replace(poolHead, "tasks:\n", "includes:\n  r: {taskfile: 'git::https://example.org/x.git//Taskfile.yml'}\n  h: {taskfile: 'https://example.org/Taskfile.yml', checksum: abc}\ntasks:\n", 1)
	got := tiersOfFiles(t, map[string]string{"Taskfile.yml": root})
	for _, want := range []string{"(task include r checksum)", "(task include r ref)", "(task include r unread)", "(task include h unread)"} {
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, want) }) {
			t.Errorf("no finding %s in:\n%s", want, strings.Join(got, "\n"))
		}
	}
	if slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "(task include h checksum)") }) {
		t.Errorf("a pinned https include has no checksum finding:\n%s", strings.Join(got, "\n"))
	}
}

// TestAKeyThatStopsACallOrItsCalleeIsAFindingAndTheMemberCountsForNothing: a call
// entry carries task, vars and silent and nothing else, and a called task is held to the keys a tier is.
// Each plant leaves lint reached only through the stopped call or callee, so it is a finding that names
// the key and a member that is missing, never a green tier.
func TestAKeyThatStopsACallOrItsCalleeIsAFindingAndTheMemberCountsForNothing(t *testing.T) {
	t.Parallel()
	noLintDep := strings.Replace(poolHead, "deps: [vet, builds, tests, lint, scan, vuln]", "deps: [vet, builds, tests, scan, vuln]", 1)
	calledBy := func(entry string) string {
		return strings.Replace(noLintDep, "    deps: [members]\n    cmds:\n", "    deps: [members]\n    cmds:\n"+entry, 1)
	}
	withKey := func(key string) string {
		return strings.Replace(poolHead, "  lint:\n    cmds:\n", "  lint:\n    "+key+"\n    cmds:\n", 1)
	}
	for name, c := range map[string]struct{ file, want string }{
		"if on a cmds call":           {calledBy("      - task: lint\n        if: \"false\"\n"), "(if: on the call of lint)"},
		"platforms on a cmds call":    {calledBy("      - task: lint\n        platforms: [darwin]\n"), "(platforms: on the call of lint)"},
		"for on a cmds call":          {calledBy("      - task: lint\n        for: []\n"), "(for: on the call of lint)"},
		"ignore_error on a cmds call": {calledBy("      - task: lint\n        ignore_error: true\n"), "(ignore_error: on the call of lint)"},
		"for on a dep":                {strings.Replace(noLintDep, "deps: [vet, builds,", "deps: [{task: lint, for: []}, vet, builds,", 1), "(for: on the call of lint)"},
		"status on a callee":          {withKey("status: ['true']"), "(status: at the task lint)"},
		"platforms on a callee":       {withKey("platforms: [darwin]"), "(platforms: at the task lint)"},
		"sources on a callee":         {withKey("sources: [a.go]"), "(sources: at the task lint)"},
		"if on a callee":              {withKey("if: 'false'"), "(if: at the task lint)"},
	} {
		got := tiersOfFiles(t, map[string]string{"Taskfile.yml": c.file})
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "gates.command.1 ") && strings.Contains(l, c.want) }) {
			t.Errorf("%s: no finding with %q in:\n%s", name, c.want, strings.Join(got, "\n"))
		}
		if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "has no ") }) {
			t.Errorf("%s: the stopped member is still counted; findings:\n%s", name, strings.Join(got, "\n"))
		}
	}
	// A call carrying only task, vars and silent is green.
	if got := tiersOfFiles(t, map[string]string{"Taskfile.yml": calledBy("      - task: lint\n        vars: {A: b}\n        silent: true\n")}); len(got) != 0 {
		t.Errorf("a call with vars and silent: findings:\n%s", strings.Join(got, "\n"))
	}
}
