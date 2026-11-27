package sandbox_test

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// interrupted is the first argument Rerun gives the child that prepares a
// folder and is interrupted holding it; the next is the cache folder.
const interrupted = "prepare-and-be-interrupted"

// TestTheHelperRunsOnlyWhenRerun is the helper process. It prepares a folder
// with Release deferred and says where it is; then it cancels on the interrupt
// as gragas's main does and tells its parent it can take it, both in
// configtest.Interrupted, which installs the handler before it writes the
// ready line; and it returns once the cancel reaches it, so the deferred
// Release runs on that path. The parent interrupts only after the ready line,
// so the folder is held when the interrupt comes. Where the platform cannot
// serve the sandbox (notServed) it says "refused:" with the answer instead of
// a folder, and is interrupted all the same; any other refusal fails it.
func TestTheHelperRunsOnlyWhenRerun(t *testing.T) {
	t.Parallel() // in a rerun child this is the only test that runs
	args, ok := configtest.Helper()
	if !ok {
		return
	}
	if len(args) != 2 || args[0] != interrupted {
		t.Fatalf("the helper was given %q", args)
	}
	s := sandbox.New(sandbox.Config{Dir: args[1]})
	sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a/b.go", Data: []byte("x")}}})
	switch {
	case notServed(err):
		t.Logf("refused: %v", err)
	case err != nil:
		t.Fatal(err)
	default:
		defer func() {
			if err := s.Release(sc); err != nil {
				t.Error(err)
			}
		}()
		t.Logf("prepared: %s", sc.Dir)
	}
	ctx, stop, err := configtest.Interrupted(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	<-ctx.Done()
	t.Log("interrupted")
}

// notServed is whether err says the platform cannot serve the sandbox:
// port.ErrUnavailable. Any other error, a refusal that is a fault, is not;
// a base on Windows is judged, so its refusal is a fault
// too.
func notServed(err error) bool { return errors.Is(err, port.ErrUnavailable) }

// judge is what an interrupted helper's output and the cache folder dir say
// is wrong, given whether this system's sandbox can be served at all
// (served): served, the helper prepared a folder, saw the interrupt and
// removed it, and the scratch folder is empty; not served, it said it was
// refused and saw the interrupt, and prepared nothing.
func judge(out string, served bool, dir string) []string {
	var bad []string
	prepared, refused := "", false
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if _, after, ok := strings.Cut(sc.Text(), "prepared: "); ok {
			prepared = after
		}
		if strings.Contains(sc.Text(), "refused: ") {
			refused = true
		}
	}
	if !strings.Contains(out, "interrupted") {
		bad = append(bad, "the helper did not see the interrupt")
	}
	switch {
	case served && prepared == "":
		bad = append(bad, "the helper prepared no folder where the sandbox can be served")
	case !served && !refused:
		bad = append(bad, "the helper did not say it was refused where the sandbox cannot be served")
	case !served && prepared != "":
		bad = append(bad, "the helper prepared "+prepared+" where the sandbox cannot be served")
	}
	if prepared != "" {
		if _, err := os.Lstat(filepath.FromSlash(prepared)); !os.IsNotExist(err) {
			bad = append(bad, prepared+" is there after the interrupt")
		}
	}
	if entries, err := os.ReadDir(filepath.Join(dir, sandbox.Folder)); err == nil && len(entries) != 0 {
		bad = append(bad, "the cache's scratch folder holds "+strconv.Itoa(len(entries))+" entries")
	}
	return bad
}

// TestTheFolderIsRemovedAfterSIGINT: a run interrupted (SIGINT, CTRL_BREAK on
// Windows, which go delivers as os.Interrupt) holding its folder removes it on
// the way out, and the cache's scratch folder is left empty. Whether this
// system can serve the sandbox is asked of it (a Prepare here, answered with
// notServed or not), never read from GOOS: where it cannot, the
// child must say it was refused and is interrupted all the same, and no
// folder is claimed. Risk: every interrupted proof run leaving its fixtures
// behind, or a test that passes on a system that cannot reach the path.
func TestTheFolderIsRemovedAfterSIGINT(t *testing.T) {
	t.Parallel()
	probe := sandbox.New(sandbox.Config{Dir: cache(t)})
	sc, err := probe.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	served := !notServed(err)
	if served {
		if err != nil {
			t.Fatal(err)
		}
		if err := probe.Release(sc); err != nil {
			t.Fatal(err)
		}
	}
	dir := cache(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	code, out, err := configtest.RerunInterrupted(ctx, interrupted, dir)
	if err != nil || code != 0 {
		t.Fatalf("the helper: %d, %v\n%s", code, err, out)
	}
	for _, b := range judge(string(out), served, dir) {
		t.Error(b)
	}
}

// TestTheJudgeReadsTheHelperByTheAnswer: planted outputs. A helper that
// prepared a folder where the sandbox cannot be served, and one that was
// refused where it can, are each found; so is a folder left behind. The
// right pair is found clean. Risk: a judge that passes whatever the helper
// said, so the interrupt path is never held.
func TestTheJudgeReadsTheHelperByTheAnswer(t *testing.T) {
	t.Parallel()
	dir := cache(t)
	gone := filepath.Join(dir, "gone")
	left := filepath.Join(dir, "left")
	if err := os.Mkdir(left, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		claim, out string
		served     bool
		want       int
	}{
		{"served, prepared, removed, interrupted", "prepared: " + gone + "\ninterrupted\n", true, 0},
		{"not served, refused, interrupted", "refused: x\ninterrupted\n", false, 0},
		{"prepared where it cannot be served", "prepared: " + gone + "\ninterrupted\n", false, 1},
		{"refused where it can be served", "refused: x\ninterrupted\n", true, 1},
		{"a folder left behind", "prepared: " + left + "\ninterrupted\n", true, 1},
		{"never interrupted", "prepared: " + gone + "\n", true, 1},
	} {
		if got := judge(tc.out, tc.served, dir); len(got) != tc.want {
			t.Errorf("%s: %d problems %q, want %d", tc.claim, len(got), got, tc.want)
		}
	}
}
