package journeys

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	testenv "github.com/birdtable/go-testkit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The crash journey's settings, read from the environment, which this
// package alone of the tests may read: the seed the points are drawn from
// (unset or 0 draws one, which every failure names), and how many points to
// kill at. The count is never below the short count make check runs on every
// push, so a setting can only make a run longer; make crash sets the release
// count. The seed is there to run a failure's points again.
const (
	seedVar     = configtest.CrashSeedVar
	pointsVar   = configtest.CrashPointsVar
	shortPoints = 20
	// maxFiles is the largest plan the points are drawn over: a plan of n
	// data files has 12+8n boundaries, so the plans of 1 to 12 files have
	// 768 between them.
	maxFiles = 12
	// killDeadline bounds a child started to be killed: testenv ends one
	// that outlives it, so no stopped child is left running.
	killDeadline = 2 * time.Minute
)

// The five stages of a run a boundary falls in, in run order.
func stages() []string {
	return []string{"before-intent", "after-intent", "between-files", "before-record", "after-record"}
}

// A point is one place to kill the child: a plan of files data files,
// stopped at boundary at, which falls in stage.
type point struct {
	files, at int
	stage     string
}

func (p point) String() string {
	return fmt.Sprintf("files %d, boundary %d (%s)", p.files, p.at, p.stage)
}

// TestACrashAtAnyBoundaryLeavesTheRootBeforeOrAfterThePlan is the crash
// injection (store.recovery.2, A37, architecture 5.2 and 13). The child runs
// one scripted plan through the write path over the real file system and
// stops at a numbered boundary between two effects; the test kills it with
// SIGKILL, starts it again to recover, and reads the root and the journal
// itself: every data file before the plan and no record of the yes, or every
// one after it with the record, and no run left open. The points are drawn
// from a seed, spread over the five stages; a failure names the seed and the
// point. Risk: a write path that leaves a root half written, or a record
// open, when the process dies between two of its effects.
func TestACrashAtAnyBoundaryLeavesTheRootBeforeOrAfterThePlan(t *testing.T) {
	t.Parallel()
	seed, count := crashSettings(t)
	points := drawPoints(seed, count, boundaryMap(t))
	t.Logf("crash: seed %d, %d points (%s=%d %s=%d go test ./internal/journeys -run '^%s$' runs them again)",
		seed, len(points), seedVar, seed, pointsVar, count, t.Name())
	if len(points) != count {
		t.Fatalf("seed %d: drew %d points, want %d", seed, len(points), count)
	}
	if count >= len(stages()) {
		for _, s := range stages() {
			if !slices.ContainsFunc(points, func(p point) bool { return p.stage == s }) {
				t.Errorf("seed %d: no point falls in %s", seed, s)
			}
		}
	}
	judged, proved := make([]bool, len(points)), make([]bool, len(points))
	t.Run("points", func(t *testing.T) {
		for i, p := range points {
			t.Run(fmt.Sprintf("killed %s at boundary %d of %d files, point %d", p.stage, p.at, p.files, i), func(t *testing.T) {
				t.Parallel()
				w := newCrashRoot(t, p.files)
				w.killAt(t, p, "")
				rep := w.recoverAfter(t)
				bad := w.check(t)
				judged[i], proved[i] = w.judged, w.judged && len(bad) == 0
				if len(bad) > 0 {
					t.Errorf("seed %d, point %d, %v: after the kill and the recovery (%+v):\n%s", seed, i, p, rep, strings.Join(bad, "\n"))
				}
			})
		}
	})
	// zero boundaries proved is a red, not a pass: a checker that found no
	// run-1 to judge proved nothing
	j, n := trues(judged), trues(proved)
	t.Logf("crash: seed %d, killed at %d boundaries, %d with run-1 to judge, %d proved", seed, len(points), j, n)
	if n == 0 {
		t.Errorf("seed %d: killed at %d boundaries and none left run-1 in the journal that the checker proved", seed, len(points))
	}
}

