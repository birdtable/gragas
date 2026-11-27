package tool_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-files/trust/trusttest"
	"github.com/birdtable/go-proc/proc"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/port"
)

// TestAStepsVersionRunStartsInAnEmptyFolderOfItsOwn (replaces
// TestEveryVersionRunStartsInAnEmptyFolderOfItsOwn): the
// run Bind makes to ask a tool its version, pinned or at the grant's own
// place, starts in an empty folder made for it, never the root and never
// below it, and the folder is gone after. The program refuses to answer its
// version in a folder that holds anything, so a run in the root fails the
// bind.
// Planted: versionBy making the run in the root (dir set to the root and
// versionFolder's refusal skipped; both rows red).
func TestAStepsVersionRunStartsInAnEmptyFolderOfItsOwn(t *testing.T) {
	t.Parallel()
	root := private(t)
	outside := private(t)
	body := fakeSteps(configtest.Case("--version", configtest.EmptyCwd(), configtest.Out("lint-a 1.2.3\n")))
	program(t, root+"/"+binDir+"/lint-a", body)
	set := program(t, outside+"/lint-a", body)
	for name, own := range map[string]bool{"pinned": false, "the grant's own place": true} {
		tl := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": set}}, nil)
		if own {
			tl = stepOver(nil, nil)
		}
		s := stepScope(t, root, own)
		if own {
			s.Manifest = atGrant()
			s.Manifest.Tool = "lint-a"
		}
		if _, err := tl.Bind(t.Context(), s); err != nil {
			t.Errorf("%s: Bind = %v; want the version asked in an empty folder", name, err)
			continue
		}
		ran := tl.Ran()
		if len(ran) != 1 || ran[0].Shape != "version" || ran[0].Dir == root || strings.HasPrefix(ran[0].Dir, root+"/") || ran[0].Dir == "" {
			t.Errorf("%s: runs = %+v; want one version run outside the root", name, ran)
			continue
		}
		if _, err := os.Lstat(ran[0].Dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: the version run's folder %s is still there: %v", name, ran[0].Dir, err)
		}
	}
}

// TestAStepsVersionRunIsNeverMadeInTheProject (replaces
// TestAVersionRunIsNeverMadeInTheProject): the folder Bind asks a
// tool its version in is made below TMPDIR. With TMPDIR at or below the root,
// by its spelling or through a link, or relative, or below a folder its group
// can write, Bind is refused by name before anything starts, and the marking
// tool is never run. The control: a TMPDIR outside the root, and the tool is
// bound. It sets TMPDIR and the folder it runs in, so it does not run beside
// the package's other tests and cannot be parallel.
// Planted: versionFolder answering nil (every refusal row red).
func TestAStepsVersionRunIsNeverMadeInTheProject(t *testing.T) {
	root := private(t)
	outside := private(t)
	mark := root + "/marked"
	program(t, root+"/"+binDir+"/lint-a", fakeSteps(configtest.Step{Write: mark, Text: "x"}, configtest.Out("lint-a 1.2.3\n")))
	for _, d := range []string{root + "/scratch", outside + "/scratch", outside + "/open", outside + "/open/tmp"} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	trusttest.Loosen(t, outside+"/open", trusttest.GroupWrites)
	testenv.Link(t, root, outside+"/to-root")
	// The tool and its scope are made before TMPDIR moves: the PATH folder and the
	// scratch folder are of the test's own, not of the case.
	tl := stepOver(nil, nil)
	s := stepScope(t, root, true)
	s.Manifest = atGrant()
	s.Manifest.Tool = "lint-a"
	t.Chdir(outside)
	// The temp folder is set as go-proc says a run's home folders are, whole:
	// every variable a system reads for it names a folder below the base, and
	// os.TempDir is the folder the product sees, made here where the case needs one.
	setTemp := func(base string) {
		for _, kv := range proc.HomeEnv(base) {
			name, value, _ := strings.Cut(kv, "=")
			t.Setenv(name, value)
		}
		if err := os.MkdirAll(os.TempDir(), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ claim, base, want string }{
		{"a TMPDIR below the root", root, "lies at or below the root"},
		{"a TMPDIR linked below the root", outside + "/to-root", "lies at or below the root"},
		{"a relative TMPDIR", "rel", "is not an absolute path"},
		// The walk that compares it with the root stops at a
		// folder its group can write, so it cannot say the TMPDIR is outside.
		{"a TMPDIR below a folder its group can write", outside + "/open", "cannot be compared with the root " + hosttrust.Quote(root) + ", as a folder on its way is not trusted: " + filepath.FromSlash(outside+"/open")},
	} {
		setTemp(c.base)
		if c.base == "rel" && filepath.IsAbs(os.TempDir()) {
			// A system that makes a relative TMP whole before a program reads it
			// never shows the product a relative one, so the refusal cannot arise
			// there: the bind is the control's.
			if r, err := tl.Bind(t.Context(), s); err != nil || r.Tool().Version != "1.2.3" {
				t.Errorf("%s, made whole by the system as %q: %v; want it bound", c.claim, os.TempDir(), err)
			}
			if err := os.Remove(mark); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			continue
		}
		_, err := tl.Bind(t.Context(), s)
		if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "a version run is made in") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v; want Bind refused with %q", c.claim, err, c.want)
		}
		if _, err := os.Stat(mark); err == nil {
			t.Errorf("%s: the tool was started", c.claim)
			if err := os.Remove(mark); err != nil {
				t.Fatal(err)
			}
		}
	}
	setTemp(outside)
	if r, err := tl.Bind(t.Context(), s); err != nil || r.Tool().Version != "1.2.3" {
		t.Errorf("the control, a TMPDIR outside the root: %v; want it bound", err)
	}
}

