package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/digest"
)

// checkedRepo is a repository folder for tools-check: tools.yaml pinning lint
// (a release tool) and holding task, a go tool, by the lock alone (its
// no-sha256, as the repository's own row says), tools/go.sum copied from the
// repository's own, and in
// .tools/bin the repository's pinned task, a placeholder for each other name of
// stale, and lint as the pin says. The pinned task is the one program built
// from the lock that every clone has once its tools are installed.
// taskByTheLock is checkedRepo's row for task: a go tool held by the lock
// alone, whose row says why it pins no sha256.
const taskByTheLock = "\n  - id: task\n    from: go\n    no-sha256: \"held by its build info\"\n"

func checkedRepo(t *testing.T, stale ...string) (dir string, lint []byte) {
	t.Helper()
	// The clone's task is under task's file name on this system.
	pinnedTask, err := os.ReadFile(filepath.Join("..", "..", ".tools", "bin", proc.LocalFileName("task")))
	if err != nil {
		t.Fatal("the pinned task is not installed: run task tools")
	}
	sum, err := os.ReadFile(filepath.Join("..", "..", "tools", "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	lint = []byte("the pinned lint")
	dir = pinnedRepo(t, "linux/amd64", lint, tarGz(t, "lint-1/lint", lint))
	pins, err := os.ReadFile(filepath.Join(dir, "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), append(pins, taskByTheLock...), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"tools/go.sum": sum, ".tools/bin/task": pinnedTask, ".tools/bin/lint": lint} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(name, ".tools/bin/") {
			if err := os.WriteFile(p, body, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		filestest.WriteProgram(t, p, body)
	}
	for _, n := range stale {
		filestest.WriteProgram(t, filepath.Join(dir, ".tools", "bin", n), []byte("a program go cannot read, built outside the lock"))
	}
	return dir, lint
}

// TestToolsCheckHoldsEveryPinInOneRunAndNamesEachThatIsNot (gragas#1215): the
// go tools are held to tools/go.sum through go version -m and the release
// tools to tools.yaml's sha256, all in one process. A clone whose tools are in
// place is green and reads nothing from the network; a go tool that is
// missing, one built outside the lock and a release tool that is not the pin
// are each red, named, with task tools as the fix, and one run names them all.
func TestToolsCheckHoldsEveryPinInOneRunAndNamesEachThatIsNot(t *testing.T) {
	t.Parallel()
	check := func(dir string, goTools ...string) ([]string, string) {
		var out bytes.Buffer
		return toolsCheck(t.Context(), toolsCheckJob{environ: os.Environ(), dir: dir, platform: "linux/amd64", goTools: goTools, ids: []string{"lint"}}, &say.Speaker{W: &out}), out.String()
	}

	dir, _ := checkedRepo(t)
	if got, said := check(dir, "task"); len(got) != 0 || !strings.Contains(said, "fetch: lint in .tools/bin is the pinned program") {
		t.Errorf("tools in place: problems %q, said %q, want none and the release tool held", got, said)
	}

	got, _ := check(dir, "task", "gotestsum")
	if len(got) != 1 || !strings.Contains(got[0], "tools-check: gotestsum is not in .tools/bin: run task tools") {
		t.Errorf("a go tool that is missing: %q", got)
	}

	stale, _ := checkedRepo(t, "gotestsum")
	got, _ = check(stale, "task", "gotestsum")
	if len(got) != 1 || !strings.Contains(got[0], "tools-check: gotestsum in .tools/bin is not built from tools/go.sum") || !strings.HasSuffix(got[0], "run task tools") {
		t.Errorf("a go tool built outside the lock: %q, want it named alone, task being the pinned one", got)
	}

	// A release tool that is not the pin, with a stale go tool beside it: both are named by the one run.
	both, _ := checkedRepo(t, "gotestsum")
	filestest.WriteProgram(t, filepath.Join(both, ".tools", "bin", "lint"), []byte("a stale lint"))
	got, _ = check(both, "task", "gotestsum")
	if len(got) != 2 || !strings.Contains(got[0], "gotestsum") || !strings.Contains(got[1], "lint") || !strings.HasSuffix(got[1], "run task tools") {
		t.Errorf("a stale go tool and a stale release tool: %q, want both named", got)
	}
}

// TestToolsCheckEndsOnTheClosingCodes: green is exitDone, a pin that is not
// held is exitFailed with its line on stderr, and a call without a platform is
// a usage error.
func TestToolsCheckEndsOnTheClosingCodes(t *testing.T) {
	t.Parallel()
	dir, _ := checkedRepo(t)
	var out, errOut bytes.Buffer
	if code := runToolsCheck(t.Context(), os.Environ(), []string{"-C", dir, "--platform", "linux/amd64", "--go", "task", "lint"}, &out, &errOut); code != exitDone {
		t.Errorf("tools in place: exit %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := runToolsCheck(t.Context(), os.Environ(), []string{"-C", dir, "--platform", "linux/amd64", "--go", "task,gotestsum", "lint"}, &out, &errOut); code != exitFailed || !strings.Contains(errOut.String(), "tools-check: gotestsum is not in .tools/bin") {
		t.Errorf("a go tool missing: exit %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := runToolsCheck(t.Context(), os.Environ(), []string{"-C", dir, "--go", "task"}, &out, &errOut); code != exitUsage {
		t.Errorf("no platform: exit %d, want the usage code", code)
	}
}

// TestToolsCheckReadsItsPathsFromItsFlags (gragas#1215): the lock file, the
// file pinning the release tools and the folder of the tools are the flags'
// --lock, --pins and --bin, each read from -C when it is not absolute, so a
// repository that keeps them elsewhere, or a preset another repository
// includes, needs no gragas path. The same repository with the defaults
// finds none of them and is red.
func TestToolsCheckReadsItsPathsFromItsFlags(t *testing.T) {
	t.Parallel()
	dir, _ := checkedRepo(t)
	moves := map[string]string{"tools/go.sum": "locks/tools.sum", "tools.yaml": "pins/release.yaml", ".tools/bin": "bin"}
	for from, to := range moves {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, to)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(dir, from), filepath.Join(dir, to)); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	args := []string{"-C", dir, "--platform", "linux/amd64", "--lock", "locks/tools.sum", "--pins", "pins/release.yaml", "--bin", filepath.Join(dir, "bin"), "--go", "task", "lint"}
	if code := runToolsCheck(t.Context(), os.Environ(), args, &out, &errOut); code != exitDone {
		t.Errorf("paths given by flags: exit %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := runToolsCheck(t.Context(), os.Environ(), []string{"-C", dir, "--platform", "linux/amd64", "--go", "task", "lint"}, &out, &errOut); code != exitFailed {
		t.Errorf("the default paths in a repository that keeps them elsewhere: exit %d, want red", code)
	}
}

// TestToolsCheckFindsWindowsToolsByTheirExeNameWithoutAnExecuteBit (gragas#1088,
// W-2 B and C): on windows/amd64 a go tool is name.exe and carries no execute
// bit, so task.exe at mode 0600, built from the lock, is held, and a bare
// gotestsum, with no .exe, is not the program Windows runs and is named
// missing. It runs the same on every system: the platform is the flag's.
func TestToolsCheckFindsWindowsToolsByTheirExeNameWithoutAnExecuteBit(t *testing.T) {
	t.Parallel()
	dir, _ := checkedRepo(t)
	bin := filepath.Join(dir, ".tools", "bin")
	task := programName("task", "windows/amd64")
	if err := os.Rename(filepath.Join(bin, "task"), filepath.Join(bin, task)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(bin, task), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gotestsum"), []byte("a program by its unix name"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	got := toolsCheck(t.Context(), toolsCheckJob{environ: os.Environ(), dir: dir, platform: "windows/amd64", goTools: []string{"task", "gotestsum"}}, &say.Speaker{W: &out})
	if len(got) != 1 || !strings.Contains(got[0], "tools-check: gotestsum is not in .tools/bin") {
		t.Errorf("%s at 0600 and a bare gotestsum on windows/amd64: %q, want gotestsum alone named missing", task, got)
	}
}

// pnpmTree writes a pnpm tree at dir: a lockfile of pnpm's two documents,
// the package-manager one and the packages one pinning left-pad at locked,
// and, when installed is not "", node_modules/.pnpm/lock.yaml as pnpm writes
// it after a frozen install of a lockfile pinning installed.
func pnpmTree(t *testing.T, dir, locked, installed string) {
	t.Helper()
	doc := func(v string) string {
		return "lockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    dependencies:\n      left-pad:\n        specifier: " + v + "\n        version: " + v + "\n"
	}
	files := map[string]string{"pnpm-lock.yaml": "---\nlockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    configDependencies: {}\n\n---\n" + doc(locked)}
	if installed != "" {
		files[filepath.Join("node_modules", ".pnpm", "lock.yaml")] = doc(installed)
	}
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestToolsCheckHoldsEachPnpmTreeToItsLockfile (gragas#1088, the review of PR
// 1124): --pnpm names the folders task tools installs with pnpm, and each
// node_modules must be the frozen install of its pnpm-lock.yaml, read offline
// from the copy pnpm keeps in node_modules/.pnpm/lock.yaml. Planted: a
// lockfile that moves left-pad with node_modules left as it was, and a tree
// never installed; each is red, names its folder and task tools. The tree
// installed from its lockfile, and the same with CRLF line ends, is green.
func TestToolsCheckHoldsEachPnpmTreeToItsLockfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pnpmTree(t, filepath.Join(dir, "web"), "1.3.0", "1.3.0")
	pnpmTree(t, filepath.Join(dir, "lint", ".node"), "1.3.0", "1.3.0")
	check := func() []string {
		var out bytes.Buffer
		return toolsCheck(t.Context(), toolsCheckJob{environ: os.Environ(), dir: dir, platform: "linux/amd64", pnpmTrees: []string{"web", "lint/.node"}}, &say.Speaker{W: &out})
	}
	if got := check(); len(got) != 0 {
		t.Fatalf("trees installed from their lockfiles are red: %q", got)
	}
	crlf := filepath.Join(dir, "web", "pnpm-lock.yaml")
	text, err := os.ReadFile(crlf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crlf, bytes.ReplaceAll(text, []byte("\n"), []byte("\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := check(); len(got) != 0 {
		t.Fatalf("a lockfile checked out with CRLF line ends is red: %q", got)
	}
	pnpmTree(t, filepath.Join(dir, "web"), "1.3.1", "1.3.0")
	if err := os.Remove(filepath.Join(dir, "lint", ".node", "node_modules", ".pnpm", "lock.yaml")); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"tools-check: web/node_modules is not the install of web/pnpm-lock.yaml (its .pnpm/lock.yaml is another lockfile): run task tools",
		"tools-check: lint/.node/node_modules is not installed (it has no .pnpm/lock.yaml): run task tools",
	}
	if got := check(); !slices.Equal(got, want) {
		t.Errorf("a moved lockfile and a tree never installed:\n got %q\nwant %q", got, want)
	}
	var out, errOut bytes.Buffer
	if code := runToolsCheck(t.Context(), os.Environ(), []string{"-C", dir, "--platform", "linux/amd64", "--pnpm", "web,lint/.node"}, &out, &errOut); code != exitFailed || !strings.Contains(errOut.String(), "web/node_modules is not the install") {
		t.Errorf("--pnpm does not reach the check: exit %d, stderr %q", code, errOut.String())
	}
}

// TestToolsCheckHoldsAGoToolToItsPinnedSha256 (gragas#1428): a go tool built
// from the lock is also the file tools.yaml pins for the system; a wrong sha256
// on its row is red and names both sums, and a tool with no row for the system
// is held by the lock alone.
func TestToolsCheckHoldsAGoToolToItsPinnedSha256(t *testing.T) {
	t.Parallel()
	dir, _ := checkedRepo(t)
	task, err := os.ReadFile(filepath.Join(dir, ".tools", "bin", "task"))
	if err != nil {
		t.Fatal(err)
	}
	pinsPath := filepath.Join(dir, "tools.yaml")
	pins, err := os.ReadFile(pinsPath)
	if err != nil {
		t.Fatal(err)
	}
	// The rows below are task's own, so checkedRepo's no-sha256 row is cut first.
	pins = bytes.Replace(pins, []byte(taskByTheLock), nil, 1)
	row := func(platform, sha string) string {
		return "\n  - id: task\n    from: go\n    platforms:\n      " + platform + ": {sha256: " + sha + "}\n"
	}
	check := func(rows string) []string {
		if err := os.WriteFile(pinsPath, append(append([]byte{}, pins...), rows...), 0o600); err != nil {
			t.Fatal(err)
		}
		return toolsCheck(t.Context(), toolsCheckJob{environ: os.Environ(), dir: dir, platform: "linux/amd64", goTools: []string{"task"}}, &say.Speaker{W: &bytes.Buffer{}})
	}
	if got := check(row("linux/amd64", digest.Hex(task))); len(got) != 0 {
		t.Errorf("a go tool that is the pinned file: %q, want none", got)
	}
	wrong := strings.Repeat("0", 64)
	got := check(row("linux/amd64", wrong))
	if len(got) != 1 || !strings.Contains(got[0], "task in .tools/bin has sha256 "+digest.Hex(task)) || !strings.Contains(got[0], "tools.yaml pins "+wrong) {
		t.Errorf("a wrong sha256 on the row: %q, want one problem naming both sums", got)
	}
	// A row that pins another system only, and says no reason, fails closed (gragas#1480).
	if got := check(row("darwin/arm64", wrong)); len(got) != 1 || !strings.Contains(got[0], "the go tool task's row in tools.yaml pins no sha256 for linux/amd64") {
		t.Errorf("a row for another system only: %q, want task named as pinning none for linux/amd64", got)
	}
}

// TestAGoPinOfAnotherShapeIsRedAndNamesTheRow: a go tool's pin is read in one shape, and every other
// shape is a problem that names the row, never a tool the lock alone holds in
// silence. Planted: a sha256 one digit short, one in capitals, one quoted, a
// sha512 key, an empty pin, the pin as a nested key, a platform pinned twice,
// a pin beside no-sha256, a pin of a system beside its no-<goos> (no-linux
// beside linux/arm64, asked for linux/amd64, which it would quiet), a row
// that pins another system only, a row that is
// not from: go and no row at all. A well-formed pin is read, and a row that
// says why it pins none (no-sha256, and no-<goos> on that system: no-windows
// on windows, and no-plan9 on plan9 the same way) is held by the lock.
func TestAGoPinOfAnotherShapeIsRedAndNamesTheRow(t *testing.T) {
	t.Parallel()
	sha := strings.Repeat("ab", 32)
	row := func(keys, platforms string) string {
		return "tools:\n  - id: dupl\n    from: go\n" + keys + "    platforms:\n" + platforms + "\n  - id: other\n    from: go\n"
	}
	red := map[string]string{
		"a sha256 one digit short":  row("", "      linux/amd64: {sha256: "+sha[1:]+"}\n"),
		"a sha256 in capitals":      row("", "      linux/amd64: {sha256: "+strings.ToUpper(sha)+"}\n"),
		"a quoted sha256":           row("", "      linux/amd64: {sha256: \""+sha+"\"}\n"),
		"a sha512 key":              row("", "      linux/amd64: {sha512: "+sha+"}\n"),
		"an empty pin":              row("", "      linux/amd64: {}\n"),
		"the pin as a nested key":   row("", "      linux/amd64:\n        sha256: "+sha+"\n"),
		"a platform pinned twice":   row("", "      linux/amd64: {sha256: "+sha+"}\n      linux/amd64: {sha256: "+sha+"}\n"),
		"a pin beside no-sha256":    row("    no-sha256: \"a reason\"\n", "      linux/amd64: {sha256: "+sha+"}\n"),
		"a pin beside its no-linux": row("    no-linux: \"a reason\"\n", "      linux/arm64: {sha256: "+sha+"}\n"),
		"another system only":       row("    no-windows: \"a reason\"\n", "      darwin/arm64: {sha256: "+sha+"}\n"),
		"a row not from: go":        "tools:\n  - id: dupl\n    from: release\n    platforms:\n      linux/amd64: {sha256: " + sha + "}\n",
		"no row":                    "tools:\n  - id: other\n    from: go\n    no-sha256: \"a reason\"\n",
	}
	for name, text := range red {
		got, problem := goToolPin([]byte(text), "dupl", "linux/amd64")
		if got != "" || !strings.Contains(problem, "dupl") {
			t.Errorf("%s: pin %q, problem %q, want red naming dupl", name, got, problem)
		}
	}
	if got, problem := goToolPin([]byte(row("", "      darwin/arm64: {sha256: "+strings.Repeat("0", 64)+"}\n      linux/amd64: {sha256: "+sha+"}\r\n")), "dupl", "linux/amd64"); got != sha || problem != "" {
		t.Errorf("a well-formed pin, CRLF ended: pin %q, problem %q, want %s", got, problem, sha)
	}
	for name, c := range map[string]struct{ text, platform string }{
		"no-sha256":               {"tools:\n  - id: dupl\n    from: go\n    no-sha256: \"a reason\"\n", "linux/amd64"},
		"no-windows on windows":   {row("    no-windows: \"a reason\"\n", "      linux/amd64: {sha256: "+sha+"}\n"), "windows/amd64"},
		"no-plan9 on plan9":       {row("    no-plan9: \"a reason\"\n", "      linux/amd64: {sha256: "+sha+"}\n"), "plan9/amd64"},
		"a comment in platforms:": {row("", "      # measured on one run\n      windows/amd64: {sha256: "+sha+"}\n"), "windows/amd64"},
	} {
		got, problem := goToolPin([]byte(c.text), "dupl", c.platform)
		if problem != "" || (name != "a comment in platforms:" && got != "") || (name == "a comment in platforms:" && got != sha) {
			t.Errorf("%s: pin %q, problem %q, want the row's answer and no problem", name, got, problem)
		}
	}
}

// TestEveryGoToolOfToolsYamlHasAWellFormedPinOrAReason (gragas#1480): the
// repository's own tools.yaml reads, under the one shape, for every go row on
// each system task tools runs on: a sha256 for linux/amd64, darwin/arm64,
// linux/arm64 and darwin/amd64 for every row but one that says no-sha256, and
// no problem on windows/amd64, where each row says no-windows. task is the one
// row with no-sha256.
func TestEveryGoToolOfToolsYamlHasAWellFormedPinOrAReason(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var ids, bare []string
	var id string
	for line := range strings.SplitSeq(string(data), "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "  - id: "):
			id = strings.TrimSpace(strings.TrimPrefix(line, "  - id: "))
		case trim == "from: go":
			ids = append(ids, id)
		}
	}
	if len(ids) < 6 {
		t.Fatalf("tools.yaml has %d go rows: %q", len(ids), ids)
	}
	for _, id := range ids {
		for _, platform := range []string{"linux/amd64", "darwin/arm64", "linux/arm64", "darwin/amd64", "windows/amd64"} {
			sha, problem := goToolPin(data, id, platform)
			if problem != "" {
				t.Errorf("%s on %s: %s", id, platform, problem)
			}
			if sha == "" && platform == "linux/amd64" {
				bare = append(bare, id)
			}
		}
	}
	if !slices.Equal(bare, []string{"task"}) {
		t.Errorf("the go rows with no sha256 on linux/amd64: %q, want task alone", bare)
	}
}
