package proof

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// ErrLevel is what Run's error matches when the sandbox cannot give the level
// asked: it answered a lower one, or none. The run is refused before the tool
// is called and no cell is written, since a cell proven at a lower level would
// say more than was done.
var ErrLevel = errors.New("the sandbox does not reach the level asked")

// ErrOutput is what Run's error matches when what a side's run gave cannot be
// trusted: a step's output was cut at the cap, the plugin says the tool did not
// run, no step ran it, it failed and the plugin read nothing, or a finding
// named a file outside the folder it ran in. The batch is refused whole
// (gates.plants.1).
var ErrOutput = errors.New("the tool's output is refused")

// Findings runs one side: the job's tool, through the plugin that serves it,
// over the fixtures placed in sc (their paths relative to sc.Dir), with the
// job's config set, and answers what the read came to. A context imports no
// other (arch.ports.8), so the operation hands it core/plugins' Findings read
// over the Steps port, bound to sc at the request's level, as plugin record
// binds it. An error matching port.ErrStepNotReady says the
// tool is not on this machine: the job's cells are not run, tool missing.
type Findings func(ctx context.Context, j Job, sc model.Scratch, fixtures []string) (Side, error)

// Side is what one side's read came to: the version of the tool the step
// runner found (the plan's tool_version), whether the plugin says the tool ran
// over the fixtures and why not, the findings it read, and every step the read
// ran, which Run holds the side to (held).
type Side struct {
	ToolVersion string
	Ran         bool
	Why         string
	Findings    []plugin.Finding
	Steps       []plugin.Result
}

// Case is one binding to prove with the pair that proves it.
type Case struct {
	Rule    model.RuleID
	Binding model.Binding
	// Pair is the pair the binding names; the zero Pair for a binding with
	// none.
	Pair Pair
}

// Job is every case of one tool at one version, read by one plugin.
type Job struct {
	Tool, Version string
	// Adapter is the version of what reads the tool's output, which the
	// cell's adapter field holds: the plugin's, until the grid's format names
	// it so.
	Adapter string
	// Config is the config set, mirrored into every folder, and ConfigPath
	// the path of the file the run is told to read inside it, or "" for a
	// run that names none.
	Config     []model.File
	ConfigPath string
	Cases      []Case
}

// Request is what Run is asked: the root, the level, the limits, the jobs and
// the read of a side.
type Request struct {
	Root model.Root
	// Run is the id of the run, which a step the step runner refuses is
	// journalled under, and Engine the version that writes the
	// record. A Run that is empty is made from the clock and the tool.
	Run, Engine string
	Level       model.Level
	Seconds     int
	MemoryMiB   int
	Jobs        []Job
	Findings    Findings
	// Hint, when not nil, is given the sandbox's own line, once, when a sandboxed
	// run finds none (the cell is then not run, sandbox missing). It is shown
	// to a person and never read by Run, and it is not put in a cell, which
	// holds no host path (S57).
	Hint func(line string)
}

// Run runs every case's pair and judges each: the good example must draw no
// finding of the binding's key at the placed path, and the bad one must draw
// one. It takes the root lock and then the scratch lock, so another proof run
// ends it with an error matching port.ErrHeld, and gives both back on every
// path. A tool that is not on this machine, or is at another version than the
// job's, makes that job's cells not run, tool missing, and the run goes on.
// The examples of a job are read in two reads at most per batch, one for the
// good side and one for the bad, and a batch is split only where two cells
// place a file at one path.
//
// The tool's exit code is not the verdict, and the tool's text never runs: the
// examples are placed as bytes, and only the findings' key and path decide.
func Run(ctx context.Context, ports port.Ports, req Request) (cells []model.Cell, err error) {
	if req.Level != model.Isolated && req.Level != model.Sandboxed {
		return nil, fmt.Errorf("%w: %q is neither isolated nor sandboxed", ErrLevel, req.Level)
	}
	req.Hint = once(req.Hint)
	release, err := hold(ctx, ports.Lock, req.Root)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	for _, j := range req.Jobs {
		got, jerr := runJob(ctx, ports, req, j)
		if jerr != nil {
			return nil, fmt.Errorf("%s %s: %w", j.Tool, j.Version, jerr)
		}
		cells = append(cells, got...)
	}
	return cells, nil
}

// once is hint, given at most one call.
func once(hint func(string)) func(string) {
	if hint == nil {
		return nil
	}
	done := false
	return func(s string) {
		if !done {
			done = true
			hint(s)
		}
	}
}

