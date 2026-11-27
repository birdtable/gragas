package git

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/gitenv"
	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/model"
)

// program writes a stand-in for git that answers by rules in a folder of
// its own, and answers an adapter at it whose runs have run id "run-7".
func program(t *testing.T, rules ...Rule) *Git {
	t.Helper()
	dir := filepath.Join(filestest.Private(t), "bin")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := StandIn(t, dir, rules...)
	g, err := New(Config{Git: path, Home: filestest.Private(t), Run: "run-7"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestAHungGitEndsAtTheDeadlineFailedWithTheRunID: a git that never ends is
// ended at the deadline, and Who fails with the word failed, the kind git
// and the run id. Planted failure: the run's context without its timeout.
// The git writes a mark, then hangs. A deadline that came before the mark
// says nothing of what the test asks, only that the machine was slow to
// start the program, so it is tried again with twice the wait, and the
// assertions are made on the first run that reached its mark: no run of a
// fixed length is held to the speed of the machine.
func TestAHungGitEndsAtTheDeadlineFailedWithTheRunID(t *testing.T) {
	t.Parallel()
	var g *Git
	var err error
	for wait := 200 * time.Millisecond; ; wait *= 2 {
		mark := filepath.Join(filestest.Private(t), "mark")
		g = program(t, Always(Answer{Mark: mark, Hang: true}))
		g.wait = wait
		_, err = g.Who(t.Context(), model.Root(filestest.Private(t)))
		if _, statErr := os.Stat(mark); statErr == nil {
			break
		}
		if wait > 30*time.Second {
			t.Fatalf("the git never reached its mark, even given %s: %v", wait, err)
		}
	}
	var f *Failed
	if !errors.As(err, &f) || f.Word() != model.Failed || f.Kind() != "git" || f.Run() != "run-7" || !errors.Is(err, errDeadline) {
		t.Fatalf("a hung git: %v, want failed, kind git, run run-7, at the deadline", err)
	}
	if !strings.Contains(err.Error(), "run-7") {
		t.Errorf("the failure %q does not name the run id", err)
	}
	if ran := g.Ran(); len(ran) != 1 || ran[0].Code != -1 {
		t.Errorf("the hung run was recorded as %s, want one run with no exit code", Shown(ran))
	}
}

// TestAGitThatPrintsPastTheCapFails: output past the cap is a failure,
// never cut and read as an identity. Planted failure: the cap's flag not
// read.
func TestAGitThatPrintsPastTheCapFails(t *testing.T) {
	t.Parallel()
	g := program(t, Always(Answer{Out: "user.name\nAda\x00user.email\na@example.com\x00", Zeros: 70000}))
	_, err := g.Who(t.Context(), model.Root(filestest.Private(t)))
	var f *Failed
	if !errors.As(err, &f) || !errors.Is(err, errPastCap) || f.Run() != "run-7" {
		t.Errorf("a git past the cap: %v, want failed past the cap", err)
	}
	g = program(t, Always(Answer{ErrZeros: 5000, Code: 1}))
	if _, err := g.Who(t.Context(), model.Root(filestest.Private(t))); !errors.Is(err, errPastCap) {
		t.Errorf("a git whose stderr is past its cap: %v, want failed past the cap", err)
	}
}

// TestAGitThatFailsIsFailedNotNotReady: an exit other than "no key matched"
// (1 with nothing said) is failed with the run id, never read as no
// identity. Planted failure: every exit 1 read as no identity.
func TestAGitThatFailsIsFailedNotNotReady(t *testing.T) {
	t.Parallel()
	cases := map[string]Answer{
		"exit 128":                {Err: "fatal: not a git repository\n", Code: 128},
		"exit 1 with a word":      {Err: "error: bad config line 1\n", Code: 1},
		"output git never prints": {Out: "user.name\nAda\x00core.editor\nvi\x00"},
	}
	for name, a := range cases {
		_, err := program(t, Always(a)).Who(t.Context(), model.Root(filestest.Private(t)))
		var f *Failed
		if !errors.As(err, &f) || f.Run() != "run-7" {
			t.Errorf("%s: %v, want failed with the run id", name, err)
		}
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
}

// TestGitsOwnWordsAreQuotedInTheFailure: what git prints on stderr reaches
// the failure's text escaped, so a log or JSON never carries its control
// sequences raw. Planted failure: stderr written with %s.
func TestGitsOwnWordsAreQuotedInTheFailure(t *testing.T) {
	t.Parallel()
	_, err := program(t, Always(Answer{Err: "fatal: \x1b[31mred\x1b[0m\n", Code: 128})).Who(t.Context(), model.Root(filestest.Private(t)))
	if _, ok := errors.AsType[*Failed](err); !ok {
		t.Fatalf("%v, want failed", err)
	}
	if s := err.Error(); strings.ContainsRune(s, '\x1b') || !strings.Contains(s, `"fatal: \x1b[31mred\x1b[0m"`) {
		t.Errorf("the failure carries git's words as %q, want them quoted", s)
	}
}

// TestAVerbOutsideTheTableIsRefused (cli.process.1): a run of a verb the
// table does not hold starts nothing. Planted failure: an unknown verb run
// as given.
func TestAVerbOutsideTheTableIsRefused(t *testing.T) {
	t.Parallel()
	g := program(t, Always(Answer{}))
	for _, verb := range []string{"push", "config", "", "config-identity "} {
		if _, err := g.run(t.Context(), verb, filestest.Private(t)); !errors.Is(err, errUnknownVerb) {
			t.Errorf("the verb %q: %v, want refused as not in the table", verb, err)
		}
	}
	if len(g.Ran()) != 0 {
		t.Errorf("a verb outside the table ran: %s", Shown(g.Ran()))
	}
	names := slices.Sorted(maps.Keys(verbs()))
	want := []string{
		"blob", "blob-objects", "commit-objects", "commit-tree", "commits-between", "config-filemode", "config-identity", "config-promisor", "fetch-tag", "fetch-tag-https", "fetch-tag-https-helper", "fetch-tag-ssh", "hash-blob", "index-files", "index-info", "index-info-real",
		"init-cache", "is-shallow", "merge-base", "merge-head-path", "range-commits", "read-tree", "resolve-commit", "show-prefix", "tag-annotated", "tags", "tree-files", "update-ref", "version", "work-objects", "work-top", "write-tree",
	}
	if !slices.Equal(names, want) {
		t.Errorf("the table holds %q, want %q", names, want)
	}
}

// TestTheChildsEnvironmentIsBare: git is handed exactly gitenv.Home, the four
// fixed variables and the system's part (proc.SystemEnv), the graft file the
// null device, whatever this process holds: a GIT_DIR planted in any letter
// case, an index, a work tree or a config override never reaches it,
// and the ssh git starts finds the system's own files.
// Planted failures: PATH passed on; this process's environment
// handed to gitenv.Env in place of the system's part; nil in its place (on
// Windows, ProgramData is missing).
func TestTheChildsEnvironmentIsBare(t *testing.T) {
	for _, name := range []string{"GIT_DIR", "Git_Dir", "git_dir", "GIT_INDEX_FILE", "git_work_tree", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"} {
		t.Setenv(name, "/elsewhere/.git")
	}
	envTo := filepath.Join(filestest.Private(t), "env")
	g := program(t, Always(Answer{EnvTo: envTo, Code: 2}))
	_, err := g.Who(t.Context(), model.Root(filestest.Private(t)))
	if _, ok := errors.AsType[*exited](err); !ok {
		t.Fatalf("the stand-in: %v", err)
	}
	got := readEnv(t, envTo)
	// what the system's start adds to a child handed no environment at all,
	// read from the same stand-in run by proc with none, is in every child's,
	// but for a name the system's part already gives, which the start then
	// leaves as given, in any letter case
	system := proc.SystemEnv()
	want := slices.DeleteFunc(addedByTheStart(t, g.cfg.Git, envTo), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(system, func(s string) bool {
			n, _, _ := strings.Cut(s, "=")
			return strings.EqualFold(n, name)
		})
	})
	home, err := gitenv.Home(g.cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	want = append(append(append(want, system...), "GIT_CONFIG_NOSYSTEM=1", "GIT_GRAFT_FILE="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"), home...)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the child's environment differs from the bare one: %s", envDiff(got, want))
	}
}

// addedByTheStart is the environment the stand-in at path, which writes
// its environment to envTo, is handed when proc runs it with none: what the
// system's start adds to every child.
func addedByTheStart(t *testing.T, path, envTo string) []string {
	t.Helper()
	res, err := proc.Run(t.Context(), proc.Cmd{Path: path, Env: []string{}, Timeout: time.Minute, MaxOutput: 1 << 20, Drain: 10 * time.Second})
	if err != nil || res.Code != 2 {
		t.Fatalf("the stand-in with no environment: code %d, %v", res.Code, err)
	}
	return readEnv(t, envTo)
}

// readEnv is the environment a stand-in wrote to path, an entry to each
// \001; none when the file is empty.
func readEnv(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the stand-in wrote no environment: %v", err)
	}
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x01"), "\x01")
}

// envName is the variable an environment entry sets: what comes before its
// first =, past a leading = that Windows keeps on its drive entries.
func envName(kv string) string {
	start := 0
	if strings.HasPrefix(kv, "=") {
		start = 1
	}
	if i := strings.IndexByte(kv[start:], '='); i >= 0 {
		return kv[:start+i]
	}
	return kv
}

// envNames is the variables env sets, in its order, and no value.
func envNames(env []string) []string {
	out := make([]string, len(env))
	for i, kv := range env {
		out[i] = envName(kv)
	}
	return out
}

// envDiff says how got differs from want by variable name alone, so no
// failure prints a value the environment holds: the names wanted and not
// there, the names there and not wanted, and the names whose values differ.
func envDiff(got, want []string) string {
	values := func(env []string) map[string][]string {
		m := map[string][]string{}
		for _, kv := range env {
			m[envName(kv)] = append(m[envName(kv)], kv)
		}
		return m
	}
	g, w := values(got), values(want)
	var missing, extra, changed []string
	for name, kvs := range w {
		switch v, ok := g[name]; {
		case !ok:
			missing = append(missing, name)
		case !slices.Equal(v, kvs):
			changed = append(changed, name)
		}
	}
	for name := range g {
		if _, ok := w[name]; !ok {
			extra = append(extra, name)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	slices.Sort(changed)
	return fmt.Sprintf("missing %q, extra %q, changed %q (%d entries, want %d)", missing, extra, changed, len(got), len(want))
}

// TestNewNeedsTheRunID: an adapter with no run id is refused, since its
// failures must name one.
func TestNewNeedsTheRunID(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Git: "/work/scratch/git"}); !errors.Is(err, model.ErrNoRun) {
		t.Errorf("New with no run id: %v, want model.ErrNoRun", err)
	}
}

// wantGlobal is the GIT_CONFIG_GLOBAL entry go-proc's NoGlobalConfig gives, taken from
// its result and not spelled here, so a test says what the platform call says.
func wantGlobal(t *testing.T) string {
	t.Helper()
	kv := gitenv.NoGlobalConfig()
	if len(kv) != 1 || !strings.HasPrefix(kv[0], "GIT_CONFIG_GLOBAL=") {
		t.Fatalf("NoGlobalConfig gives %q, want GIT_CONFIG_GLOBAL alone", kv)
	}
	return kv[0]
}