// TestTheCrashCheckerGoesRedOnAPlantedRecordFirstWrite: the checker is held
// to a planted bug, a write path that appends the record of the yes before it
// writes the data and outside the journal. At a point in each stage the
// planted run is red, and the same point without the plant is green. Risk: a
// checker that passes every root and so proves nothing.
func TestTheCrashCheckerGoesRedOnAPlantedRecordFirstWrite(t *testing.T) {
	t.Parallel()
	seed, _ := crashSettings(t)
	for _, p := range drawPoints(seed, len(stages()), boundaryMap(t)) {
		t.Run(p.stage, func(t *testing.T) {
			t.Parallel()
			planted := newCrashRoot(t, p.files)
			planted.killAt(t, p, "record-first")
			planted.recoverAfter(t)
			if bad := planted.check(t); len(bad) == 0 {
				t.Errorf("seed %d, %v: the planted record-first write passed the checker", seed, p)
			}
			clean := newCrashRoot(t, p.files)
			clean.killAt(t, p, "")
			clean.recoverAfter(t)
			if bad := clean.check(t); len(bad) > 0 {
				t.Errorf("seed %d, %v: the clean control failed the checker:\n%s", seed, p, strings.Join(bad, "\n"))
			}
		})
	}
}

// TestTheCrashCheckerGoesRedWhenItHasNoRunToJudge: the checker is held to a
// journal that lists no run, which is what a reader blind to the journal's
// form sees. A plan killed between its files and recovered is
// green; the same root with its journal emptied after the recovery is red,
// and red only for having no run-1 to judge. Risk: a checker that passes
// every root because it reads no run, so the kill proves nothing.
func TestTheCrashCheckerGoesRedWhenItHasNoRunToJudge(t *testing.T) {
	t.Parallel()
	m := boundaryMap(t)
	at := slices.Index(m[3], "between-files")
	if at < 0 {
		t.Fatal("a plan of 3 files has no boundary between its files")
	}
	w := newCrashRoot(t, 3)
	w.killAt(t, point{files: 3, at: at, stage: "between-files"}, "")
	w.recoverAfter(t)
	if bad := w.check(t); len(bad) > 0 || !w.judged {
		t.Fatalf("the control was judged %t and the checker said %q; want run-1 judged and green", w.judged, bad)
	}
	emptyJournals(t, w.journal)
	bad := w.check(t)
	if w.judged || len(bad) != 1 || !strings.Contains(bad[0], "no run to judge") {
		t.Errorf("over an emptied journal the checker was judged %t and said %q; want only that it has no run to judge", w.judged, bad)
	}
}

// emptyJournals empties every journal under dir, as a planted blind reader:
// the file stays, and lists no run.
func emptyJournals(t *testing.T, dir string) {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".journal") {
			return err
		}
		n++
		return os.WriteFile(p, nil, 0o600)
	})
	if err != nil || n == 0 {
		t.Fatalf("emptied %d journals under %s (%v); want at least one", n, dir, err)
	}
}

// TestARecoveryThatCannotPutAFileBackLeavesTheRunOpenAndTheCheckerRed: a
// kept copy changed after the kill is not the file its copy record names, so
// the recovery refuses to write it back and fails, the killed run stays open,
// and the checker says so by the run's id. Risk: a recovery that writes back
// bytes it cannot vouch for, or closes a run it did not put back.
func TestARecoveryThatCannotPutAFileBackLeavesTheRunOpenAndTheCheckerRed(t *testing.T) {
	t.Parallel()
	m := boundaryMap(t)
	at := slices.Index(m[3], "between-files")
	if at < 0 {
		t.Fatal("a plan of 3 files has no boundary between its files")
	}
	w := newCrashRoot(t, 3)
	w.killAt(t, point{files: 3, at: at, stage: "between-files"}, "")
	copies, err := filepath.Glob(filepath.Join(w.journal, "*.copies", "run-1", "*"))
	if err != nil || len(copies) == 0 {
		t.Fatalf("no kept copy of run-1 to change (%v)", err)
	}
	for _, c := range copies {
		if err := os.WriteFile(c, []byte("changed after the kill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := w.recoverRun(t)
	rep := reportOf(t, res)
	if res.code == 0 || rep.Word == "done" || rep.Failure == "" || !strings.Contains(res.stderr, "run-1") {
		t.Errorf("the recovery over a changed copy exited %d with %+v, stderr %q; want a failure naming run-1", res.code, rep, res.stderr)
	}
	bad := w.check(t)
	if !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, "run run-1 is open") }) {
		t.Errorf("the checker said %q; want the open run named", bad)
	}
}

// rollingBack is the stage of every boundary of a recovery's roll-back.
const rollingBack = "rolling-back"

