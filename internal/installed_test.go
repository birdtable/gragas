package internal

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/gitenv/gittest"
)

// installedOrBuilt is each tracked path under a node_modules or a dist folder,
// at any depth: an installed dependency or a build output in git (ts.cycle.3).
func installedOrBuilt(tracked []string) []string {
	var out []string
	for _, p := range tracked {
		for seg := range strings.SplitSeq(path.Dir(p), "/") {
			if seg == "node_modules" || seg == "dist" {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// TestNoInstalledOrBuiltFileIsTrackedInGit: git's index holds no path under a
// node_modules or a dist folder (ts.cycle.3). The control, in the next test,
// is that the check sees one.
func TestNoInstalledOrBuiltFileIsTrackedInGit(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z")
	cmd.Dir = root
	cmd.Env = gittest.Git(t.TempDir())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	tracked := strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 })
	if len(tracked) < 100 {
		t.Fatalf("git lists %d files; the check could not read the tree", len(tracked))
	}
	if bad := installedOrBuilt(tracked); len(bad) != 0 {
		t.Errorf("git tracks installed or built files (ts.cycle.3), %d, the first %s", len(bad), bad[0])
	}
}

// TestATrackedPathUnderNodeModulesOrDistIsSeen: a planted path is found at any
// depth, in either folder; a name that only looks like one is not.
func TestATrackedPathUnderNodeModulesOrDistIsSeen(t *testing.T) {
	t.Parallel()
	for _, p := range []string{
		"explorer/node_modules/zod/index.js",
		"node_modules/a",
		"explorer/packages/contract/dist/index.js",
		"dist/gragas",
		"a/b/node_modules/c/d/e.js",
	} {
		if got := installedOrBuilt([]string{"go.mod", p}); !slices.Equal(got, []string{p}) {
			t.Errorf("%s: found %v", p, got)
		}
	}
	for _, p := range []string{
		"explorer/src/dist.ts",
		"docs/node_modules.md",
		"explorer/packages/contract/src/distance/a.ts",
		"node_modules_notes/a",
	} {
		if got := installedOrBuilt([]string{p}); len(got) != 0 {
			t.Errorf("%s: wrongly found %v", p, got)
		}
	}
}

// coverageMarks are the flags and tools that instrument code to count what ran.
func coverageMarks() []string {
	return []string{"--coverage", "-cover", "-coverprofile", "-coverpkg", "c8 ", "nyc ", "istanbul"}
}

// coverageAt is each task of the blocking tiers (pre-commit and pre-push),
// by name with its text, whose text carries a coverage mark, with the mark.
// A package script a task runs counts as part of its text.
func coverageAt(tasks map[string]string, scripts map[string]string) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(tasks)) {
		text := tasks[name]
		for key, cmd := range scripts {
			_, script, _ := strings.Cut(key, ":")
			if strings.Contains(text, "pnpm run "+script) {
				text += " " + cmd
			}
		}
		for _, mark := range coverageMarks() {
			if strings.Contains(text, mark) {
				out = append(out, name+": "+strings.TrimSpace(mark))
				break
			}
		}
	}
	return out
}

