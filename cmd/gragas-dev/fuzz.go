package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/host/clock"
)

// fuzzTarget is one Fuzz function and the package that holds it.
type fuzzTarget struct{ name, pkg string }

// fuzzRun is one `go test -fuzz` run: it gets the target, the time to fuzz and
// the number of fuzz workers it may start, and returns what the run printed.
// The context carries the run's deadline; a run that outlives it is stopped.
type fuzzRun func(ctx context.Context, dir string, t fuzzTarget, fuzztime time.Duration, workers int) (string, error)

var fuzzFunc = regexp.MustCompile(`(?m)^func (Fuzz[A-Za-z0-9_]*)\(`)

// fuzzTargets lists every Fuzz function in the _test.go files under cmd and
// internal of dir, by package and name, so the order does not move between runs.
func fuzzTargets(dir string) ([]fuzzTarget, error) {
	var out []fuzzTarget
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(dir, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dir, filepath.Dir(p))
			if err != nil {
				return err
			}
			for _, m := range fuzzFunc.FindAllSubmatch(b, -1) {
				out = append(out, fuzzTarget{name: string(m[1]), pkg: "./" + filepath.ToSlash(rel)})
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].pkg != out[j].pkg {
			return out[i].pkg < out[j].pkg
		}
		return out[i].name < out[j].name
	})
	return out, nil
}

// fuzzJobs is the default number of targets run at once: a quarter of the
// CPUs, and at least one. Each target starts its own fuzz workers, so more
// than that would make the targets fight over the same cores.
func fuzzJobs(cpus int) int { return max(1, cpus/4) }

// fuzzCommand runs every target for fuzztime each, at most jobs at once, and
// prints each target's header, its output and its result together, so the
// lines of two targets never mix. It stops starting targets after the first
// failure, lets the ones running finish, and names the first that failed.
// A target that has not stopped fuzztime plus grace after it began is a failure.
// A pass ends on its count line (gragas#695): the targets fuzzed, as tests.
// fuzzPlan is what one run of the step is told: where, for how long, and how wide.
type fuzzPlan struct {
	dir             string
	fuzztime, grace time.Duration
	jobs, cpus      int
}

func fuzzCommand(ctx context.Context, p fuzzPlan, run fuzzRun, out io.Writer) (bool, error) {
	return fuzzCommandLimited(ctx, p, run, func(c context.Context, _ fuzzTarget) (context.Context, context.CancelFunc) {
		return context.WithTimeout(c, p.fuzztime+p.grace)
	}, out)
}

// fuzzLimit is the context one target runs under: it ends with a DeadlineExceeded
// error when the target has run fuzztime plus grace. fuzzCommand uses the wall
// clock; a test hands one that ends on a signal, so no verdict depends on how
// fast a loaded host schedules the goroutines (gragas#741).
type fuzzLimit func(context.Context, fuzzTarget) (context.Context, context.CancelFunc)

