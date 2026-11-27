package internal

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// The release (gragas#914): the tier holds what a tree must pass before a tag,
// and the recipe runs the tier and then its own steps. The guard that refuses
// a tree a release would not be the commit of is hatch tag, which hatch's own
// tests hold (gragas#1655, gragas#2260).

// taskCalls are the tasks one task of Taskfile.yml calls, in order.
func taskCalls(t *testing.T, name string) []string {
	t.Helper()
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^      - task: ([a-z][a-z-]*)$`).FindAllStringSubmatch(taskSection(t, name), -1) {
		out = append(out, m[1])
	}
	return out
}

// TestTheReleaseTierKeepsWhatATreeMustPassBeforeATag holds the dispatch's
// call: the tier is the guard, then #910's tier as it landed (the push tier,
// the uncached tests, the gate's own tests, the heavy tests, the full secret
// scan) and the pty tests; the cross build, the dry release and crash
// injection are the recipe's own steps, after the tier.
func TestTheReleaseTierKeepsWhatATreeMustPassBeforeATag(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		task string
		want []string
	}{
		{"release", []string{"gate-build"}},
		{"release-body", []string{"tools-check", "release-guard", "warm", "release-members"}},
		{"release-members", []string{"push-checks", "full-checks", "test-uncached", "gate-tests", "test-heavy", "names", "secrets-full", "pty", "self-check-issues"}},
		{"release-recipe", []string{"release-fetch", "release", "cross", "crash", "release-build"}},
		{"release-publish", []string{"gate-build", "release-fetch", "tools-check", "release-guard-tagged", "release-build", "release-undraft"}},
	} {
		if got := taskCalls(t, c.task); !slices.Equal(got, c.want) {
			t.Errorf("task %s calls %q, not %q", c.task, got, c.want)
		}
	}
	if slices.Contains(taskNames(t), "release-dry") {
		t.Error("the Taskfile still has release-dry: release-build is the dry release")
	}
	if !strings.Contains(taskSection(t, "release-publish"), "- task: release-build\n        vars: {PUBLISH: \"1\"}\n") {
		t.Error("release-publish does not build with PUBLISH=1")
	}
}

// TestTheReleaseBuildFetchesNothing holds the fetch ruling: release-fetch is
// the recipe's one step with the network open, so the build runs with the
// module proxy off and the module graph read-only. The build is gragas-release
// build (gragas#1655, X211), which clones twice, holds each clone's sources
// before goreleaser runs (gragas#1206), and compares and unpacks what it
// built; its own tests hold each of those, and this one the order in its file.
func TestTheReleaseBuildFetchesNothing(t *testing.T) {
	t.Parallel()
	build := taskSection(t, "release-build")
	for _, want := range []string{
		"GOPROXY: \"off\"", "GOFLAGS: -mod=readonly",
		"go run -trimpath ./cmd/gragas-release build -out '{{.RELEASE_DIR}}' -goreleaser '{{.TOOLS}}/goreleaser{{exeExt}}' -systems '{{join \",\" .CROSS}}'",
		"{{if .PUBLISH}} -publish -tag \"$('{{.TOOLS}}/changie{{exeExt}}' latest)\" -notes '{{.ROOT_DIR}}/.changes'{{end}}",
	} {
		if !strings.Contains(build, want) {
			t.Errorf("release-build lacks %q", want)
		}
	}
	step := read(t, "cmd/gragas-release/build.go")
	// buildStep, then the cleanClones it calls first, below it in the file
	order := []string{
		"heldGoreleaser(", "cleanClones(", `"GOCACHE="`, "tc.Release(", "compareStep(", "verified(",
		`"clone", "-q", "--no-local", "--no-checkout"`, `"checkout", "-q", "--detach"`, "sourcesStep(",
	}
	at := -1
	for _, want := range order {
		i := strings.Index(step, want)
		switch {
		case i < 0:
			t.Errorf("gragas-release build lacks %q", want)
		case i < at:
			t.Errorf("gragas-release build does not run %q after %q, in the order %q", want, order[max(0, slices.Index(order, want)-1)], order)
		}
		at = max(at, i)
	}
	if fetch := taskSection(t, "release-fetch"); !strings.Contains(fetch, "go mod download") || !strings.Contains(fetch, "ref: .CROSS") {
		t.Error("release-fetch does not download the modules of every system of CROSS")
	}
}

// crossList is the Taskfile's CROSS, the list every system is built for.
func crossList(t *testing.T) []string {
	t.Helper()
	m := regexp.MustCompile(`\n  CROSS: \[([^\]]*)\]\n`).FindStringSubmatch(read(t, "Taskfile.yml"))
	if m == nil {
		t.Fatal("the Taskfile has no CROSS")
	}
	var out []string
	for s := range strings.SplitSeq(m[1], ",") {
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

// releaseSystems says what is wrong with the systems a goreleaser config
// builds, held to cross: a config with no build, a build that names no system
// (goreleaser's defaults are not the list), a config that builds no windows
// (arch.ports.4: every system of the list, windows among them), and systems
// that are not cross. A target is os_arch with a variant after it, or goos by
// goarch.
func releaseSystems(text string, cross []string) []string {
	v, err := ordered.ParseYAML([]byte(text))
	if err != nil {
		return []string{"the release config does not parse: " + err.Error()}
	}
	doc, _ := v.(*ordered.Map)
	if doc == nil || len(ordered.Maps(doc.List("builds"))) == 0 {
		return []string{"the release config has no build"}
	}
	var out, systems []string
	add := func(s string) {
		if !slices.Contains(systems, s) {
			systems = append(systems, s)
		}
	}
	for _, b := range ordered.Maps(doc.List("builds")) {
		id := b.Str("id")
		targets, goos, goarch := ordered.Strings(b.List("targets")), ordered.Strings(b.List("goos")), ordered.Strings(b.List("goarch"))
		for _, tg := range targets {
			parts := strings.SplitN(tg, "_", 3)
			if len(parts) < 2 {
				out = append(out, "build "+id+": target "+tg+" is not os_arch")
				continue
			}
			add(parts[0] + "/" + parts[1])
		}
		if len(targets) > 0 {
			continue
		}
		for _, o := range goos {
			for _, a := range goarch {
				add(o + "/" + a)
			}
		}
		if len(goos) == 0 || len(goarch) == 0 {
			out = append(out, "build "+id+" names no system: goreleaser's defaults are not the list")
		}
	}
	if !slices.ContainsFunc(systems, func(s string) bool { return strings.HasPrefix(s, "windows/") }) {
		out = append(out, "the release config builds no windows (arch.ports.4)")
	}
	got, want := slices.Sorted(slices.Values(systems)), slices.Sorted(slices.Values(cross))
	if !slices.Equal(got, want) {
		out = append(out, "the release config builds "+strings.Join(got, ",")+"; the Taskfile's CROSS is "+strings.Join(want, ","))
	}
	return out
}

// TestTheReleaseConfigBuildsEverySystemWindowsAmongThem holds gragas's own
// .goreleaser.yaml to the Taskfile's CROSS, and plants a config without
// windows, one that names its systems by goos and leaves windows out, and one
// that names none.
func TestTheReleaseConfigBuildsEverySystemWindowsAmongThem(t *testing.T) {
	t.Parallel()
	cross := crossList(t)
	if f := releaseSystems(read(t, ".goreleaser.yaml"), cross); len(f) != 0 {
		t.Errorf(".goreleaser.yaml: %q", f)
	}
	noWindows := "builds:\n  - id: g\n    targets: [linux_amd64_v1, linux_arm64, darwin_arm64, freebsd_amd64_v1]\n"
	byGoos := "builds:\n  - id: g\n    goos: [linux, darwin]\n    goarch: [amd64, arm64]\n"
	for _, c := range []struct{ name, text, want string }{
		{"no windows", noWindows, "builds no windows"},
		{"no windows", noWindows, "the Taskfile's CROSS is"},
		{"goos without windows", byGoos, "builds no windows"},
		{"no system", "builds:\n  - id: g\n", "build g names no system"},
		{"no build", "project_name: g\n", "has no build"},
		{"not yaml", "builds: [\n", "does not parse"},
	} {
		if f := releaseSystems(c.text, cross); !slices.ContainsFunc(f, func(s string) bool { return strings.Contains(s, c.want) }) {
			t.Errorf("%s: findings %q lack %q", c.name, f, c.want)
		}
	}
}

// releaseNative says what is wrong with the build of the gragas program in a
// goreleaser config, for the PowerShell and cmd branch of the root's line maker
// (pasteLine, gragas#1695, go-console#218): it writes a line whose first word
// is gragas there only because the program is a native executable, which
// goreleaser names gragas.exe on windows, and neither a batch file nor a
// script runs in its place. So the build is the Go builder's, of
// ./cmd/gragas, as the binary gragas, with cgo off, and no hook rewrites what
// it places.
func releaseNative(text string) []string {
	v, err := ordered.ParseYAML([]byte(text))
	if err != nil {
		return []string{"the release config does not parse: " + err.Error()}
	}
	doc, _ := v.(*ordered.Map)
	if doc == nil {
		return []string{"the release config has no build"}
	}
	for _, b := range ordered.Maps(doc.List("builds")) {
		if b.Str("main") != "./cmd/gragas" {
			continue
		}
		var out []string
		if b.Str("binary") != "gragas" {
			out = append(out, "the build of ./cmd/gragas names its binary "+b.Str("binary")+", not gragas")
		}
		if bu := b.Str("builder"); bu != "" && bu != "go" {
			out = append(out, "the build of ./cmd/gragas uses the builder "+bu+", not go")
		}
		if _, hooked := b.Get("hooks"); hooked {
			out = append(out, "the build of ./cmd/gragas has hooks, which may replace the native program")
		}
		if !slices.Contains(ordered.Strings(b.List("env")), "CGO_ENABLED=0") {
			out = append(out, "the build of ./cmd/gragas does not turn cgo off")
		}
		return out
	}
	return []string{"the release config has no build of ./cmd/gragas"}
}

// TestTheReleaseBuildsGragasAsANativeProgram holds gragas's own
// .goreleaser.yaml to what the line maker assumes in PowerShell and cmd, and
// plants a build that renames the binary, one by another builder, one with a
// hook, one with cgo on, and a config with no build of the program.
func TestTheReleaseBuildsGragasAsANativeProgram(t *testing.T) {
	t.Parallel()
	if f := releaseNative(read(t, ".goreleaser.yaml")); len(f) != 0 {
		t.Errorf(".goreleaser.yaml: %q", f)
	}
	const ok = "builds:\n  - id: g\n    main: ./cmd/gragas\n    binary: gragas\n    env:\n      - CGO_ENABLED=0\n"
	if f := releaseNative(ok); len(f) != 0 {
		t.Fatalf("the planted good build is refused: %q", f)
	}
	for _, c := range []struct{ name, text, want string }{
		{"renamed", strings.Replace(ok, "binary: gragas", "binary: gragas-cli", 1), "not gragas"},
		{"other builder", ok + "    builder: prebuilt\n", "not go"},
		{"hooks", ok + "    hooks:\n      post: ./wrap.sh\n", "has hooks"},
		{"cgo", strings.Replace(ok, "CGO_ENABLED=0", "CGO_ENABLED=1", 1), "cgo off"},
		{"other program", strings.Replace(ok, "./cmd/gragas", "./cmd/other", 1), "no build of ./cmd/gragas"},
		{"not yaml", "builds: [\n", "does not parse"},
	} {
		if f := releaseNative(c.text); !slices.ContainsFunc(f, func(s string) bool { return strings.Contains(s, c.want) }) {
			t.Errorf("%s: findings %q lack %q", c.name, f, c.want)
		}
	}
}

// The explorer's three members each build the contract's dist before they read a thing
// (gragas#1477): the typecheck and the lint read the contract from its built form, and a run on a
// tree with no dist is red. The typecheck and the lint then run their command through
// scripts/write-nothing.ts, which names each file the command added, changed or removed, so a
// member that begins to write under explorer/ fails the push that runs it (gragas#1510). The
// contract's build runs before the wrapper, as the step that places the dist, and only when the dist
// is older than what it builds from (scripts/ensure-contract.ts): the build's own incremental emit
// rewrote dist/fixtures under load and the guard read that as a write (gragas#1628).
func TestEachExplorerMemberBuildsTheContractFirstAndTheTwoRunThroughTheGuard(t *testing.T) {
	t.Parallel()
	scripts := explorerScripts(t)
	const build = "node scripts/ensure-contract.ts -- pnpm --filter @gragas/contract run build && "
	const guard = "node scripts/write-nothing.ts -- "
	for _, name := range []string{"typecheck", "lint", "test"} {
		cmd, ok := scripts["gragas-explorer:"+name]
		if !ok {
			t.Errorf("the explorer has no %s script", name)
			continue
		}
		if !strings.HasPrefix(cmd, build) {
			t.Errorf("explorer script %s is %q: it must start with %q, since a run on a tree with no dist is red", name, cmd, build)
		}
		rest := strings.TrimPrefix(cmd, build)
		if name != "test" && !strings.HasPrefix(rest, guard) {
			t.Errorf("explorer script %s is %q: after the build it must run through %q, so a member that writes under explorer/ is red", name, cmd, guard)
		}
		if name == "lint" && strings.Count(cmd, guard) != 2 {
			t.Errorf("explorer script lint is %q: both the contract's build (run again, to show an up-to-date build writes nothing, gragas#829) and eslint must run through %q", cmd, guard)
		}
		if name == "typecheck" && strings.Count(rest, guard) != 2 {
			t.Errorf("explorer script typecheck is %q: both its commands (tsc and the packages' typecheck) must run through %q", cmd, guard)
		}
	}
	if section := taskSection(t, "explorer-typecheck"); !strings.Contains(section, "explorer/scripts/write-nothing.ts") {
		t.Error("the explorer-typecheck member does not run its tsc through explorer/scripts/write-nothing.ts: it runs tsc itself, not the package.json script, so a push would not hold the typecheck to the guard")
	}
	if _, ok := scripts["gragas-explorer:test:heavy"]; ok {
		t.Error("the explorer has a test:heavy script: the write-nothing proof runs inside typecheck and lint, not in a second run of them")
	}
}