// TestAStepsTwoRunsAtOnceDoNotShareATempFolder (replaces
// TestTwoRunsAtOnceDoNotShareATempFolder): golangci-lint keeps one
// lock in the temp folder for the whole machine and exits 3 when a second run
// finds it held. The plant is a tool that does the same under its own lock
// name: two steps started together, each bound by a Tool with no Home of its
// own, must both exit 0, which they do only while each step's temp folder is
// the Home made for it. Without a Home in the environment both take the one
// folder of the machine and one exits 3.
// Planted: bound.run making no Home for the step (the Home condition forced
// false: a step exits 3, or is not started).
func TestAStepsTwoRunsAtOnceDoNotShareATempFolder(t *testing.T) {
	t.Parallel()
	root := private(t)
	lock := "gragas-956-" + filepath.Base(root)
	prog := program(t, private(t)+"/lint-a", answering(configtest.LockUnder(homeName(), lock, 1000)))
	set := map[string]map[string]string{root: {"lint-a@1.2.3": prog}}
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		r, err := stepOver(set, nil).Bind(t.Context(), stepScope(t, root, false))
		if err != nil {
			t.Fatal(err)
		}
		wg.Go(func() {
			res, err := stepRunOf(t, r)
			if err != nil || res.Exit == nil {
				t.Log(err)
				codes[i] = -1
				return
			}
			codes[i] = *res.Exit
		})
	}
	wg.Wait()
	for i, code := range codes {
		if code != 0 {
			t.Errorf("step %d exited %d; want 0, each with a temp folder of its own", i, code)
		}
	}
}