// fuzzCommandLimited is fuzzCommand with the limit of each target given.
func fuzzCommandLimited(ctx context.Context, p fuzzPlan, run fuzzRun, limit fuzzLimit, out io.Writer) (bool, error) {
	dir, fuzztime, grace, jobs, cpus := p.dir, p.fuzztime, p.grace, p.jobs, p.cpus
	targets, err := fuzzTargets(dir)
	if err != nil {
		return false, err
	}
	if len(targets) == 0 {
		return false, errors.New("no fuzz target found under cmd or internal")
	}
	workers := max(1, cpus/jobs)
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		first  string
		failed []string
		slots  = make(chan struct{}, jobs)
		talk   = &say.Speaker{W: out}
	)
	for _, t := range targets {
		slots <- struct{}{}
		mu.Lock()
		stop := first != "" || ctx.Err() != nil
		mu.Unlock()
		if stop {
			<-slots
			break
		}
		// measured: the 35 fuzz targets 3 at once, 2100s to 739s at 15 CPUs, run on 2026-11-09
		wg.Go(func() {
			defer func() { <-slots }()
			tctx, cancel := limit(ctx, t)
			start := clock.Clock{}.Now()
			text, err := run(tctx, dir, t, fuzztime, workers)
			timedOut := errors.Is(tctx.Err(), context.DeadlineExceeded)
			cancel()
			took := clock.Clock{}.Now().Sub(start).Round(time.Second)
			var result string
			switch {
			case timedOut:
				err = fmt.Errorf("did not stop within %s of its %s", grace, fuzztime)
				result = fmt.Sprintf("fuzz: %s FAILED after %s: %v", t.name, took, err)
			case err != nil:
				result = fmt.Sprintf("fuzz: %s FAILED after %s: %v", t.name, took, err)
			default:
				result = fmt.Sprintf("fuzz: %s ok in %s", t.name, took)
			}
			mu.Lock()
			defer mu.Unlock()
			talk.Text(fmt.Sprintf("fuzz: %s in %s for %s\n%s%s\n", t.name, t.pkg, fuzztime, withNewline(text), result))
			if err != nil {
				if first == "" {
					first = t.name + " in " + t.pkg
				}
				failed = append(failed, t.name)
			}
		})
	}
	wg.Wait()
	switch {
	case talk.Err != nil:
		return false, talk.Err
	case first != "":
		talk.Text(fmt.Sprintf("fuzz: %d of %d targets failed (%s); the first was %s\n", len(failed), len(targets), strings.Join(failed, ", "), first))
		return false, nil
	case ctx.Err() != nil:
		return false, ctx.Err()
	}
	talk.Text(fmt.Sprintf("fuzz: %d targets, %d at a time, %s each: all ok\n", len(targets), jobs, fuzztime))
	talk.Text(countLine(len(targets), "tests") + "\n")
	return true, talk.Err
}

func withNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// fuzzGo is the go the fuzz runs start, from the PATH, as it always was; it is
// not run under goEnv, as the gate's go commands are.
const fuzzGo = "go"

// fuzzArgs is the go command line for one target: the target alone, for the
// whole of fuzztime, whatever else runs beside it.
func fuzzArgs(t fuzzTarget, fuzztime time.Duration, workers int) []string {
	return []string{"-run", "^$", "-fuzz", "^" + t.name + "$",
		"-fuzztime", fuzztime.String(), "-parallel", fmt.Sprint(workers), t.pkg}
}

// goFuzz is the real fuzzRun: go test for the one target, with the tests
// switched off, so only the fuzz run is timed.
func goFuzz(ctx context.Context, dir string, t fuzzTarget, fuzztime time.Duration, workers int) (string, error) {
	cmd := exec.CommandContext(ctx, fuzzGo, "test")
	cmd.Args = append(cmd.Args, fuzzArgs(t, fuzztime, workers)...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	return buf.String(), err
}

// runFuzz is the fuzz command with its flags.
func runFuzz(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("fuzz", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fuzztime := fl.Duration("fuzztime", time.Minute, "how long each target fuzzes")
	grace := fl.Duration("grace", 30*time.Second, "how long past fuzztime a target may run before it fails")
	cpus := runtime.NumCPU()
	jobs := fl.Int("jobs", fuzzJobs(cpus), "how many targets run at once")
	dir := fl.String("C", ".", "the repository to fuzz")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 || *jobs < 1 || *fuzztime <= 0 || *grace <= 0 {
		talk.Text(usage)
		return exitUsage
	}
	ok, err := fuzzCommand(ctx, fuzzPlan{*dir, *fuzztime, *grace, *jobs, cpus}, goFuzz, stdout)
	switch {
	case err != nil:
		talk.Text("gragas-dev fuzz: " + err.Error() + "\n")
		return exitFailed
	case !ok:
		return exitFailed
	}
	return exitDone
}
