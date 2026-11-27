package porttest

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"slices"
	"sync"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// The fake of port.Steps: a grant's program is the tool
// SeedTool made locatable under the grant's id, and a step's result is the
// one SeedStep or SeedRead gave. It holds a step to what the port says,
// nothing of how the adapter runs one: no program is started and no file
// placed.

// SeedStep makes a run of the grant program answer r.
func (w *World) SeedStep(program string, r plugin.Result) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seedStep("run "+program, r)
}

// SeedRead makes a read of glob answer files.
func (w *World) SeedRead(glob string, files []plugin.File) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seedStep("read "+glob, plugin.Result{Files: files})
}

// KeepCaches makes the Steps hold n folders of a grant's caches, kept from
// one step to the next, for a clear to count and remove.
func (w *World) KeepCaches(n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.caches = n
}

// SweepCaches removes the folders KeepCaches kept and answers how many.
func (x stepsPort) SweepCaches() (int, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	n := x.w.caches
	x.w.caches = 0
	return n, nil
}

// CachesLeft answers how many folders SweepCaches would remove, and removes none.
func (x stepsPort) CachesLeft() (int, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	return x.w.caches, nil
}

// seedStep keeps r under key. The caller holds w.mu.
func (w *World) seedStep(key string, r plugin.Result) {
	if w.steps == nil {
		w.steps = map[string]plugin.Result{}
	}
	w.steps[key] = r
}

type stepsPort struct{ w *World }

// stepRunner is the fake's runner of one scope.
type stepRunner struct {
	w     *World
	scope plugin.Scope
	tool  model.Found
	// asked is the grants whose version ask was journalled.
	asked *asked
}

// Bind answers as the port says: a scope with no note, a root or scratch
// folder that is no absolute clean path, a level that is neither, or a
// deadline below 1 s or a memory limit below 0, is refused; a manifest with no grant of its tool matches ErrGrant; a tool not
// seeded at a version its grant serves is not ready.
func (x stepsPort) Bind(ctx context.Context, s plugin.Scope) (port.StepRunner, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch {
	case s.Note == nil:
		return nil, fmt.Errorf("%w: a step runner is bound with no note", port.ErrRefused)
	case !absClean(string(s.Root)) || !absClean(s.Scratch.Dir):
		return nil, fmt.Errorf("%w: the root %q or the scratch folder %q is no absolute clean path", port.ErrRefused, s.Root, s.Scratch.Dir)
	case !plugin.Level(s.Scratch.Level).Valid() || !s.Floor.Valid():
		return nil, fmt.Errorf("%w: the levels %q and %q are not each isolated or sandboxed", port.ErrRefused, s.Scratch.Level, s.Floor)
	case s.Scratch.Seconds < 1 || s.Scratch.MemoryMiB < 0:
		return nil, fmt.Errorf("%w: a deadline of %d s and a memory limit of %d MiB, and a deadline is 1 s or more and a limit 0 or more", port.ErrRefused, s.Scratch.Seconds, s.Scratch.MemoryMiB)
	}
	r := stepRunner{w: x.w, scope: s, asked: &asked{ids: map[string]bool{}}}
	f, err := r.program(ctx, 0, 0, s.Manifest.Tool)
	if err != nil {
		return nil, err
	}
	r.tool = f
	return r, nil
}

// absClean is whether p is an absolute, clean, slash-separated path.
func absClean(p string) bool { return path.IsAbs(p) && path.Clean(p) == p }

func (r stepRunner) Tool() model.Found { return r.tool }

func (r stepRunner) Granted(s plugin.Step) error {
	if s.Run == nil {
		return nil
	}
	_, _, err := grantOf(r.scope.Manifest, s.Run.Program)
	return err
}

