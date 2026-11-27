package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/digest"
)

// runToolsCheck is `tools-check --platform goos/goarch --go name,... id...`: the
// whole of the Taskfile's tools-check in one process. Each go tool named by
// --go must be in .tools/bin and built from the lock, and each release tool id
// must be the program tools.yaml pins; each pnpm tree named by --pnpm must be
// installed from its lockfile; all of them are checked, and each that
// is not is named, so one run reports every stale pin. Nothing is fetched or
// installed: a red check names task tools (supply.install.10).
func runToolsCheck(ctx context.Context, environ, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("tools-check", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the folder the other paths are read from")
	lock := fl.String("lock", "tools/go.sum", "the lock file the go tools are built from, as go.sum lines")
	pins := fl.String("pins", "tools.yaml", "the file pinning the release tools by sha256")
	bin := fl.String("bin", ".tools/bin", "the folder the tools are in")
	platform := fl.String("platform", "", "the system, goos/goarch, whose pins are read")
	goTools := fl.String("go", "", "the go tools, comma separated, built into .tools/bin from tools/go.mod")
	pnpmTrees := fl.String("pnpm", "", "the folders, comma separated, whose node_modules pnpm installs from their pnpm-lock.yaml")
	nodeVersion := fl.String("node-version", "", "the file that pins the node .tools/bin holds, as its version without the v (.node-version)")
	ids, err := parseInterspersed(fl, args)
	if err != nil || *platform == "" {
		talk.Text(usage)
		return exitUsage
	}
	problems := toolsCheck(ctx, toolsCheckJob{dir: *dir, lock: *lock, pins: *pins, bin: *bin, platform: *platform, goTools: commaList(*goTools), pnpmTrees: commaList(*pnpmTrees), nodeVersion: *nodeVersion, ids: ids, environ: environ}, &say.Speaker{W: stdout})
	for _, p := range problems {
		talk.Text(p + "\n")
	}
	if len(problems) > 0 {
		return exitFailed
	}
	return exitDone
}

// commaList is the names of a comma-separated flag, the empty ones left out.
func commaList(s string) []string {
	var out []string
	for n := range strings.SplitSeq(s, ",") {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// toolsCheckJob is one tools-check: the repository, the platform, the go tools,
// the pnpm trees and the release tool ids to hold to their pins.
type toolsCheckJob struct {
	dir, lock, pins, bin, platform string
	goTools, pnpmTrees, ids        []string
	// nodeVersion is the file .node-version, when node's version is held to it.
	nodeVersion string
	// environ is the environment main read, the whole of what node and go run with.
	environ []string
}

// under is p read from dir: p as it is when it is absolute.
func under(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// toolsCheck answers a line for each go tool that is missing or is not built
// from the lock, and for each release tool that is not the pinned program; none
// when every pin holds. A release tool that holds says so on out.
func toolsCheck(ctx context.Context, j toolsCheckJob, out *say.Speaker) (problems []string) {
	if j.lock == "" {
		j.lock = "tools/go.sum"
	}
	if j.bin == "" {
		j.bin = ".tools/bin"
	}
	if j.pins == "" {
		j.pins = "tools.yaml"
	}
	var files goToolFiles
	files.sums, files.sumErr = lockLines(under(j.dir, j.lock))
	files.pins, files.pinsErr = os.ReadFile(under(j.dir, j.pins))
	if files.pinsErr != nil && len(j.goTools) > 0 {
		problems = append(problems, "tools-check: "+j.pins+" cannot be read, so no go tool is held to its sha256: "+files.pinsErr.Error())
	}
	for _, name := range j.goTools {
		if p := goToolProblem(ctx, j, name, files); p != "" {
			problems = append(problems, "tools-check: "+p)
		}
	}
	for _, tree := range j.pnpmTrees {
		if p := pnpmTreeProblem(under(j.dir, tree), tree); p != "" {
			problems = append(problems, "tools-check: "+p)
		}
	}
	for _, id := range j.ids {
		if err := fetchTool(ctx, fetchJob{dir: j.dir, pins: j.pins, bin: j.bin, id: id, platform: j.platform, check: true}, out); err != nil {
			problems = append(problems, "gragas-dev tools-check: "+err.Error())
		}
	}
	if j.nodeVersion != "" {
		if p := nodeVersionProblem(ctx, j); p != "" {
			problems = append(problems, "tools-check: "+p)
		}
	}
	return problems
}

// nodeVersionProblem is why the node of j's folder does not print the version
// .node-version pins, or "". Both are read with their space trimmed, so the
// carriage return node prints on Windows (PR 1124, W-2) and a file saved with
// CRLF line ends are not a difference (gragas#1655, X106). node --version
// prints v and the version; the file holds the version alone.
func nodeVersionProblem(ctx context.Context, j toolsCheckJob) string {
	data, err := os.ReadFile(under(j.dir, j.nodeVersion))
	if err != nil {
		return j.nodeVersion + " cannot be read, so node's version is not held to it: " + err.Error()
	}
	want := strings.TrimSpace(string(data))
	if want == "" {
		return j.nodeVersion + " is empty, so it pins no node"
	}
	node := filepath.Join(under(j.dir, j.bin), programName("node", j.platform))
	got, err := output(ctx, j.environ, j.dir, nil, node, "--version")
	if err != nil {
		return "node in " + j.bin + " did not print its version (" + err.Error() + "): run task tools to fetch the pinned node into " + j.bin
	}
	if have := strings.TrimSpace(string(got)); have != "v"+want {
		return "node is '" + have + "', " + j.nodeVersion + " pins v" + want + ": run task tools to fetch the pinned node into " + j.bin + ", and move " + j.nodeVersion + " only with tools.yaml"
	}
	return ""
}

// goToolFiles is what toolsCheck reads once for every go tool: the lines of the
// lock and the text of tools.yaml, each with the error of its read.
type goToolFiles struct {
	sums    map[string]bool
	sumErr  error
	pins    []byte
	pinsErr error
}

// goToolProblem is why the go tool name in j's folder is not the one task tools
// builds, or "": it is missing, not built from the lock, or not its pin. With
// tools.yaml unread (pinsErr, said once by the caller) the pin is not judged.
func goToolProblem(ctx context.Context, j toolsCheckJob, name string, f goToolFiles) string {
	bin := filepath.Join(under(j.dir, j.bin), programName(name, j.platform))
	if _, err := os.Stat(bin); err != nil {
		return name + " is not in " + j.bin + ": run task tools"
	}
	if f.sumErr != nil || !builtFromTheLock(ctx, j.environ, j.dir, bin, f.sums) {
		return name + " in " + j.bin + " is not built from " + j.lock + " (go version -m names a module, dependency or replacement that is not a line of it): run task tools"
	}
	if f.pinsErr != nil {
		return ""
	}
	return goToolShaProblem(f.pins, name, j.platform, bin, j.bin)
}

// goToolShaProblem is why the program at bin, the go tool id in the folder
// binDir names, is not the file tools.yaml pins for platform, or "". A go tool
// is built by task tools (CGO_ENABLED=0, -trimpath, the row's ldflags, the
// toolchain go.mod pins), so its file is the same on every machine of a
// system, and its sha256 is the pin. A row that says why it has no pin on
// platform (no-sha256, as task's, held by its build info, or no-<goos> for
// platform's system, as no-windows) is held by the lock alone; any other row with no well-formed pin is the problem.
func goToolShaProblem(pins []byte, id, platform, bin, binDir string) string {
	want, problem := goToolPin(pins, id, platform)
	if problem != "" || want == "" {
		return problem
	}
	have, err := os.ReadFile(bin)
	if err != nil {
		return id + " cannot be read: " + err.Error()
	}
	if got := digest.Hex(have); got != want {
		return id + " in " + binDir + " has sha256 " + got + " and tools.yaml pins " + want + " for " + platform + " (it is not the build task tools makes): run task tools, which builds it again, or, if the build is right, re-measure the pin"
	}
	return ""
}

// goPinLine is the one shape of a go tool's pin: a six-space platform key under
// its platforms: and the sha256 of the file, 64 lowercase hex digits.
var goPinLine = regexp.MustCompile(`^      ([a-z0-9]+/[a-z0-9]+): \{sha256: ([0-9a-f]{64})\}\s*$`)

// goToolPin is the sha256 tools.yaml pins for the go tool id on platform, read
// line by line: a tool is a "  - id:" item, its keys four-space lines, and its
// pins the six-space lines of its platforms:. It is "" with no problem when the
// row says why it pins none there: no-sha256 for every system, and no-<goos>
// for a platform of that system, as no-windows, read from the row as data so
// no system is named here. Every other shape is a problem that names the row, so a
// reformat never turns the check off in silence (gragas#1480): no row, a row
// that is not from: go, a platforms line of another shape, a platform pinned
// twice, a pin beside no-sha256, a pin of a system beside its no-<goos>, so
// a no-<goos> quiets only a system the row pins nothing for, and no pin for
// platform with no reason.
func goToolPin(data []byte, id, platform string) (sha, problem string) {
	lines, found := goRowLines(data, id)
	if !found {
		return "", "tools.yaml has no row for the go tool " + id + ", so nothing pins its file"
	}
	r := goRow{id: id, noFor: map[string]bool{}, pins: map[string]string{}}
	for _, line := range lines {
		if p := r.read(line); p != "" {
			return "", p
		}
	}
	switch {
	case !r.goTool:
		return "", id + "'s row in tools.yaml does not say from: go, so it pins no go build"
	case r.noSha && len(r.pins) > 0:
		return "", "the go tool " + id + "'s row in tools.yaml says no-sha256 and pins a sha256: keep one"
	}
	for _, p := range slices.Sorted(maps.Keys(r.pins)) {
		if goos, _, _ := strings.Cut(p, "/"); r.noFor[goos] {
			return "", "the go tool " + id + "'s row in tools.yaml says no-" + goos + " and pins a sha256 for " + p + ": keep one"
		}
	}
	if sha, ok := r.pins[platform]; ok {
		return sha, ""
	}
	goos, _, _ := strings.Cut(platform, "/")
	if r.noSha || r.noFor[goos] {
		return "", ""
	}
	return "", "the go tool " + id + "'s row in tools.yaml pins no sha256 for " + platform + " and says no no-sha256 (nor no-" + goos + ") reason: measure the pin with task tools on " + platform + ", as tools.yaml's header says"
}

// goRowLines is the lines of the "  - id: <id>" item of tools.yaml, past its id
// line and up to the next item or unindented line, with blank and comment
// lines left out; found is whether the item is there.
func goRowLines(data []byte, id string) (lines []string, found bool) {
	for line := range strings.SplitSeq(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "  - id: "); ok {
			if found {
				return lines, true
			}
			found = strings.TrimSpace(rest) == id
			continue
		}
		if !found || trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if line[0] != ' ' {
			return lines, true
		}
		lines = append(lines, line)
	}
	return lines, found
}

// goRow is what goToolPin reads from a go tool's row: its four-space keys and
// the pins of its platforms:. noFor is the systems a no-<goos> key says why
// the row pins none for.
type goRow struct {
	id                     string
	goTool, noSha, inPlats bool
	noFor                  map[string]bool
	pins                   map[string]string
}

// read takes one line of the row, and is the problem it has, or "".
func (r *goRow) read(line string) string {
	trim := strings.TrimSpace(line)
	if indent := len(line) - len(strings.TrimLeft(line, " ")); indent <= 4 {
		r.inPlats = indent == 4 && trim == "platforms:"
		if indent == 4 {
			r.goTool = r.goTool || trim == "from: go"
			r.noSha = r.noSha || strings.HasPrefix(trim, "no-sha256: ")
			if key, _, ok := strings.Cut(trim, ": "); ok {
				if goos, ok := strings.CutPrefix(key, "no-"); ok && goos != "sha256" {
					r.noFor[goos] = true
				}
			}
		}
		return ""
	}
	if !r.inPlats {
		return ""
	}
	m := goPinLine.FindStringSubmatch(line)
	if m == nil {
		return "the go tool " + r.id + "'s row in tools.yaml has a platforms line that is not \"<goos>/<goarch>: {sha256: <64 lowercase hex>}\": " + trim
	}
	if _, twice := r.pins[m[1]]; twice {
		return "the go tool " + r.id + "'s row in tools.yaml pins " + m[1] + " twice"
	}
	r.pins[m[1]] = m[2]
	return ""
}

// pnpmTreeProblem is why the node_modules of the pnpm tree at dir, named name,
// is not the install of its pnpm-lock.yaml, or "". pnpm writes the lockfile it
// installed to node_modules/.pnpm/lock.yaml: the lockfile's last document, the
// one after pnpm's own package-manager document. A lockfile that moved since
// the install (a pin moved, a pull merged) is not that copy, and a tree never
// installed has none; both name task tools, which installs it frozen. Nothing
// is read from the network.
func pnpmTreeProblem(dir, name string) string {
	lock, err := os.ReadFile(filepath.Join(dir, "pnpm-lock.yaml"))
	if err != nil {
		return name + " has no pnpm-lock.yaml to install from: " + err.Error()
	}
	installed, err := os.ReadFile(filepath.Join(dir, "node_modules", ".pnpm", "lock.yaml"))
	if err != nil {
		return name + "/node_modules is not installed (it has no .pnpm/lock.yaml): run task tools"
	}
	if lockedDocument(string(lock)) != strings.ReplaceAll(string(installed), "\r\n", "\n") {
		return name + "/node_modules is not the install of " + name + "/pnpm-lock.yaml (its .pnpm/lock.yaml is another lockfile): run task tools"
	}
	return ""
}

// lockedDocument is the packages document of a pnpm-lock.yaml, with LF line
// ends: the text after the last line "---" when the file starts with one (pnpm
// writes its package-manager document first), else the whole file.
func lockedDocument(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	return text[strings.LastIndex(text, "\n---\n")+len("\n---\n"):]
}

// lockLines is the lines of the lock file, as a set.
func lockLines(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		out[line] = true
	}
	return out, nil
}

// builtFromTheLock is whether go version -m says bin is built from the lock:
// it has a mod line, and its mod line, every dep line with a sum and every =>
// replacement line each name a module, version and sum that is a line of the
// lock. A binary built before a pin moved, or against other dependencies than
// the lock's, has a line that is not there; so has one go cannot read.
func builtFromTheLock(ctx context.Context, environ []string, dir, bin string, sums map[string]bool) bool {
	out, err := output(ctx, environ, dir, nil, "go", "version", "-m", bin)
	if err != nil {
		return false // go could not read bin: no mod line, so not the lock's
	}
	return versionLinesAreTheLocks(string(out), sums)
}

// versionLinesAreTheLocks is builtFromTheLock's judgement of the text go
// version -m printed: a mod line, and its mod line, every dep line with a sum
// and every => line each a line of the lock. A mod or dep line that carries no
// sum because the module is replaced (gragas#2447) is held by the => line that
// follows it, which carries the replacement's sum; a mod line with neither is
// not the lock's.
func versionLinesAreTheLocks(out string, sums map[string]bool) bool {
	seen := false
	awaitsReplacement := false // a mod line with no sum, till its => line
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if awaitsReplacement && f[0] != "=>" {
			return false
		}
		if !slices.Contains([]string{"mod", "dep", "=>"}, f[0]) {
			continue
		}
		seen = seen || f[0] == "mod"
		key, sum := lockKey(f)
		if !sum {
			if f[0] == "=>" {
				return false
			}
			awaitsReplacement = f[0] == "mod"
			continue
		}
		awaitsReplacement = false
		if !sums[key] {
			return false
		}
	}
	return seen && !awaitsReplacement
}

// lockKey is the lock line a mod, dep or => line of go version -m names, its
// module, version and sum, and whether the line has a sum at all.
func lockKey(f []string) (key string, sum bool) {
	if len(f) < 4 {
		return "", false
	}
	return fmt.Sprintf("%s %s %s", f[1], f[2], strings.Join(f[3:], " ")), true
}