// TestACrashInsideTheRollBackIsRolledBackAgain: a roll-back can be killed
// too, and the next recovery finishes it (A37). For points of the plan
// drawn from the seed, a first root is killed there and recovered to its
// end, to count the boundaries of the roll-back; a point that leaves no run
// open has none and is passed over, and at least one point must have them.
// A second root is killed at the same point, its recovery is stopped at a
// boundary of the roll-back drawn from the seed and killed with SIGKILL,
// and a third run recovers it. The checker holds the root as it holds
// every other. Planted failure: the close journalled before the files are
// put back. Risk: a roll-back that cannot itself be interrupted.
func TestACrashInsideTheRollBackIsRolledBackAgain(t *testing.T) {
	t.Parallel()
	seed, count := crashSettings(t)
	m := boundaryMap(t)
	for n, at := range m {
		// a kill before the intent leaves nothing to roll back
		m[n] = slices.Clone(at)
		for i, s := range m[n] {
			if s == "before-intent" {
				m[n][i] = ""
			}
		}
	}
	points := drawPoints(seed^0x5bd1e995, max(len(stages()), count/4), m)
	// A seeded draw, so a failure can name its points; not a secret.
	r := configtest.NewDraw(seed ^ 0xc2b2ae3d27d4eb4f)
	stops := make([]uint64, len(points))
	for i := range stops {
		stops[i] = r.Uint64()
	}
	t.Logf("crash in the roll-back: seed %d, %d points", seed, len(points))
	rolled := make([]bool, len(points))
	t.Run("points", func(t *testing.T) {
		for i, p := range points {
			t.Run(fmt.Sprintf("killed %s at boundary %d of %d files, then in the roll-back, point %d", p.stage, p.at, p.files, i), func(t *testing.T) {
				t.Parallel()
				first := newCrashRoot(t, p.files)
				first.killAt(t, p, "")
				rep := first.recoverAfter(t)
				if len(rep.Points) == 0 {
					return // nothing was open, so there is no roll-back to kill
				}
				rolled[i] = true
				at := int(stops[i] % uint64(len(rep.Points)))
				w := newCrashRoot(t, p.files)
				w.killAt(t, p, "")
				w.killRecoveryAt(t, at)
				again := w.recoverAfterAs(t, "run-3")
				if bad := w.check(t); len(bad) > 0 {
					t.Errorf("seed %d, point %d, %v, killed again at boundary %d of %d of the roll-back: after the second recovery (%+v):\n%s",
						seed, i, p, at, len(rep.Points), again, strings.Join(bad, "\n"))
				}
			})
		}
	})
	if !slices.Contains(rolled, true) {
		t.Errorf("seed %d: no point drawn left a run to roll back, so no roll-back was killed", seed)
	}
}

// crashSettings reads the seed and the count from the environment, drawing
// a seed when none is given.
func crashSettings(t *testing.T) (seed uint64, count int) {
	t.Helper()
	if v := os.Getenv(seedVar); v != "" {
		s, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			t.Fatalf("%s is %q: want a seed, a whole number", seedVar, v)
		}
		seed = s
	}
	count = shortPoints
	if v := os.Getenv(pointsVar); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("%s is %q: want a count of 1 or more", pointsVar, v)
		}
		count = max(n, shortPoints)
	}
	if seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatal(err)
		}
		seed = binary.LittleEndian.Uint64(b[:]) | 1
	}
	return seed, count
}

// boundaryMap runs the plan of each size once to its end and reads the
// stage of every boundary it numbered: map[files][boundary] is the stage.
// The runs go side by side, each in a root of its own.
func boundaryMap(t *testing.T) map[int][]string {
	t.Helper()
	got := make([][]string, maxFiles+1)
	t.Run("boundaries", func(t *testing.T) {
		for n := 1; n <= maxFiles; n++ {
			t.Run(fmt.Sprintf("a plan of %d files numbers its boundaries", n), func(t *testing.T) {
				t.Parallel()
				got[n] = fullRun(t, n)
			})
		}
	})
	out := map[int][]string{}
	for n := 1; n <= maxFiles; n++ {
		if len(got[n]) == 0 {
			t.Fatalf("the full run of %d files numbered no boundary", n)
		}
		out[n] = got[n]
	}
	return out
}

// fullRun runs the plan of n files to its end, holds the root it leaves to
// the checker, and answers the stage of each boundary.
func fullRun(t *testing.T, n int) []string {
	t.Helper()
	w := newCrashRoot(t, n)
	res := w.crash(t, "apply", "-run", "run-1", "-files", strconv.Itoa(n))
	if res.code != 0 {
		t.Fatalf("the full run of %d files exited %d: %s", n, res.code, res.stderr)
	}
	var out []string
	for i, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != "point" || f[1] != strconv.Itoa(i) || !slices.Contains(stages(), f[2]) {
			t.Fatalf("the full run of %d files said %q at line %d", n, line, i)
		}
		out = append(out, f[2])
	}
	if bad := w.check(t); len(bad) > 0 {
		t.Fatalf("the full run of %d files left:\n%s", n, strings.Join(bad, "\n"))
	}
	return out
}