func (r stepRunner) Run(ctx context.Context, round, step int, s plugin.Step) (plugin.Result, error) {
	if err := ctx.Err(); err != nil {
		return plugin.Result{}, err
	}
	if (s.Run == nil) == (s.Read == nil) {
		return plugin.Result{}, fmt.Errorf("%w: a step is one run or one read", port.ErrRefused)
	}
	if s.Read != nil {
		return r.answer("read "+s.Read.Glob, "StepRunner.Run", "no read of %q was seeded; call SeedRead", s.Read.Glob)
	}
	run := *s.Run
	f, err := r.program(ctx, round, step, run.Program)
	if err != nil {
		return plugin.Result{}, err
	}
	level := r.scope.Floor
	if run.Level != nil && rank(*run.Level) > rank(level) {
		level = *run.Level
	}
	if rank(level) > rank(plugin.Level(r.scope.Scratch.Level)) {
		return plugin.Result{}, fmt.Errorf("%w: the step runs at %s and the scratch folder reached %s", port.ErrStepNotReady, level, r.scope.Scratch.Level)
	}
	size, err := stdinLen(run.Stdin)
	if err != nil {
		return plugin.Result{}, fmt.Errorf("%w: the stdin is not bytes: %w", port.ErrRefused, err)
	}
	if size > plugin.MaxStdin {
		return plugin.Result{}, fmt.Errorf("%w: the stdin is %d bytes, more than %d", port.ErrRefused, size, plugin.MaxStdin)
	}
	n := plugin.Noted{Round: round, Step: step, Grant: run.Program, Program: f.Path, Version: f.Version, Words: slices.Clone(run.Argv), Level: level}
	if err := r.scope.Note(ctx, n); err != nil {
		return plugin.Result{}, fmt.Errorf("step %d of round %d is not journalled, so nothing of it is started: %w", step, round, err)
	}
	return r.answer("run "+run.Program, "StepRunner.Run", "no run of %s was seeded; call SeedStep", run.Program)
}

// answer is the result seeded under key, or the test failed naming call.
func (r stepRunner) answer(key, call, format string, args ...any) (plugin.Result, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if res, ok := r.w.steps[key]; ok {
		res.Files = slices.Clone(res.Files)
		return res, nil
	}
	return plugin.Result{}, r.w.unseeded(call, format, args...)
}

// program is the tool seeded under the grant id, held to the grant's range.
// A program that is seeded is asked its version, so its ask is journalled
// first, as a VersionAsk of round and step (0 and 0 at Bind), the first time
// the runner finds it; a note that fails ends the find with its error.
func (r stepRunner) program(ctx context.Context, round, step int, id string) (model.Found, error) {
	versions, argv, err := grantOf(r.scope.Manifest, id)
	if err != nil {
		return model.Found{}, err
	}
	rng, err := model.ParseRange(versions)
	if err != nil {
		return model.Found{}, fmt.Errorf("%w: the grant %s serves %q, which is no range: %w", port.ErrRefused, id, versions, err)
	}
	r.w.mu.Lock()
	f, ok := r.w.tools[id]
	r.w.mu.Unlock()
	if ok && !r.asked.has(id) {
		n := plugin.Noted{Round: round, Step: step, Grant: id, Program: f.Path, Script: f.Script, Words: slices.Clone(argv), VersionAsk: true}
		if err := r.scope.Note(ctx, n); err != nil {
			return model.Found{}, fmt.Errorf("the version ask of %s is not journalled, so it is not started: %w", id, err)
		}
		r.asked.add(id)
	}
	if in, err := model.InRange(rng, f.Version); !ok || err != nil || !in {
		return model.Found{}, fmt.Errorf("%w: %s is not here at a version inside %s", port.ErrStepNotReady, id, versions)
	}
	return f, nil
}

// grantOf is the versions and the version argv of the grant id of m;
// ErrGrant for none.
func grantOf(m plugin.Manifest, id string) (versions string, argv []string, err error) {
	for _, p := range m.Programs {
		switch {
		case p.Found != nil && p.Found.ID == id:
			return p.Found.Versions, p.Found.VersionArgv, nil
		case p.At != nil && p.At.ID == id:
			return p.At.Versions, p.At.VersionArgv, nil
		case p.Script != nil && p.Script.ID == id:
			return p.Script.Versions, p.Script.VersionArgv, nil
		}
	}
	return "", nil, fmt.Errorf("%w: %q", port.ErrGrant, id)
}

// rank orders the levels: isolated below sandboxed, anything else below both.
func rank(l plugin.Level) int {
	switch l {
	case plugin.LevelIsolated:
		return 1
	case plugin.LevelSandboxed:
		return 2
	}
	return 0
}

// stdinLen is how many bytes b holds; nil is none.
func stdinLen(b *plugin.Bytes) (int, error) {
	switch {
	case b == nil:
		return 0, nil
	case (b.Text == nil) == (b.Base64 == nil):
		return 0, fmt.Errorf("bytes are text or base64, one of the two")
	case b.Text != nil:
		return len(*b.Text), nil
	}
	data, err := base64.StdEncoding.DecodeString(*b.Base64)
	return len(data), err
}

// asked is the ids of the grants a runner has asked their version, held for
// the runner's steps, which may run at once.
type asked struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (a *asked) has(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ids[id]
}

func (a *asked) add(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ids[id] = true
}