// hold takes the root lock and then the scratch lock and answers the call
// that puts both back, the scratch lock first.
func hold(ctx context.Context, l port.Lock, root model.Root) (func() error, error) {
	r, err := l.Take(ctx, root, model.RootLock)
	if err != nil {
		return nil, fmt.Errorf("the root lock could not be taken: %w", err)
	}
	s, err := l.Take(ctx, root, model.ScratchLock)
	if err != nil {
		rerr := l.Release(r)
		return nil, errors.Join(fmt.Errorf("the scratch lock could not be taken: %w", err), rerr)
	}
	return func() error {
		return errors.Join(l.Release(s), l.Release(r))
	}, nil
}

// placed is a case with the folders it is run in.
type placed struct {
	c  Case
	pl Placement
}

// runJob answers the cells of one job, in the order of its cases.
func runJob(ctx context.Context, ports port.Ports, req Request, j Job) ([]model.Cell, error) {
	if err := checkConfigPath(j); err != nil {
		return nil, err
	}
	cells := make([]model.Cell, len(j.Cases))
	var todo []placed
	var at []int
	for i, c := range j.Cases {
		cells[i] = cellOf(j, c, "", "")
		switch {
		case c.Binding.Rung == model.MaxRung:
			cells[i].Result = model.TruthReviewed
		case c.Binding.PairPath == "":
			cells[i].Result, cells[i].Reason = model.TruthNotRun, model.ReasonNoPair
		default:
			pl, err := Place(c.Pair, c.Binding, j.Config)
			if err != nil {
				return nil, err
			}
			todo, at = append(todo, placed{c, pl}), append(at, i)
		}
	}
	if len(todo) == 0 {
		return cells, nil
	}
	results, err := runBatches(ctx, ports, req, j, todo)
	if err != nil {
		return nil, err
	}
	for k, i := range at {
		cells[i].Result, cells[i].Reason = results[k].truth, results[k].reason
	}
	return cells, nil
}

// cellOf is the cell of c under j, with the result and reason given, and the
// tool and the inputs it is proven on, which a later Plan compares.
func cellOf(j Job, c Case, r model.Truth, why model.Reason) model.Cell {
	return model.Cell{
		Binding:     BindingText(c.Binding.Ref(c.Rule)),
		ToolVersion: j.Version,
		Tool:        j.Tool,
		Inputs:      Inputs(j, c),
		Result:      r,
		Rung:        int(c.Binding.Rung),
		Level:       model.RunPair,
		Adapter:     j.Adapter,
		Reason:      why,
	}
}

// verdict is what a case came to.
type verdict struct {
	truth  model.Truth
	reason model.Reason
}

// runBatches runs the cases in as few batches as their paths allow and
// answers each case's verdict, in the order given. A tool found missing, or at
// another version, on either side makes every later batch tool missing with
// nothing run.
func runBatches(ctx context.Context, ports port.Ports, req Request, j Job, todo []placed) ([]verdict, error) {
	out := make([]verdict, len(todo))
	missing := false
	for _, idx := range batchesOf(todo) {
		if missing {
			for _, i := range idx {
				out[i] = verdict{truth: model.TruthNotRun, reason: model.ReasonToolMissing}
			}
			continue
		}
		batch := make([]placed, len(idx))
		for k, i := range idx {
			batch[k] = todo[i]
		}
		good, gwhy, err := runSide(ctx, ports, req, j, batch, func(p placed) []model.File { return p.pl.Good })
		if err != nil {
			return nil, err
		}
		missing = gwhy == model.ReasonToolMissing
		var bad []found
		var bwhy model.Reason
		if gwhy != model.ReasonSandboxMissing && !missing {
			if bad, bwhy, err = runSide(ctx, ports, req, j, batch, func(p placed) []model.File { return p.pl.Bad }); err != nil {
				return nil, err
			}
			missing = bwhy == model.ReasonToolMissing
		}
		for k, i := range idx {
			out[i] = judge(batch[k].c.Binding, good, bad, firstWhy(gwhy, bwhy))
		}
	}
	return out, nil
}

func firstWhy(a, b model.Reason) model.Reason {
	if a != "" {
		return a
	}
	return b
}

// batchesOf groups the indexes of cases so that no group places two files at
// one path or one inside another.
func batchesOf(todo []placed) [][]int {
	var groups [][]int
	var used []map[string]bool
	for i, p := range todo {
		at := p.c.Binding.PairPath
		g := slices.IndexFunc(used, func(u map[string]bool) bool { return !crowded(u, at) })
		if g < 0 {
			groups, used, g = append(groups, nil), append(used, map[string]bool{}), len(groups)
		}
		groups[g] = append(groups[g], i)
		used[g][at] = true
	}
	return groups
}

