package journeys

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// The transcripts: every current command, every flag, every dry run, gen
// --check and the exit code of each, over the owner's own rulebook as it is
// in this repository (schema v2), never a fixture. The point is that the
// owner's gate (make rules, make gen-check) stays identical while the code
// moves under it. They live in testdata/owner-*.golden; see keys_test.go for
// which one each later move re-runs and how a person rewrites them.
//
// Check's table counts the owner's rules, so a rule added or retired moves
// owner-check-*.golden: the same commit regenerates it, and the diff shows the
// count and nothing else.

// treeKind is the folder a scenario runs in.
type treeKind int

const (
	ownerTree treeKind = iota // a copy of this repository's rulebook and what gen renders
	staleTree                 // genTree with its rules.json overwritten
	genTree                   // a format-3 book with the rules.json it makes, for gen
	emptyTree                 // a folder that is not a rulebook repository
)

// scenario is one run of the binary.
type scenario struct {
	name string
	tree treeKind
	args []string
	// narrow also records it at 60 and 40 columns (cli.commands.15); the rest
	// record at 80, since their words do not depend on the width.
	narrow bool
}

// The surface, as literals: every command with its own flags. A command or
// flag the binary gains and this table lacks fails
// TestEveryCommandAndFlagHasATranscript.
func commandFlags() map[string][]string {
	return map[string][]string{
		"check":   {"--accessible", "-C", "--config", "--dir", "--fail-on", "--json", "--no-color", "--plain"},
		"gen":     {"--accessible", "--apply", "--check", "-C", "--config", "--dir", "--json", "--no-color", "--plain"},
		"init":    {"--accessible", "--apply", "--config", "--docs-url", "--import", "--json", "--kind", "--language", "--layer", "--licence", "--name", "--no-color", "--owner", "--parent", "--parent-hash", "--parent-version", "--part", "--plain", "--presets-licence", "--rulebook-kind", "--version"},
		"version": {"--accessible", "--config", "--json", "--no-color", "--plain"},
	}
}

func rootFlags() []string {
	return []string{"--accessible", "--config", "--json", "--no-color", "--plain"}
}

// switches are the root flags that take no value, each run alone and before
// a command.
func switches() []string { return []string{"--accessible", "--json", "--no-color", "--plain"} }

