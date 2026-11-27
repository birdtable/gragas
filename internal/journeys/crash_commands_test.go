package journeys

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	testenv "github.com/birdtable/go-testkit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The crash injection of every writing command (store.recovery.2).
// gate/crash.json names each command of the tree that writes;
// a workload row is a command line that cmd/gragas-crash run runs as
// cmd/gragas wires it, over a seeded root, with the writer and the journal
// counted, and stopped at a numbered boundary to be killed with SIGKILL.
// After the kill, gragas journal recover --apply runs in the same child, and
// the root must be the root before the command or the root a whole run of it
// leaves, with no run open. internal/cli/root holds the list to the registry.

// crashListFile is the crash list, from the module root.
const crashListFile = "gate/crash.json"

// minPoints is the fewest kill points a workload states: one in each stage a
// run of a command numbers, before-intent, after-intent, between-files and
// after-record.
const minPoints = 4

// A workload is one row of the crash list that is crash-injected.
type workload struct {
	Command string   `json:"command"`
	Seed    string   `json:"seed"`
	Args    []string `json:"args"`
	Points  int      `json:"points"`
}

// workloads are the crash list's workload rows; none fails, since the list
// then kills nothing.
func workloads(t *testing.T) []workload {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot(), filepath.FromSlash(crashListFile)))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Crash []workload `json:"crash"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", crashListFile, err)
	}
	out := slices.DeleteFunc(doc.Crash, func(w workload) bool { return w.Args == nil })
	for _, w := range out {
		if w.Points < minPoints {
			t.Fatalf("%s: %s kills at %d points, fewer than one in each of the %d stages", crashListFile, w.Command, w.Points, minPoints)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no workload, so no command is killed", crashListFile)
	}
	return out
}

// A commandRoot is one run's folders: the root the command works in, its
// environment, and the state folder its lock and journal live in.
type commandRoot struct {
	dir, state string
	env        map[string]string
	// judged is whether the last recovery's check found a run in the
	// journal to judge.
	judged bool
}

// seeded is a root of w's seed with an environment of its own: the account's
// git identity, the settings naming git, and fresh state and cache folders.
func seeded(t *testing.T, w workload) *commandRoot {
	t.Helper()
	var dir string
	switch w.Seed {
	case "empty":
		dir = newFolder(t)
	case "v2":
		dir = v2Folder(t)
	case "history":
		dir = historyRepo(t, "")
	case "undo":
		dir = promoteRepo(t)
	case "import":
		dir = importRepo(t)
	case "bind":
		dir = bindRepo(t)
	case "bound", "unwritten":
		dir = boundRepo(t)
	default:
		t.Fatalf("%s: the seed %q is none the journey knows", w.Command, w.Seed)
	}
	env := ruleEnv(t)
	env["COLUMNS"], env["GRAGAS_NAMES_FILE"] = "80", ""
	root := &commandRoot{dir: dir, state: appFolders(t, env).State, env: env}
	if w.Seed == "undo" {
		// the undo journeys' fixture: one promote recorded, entry 1, the last.
		// It runs in the crash child, whose clock is fixed, so each root holds
		// the same history.jsonl: a promote through the real binary would
		// stamp the entry with the moment it ran, and no two roots would match.
		if r := root.runTo(t, nil, with([]string{"rule", "promote", "go.errors.2"}, append(decide(), "--apply")...)...); r.code != 0 {
			t.Fatalf("%s: the seed's promote exited %d\n%s%s", w.Command, r.code, r.stdout, r.stderr)
		}
	}
	if w.Seed == "bind" || w.Seed == "bound" || w.Seed == "unwritten" {
		// the plugin that writes golangci-lint's preset, installed
		// in the crash child so each root holds the same history
		servePresets(t, dir, func(args ...string) result { return root.runTo(t, nil, args...) })
	}
	if w.Seed == "bound" || w.Seed == "unwritten" {
		// the bind applied in the crash child, as the undo seed applies its
		// promote: its clock is fixed, so each root holds the same
		// history.jsonl, which the same bind through the real binary would
		// stamp with the moment it ran.
		if r := root.runTo(t, nil, bindArgs("--apply")...); r.code != 0 {
			t.Fatalf("%s: the seed's bind exited %d\n%s%s", w.Command, r.code, r.stdout, r.stderr)
		}
	}
	if w.Seed == "unwritten" {
		// the bind wrote the preset; preset has something to write only where
		// the file is not there
		if err := os.Remove(presetOf(dir)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// boundRepo is the bind seed with the decision "wrap" a move of the errorlint
// binding stands on; seeded applies the binding itself, in the crash child.
func boundRepo(t *testing.T) string {
	t.Helper()
	dir := bindRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "decisions.yaml"), []byte(wrapDecisions), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// child is the crash child's binary over this root: it starts in the root,
// in the isolated environment with this root's variables on top.
func (r *commandRoot) child() *testenv.Binary {
	// The env as the system the child is built for reads it (placed): on Windows,
	// where appdirs reads only LOCALAPPDATA and USERPROFILE, a raw env would put
	// every child's lock and journal in one shared folder.
	env := placed(r.env)
	kv := make([]string, 0, len(env))
	for _, k := range slices.Sorted(maps.Keys(env)) {
		kv = append(kv, k+"="+env[k])
	}
	return testenv.Build("cmd/gragas-crash").Dir(r.dir).Env(kv...)
}

// runTo runs the command line to its end in the child, with the run mode's
// flags before --.
func (r *commandRoot) runTo(t *testing.T, flags []string, line ...string) result {
	t.Helper()
	res := r.child().Run(append(append(append([]string{"run"}, flags...), "--"), line...)...)
	if res.Code == -1 {
		t.Fatalf("could not run gragas-crash:\n%s", res.Stderr)
	}
	return result{stdout: res.Stdout, stderr: res.Stderr, code: res.Code}
}

// plantedStop is where a planted run stops: its first file written, its
// intent never journalled.
var plantedStop = regexp.MustCompile(`^stopped [0-9]+ planted$`)

// pointLine is a boundary the child says once a run ends.
var pointLine = regexp.MustCompile(`^point (\d+) (before-intent|after-intent|between-files|after-record)$`)

// boundariesOf reads the stage of each boundary from a whole run's stderr.
func boundariesOf(t *testing.T, res result) []string {
	t.Helper()
	var out []string
	for line := range strings.SplitSeq(res.stderr, "\n") {
		m := pointLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[1] != strconv.Itoa(len(out)) {
			t.Fatalf("the child said %q, not boundary %d", line, len(out))
		}
		out = append(out, m[2])
	}
	return out
}

// A holder is a child stopped at a boundary and still alive, so whatever it
// holds, its lock among it, it holds until end kills it.
type holder struct {
	run *testenv.Running
	at  int
}

// killAt starts the command line, waits for the child to say it stopped at
// boundary at, and kills it with SIGKILL.
func (r *commandRoot) killAt(t *testing.T, at int, stage string, plant string, line ...string) {
	t.Helper()
	r.start(t, at, stage, plant, line...).end(t)
}

// stopAt starts the command line and waits for the child to stop at
// boundary at, and leaves it there.
func (r *commandRoot) stopAt(t *testing.T, at int, stage string, line ...string) holder {
	t.Helper()
	return r.start(t, at, stage, "", line...)
}

// start runs the child to its stop: boundary at, or with a plant, the
// planted stop.
func (r *commandRoot) start(t *testing.T, at int, stage string, plant string, line ...string) holder {
	t.Helper()
	flags := []string{"run", "-stop", strconv.Itoa(at)}
	if plant != "" {
		flags = append(flags, "-plant", plant)
	}
	// The holder outlives start, so the deadline is the test's to end.
	ctx, cancel := context.WithTimeout(t.Context(), killDeadline)
	t.Cleanup(cancel)
	run, err := r.child().Start(ctx, append(append(flags, "--"), line...)...)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("stopped %d %s", at, stage)
	says := func(line string) bool { return line == want }
	if plant != "" {
		want = "stopped <n> planted"
		says = plantedStop.MatchString
	}
	var said []string
	found := false
	sc := bufio.NewScanner(run.Stderr())
	for !found && sc.Scan() {
		found = says(sc.Text())
		if !found {
			said = append(said, sc.Text())
		}
	}
	if !found {
		_ = run.Kill()
		ended := run.Wait()
		t.Fatalf("boundary %d: the child never said %q; it said %q, and ended with code %d: %s", at, want, said, ended.Code, ended.Stderr)
	}
	return holder{run: run, at: at}
}

// end kills the stopped child with SIGKILL, as a crash would end it.
func (h holder) end(t *testing.T) {
	t.Helper()
	if err := h.run.Kill(); err != nil {
		t.Fatal(err)
	}
	if res := h.run.Wait(); res.Code != -1 {
		t.Fatalf("boundary %d: the child was not ended by the kill: code %d", h.at, res.Code)
	}
}

// snapshot is every file of the root but git's, by slash path, with the hex
// SHA-256 of its bytes.
func (r *commandRoot) snapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(r.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(r.dir, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// hasJournal is whether any journal file is under the root's state folder.
func (r *commandRoot) hasJournal(t *testing.T) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(r.state, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".journal") {
			found = true
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// listed is each run of the root's journal and its state, open or the
// outcome its close records, as journal list --json reads them through the
// Journal port, run in the child over this root as cmd/gragas runs it. The
// checker never reads the fields of a line, so a form of the line that moves
// cannot blind it, and a journal the port refuses fails the test.
func (r *commandRoot) listed(t *testing.T) map[string]string {
	t.Helper()
	res := r.runTo(t, nil, "journal", "list", "--json")
	var l struct {
		Runs []struct {
			Run   string `json:"run"`
			State string `json:"state"`
		} `json:"runs"`
	}
	if res.code != 0 {
		t.Fatalf("journal list --json exited %d: the journal could not be read through the port\n%s%s", res.code, res.stdout, res.stderr)
	}
	if err := json.Unmarshal([]byte(res.stdout), &l); err != nil {
		t.Fatalf("journal list --json printed %q, not a listing: %v", res.stdout, err)
	}
	out := map[string]string{}
	for _, run := range l.Runs {
		if run.Run == "" || run.State == "" {
			t.Fatalf("journal list --json listed %+v, not a run and its state", run)
		}
		out[run.Run] = run.State
	}
	return out
}

// openRuns is the runs the root's journal holds with no close.
func (r *commandRoot) openRuns(t *testing.T) []string {
	t.Helper()
	var out []string
	for run, state := range r.listed(t) {
		if state == "open" {
			out = append(out, run)
		}
	}
	slices.Sort(out)
	return out
}

// whole is one workload run whole in a fresh root: the root before, the
// root after, and the stage of each boundary it numbered.
type whole struct {
	before, after map[string]string
	stages        []string
}

func runWhole(t *testing.T, w workload) whole {
	t.Helper()
	r := seeded(t, w)
	before := r.snapshot(t)
	res := r.runTo(t, nil, w.Args...)
	if res.code != 0 {
		t.Fatalf("%s: the whole run exited %d\n%s%s", w.Command, res.code, res.stdout, res.stderr)
	}
	out := whole{before: before, after: r.snapshot(t), stages: boundariesOf(t, res)}
	if len(out.stages) == 0 || sameFiles(out.before, out.after) {
		t.Fatalf("%s: the whole run numbered %d boundaries and changed the root %t: it proves nothing", w.Command, len(out.stages), !sameFiles(out.before, out.after))
	}
	if open := r.openRuns(t); len(open) > 0 {
		t.Fatalf("%s: the whole run left %v open", w.Command, open)
	}
	return out
}

// sameFiles is whether two snapshots are one.
func sameFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// endState says which end a root is: "before", "after", or how it is
// neither.
func endState(got map[string]string, w whole) string {
	switch {
	case sameFiles(got, w.before):
		return "before"
	case sameFiles(got, w.after):
		return "after"
	}
	var diff []string
	for p, sum := range got {
		if sum != w.before[p] && sum != w.after[p] {
			diff = append(diff, p+" is neither its before nor its after")
		}
	}
	for p := range w.after {
		if _, ok := got[p]; !ok && w.before[p] != "" {
			diff = append(diff, p+" is gone")
		}
	}
	slices.Sort(diff)
	if len(diff) == 0 {
		diff = []string{"some files are before the command and some after it"}
	}
	return strings.Join(diff, "; ")
}

// killPoints draws the boundaries to kill at from seed: points of them,
// taking each stage in turn so every stage that has boundaries is hit, or
// every boundary when all is set.
func killPoints(seed uint64, stages []string, points int, all bool) []int {
	by := map[string][]int{}
	var order []string
	for at, s := range stages {
		if by[s] == nil {
			order = append(order, s)
		}
		by[s] = append(by[s], at)
	}
	if all {
		points = len(stages)
	}
	// A seeded draw, so a failure can name its points; not a secret.
	r := configtest.NewDraw(seed ^ 0x5bd1e995)
	for _, s := range order {
		r.Shuffle(len(by[s]), func(i, j int) { by[s][i], by[s][j] = by[s][j], by[s][i] })
	}
	var out []int
	for len(out) < points {
		took := false
		for _, s := range order {
			if len(out) < points && len(by[s]) > 0 {
				out, by[s], took = append(out, by[s][0]), by[s][1:], true
			}
		}
		if !took {
			break
		}
	}
	slices.Sort(out)
	return out
}

// TestEveryWritingCommandKilledAtABoundaryIsBeforeOrAfter is the crash
// injection of each workload of gate/crash.json: killed with SIGKILL at
// boundaries drawn from a seed over its stages (at least its points, every
// boundary under task crash), then recovered by journal recover --apply,
// the root is the root before the command or after a whole run of it, the
// recovery ends done (so the kill left no lock held), and no run is open. A
// failure names the seed and the boundary. Risk: a writing command that
// leaves a root half written, or a run open, when it dies between two of
// its effects.
func TestEveryWritingCommandKilledAtABoundaryIsBeforeOrAfter(t *testing.T) {
	t.Parallel()
	seed, count := crashSettings(t)
	all := count > shortPoints
	for _, w := range workloads(t) {
		t.Run(w.Command, func(t *testing.T) {
			t.Parallel()
			whole := runWhole(t, w)
			at := killPoints(seed, whole.stages, w.Points, all)
			if len(at) < w.Points {
				t.Fatalf("%s numbers %d boundaries, fewer than its %d points", w.Command, len(whole.stages), w.Points)
			}
			for _, s := range []string{"before-intent", "after-intent", "between-files", "after-record"} {
				// a run that writes no record (preset) has no boundary after one
				if !slices.Contains(whole.stages, s) {
					continue
				}
				if !slices.ContainsFunc(at, func(n int) bool { return whole.stages[n] == s }) {
					t.Errorf("seed %d: no kill point of %s falls %s", seed, w.Command, s)
				}
			}
			t.Logf("crash %s: seed %d, boundaries %v of %d (%s=%d runs them again)", w.Command, seed, at, len(whole.stages), seedVar, seed)
			judged, proved := make([]bool, len(at)), make([]bool, len(at))
			t.Run("kills", func(t *testing.T) {
				for i, n := range at {
					t.Run(fmt.Sprintf("killed %s at boundary %d", whole.stages[n], n), func(t *testing.T) {
						t.Parallel()
						r := seeded(t, w)
						r.killAt(t, n, whole.stages[n], "", w.Args...)
						stage := whole.stages[n]
						if n == len(whole.stages)-1 && !slices.Contains(whole.stages, "after-record") {
							// a run with no record ends on its last file: nothing is
							// left open after it, as after a record
							stage = "after-record"
						}
						bad := recovered(t, r, whole, stage)
						judged[i], proved[i] = r.judged, r.judged && len(bad) == 0
						if len(bad) > 0 {
							t.Error(strings.Join(bad, "\n"))
						}
					})
				}
			})
			// zero boundaries proved is a red, not a pass: a checker that
			// found no run to judge proved nothing
			j, p := trues(judged), trues(proved)
			t.Logf("crash %s: killed at %d boundaries, %d with a run to judge, %d proved", w.Command, len(at), j, p)
			if p == 0 {
				t.Errorf("seed %d: %s was killed at %d boundaries and none left a run in the journal that the checker proved", seed, w.Command, len(at))
			}
		})
	}
}

// trues is how many of bs are true.
func trues(bs []bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}

// recovered is what is wrong with a killed root, none when it holds: the
// journal held what it must at the kill (no run before the intent, the one
// run to recover after it and before the record, at most that one once the
// record is written), journal recover --apply ends done (a lock still held
// ends it busy), and the root is before or after with no run open.
func recovered(t *testing.T, r *commandRoot, w whole, stage string) []string {
	t.Helper()
	var bad []string
	if stage != "before-intent" && !r.hasJournal(t) {
		// the child's journal is not where the root reads it: openRuns would read an empty folder
		bad = append(bad, "killed "+stage+", no journal is under the root's state folder "+r.state)
	}
	held := r.openRuns(t)
	switch {
	case stage == "before-intent" && len(held) != 0:
		bad = append(bad, fmt.Sprintf("killed before the intent, the journal holds %v open", held))
	case (stage == "after-intent" || stage == "between-files") && len(held) != 1:
		bad = append(bad, fmt.Sprintf("killed %s, the journal holds %v open, want the one run journal recover rolls back", stage, held))
	case stage == "after-record" && len(held) > 1:
		bad = append(bad, fmt.Sprintf("killed after the record, the journal holds %v open, want the run or none", held))
	}
	rec := r.runTo(t, nil, "journal", "recover", "--apply")
	if rec.code != 0 {
		bad = append(bad, fmt.Sprintf("journal recover --apply exited %d after the kill (a lock still held ends it busy)\n%s%s", rec.code, rec.stdout, rec.stderr))
	}
	if end := endState(r.snapshot(t), w); end != "before" && end != "after" {
		bad = append(bad, "after the kill and the recovery the root is neither before nor after the command: "+end)
	}
	after := r.listed(t)
	r.judged = len(after) > 0
	if !r.judged && stage != "before-intent" {
		// killed past the intent, the run was journalled: a journal that lists
		// no run is a checker with no run to judge, and red
		bad = append(bad, "killed "+stage+", the journal lists no run after the recovery: the checker has no run to judge")
	}
	for _, run := range slices.Sorted(maps.Keys(after)) {
		if after[run] == "open" {
			bad = append(bad, fmt.Sprintf("after the recovery run %s is open", run))
		}
	}
	return bad
}

// checkRecovered fails the test with what recovered finds.
func checkRecovered(t *testing.T, r *commandRoot, w whole, stage string) {
	t.Helper()
	if bad := recovered(t, r, w, stage); len(bad) > 0 {
		t.Error(strings.Join(bad, "\n"))
	}
}

// TestTheCommandCrashCheckerGoesRedOnItsPlants: the checker, recovered, is
// held to planted failures on the first workload, each beside a control
// that passes it (gates.plants.1). Planted: a write path that writes its
// first file before it journals anything of the run, its intent first
// (store.consistency.4), so recovery has nothing to roll back; and a lock
// still held, a run of the command stopped and not killed, so journal
// recover ends busy. The control is the same command killed unplanted at
// its first boundary between files. The refusal: the child refuses a plant
// it does not know. Risk: a checker that passes every root and so proves
// nothing.
func TestTheCommandCrashCheckerGoesRedOnItsPlants(t *testing.T) {
	t.Parallel()
	w := workloads(t)[0]
	whole := runWhole(t, w)
	between := slices.Index(whole.stages, "between-files")
	if between < 0 {
		t.Fatalf("%s numbers no boundary between files", w.Command)
	}
	t.Run("a write before the intent", func(t *testing.T) {
		t.Parallel()
		planted := seeded(t, w)
		planted.killAt(t, -1, "", "intent-late", w.Args...)
		if bad := recovered(t, planted, whole, "planted"); len(bad) == 0 {
			t.Error("the planted write before the intent passed the checker")
		}
	})
	t.Run("a lock still held", func(t *testing.T) {
		t.Parallel()
		held := seeded(t, w)
		holder := held.stopAt(t, between, whole.stages[between], w.Args...)
		bad := recovered(t, held, whole, "held")
		holder.end(t)
		if !slices.ContainsFunc(bad, func(b string) bool { return strings.Contains(b, "journal recover --apply exited") }) {
			t.Errorf("a recovery under a lock still held passed the checker: %q", bad)
		}
	})
	t.Run("the control", func(t *testing.T) {
		t.Parallel()
		clean := seeded(t, w)
		clean.killAt(t, between, whole.stages[between], "", w.Args...)
		checkRecovered(t, clean, whole, whole.stages[between])
		if !clean.judged {
			t.Error("the control was not judged: the journal listed no run after the recovery")
		}
	})
	t.Run("a journal that lists no run", func(t *testing.T) {
		// what a reader blind to the journal's form sees: the
		// root recovered, then its journal emptied, is red for having no run
		// to judge, whatever else the empty journal fails (the open run the
		// kill left is not listed either)
		t.Parallel()
		blind := seeded(t, w)
		blind.killAt(t, between, whole.stages[between], "", w.Args...)
		if bad := recovered(t, blind, whole, whole.stages[between]); len(bad) > 0 {
			t.Fatalf("the root before the plant failed the checker:\n%s", strings.Join(bad, "\n"))
		}
		emptyJournals(t, blind.state)
		bad := recovered(t, blind, whole, whole.stages[between])
		if blind.judged || !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, "no run to judge") }) {
			t.Errorf("over an emptied journal the checker was judged %t and said %q; want it red for having no run to judge", blind.judged, bad)
		}
	})
	t.Run("an unknown plant is refused", func(t *testing.T) {
		t.Parallel()
		if res := seeded(t, w).runTo(t, []string{"-plant", "nonsense"}, w.Args...); res.code != 2 {
			t.Errorf("-plant nonsense exited %d, want the usage exit 2\n%s", res.code, res.stderr)
		}
	})
}

// TestTheCrashGoldens: each workload of gate/crash.json run whole, its root
// before and after as a golden: the two end states a kill may leave
// (cli.commands.7, A37), and the stages its boundaries fall in, written only
// with -update by a person who reads the diff. The kill test holds every
// killed root to these two. Risk: a command whose allowed ends move unseen.
func TestTheCrashGoldens(t *testing.T) {
	t.Parallel()
	for _, w := range workloads(t) {
		t.Run(w.Command, func(t *testing.T) {
			t.Parallel()
			whole := runWhole(t, w)
			var b strings.Builder
			fmt.Fprintf(&b, "$ gragas %s\n", strings.Join(w.Args, " "))
			for _, side := range []struct {
				name string
				snap map[string]string
			}{{"before", whole.before}, {"after", whole.after}} {
				fmt.Fprintf(&b, "--- %s\n", side.name)
				paths := make([]string, 0, len(side.snap))
				for p := range side.snap {
					paths = append(paths, p)
				}
				slices.Sort(paths)
				for _, p := range paths {
					sum := side.snap[p]
					if strings.HasPrefix(p, "author/") || strings.HasPrefix(p, "plugins/") {
						// the stand-in's bundle, its lock and its approval: the module is
						// built from this tree at test time, so its sum, and the sums of
						// the files that carry its digest, move with any change to the
						// stand-in or to what it imports. The path stays in the golden.
						sum = strings.Repeat("-", len(sum))
					}
					fmt.Fprintf(&b, "%s %s\n", sum, p)
				}
			}
			fmt.Fprintf(&b, "--- boundaries\n%s\n", strings.Join(whole.stages, "\n"))
			uitest.Golden(t, "crash-"+w.Command, b.String())
		})
	}
}