// TestAStepsVersionIsTheFirstLineOfARelease (replaces
// TestAToolThatPrintsNoVersionOrFailsToIsRefused): Bind refuses a
// tool that prints no version on its first line, one whose version ask exits
// non-zero, one that prints a fourth part, a prerelease, a local build, a
// letter suffix or a dirty build tree after the patch, as a trust failure and
// starts no later place; the refusal says the version holds more after the
// patch. The control: 1.2.3 with a word after it. A version on the second line
// alone is no version of the first.
// Planted: versionBy reading all the output and not the first line (the
// second-line row red); the readVersion case for more after the patch
// dropped (every prerelease row red).
func TestAStepsVersionIsTheFirstLineOfARelease(t *testing.T) {
	t.Parallel()
	more := "holds more after the patch"
	for name, c := range map[string]struct{ body, says string }{
		"no version":                {printing("lint-a, the latest\n", 0), "prints no version"},
		"exits 1":                   {printing("lint-a 1.2.3\n", 1), "exits 1 when asked its version"},
		"a 4th part":                {printing("lint-a 1.2.3.4\n", 0), more},
		"a prerelease":              {printing("lint-a 1.2.3-rc.1\n", 0), more},
		"a local build":             {printing("lint-a v1.2.3+local\n", 0), more},
		"a letter suffix":           {printing("lint-a 1.2.3a1\n", 0), more},
		"a dirty build tree":        {printing("lint-a 1.2.3-dirty\n", 0), more},
		"a version on a later line": {printing("lint-a, the latest\nlint-a 1.2.3\n", 0), "prints no version"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := private(t)
			prog := program(t, private(t)+"/lint-a", c.body)
			nextAt := private(t) + "/lint-a"
			next := program(t, nextAt, marking(nextAt))
			tl := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": prog}}, map[string]string{"lint-a@1.2.3": next})
			_, err := tl.Bind(t.Context(), stepScope(t, root, false))
			if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), c.says) {
				t.Errorf("Bind = %v, want refused, saying %q", err, c.says)
			}
			if len(tl.Ran()) != 1 {
				t.Errorf("runs = %+v; want the one version run of the refused place and no next place's", tl.Ran())
			}
		})
	}
	t.Run("the control, 1.2.3 with a word after it", func(t *testing.T) {
		t.Parallel()
		root := private(t)
		prog := program(t, private(t)+"/lint-a", printing("lint-a 1.2.3 (built 2026)\n", 0))
		r, err := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": prog}}, nil).Bind(t.Context(), stepScope(t, root, false))
		if err != nil || r.Tool().Version != "1.2.3" {
			t.Errorf("Bind = %v; want 1.2.3 found", err)
		}
	})
}

// TestAStepsVersionPrereleaseInOnePlaceRefusesTheWholeFind (replaces
// checkAPrereleaseInOnePlaceRefusesTheWholeFind):
// a tool that prints 2.0.0-rc.1 at the settings' place, with the tools file
// holding a sound program that prints the release, refuses the bind and asks
// the tools file's program nothing; a prerelease is never passed over for the
// next place unseen.
// Planted: reach or locateGrant going on to the next place when versionBy
// refuses (the tools file's program is asked and bound; the row red).
func TestAStepsVersionPrereleaseInOnePlaceRefusesTheWholeFind(t *testing.T) {
	t.Parallel()
	root := private(t)
	rc := program(t, private(t)+"/lint-a", says("1.2.3-rc.1"))
	mineAt := private(t) + "/lint-a"
	mine := program(t, mineAt, marking(mineAt))
	tl := stepOver(map[string]map[string]string{root: {"lint-a@1.2.3": rc}}, map[string]string{"lint-a@1.2.3": mine})
	_, err := tl.Bind(t.Context(), stepScope(t, root, false))
	if !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "holds more after the patch") {
		t.Errorf("a prerelease in one place: %v, want refused", err)
	}
	if asked(mineAt) || len(tl.Ran()) != 1 {
		t.Errorf("the next place was asked its version: %+v", tl.Ran())
	}
}

// TestAStepsVersionRunIsHeldToTheDeadlineTheConfigGives (replaces
// TestTheVersionRunIsHeldToTheDeadlineTheConfigGives): the
// run that asks a tool its version ends at Config.VersionWait. A tool that is
// slow to print its version is bound when the caller allows the wait, and
// refused, as a run past its deadline, when it does not.
// Planted: the wait ignored, so the deadline is ten seconds where 200ms is
// set and the slow tool is bound (the first half red).
func TestAStepsVersionRunIsHeldToTheDeadlineTheConfigGives(t *testing.T) {
	t.Parallel()
	root := private(t)
	prog := program(t, private(t)+"/lint-a", fakeSteps(configtest.Case("--version", configtest.Step{SleepMS: 3000}, configtest.Out("lint-a 1.2.3\n"))))
	set := map[string]map[string]string{root: {"lint-a@1.2.3": prog}}
	waits := func(wait time.Duration) *tool.Tool {
		return tool.New(tool.Config{Paths: set, Path: pathFolder(), Lang: "C", VersionWait: wait})
	}
	_, err := waits(200*time.Millisecond).Bind(t.Context(), stepScope(t, root, false))
	if err == nil || !strings.Contains(err.Error(), "ran past its deadline of 200ms") {
		t.Errorf("a 3s version run under a wait of 200ms: %v; want it ended at 200ms", err)
	}
	if _, err := waits(time.Minute).Bind(t.Context(), stepScope(t, root, false)); err != nil {
		t.Errorf("the same run under a wait of a minute: %v", err)
	}
}