// crowded is whether at is a path of used, or above or below one.
func crowded(used map[string]bool, at string) bool {
	for u := range used {
		if u == at || strings.HasPrefix(u, at+"/") || strings.HasPrefix(at, u+"/") {
			return true
		}
	}
	return false
}

// found is one diagnostic of a side, its path made relative to the folder.
type found struct{ key, path string }

// runSide runs one side of a batch in a prepared folder and answers what the
// tool found, as relative paths, or the reason it was not run. The folder is
// released on every path.
func runSide(ctx context.Context, ports port.Ports, req Request, j Job, batch []placed, files func(placed) []model.File) (_ []found, _ model.Reason, err error) {
	fixtures := fixturesOf(j, batch, files)
	sc, err := prepare(ctx, ports.Sandbox, req, j, fixtures)
	if gone, ok := errors.AsType[*sandboxGone](err); ok {
		if req.Hint != nil {
			req.Hint(gone.err.Error())
		}
		return nil, model.ReasonSandboxMissing, nil
	}
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if rerr := ports.Sandbox.Release(sc); rerr != nil {
			err = errors.Join(err, fmt.Errorf("the scratch folder could not be removed: %w", rerr))
		}
	}()
	paths := make([]string, len(fixtures))
	for i, f := range fixtures {
		paths[i] = f.Path
	}
	side, err := req.Findings(ctx, j, sc, paths)
	if errors.Is(err, port.ErrStepNotReady) {
		return nil, model.ReasonToolMissing, nil
	}
	if errors.Is(err, port.ErrRefused) {
		if jerr := journalRefused(context.WithoutCancel(ctx), ports, req, j, err); jerr != nil {
			err = errors.Join(err, jerr)
		}
	}
	var limit interface{ NotRun() (string, bool) }
	if errors.As(err, &limit) {
		if _, ok := limit.NotRun(); ok {
			return nil, model.ReasonLimitHit, nil
		}
	}
	if err != nil {
		return nil, "", err
	}
	if side.ToolVersion != j.Version {
		return nil, model.ReasonToolMissing, nil
	}
	if err := held(side); err != nil {
		return nil, "", err
	}
	got, err := within(sc.Dir, side.Findings)
	return got, "", err
}

// sandboxGone is a sandboxed run that found no sandbox on the machine: the
// cell is not run, sandbox missing, and Run goes on. The answer is told by
// errors.Is(port.ErrUnavailable); the text is only the line a person is shown.
type sandboxGone struct{ err error }

func (g *sandboxGone) Error() string { return g.err.Error() }
func (g *sandboxGone) Unwrap() error { return g.err }

// held says whether the side can be counted, or refuses the batch (ErrOutput).
// It holds only what is free of format (gates.plants.1): which of the tool's
// codes mean it ran, and which of its findings are its own failure, are the
// plugin's to say, in Ran. A side is refused when a step's output was cut at
// the cap, when the plugin says the tool did not run, when no step ran the
// tool, and when a run exited non-zero and the plugin read nothing, since a
// crash and a rejected config print no finding and exit non-zero. Only a
// finding that names a file counts as read: within leaves the others out, so a
// crash the plugin reads as one pathless finding would otherwise count as clean.
func held(s Side) error {
	for _, r := range s.Steps {
		if r.Cut != nil && *r.Cut {
			return fmt.Errorf("%w: step %d of round %d printed more than the cap, so the run is not counted", ErrOutput, r.Step, r.Round)
		}
	}
	if !s.Ran {
		return fmt.Errorf("%w: the plugin says the tool did not run over the examples: %s", ErrOutput, clip(s.Why))
	}
	read := slices.ContainsFunc(s.Findings, func(f plugin.Finding) bool { return f.Path != nil })
	ran := false
	for _, r := range s.Steps {
		if r.Kind != plugin.KindRun {
			continue
		}
		ran = true
		if r.Exit != nil && *r.Exit != 0 && !read {
			return fmt.Errorf("%w: the tool exited %d and the plugin read no finding that names a file, so it did not say clean", ErrOutput, *r.Exit)
		}
	}
	if !ran {
		return fmt.Errorf("%w: the plugin's read ran no step of the tool, so it did not say clean", ErrOutput)
	}
	return nil
}