// calledBy is the tasks a task of Taskfile.yml calls with "- task: name".
func calledBy(t *testing.T, name string) []string {
	t.Helper()
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^\s+- task: ([a-z][a-z0-9-]*)$`).FindAllStringSubmatch(taskSection(t, name), -1) {
		out = append(out, m[1])
	}
	return out
}

// blockingTasks is each task the pre-commit, pre-push and full tiers call, by name
// with its text, the members pre-push-members and full-members run as deps among them; the release tier's own tasks are not in it.
func blockingTasks(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	// The tiers and the wrappers whose deps they run: a wrapper is read for its members, not as one.
	wrappers := []string{"pre-commit-members", "commit-checks", "pre-push-members", "push-checks", "full-members", "full-checks"}
	for _, tier := range append([]string{"pre-commit", "pre-push", "full"}, wrappers...) {
		for _, name := range calledBy(t, tier) {
			if !slices.Contains(wrappers, name) {
				out[name] = taskSection(t, name)
			}
		}
	}
	return out
}

// explorerScripts is each script of the explorer's package.json files, by
// "<package>:<name>", with its command.
func explorerScripts(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range []string{"explorer/package.json", "explorer/packages/contract/package.json"} {
		var pkg struct {
			Name    string
			Scripts map[string]string
		}
		if err := json.Unmarshal([]byte(read(t, rel)), &pkg); err != nil {
			t.Fatal(err)
		}
		for name, cmd := range pkg.Scripts {
			out[pkg.Name+":"+name] = cmd
		}
	}
	return out
}

// vitestFlagProblems: a package that holds a vitest.config.ts runs its tests
// bare, so it fails when they go missing; only one without a config may pass
// with none (gragas#1219). dirs maps a directory to its test script, and has
// lists the directories that hold the config.
func vitestFlagProblems(dirs map[string]string, has map[string]bool) []string {
	var out []string
	for dir, script := range dirs {
		if has[dir] && strings.Contains(script, "--passWithNoTests") {
			out = append(out, dir+" holds a vitest.config.ts and still passes with no tests: "+script)
		}
	}
	return out
}

// TestNoConfiguredVitestPackagePassesWithNoTests: ts.cycle.5, the test half.
func TestNoConfiguredVitestPackagePassesWithNoTests(t *testing.T) {
	t.Parallel()
	dirs, has := map[string]string{}, map[string]bool{}
	matches, err := filepath.Glob(filepath.Join(root, "explorer", "packages", "*", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range append(matches, filepath.Join(root, "explorer", "package.json")) {
		var pkg struct{ Scripts map[string]string }
		raw, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &pkg); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(m)
		dirs[dir] = pkg.Scripts["test"]
		_, statErr := os.Stat(filepath.Join(dir, "vitest.config.ts"))
		has[dir] = statErr == nil
	}
	if len(dirs) < 3 {
		t.Fatalf("read %d packages", len(dirs))
	}
	if bad := vitestFlagProblems(dirs, has); len(bad) != 0 {
		t.Errorf("a configured package passes with no tests: %v", bad)
	}
}

// TestTheVitestFlagScanSeesAConfiguredPackageKeepingTheFlag: the plants.
func TestTheVitestFlagScanSeesAConfiguredPackageKeepingTheFlag(t *testing.T) {
	t.Parallel()
	dirs := map[string]string{"a": "vitest run", "b": "vitest run --passWithNoTests"}
	if got := vitestFlagProblems(dirs, map[string]bool{"a": true}); len(got) != 0 {
		t.Errorf("an unconfigured package may keep the flag: %v", got)
	}
	if got := vitestFlagProblems(dirs, map[string]bool{"a": true, "b": true}); len(got) != 1 {
		t.Errorf("a configured package keeping the flag was not found once: %v", got)
	}
}

// TestNoBlockingMemberRunsWithCoverage: no pre-commit or pre-push task, nor
// a package script it runs, carries a coverage flag (ts.cycle.6); coverage
// runs at release or on demand.
func TestNoBlockingMemberRunsWithCoverage(t *testing.T) {
	t.Parallel()
	tasks := blockingTasks(t)
	if len(tasks) < 10 {
		t.Fatalf("read %d tasks", len(tasks))
	}
	if bad := coverageAt(tasks, explorerScripts(t)); len(bad) != 0 {
		t.Errorf("a blocking task runs with coverage (ts.cycle.6): %v", bad)
	}
}

// TestACoverageFlagInABlockingMemberIsSeen: the scan finds each mark in a
// blocking task and in the package script it runs.
func TestACoverageFlagInABlockingMemberIsSeen(t *testing.T) {
	t.Parallel()
	if got := coverageAt(map[string]string{"t": "go test -cover ./..."}, nil); len(got) != 1 {
		t.Errorf("a go -cover in a task: %v", got)
	}
	if got := coverageAt(map[string]string{"v": "cd explorer && pnpm run test"}, map[string]string{"x:test": "vitest run --coverage"}); len(got) != 1 {
		t.Errorf("--coverage in the script a task runs: %v", got)
	}
	if got := coverageAt(map[string]string{"v": "cd explorer && pnpm run test"}, map[string]string{"x:test": "vitest run"}); len(got) != 0 {
		t.Errorf("a clean script: %v", got)
	}
}

// explorerMemberProblems is what is wrong with the explorer's push tasks: the
// type check and the tests are two tasks of the pre-push tier (ts.cycle.5,
// gragas#1219). Neither goes through `pnpm run`: the pinned node runs tsc and
// vitest. The type check writes its build info under .tools/cache, which git
// ignores. The tests run whole, never vitest --changed: conformance.test.ts
// reads the Go contract fixtures outside vitest's module graph, so a
// changed-only run would miss a contract change. pushTasks is the tasks the
// tier calls; text is a task's text.
func explorerMemberProblems(pushTasks []string, text func(string) string, scripts map[string]string) []string {
	var out []string
	for _, w := range []struct {
		id    string
		must  []string
		never []string
	}{
		{"explorer-typecheck", []string{`for d in explorer explorer/packages/*/`, `{{.TOOLS}}/node{{exeExt}}" "$tsc"`, "typescript/lib/tsc.js", "--noEmit", "--tsBuildInfoFile \"$cache/", ".tools/cache/tsc"}, []string{"pnpm", "run test"}},
		{"explorer-test", []string{`-- run test`}, []string{"--changed", "run typecheck", "--passWithNoTests"}},
		{"explorer-lint", []string{`-- run lint`}, []string{"run typecheck"}},
	} {
		if !slices.Contains(pushTasks, w.id) {
			out = append(out, w.id+" is no pre-push task")
			continue
		}
		recipe := text(w.id)
		for _, m := range w.must {
			if !strings.Contains(recipe, m) {
				out = append(out, w.id+" does not hold "+m)
			}
		}
		for _, n := range w.never {
			if strings.Contains(recipe, n) {
				out = append(out, w.id+" holds "+n)
			}
		}
	}
	for name, cmd := range scripts {
		if strings.HasSuffix(name, ":typecheck") && !strings.Contains(cmd, "pnpm -r run typecheck") && !strings.Contains(cmd, "--noEmit") {
			out = append(out, name+" may emit: no --noEmit in "+cmd)
		}
	}
	return out
}

// TestTheExplorersTypeCheckAndTestsAreTheirOwnFullMembers: ts.cycle.5's type
// check half, and the lint member beside them (#283); they left the push for full (gragas#1619).
func TestTheExplorersTypeCheckAndTestsAreTheirOwnFullMembers(t *testing.T) {
	t.Parallel()
	text := func(name string) string { return taskSection(t, name) }
	if bad := explorerMemberProblems(slices.Concat(calledBy(t, "full"), calledBy(t, "full-members"), calledBy(t, "full-checks")), text, explorerScripts(t)); len(bad) != 0 {
		t.Errorf("the explorer's tasks (ts.cycle.5): %v", bad)
	}
}

// TestTheExplorerMemberScanSeesAMissingMemberAndATypeCheckThatEmits: the
// tasks the scan holds are found missing, through pnpm, changed-only, or
// emitting.
func TestTheExplorerMemberScanSeesAMissingMemberAndATypeCheckThatEmits(t *testing.T) {
	t.Parallel()
	const tsc = `tsc="{{.ROOT_DIR}}/explorer/node_modules/typescript/lib/tsc.js"; "{{.TOOLS}}/node{{exeExt}}" "$tsc" `
	texts := map[string]string{
		"explorer-typecheck": "cache=\"{{.ROOT_DIR}}/.tools/cache/tsc\"; for d in explorer explorer/packages/*/; do " + tsc + "--noEmit --incremental --tsBuildInfoFile \"$cache/x.tsbuildinfo\"",
		"explorer-test":      `{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run test`,
		"explorer-lint":      `{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run lint --concurrency=auto`,
		"pnpm-typecheck":     "cd explorer && pnpm run typecheck",
		"lint-typecheck":     `{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run typecheck`,
		"in-tree":            "for d in explorer explorer/packages/*/; do " + tsc + "--noEmit --tsBuildInfoFile x.tsbuildinfo",
		"changed":            `{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run test --changed`,
		"lint-and-typecheck": `{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run lint && {{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" -- run typecheck`,
		"packages-only":      "for d in explorer/packages/*/; do " + tsc + "--noEmit --tsBuildInfoFile .tools/cache/tsc/x.tsbuildinfo; done",
	}
	text := func(n string) string { return texts[n] }
	both := []string{"explorer-typecheck", "explorer-test", "explorer-lint"}
	scripts := map[string]string{"c:typecheck": "tsc --noEmit -p tsconfig.json"}
	if got := explorerMemberProblems(both, text, scripts); len(got) != 0 {
		t.Errorf("the split as it is: %v", got)
	}
	swap := func(task, with string) func(string) string {
		return func(n string) string {
			if n == task {
				return texts[with]
			}
			return texts[n]
		}
	}
	cases := map[string]struct {
		tasks   []string
		text    func(string) string
		scripts map[string]string
	}{
		"one task missing":               {both[:1], text, scripts},
		"the lint task missing":          {both[:2], text, scripts},
		"a type check emitting":          {both, text, map[string]string{"c:typecheck": "tsc -p tsconfig.json"}},
		"a type check via pnpm":          {both, swap("explorer-typecheck", "pnpm-typecheck"), scripts},
		"the lint run is the type check": {both, swap("explorer-lint", "lint-typecheck"), scripts},
		"the lint run also type-checks":  {both, swap("explorer-lint", "lint-and-typecheck"), scripts},
		"the root type check dropped":    {both, swap("explorer-typecheck", "packages-only"), scripts},
		"build info in the tree":         {both, swap("explorer-typecheck", "in-tree"), scripts},
		"a changed-only test run":        {both, swap("explorer-test", "changed"), scripts},
	}
	for name, c := range cases {
		if got := explorerMemberProblems(c.tasks, c.text, c.scripts); len(got) == 0 {
			t.Errorf("%s: nothing failed", name)
		}
	}
}