func scenarios() []scenario {
	var out []scenario
	add := func(name string, tree treeKind, narrow bool, args ...string) {
		if tree == ownerTree && len(args) > 0 && args[0] == "gen" {
			tree = genTree
		}
		out = append(out, scenario{name, tree, args, narrow})
	}
	// Every command bare, then with each of its flags.
	for _, cmd := range []string{"check", "gen", "init", "version"} {
		add(cmd, ownerTree, true, cmd)
		for _, f := range []string{"--json", "--plain", "--accessible", "--no-color"} {
			add(cmd+f, ownerTree, false, cmd, f)
		}
		add(cmd+"-help", ownerTree, true, cmd, "--help")
		add(cmd+"-h", ownerTree, false, cmd, "-h")
		add("help-"+cmd, ownerTree, false, "help", cmd)
		// --config naming a file the tree does not hold: not ready, named.
		add(cmd+"--config", ownerTree, false, cmd, "--config", missingConfig())
	}
	// --fail-on names the kinds that end a project check on findings; the
	// owner's tree holds no pin, so the flag is read and the old check runs.
	add("check--fail-on", ownerTree, false, "check", "--fail-on", "tool-config")
	for _, f := range []string{"-C", "--dir"} {
		add("check"+f, ownerTree, false, "check", f, ".")
		add("gen"+f, ownerTree, false, "gen", f, ".")
		add("check"+f+"-json", ownerTree, false, "check", f, ".", "--json")
	}
	// The root.
	add("root", ownerTree, true)
	add("root-help", ownerTree, true, "--help")
	add("root-h", ownerTree, false, "-h")
	add("help", ownerTree, true, "help")
	add("root-version", ownerTree, false, "--version")
	add("root--config-check", ownerTree, false, "--config", missingConfig(), "check")
	for _, f := range switches() {
		add("root"+f, ownerTree, false, f)
		add("root"+f+"-check", ownerTree, false, f, "check")
	}
	// gen on the owner's current tree: nothing to write.
	add("gen-check", ownerTree, true, "gen", "--check")
	add("gen-check-json", ownerTree, false, "gen", "--check", "--json")
	add("gen-apply-current", ownerTree, false, "gen", "--apply")
	add("gen-apply-current-json", ownerTree, false, "gen", "--apply", "--json")
	// gen on a tree with one stale file: the plan, the failure, the write.
	add("stale-gen", staleTree, true, "gen")
	add("stale-gen-json", staleTree, false, "gen", "--json")
	add("stale-gen-check", staleTree, true, "gen", "--check")
	add("stale-gen-check-json", staleTree, false, "gen", "--check", "--json")
	add("stale-gen-apply", staleTree, true, "gen", "--apply")
	add("stale-gen-apply-json", staleTree, false, "gen", "--apply", "--json")
	// gragas-dev renders the layer files; gragas gen no longer lists them.
	// Refusals and mistakes.
	add("gen-apply-check", ownerTree, true, "gen", "--apply", "--check")
	add("gen-apply-check-json", ownerTree, false, "gen", "--apply", "--check", "--json")
	add("mistyped-command", ownerTree, true, "chek")
	add("mistyped-flag", ownerTree, true, "gen", "--aply")
	add("extra-argument", ownerTree, true, "check", "extra")
	add("extra-argument-version", ownerTree, false, "version", "extra")
	add("help-nothing", ownerTree, false, "help", "nothing")
	add("check-not-a-repository", emptyTree, true, "check")
	add("check-not-a-repository-json", emptyTree, false, "check", "--json")
	add("gen-not-a-repository", emptyTree, false, "gen")
	add("gen-check-not-a-repository", emptyTree, false, "gen", "--check")
	add("gen-apply-not-a-repository", emptyTree, false, "gen", "--apply")
	// init in the owner's own rulebook, with every flag it takes on the line: it
	// ends not ready, already a rulebook, and writes nothing even with --apply.
	add("init-every-flag", ownerTree, false, "init", "--kind", "rulebook", "--name", "demo", "--rulebook-kind", "overlay", "--version", "1.0.0", "--language", "en", "--licence", "CC-BY-4.0", "--presets-licence", "MIT", "--docs-url", "https://example.test", "--owner", "team-a", "--parent", "base", "--parent-version", "1.0.0", "--parent-hash", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--import", "other@2.0.0=go,ts=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--layer", "language", "--part", "go", "--apply")
	add("version-in-no-repository", emptyTree, false, "version")
	return out
}

// ownerRepo is a copy of what the owner's gate reads and gen renders, in a
// folder of the test's own: the format 2 rulebook/ and presets/golangci/base.yml
// as they stood before the switch to format 3 (frozen under testdata, with
// the old check), the rest of presets/, and the repository's
// layers.yaml, suppressions.txt, docs/layers.md and .golangci.yml.
func ownerRepo(t *testing.T) string {
	t.Helper()
	root := moduleRoot()
	dir := t.TempDir()
	copyTree(t, oldBook(), filepath.Join(dir, "rulebook"))
	copyTree(t, filepath.Join(root, "presets"), filepath.Join(dir, "presets"))
	copyTree(t, filepath.Join(root, "internal", "journeys", "testdata", "presets-v2"), filepath.Join(dir, "presets"))
	for _, f := range []string{"layers.yaml", "suppressions.txt", "docs/layers.md", ".golangci.yml"} {
		copyFile(t, filepath.Join(root, f), filepath.Join(dir, f))
	}
	return dir
}

// staleFile is the generated file the stale tree overwrites.
const staleFile = "rulebook/rules.json"

func tree(t *testing.T, k treeKind) string {
	t.Helper()
	switch k {
	case emptyTree:
		return t.TempDir()
	case genTree:
		return genFixture(t, true)
	case staleTree:
		dir := genFixture(t, true)
		if err := os.WriteFile(filepath.Join(dir, staleFile), []byte("edited by hand\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	default:
		return ownerRepo(t)
	}
}

var (
	// A run id is go-run's sixteen hex digits, or a prefix of eight or more.
	runIDJSON = regexp.MustCompile(`"run": "[0-9a-f]{8,16}"`)
	runIDText = regexp.MustCompile(`\(run [0-9a-f]{8,16}\)`)
	target    = regexp.MustCompile(`\b(?:darwin|linux|windows|freebsd)/[a-z0-9]+\b`)
)

// missingConfig is a settings file no system holds, /nowhere/settings.yaml
// made absolute by filepath.Abs: unchanged where a path has no volume, and
// where one has, the volume of the folder the test runs in, so a run names it
// missing instead of refusing it as not absolute. Nothing is
// made or read there. scrub writes it back as /nowhere/settings.yaml, and
// TestTheMissingConfigFitsTheLine holds the line it is laid out on.
func missingConfig() string {
	const p = "/nowhere/settings.yaml"
	abs, err := filepath.Abs(p)
	if err != nil {
		return p // a run refuses it as not absolute, and the golden says so
	}
	return abs
}

// TestTheMissingConfigFitsTheLine: the --config transcripts' refusal, laid
// out at 80 columns with missingConfig as this system makes it, fits on one
// line as the goldens hold it, so a volume that lengthens the path past that
// fails here by name and not as a wrap in five goldens. Risk: a path that
// differs by system and so a golden that does.
func TestTheMissingConfigFitsTheLine(t *testing.T) {
	t.Parallel()
	p := missingConfig()
	if !filepath.IsAbs(p) {
		t.Fatalf("missingConfig = %q, want an absolute path", p)
	}
	if line := "  not ready  --config names " + p + ", which does not exist"; len(line) > 80 {
		t.Fatalf("the --config refusal is %d columns with %q, and the goldens hold it on one line of 80", len(line), p)
	}
}

// scrub puts back as words what depends on the clock or the machine and
// nothing else: the Go release, the target, a run id, the test's folder and
// missingConfig as this system makes it. Every other byte of a transcript is
// kept (the one helper the issue allows).
func scrub(s, dir string) string {
	if dir != "" {
		s = strings.ReplaceAll(s, dir, "<repo>")
	}
	s = strings.ReplaceAll(s, missingConfig(), "/nowhere/settings.yaml")
	s = goVersion.ReplaceAllString(s, "go<version>")
	s = target.ReplaceAllString(s, "<target>")
	s = runIDJSON.ReplaceAllString(s, `"run": "<run>"`)
	return runIDText.ReplaceAllString(s, "(run <run>)")
}

// TestScrubChangesOnlyWhatMovesWithTheMachine: the helper's cases, a planted
// one that must change and the near misses that must not.
func TestScrubChangesOnlyWhatMovesWithTheMachine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, in, dir, want string }{
		{"a go release", "go  go1.27.3\nand go1.27 too", "", "go  go<version>\nand go<version> too"},
		{"a target", "target    linux/amd64\nand darwin/arm64", "", "target    <target>\nand <target>"},
		{"an os alone stays", "the linux box and darwin/", "", "the linux box and darwin/"},
		{"a json run id", `"run": "fed6b301",`, "", `"run": "<run>",`},
		{"a text run id", "failed  no (run 3d2e0a7b)", "", "failed  no (run <run>)"},
		{"go-run's json run id", `"run": "c9b5f6599e523b20",`, "", `"run": "<run>",`},
		{"go-run's text run id", "failed  no (run c9b5f6599e523b20)", "", "failed  no (run <run>)"},
		{"a longer hex stays", "(run c9b5f6599e523b2011)", "", "(run c9b5f6599e523b2011)"},
		{"the folder", "in /work/x1/rulebook", "/work/x1", "in <repo>/rulebook"},
		{"a short id stays", "(run abc)", "", "(run abc)"},
		{"a count stays", "1,003 entries in 154 files", "", "1,003 entries in 154 files"},
		{"a run word stays", "the run id is 3d2e0a7b", "", "the run id is 3d2e0a7b"},
		{"no folder named", "in /work/x1", "", "in /work/x1"},
		{"the missing config", "--config names " + missingConfig() + ", which", "", "--config names /nowhere/settings.yaml, which"},
	} {
		if got := scrub(tc.in, tc.dir); got != tc.want {
			t.Errorf("%s: scrub(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestTheOwnersTranscripts: each scenario's command line, stdout, stderr and
// exit code, compared with its golden and never rewritten by the gate
// (tests.generated.5, cli.output.3). Risk: any move of check or gen that
// changes what the owner's gate prints, writes or exits with.
func TestTheOwnersTranscripts(t *testing.T) {
	t.Parallel()
	all := scenarios()
	ran := 0
	seen := map[string]bool{}
	for _, sc := range all {
		widths := []string{"80"}
		if sc.narrow {
			widths = []string{"80", "60", "40"}
		}
		for _, cols := range widths {
			name := "owner-" + sc.name + "-" + cols
			if seen[name] {
				t.Fatalf("two scenarios are named %s", name)
			}
			seen[name] = true
			t.Run(name, func(t *testing.T) {
				// A run that writes (--apply) takes the lock in the state folder every
				// run shares, and is told busy when another holds it, so the writers stay
				// serial. The rest have their own tree and read only.
				if !slices.Contains(sc.args, "--apply") {
					t.Parallel()
				}
				dir := tree(t, sc.tree)
				r := run(t, dir, map[string]string{"COLUMNS": cols}, sc.args...)
				got := "$ gragas " + strings.Join(sc.args, " ") + "\n--- stdout\n" + r.stdout +
					"--- stderr\n" + r.stderr + "--- exit " + strconv.Itoa(r.code) + "\n"
				uitest.Golden(t, name, scrub(got, dir))
			})
			ran++
		}
	}
	if ran < 100 {
		t.Fatalf("only %d transcripts ran: the table was emptied", ran)
	}
}

var helpFlag = regexp.MustCompile(`(?m)^\s+(?:(-[A-Za-z]) )?(--[a-z-]+)\s`)

// helpFlags are the flags a command's help lists, minus -h/--help.
func helpFlags(t *testing.T, args ...string) []string {
	t.Helper()
	r := run(t, t.TempDir(), nil, append(args, "--help")...)
	if r.code != 0 {
		t.Fatalf("%v --help exited %d", args, r.code)
	}
	var out []string
	n := 0
	for _, m := range helpFlag.FindAllStringSubmatch(flagsSection(r.stdout), -1) {
		n++
		if m[2] == "--help" {
			continue
		}
		if m[1] != "" {
			out = append(out, m[1])
		}
		out = append(out, m[2])
	}
	if n == 0 {
		t.Fatalf("no flags found in the help of %v:\n%s", args, r.stdout)
	}
	sort.Strings(out)
	return out
}

// TestEveryCommandAndFlagHasATranscript: the binary's commands and flags are
// the literal surface above, and every one appears on the command line of a
// scenario. Risk: a command or flag added or dropped with no transcript.
func TestEveryCommandAndFlagHasATranscript(t *testing.T) {
	t.Parallel()
	root := run(t, t.TempDir(), nil, "--help")
	listed := regexp.MustCompile(`(?m)^    (check|gen|help|init|version)\b`).FindAllStringSubmatch(root.stdout, -1)
	if len(listed) != 5 {
		t.Fatalf("the root help lists %d of the five known commands:\n%s", len(listed), root.stdout)
	}
	if got := helpFlags(t); !slices.Equal(got, rootFlags()) {
		t.Errorf("root flags are %v, the contract lists %v", got, rootFlags())
	}
	used := map[string]bool{}
	for _, sc := range scenarios() {
		for _, a := range sc.args {
			used[sc.args[0]+" "+a] = true
		}
		if len(sc.args) > 0 {
			for _, a := range sc.args {
				used["* "+a] = true
			}
		}
	}
	checked := 0
	for cmd, want := range commandFlags() {
		got := helpFlags(t, cmd)
		w := slices.Clone(want)
		sort.Strings(w)
		if !slices.Equal(got, w) {
			t.Errorf("%s flags are %v, the contract lists %v", cmd, got, w)
		}
		for _, f := range want {
			checked++
			if !used[cmd+" "+f] {
				t.Errorf("no transcript runs %s %s", cmd, f)
			}
		}
		if !used[cmd+" "+cmd] || !used[cmd+" --help"] {
			t.Errorf("no transcript runs %s bare and with --help", cmd)
		}
	}
	if checked < 20 {
		t.Fatalf("only %d flags checked", checked)
	}
	for _, f := range rootFlags() {
		if !used["* "+f] {
			t.Errorf("no transcript runs the root flag %s", f)
		}
	}
}

// TestNoGateRewritesAGolden: -update is a person's act, so nothing the gates
// run passes it (tests.generated.5). A planted line must be seen.
func TestNoGateRewritesAGolden(t *testing.T) {
	t.Parallel()
	root := moduleRoot()
	files := []string{"Taskfile.yml", "hooks/pre-commit", "hooks/pre-push"}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		if passesUpdate(string(data)) {
			t.Errorf("%s passes -update", f)
		}
	}
	if !passesUpdate("\tgo test ./... -update\n") {
		t.Error("the guard does not see a planted -update")
	}
	if passesUpdate("go test ./...\n--no-update-notifier\n") {
		t.Error("the guard sees -update in an unrelated word")
	}
}

var updateFlag = regexp.MustCompile(`(^|\s)-{1,2}update(\s|=|$)`)

func passesUpdate(s string) bool {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.Contains(line, "go test") && updateFlag.MatchString(line) {
			return true
		}
	}
	return false
}

// flagsSection is a help text from its FLAGS heading on, so a flag named in
// the prose above is not counted.
func flagsSection(help string) string {
	_, after, _ := strings.Cut(help, "FLAGS")
	return after
}