// fixturesOf is the files of one side of a batch, those of the config set left
// out: they are mirrored into the folder as the config, once.
func fixturesOf(j Job, batch []placed, files func(placed) []model.File) []model.File {
	isConfig := map[string]bool{}
	for _, f := range j.Config {
		isConfig[f.Path] = true
	}
	var fixtures []model.File
	for _, p := range batch {
		for _, f := range files(p) {
			if !isConfig[f.Path] {
				fixtures = append(fixtures, f)
			}
		}
	}
	return fixtures
}

// prepare makes the folder of one side and refuses a level lower than asked.
func prepare(ctx context.Context, sb port.Sandbox, req Request, j Job, fixtures []model.File) (model.Scratch, error) {
	sc, err := sb.Prepare(ctx, model.Prepare{Level: req.Level, Fixtures: fixtures, Config: j.Config, Seconds: req.Seconds, MemoryMiB: req.MemoryMiB})
	if errors.Is(err, port.ErrUnavailable) {
		if req.Level == model.Sandboxed {
			return model.Scratch{}, &sandboxGone{err}
		}
		return model.Scratch{}, fmt.Errorf("%w: %w", ErrLevel, err)
	}
	if err != nil {
		return model.Scratch{}, err
	}
	if sc.Level != req.Level {
		rerr := sb.Release(sc)
		return model.Scratch{}, errors.Join(fmt.Errorf("%w: %s was asked and %s was reached", ErrLevel, req.Level, sc.Level), rerr)
	}
	return sc, nil
}

// within makes each finding's path relative to dir. A path that is not
// inside dir, or climbs out of it, refuses the whole output. A finding that
// names no file is left out: it flags no binding, which names a path, and it
// names nothing outside the folder.
//
// The folder's own prefix is judged first, in whatever spelling dir has:
// package path calls a folder with a drive letter relative, so asking IsAbs
// first refused every finding inside such a folder. What is
// left after the prefix, and any path without it, still goes through Confine,
// which refuses a climb, a drive letter, a backslash and a rooted path.
func within(dir string, fs []plugin.Finding) ([]found, error) {
	out := make([]found, 0, len(fs))
	for _, d := range fs {
		if d.Path == nil {
			continue
		}
		p := *d.Path
		if rel, ok := strings.CutPrefix(p, dir+"/"); ok && dir != "" {
			p = rel
		} else if path.IsAbs(p) {
			return nil, fmt.Errorf("%w: a finding names %s, outside the folder", ErrOutput, clip(p))
		}
		p = strings.TrimPrefix(p, "./")
		if err := Confine(p); err != nil {
			return nil, fmt.Errorf("%w: a finding names %s: %w", ErrOutput, clip(*d.Path), err)
		}
		out = append(out, found{key: d.Key, path: p})
	}
	return out, nil
}

// checkConfigPath refuses a config path the tool would be told to load that is
// not a clean path inside the folder, or that names no file of the config set:
// a config file is code for most tools, so it is never a path of the caller's
// own making.
func checkConfigPath(j Job) error {
	if j.ConfigPath == "" {
		return nil
	}
	if err := Confine(j.ConfigPath); err != nil {
		return fmt.Errorf("%w: the config path %s", err, clip(j.ConfigPath))
	}
	for _, f := range j.Config {
		if f.Path == j.ConfigPath {
			return nil
		}
	}
	return fmt.Errorf("%w: the config path %s names no file of the config set", ErrPath, clip(j.ConfigPath))
}

// journalRefused writes one record for a step the step runner refused before
// it started: a memory limit the platform cannot judge, or limits the process
// library will not hold. The record
// is a close under the run, its outcome refused, naming the tool and its
// version and the refusal's own text as observed, so a person who finds the
// run ended can read why. The refusal ends the run, and it is
// the only record the refusal makes.
func journalRefused(ctx context.Context, ports port.Ports, req Request, j Job, refusal error) error {
	detail, err := canon.Encode(canon.Object{"outcome": canon.String("refused"), "cause": canon.String("tool refused"), "tool": canon.String(j.Tool),
		"version": canon.String(j.Version), "why": canon.String(fmt.Sprint(refusal))}, canon.Canonical)
	if err != nil {
		return err
	}
	run := req.Run
	if run == "" {
		run = fmt.Sprintf("prove-%d-%s", ports.Clock.Now().UnixNano(), digest.Hex([]byte(j.Tool + " " + j.Version))[:8])
	}
	if err := ports.Journal.Append(ctx, req.Root, model.Record{Run: run, Kind: model.KindClose, Command: "prove", Engine: req.Engine, Detail: detail}); err != nil {
		return fmt.Errorf("the refusal could not be journalled: %w", err)
	}
	return nil
}