// drawPoints draws count points from seed, taking each stage in turn so
// every stage is hit, and never one point twice. A stage whose points are
// all drawn is passed over.
func drawPoints(seed uint64, count int, m map[int][]string) []point {
	by := map[string][]point{}
	for n := 1; n <= maxFiles; n++ {
		for at, s := range m[n] {
			by[s] = append(by[s], point{files: n, at: at, stage: s})
		}
	}
	// A seeded draw, so a failure can name its points; not a secret.
	r := configtest.NewDraw(seed ^ 0x9e3779b97f4a7c15)
	for _, s := range stages() {
		r.Shuffle(len(by[s]), func(i, j int) { by[s][i], by[s][j] = by[s][j], by[s][i] })
	}
	var out []point
	for len(out) < count {
		took := false
		for _, s := range stages() {
			if len(out) == count || len(by[s]) == 0 {
				continue
			}
			out = append(out, by[s][0])
			by[s] = by[s][1:]
			took = true
		}
		if !took {
			break
		}
	}
	return out
}

// A crashRoot is one point's folders: the root the plan writes, and the
// journal folder its lock, journal and copies live in.
type crashRoot struct {
	root, journal string
	files         int
	// openAtKill is whether the journal held run-1 open once the child died.
	openAtKill bool
	// stage is the stage the plan was killed in, and judged whether the
	// last check found run-1 in the journal to judge.
	stage  string
	judged bool
}

// newCrashRoot is a root seeded for a plan of n files: each rule the plan
// replaces holds its before, and no file the plan makes is there.
func newCrashRoot(t *testing.T, n int) *crashRoot {
	t.Helper()
	base := t.TempDir()
	w := &crashRoot{root: filepath.Join(base, "root"), journal: filepath.Join(base, "journal"), files: n}
	for _, d := range []string{"rules", "state"} {
		if err := os.MkdirAll(filepath.Join(w.root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range crashData(n) {
		if strings.HasPrefix(p, "rules/") {
			if err := os.WriteFile(filepath.Join(w.root, p), []byte(crashBefore), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return w
}

// The child's script, as cmd/gragas-crash writes it: what a rule holds
// before the plan, what every data file holds after it, and the file the
// record of the yes is appended to.
const (
	crashBefore = "status: active\n"
	crashAfter  = "status: retired\n"
	crashRecord = "history.jsonl"
)

// crashData is the plan's data files, as cmd/gragas-crash's DataPaths
// names them: every third one, from the second, made under state/.
func crashData(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		if i%3 == 1 {
			out = append(out, fmt.Sprintf("state/new%02d.yaml", i))
			continue
		}
		out = append(out, fmt.Sprintf("rules/r%02d.yaml", i))
	}
	return out
}

// crash runs the child to its end over this root.
func (w *crashRoot) crash(t *testing.T, args ...string) result {
	t.Helper()
	c := w.command(t, args...)
	var out, errb strings.Builder
	c.Stdout, c.Stderr = &out, &errb
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("could not run gragas-crash: %v", err)
		}
		code = ee.ExitCode()
	}
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

func (w *crashRoot) command(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	bin := testenv.Build("cmd/gragas-crash").Path()
	if bin == "" {
		t.Fatalf("gragas-crash did not build:\n%s", configtest.GoSaid(testenv.Build("cmd/gragas-crash").Run().Stderr))
	}
	return commandOf(t, bin, w.root, nil, append([]string{args[0], "-root", w.root, "-state", w.journal}, args[1:]...)...)
}

// killAt starts the plan, waits for the child to say it stopped at p, and
// kills it with SIGKILL, as a crash would end it.
func (w *crashRoot) killAt(t *testing.T, p point, plant string) {
	t.Helper()
	args := []string{"apply", "-run", "run-1", "-files", strconv.Itoa(p.files), "-stop", strconv.Itoa(p.at)}
	if plant != "" {
		args = append(args, "-plant", plant)
	}
	w.stage = p.stage
	w.kill(t, p.String(), fmt.Sprintf("stopped %d %s\n", p.at, p.stage), args...)
}

// killRecoveryAt starts a recovery, run-2, waits for it to say it stopped
// at boundary at of its roll-back, and kills it with SIGKILL.
func (w *crashRoot) killRecoveryAt(t *testing.T, at int) {
	t.Helper()
	w.kill(t, fmt.Sprintf("the recovery at boundary %d", at), fmt.Sprintf("stopped %d %s\n", at, rollingBack),
		"recover", "-run", "run-2", "-stop", strconv.Itoa(at))
}

// kill starts the child with args, waits for it to say want, and kills it
// with SIGKILL; what names the stop in a failure.
func (w *crashRoot) kill(t *testing.T, what, want string, args ...string) {
	t.Helper()
	// The child's flags name its root and its journal, so it needs no folder or
	// variable of its own and testenv starts it in the isolated environment.
	ctx, cancel := context.WithTimeout(t.Context(), killDeadline)
	defer cancel()
	run, err := testenv.Build("cmd/gragas-crash").Start(ctx, append([]string{args[0], "-root", w.root, "-state", w.journal}, args[1:]...)...)
	if err != nil {
		t.Fatalf("%s: gragas-crash did not start: %v", what, err)
	}
	line, err := bufio.NewReader(run.Stdout()).ReadString('\n')
	if err != nil || line != want {
		_ = run.Kill()
		res := run.Wait()
		t.Fatalf("%s: the child said %q (%v), stderr %q; want %q", what, line, err, res.Stderr, want)
	}
	if err := run.Kill(); err != nil {
		t.Fatal(err)
	}
	if res := run.Wait(); res.Code != -1 {
		t.Fatalf("%s: the child was not ended by the kill: code %d, stderr %q", what, res.Code, res.Stderr)
	}
	w.openAtKill = slices.Contains(w.openRuns(t), "run-1")
}

// A recoverReport is what the child's recover says, a line each: its word,
// each run it settled, the kind of a failure, and the stage of each
// boundary of its roll-back.
type recoverReport struct {
	Word, Failure string
	Recovered     []string
	Points        []string
}

// reportOf reads the recover child's lines.
func reportOf(t *testing.T, res result) recoverReport {
	t.Helper()
	rep := recoverReport{Recovered: []string{}}
	for line := range strings.SplitSeq(strings.TrimSuffix(res.stdout, "\n"), "\n") {
		key, value, ok := strings.Cut(line, " ")
		switch {
		case !ok:
			t.Fatalf("the recovery exited %d and said %q, not its report (stderr %q)", res.code, res.stdout, res.stderr)
		case key == "word":
			rep.Word = value
		case key == "recovered":
			rep.Recovered = append(rep.Recovered, value)
		case key == "failure":
			rep.Failure = value
		case key == "point":
			at, stage, _ := strings.Cut(value, " ")
			if at != strconv.Itoa(len(rep.Points)) || stage != rollingBack {
				t.Fatalf("the recovery said %q, not the next boundary of its roll-back", line)
			}
			rep.Points = append(rep.Points, stage)
		default:
			t.Fatalf("the recovery said %q, a line its report has no place for", line)
		}
	}
	return rep
}

func (w *crashRoot) recoverRun(t *testing.T) result {
	t.Helper()
	return w.recoverRunAs(t, "run-2")
}

func (w *crashRoot) recoverRunAs(t *testing.T, run string) result {
	t.Helper()
	return w.crash(t, "recover", "-run", run)
}

// recoverAfter starts the child again to recover, as run-2, and holds it to
// its word: done, having settled run-1 exactly when the kill left it open.
func (w *crashRoot) recoverAfter(t *testing.T) recoverReport {
	t.Helper()
	return w.recoverAfterAs(t, "run-2")
}

// recoverAfterAs is recoverAfter as the run id run.
func (w *crashRoot) recoverAfterAs(t *testing.T, run string) recoverReport {
	t.Helper()
	res := w.recoverRunAs(t, run)
	rep := reportOf(t, res)
	want := []string{}
	if w.openAtKill {
		want = []string{"run-1"}
	}
	if res.code != 0 || rep.Word != "done" || !slices.Equal(rep.Recovered, want) {
		t.Errorf("the recovery exited %d with %+v; want done, recovering %q", res.code, rep, want)
	}
	return rep
}

// listed is each run of the journal and its state, open or the outcome its
// close records, as the Journal port answers them through the child's
// records mode, the listing journal list prints. The checker never reads
// the fields of a line, so a form of the line that moves cannot blind it,
// and a journal the port refuses fails the test.
func (w *crashRoot) listed(t *testing.T) map[string]string {
	t.Helper()
	res := w.crash(t, "records")
	if res.code != 0 {
		t.Fatalf("the journal could not be read through the port: exit %d\n%s", res.code, res.stderr)
	}
	out := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSuffix(res.stdout, "\n"), "\n") {
		if line == "" {
			continue
		}
		rest, ok := strings.CutPrefix(line, "run ")
		run, state, cut := strings.Cut(rest, " ")
		if !ok || !cut || run == "" || state == "" {
			t.Fatalf("the child's records said %q, not a run and its state", line)
		}
		out[run] = state
	}
	return out
}

// openRuns is the runs the journal holds with no close.
func (w *crashRoot) openRuns(t *testing.T) []string {
	t.Helper()
	var out []string
	for run, state := range w.listed(t) {
		if state == "open" {
			out = append(out, run)
		}
	}
	slices.Sort(out)
	return out
}

// check reads the root from its bytes and the journal through the port, and
// says what is wrong: the data files all before the plan and no record of
// the yes, or all after it and the one record, no other file in the root, no
// run open, and run-1 closed done exactly when the root is after. A plan
// killed past its intent journalled run-1, so a journal that lists no run-1
// then is a checker with no run to judge, and red: a check that proves
// nothing never passes.
func (w *crashRoot) check(t *testing.T) []string {
	t.Helper()
	var bad []string
	before, after := 0, 0
	for _, p := range crashData(w.files) {
		data, err := os.ReadFile(filepath.Join(w.root, p))
		switch {
		case errors.Is(err, fs.ErrNotExist) && strings.HasPrefix(p, "state/"):
			before++
		case err != nil:
			bad = append(bad, fmt.Sprintf("%s: %v", p, err))
		case string(data) == crashAfter:
			after++
		case string(data) == crashBefore && strings.HasPrefix(p, "rules/"):
			before++
		default:
			bad = append(bad, fmt.Sprintf("%s holds %q, neither before nor after the plan", p, data))
		}
	}
	if before > 0 && after > 0 {
		bad = append(bad, fmt.Sprintf("%d data files are before the plan and %d after it", before, after))
	}
	applied := after == w.files
	record, err := os.ReadFile(filepath.Join(w.root, crashRecord))
	lines := strings.Split(strings.TrimSuffix(string(record), "\n"), "\n")
	switch {
	case errors.Is(err, fs.ErrNotExist) || len(record) == 0:
		if applied {
			bad = append(bad, "the data is after the plan and there is no record of the yes")
		}
	case err != nil:
		bad = append(bad, fmt.Sprintf("%s: %v", crashRecord, err))
	case !applied:
		bad = append(bad, fmt.Sprintf("the data is before the plan and %s holds %q", crashRecord, record))
	case len(lines) != 1 || !strings.HasPrefix(lines[0], `{"plan":"sha256:`):
		bad = append(bad, fmt.Sprintf("the data is after the plan and %s holds %q, not one record of the plan", crashRecord, record))
	}
	bad = append(bad, w.strays(t)...)
	runs := w.listed(t)
	for _, run := range slices.Sorted(maps.Keys(runs)) {
		if runs[run] == "open" {
			bad = append(bad, fmt.Sprintf("run %s is open: the journal holds no close of it", run))
		}
	}
	got, listed := runs["run-1"]
	w.judged = listed
	if !listed && w.stage != "" && w.stage != "before-intent" {
		bad = append(bad, fmt.Sprintf("killed %s, the journal lists no run-1: the checker has no run to judge", w.stage))
	}
	if got == "open" {
		got = ""
	}
	switch {
	case applied && got != "done":
		bad = append(bad, fmt.Sprintf("the root is after the plan and run-1 closed %q, not done", got))
	case !applied && got == "done":
		bad = append(bad, "the root is before the plan and run-1 closed done")
	}
	return bad
}

// strays is every file in the root the plan does not name.
func (w *crashRoot) strays(t *testing.T) []string {
	t.Helper()
	known := append(crashData(w.files), crashRecord)
	var bad []string
	err := filepath.WalkDir(w.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(w.root, p)
		if err != nil {
			return err
		}
		if !slices.Contains(known, filepath.ToSlash(rel)) {
			bad = append(bad, fmt.Sprintf("%s is in the root and the plan names no such file", rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bad
}
