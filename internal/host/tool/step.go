package tool

// The step runner: what a plugin's plan asks run or read, and
// only what its manifest grants. The plugin never runs anything; this file is
// where that is kept. It holds no table of tools: each program is the grant's,
// found as step_grant.go finds it, and each word, path and byte is held as
// step_words.go and step_read.go hold it.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/birdtable/go-files/confine"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// shapeStep is the shape a step's run is recorded under (Ran.Shape): a step
// has no shape of an adapter's, its words are the plan's.
const shapeStep = "step"

// bound is the runner of one read's steps: one scope, the tool's grant found
// before any plugin call, and each other grant found once, at its first step,
// so every step of the read that names a grant runs the one program.
type bound struct {
	t     *Tool
	scope plugin.Scope
	tool  model.Found
	mu    sync.Mutex
	found map[string]model.Found
}

// Bind is the runner of the steps of one read under s (port.Steps). A scope
// with no Note, a root or a scratch folder that is no absolute clean path, or
// a level that is neither of the two is refused before anything is looked
// at. Then the grant whose id is the manifest's tool is found and asked its
// version (item 8), before Bind answers and so before any plugin call; a tool
// that is not here, at a version its grant serves, makes the read not ready.
func (t *Tool) Bind(ctx context.Context, s plugin.Scope) (port.StepRunner, error) {
	if s.Note == nil {
		return nil, refused("a step runner is bound with no note to journal its steps by", "bind it with the journal's note: no step runs unjournalled")
	}
	if fault := pathFault(s.Scratch.Dir); fault != "" {
		return nil, refused("the scratch folder "+hosttrust.Quote(s.Scratch.Dir)+" "+fault, "bind the runner to a folder the Sandbox port prepared")
	}
	if !plugin.Level(s.Scratch.Level).Valid() || !s.Floor.Valid() {
		return nil, refused("a step runner is bound with the scratch folder's level "+hosttrust.Quote(string(s.Scratch.Level))+" and the floor "+hosttrust.Quote(string(s.Floor))+", and a level is "+string(plugin.LevelIsolated)+" or "+string(plugin.LevelSandboxed),
			"bind it to a prepared folder's level and the engine's floor")
	}
	// A prepared folder's deadline is never 0 and its memory limit never below
	// 0 (model.Scratch): one that is was not prepared, and a run in it would go
	// to the host's own wait, or with no limit, unsaid.
	if s.Scratch.Seconds < 1 || s.Scratch.MemoryMiB < 0 {
		return nil, refused("a step runner is bound with the scratch folder's deadline "+strconv.Itoa(s.Scratch.Seconds)+" s and memory limit "+strconv.Itoa(s.Scratch.MemoryMiB)+" MiB, and a deadline is 1 s or more and a limit 0 (none) or more",
			"bind it to a folder the Sandbox port prepared, with its deadline and limit as it set them")
	}
	b := &bound{t: t, scope: s, found: map[string]model.Found{}}
	f, err := b.program(ctx, 0, 0, s.Manifest.Tool)
	if err != nil {
		return nil, err
	}
	b.tool = f
	return b, nil
}

// Tool is the program of the manifest's tool, as Bind found it.
func (b *bound) Tool() model.Found { return b.tool }

// Granted answers nil for a read, and for a run whose program is one of the
// manifest's grants; port.ErrGrant for one that is not. It looks for nothing.
func (b *bound) Granted(s plugin.Step) error {
	if s.Run == nil {
		return nil
	}
	_, err := grantOf(b.scope.Manifest, s.Run.Program)
	return err
}

// Run runs or reads one step and answers its result. The engine places the
// result (its round, step and kind); Run fills what the step gave.
func (b *bound) Run(ctx context.Context, round, step int, s plugin.Step) (plugin.Result, error) {
	switch {
	case (s.Run == nil) == (s.Read == nil):
		return plugin.Result{}, refused("a step is one run or one read", "answer each step as one run or one read")
	case s.Read != nil:
		return b.read(s.Read.Glob)
	}
	return b.run(ctx, round, step, *s.Run)
}

// program is the grant id's program, found the first time a step of the read
// names it and kept for the rest: a grant the manifest does not have matches
// port.ErrGrant, and a program that is nowhere here, port.ErrStepNotReady.
// Its version ask is journalled by the scope's Note before it starts, as a
// VersionAsk of round and step (0 and 0 at Bind), and a note that fails
// starts nothing and ends the find with its error.
func (b *bound) program(ctx context.Context, round, step int, id string) (model.Found, error) {
	g, err := grantOf(b.scope.Manifest, id)
	if err != nil {
		return model.Found{}, err
	}
	b.mu.Lock()
	f, ok := b.found[id]
	b.mu.Unlock()
	if ok {
		return f, nil
	}
	note := func(ctx context.Context, at model.Found) error {
		return b.scope.Note(ctx, plugin.Noted{Round: round, Step: step, Grant: id, Program: at.Path, Script: at.Script,
			Words: slices.Clone(g.argv), VersionAsk: true})
	}
	f, err = b.t.locateGrant(ctx, b.scope.Root, g, !b.scope.OwnPlace, note)
	if nf, ok := errors.AsType[*NotFound](err); ok {
		return model.Found{}, notReady(nf.Error(), "install "+id+" at a version inside "+g.versions+", or name it under tool-paths in the settings file or in the tools file")
	}
	if err != nil {
		return model.Found{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if was, ok := b.found[id]; ok {
		return was, nil
	}
	b.found[id] = f
	return f, nil
}

// run is one run step: its grant, its level, its words, its stdin and its
// files judged, then journalled, then its files placed and its program
// started in the scratch folder. Nothing is placed or started before the note
// is written, and nothing at all when it fails.
func (b *bound) run(ctx context.Context, round, step int, r plugin.Run) (_ plugin.Result, err error) {
	level, err := b.level(r.Level)
	if err != nil {
		return plugin.Result{}, err
	}
	f, err := b.program(ctx, round, step, r.Program)
	if err != nil {
		return plugin.Result{}, err
	}
	scratch := b.scope.Scratch
	words, err := stepWords(r.Argv, filepath.FromSlash(scratch.Dir))
	if err != nil {
		return plugin.Result{}, err
	}
	argv := words
	if f.Script != "" {
		argv = append([]string{filepath.FromSlash(f.Script)}, words...)
	}
	stdin, err := stdinOf(r.Stdin)
	if err != nil {
		return plugin.Result{}, err
	}
	places, err := placesOf(r.Place)
	if err != nil {
		return plugin.Result{}, err
	}
	in := b.t.within(b.scope.Root, !b.scope.OwnPlace)
	if in.empty() {
		return plugin.Result{}, refused(r.Program+" was found by a pinned find, but "+emptyPinnedPath(b.t.cfg.Path), emptyPinnedPathFix)
	}
	n := plugin.Noted{Round: round, Step: step, Grant: r.Program, Program: f.Path, Script: f.Script, Version: f.Version, Words: words, Level: level}
	if err := b.scope.Note(ctx, n); err != nil {
		return plugin.Result{}, fmt.Errorf("step %d of round %d is not journalled, so nothing of it is placed or started: %w", step, round, err)
	}
	root, err := os.OpenRoot(filepath.FromSlash(scratch.Dir))
	if err != nil {
		return plugin.Result{}, fmt.Errorf("the scratch folder %s: %w", hosttrust.Quote(scratch.Dir), err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := placeAll(root, places); err != nil {
		return plugin.Result{}, err
	}
	cache, err := b.cacheEnv(r.Program)
	if err != nil {
		return plugin.Result{}, err
	}
	l := launch{name: r.Program, shape: shapeStep, path: f.Path, argv: argv, dir: scratch.Dir, root: string(b.scope.Root),
		env: in.path, wait: b.t.wait, mib: scratch.MemoryMiB, stdin: stdin, max: plugin.MaxStream, cache: cache.vars, homeCache: cache.home}
	l.wait = min(l.wait, time.Duration(scratch.Seconds)*time.Second) // Bind held Seconds to 1 or more
	if b.t.cfg.Home == "" {
		// A step is given a Home of its own, made for this run and removed with it,
		// so a program that keeps a cache has one, and a plugin record and a real run
		// see the one environment.
		home, end, herr := b.t.runHome("step", string(b.scope.Root), scratch.Dir)
		if herr != nil {
			return plugin.Result{}, herr
		}
		defer end()
		l.home = filepath.ToSlash(home)
	}
	out, err := b.t.start(ctx, l)
	cut := false
	if e, ok := errors.AsType[*Ended](err); ok && e.Why == EndedCap {
		cut, err = true, nil
	}
	if err != nil {
		return plugin.Result{}, err
	}
	files, err := collect(root, r.Collect)
	if err != nil {
		return plugin.Result{}, err
	}
	res := plugin.Result{Stdout: new(stepBytes(out.Stdout)), Stderr: new(stepBytes(out.Stderr)), Cut: &cut, Files: files}
	if !cut {
		res.Exit = &out.Code
	}
	return res, nil
}

// level is the level a run asks for, held to the floor: the higher of the
// two (item 6). One the scratch folder did not reach is a level this
// machine lacks, and the step is not ready.
func (b *bound) level(asked *plugin.Level) (plugin.Level, error) {
	floor, reached := b.scope.Floor, plugin.Level(b.scope.Scratch.Level)
	level := floor
	if asked != nil && rank(*asked) > rank(floor) {
		level = *asked
	}
	if rank(level) > rank(reached) {
		return "", notReady("the step runs at the level "+string(level)+", the higher of the engine's floor "+string(floor)+" and its own, and the scratch folder this read was prepared in reached "+string(reached),
			"run it on a machine whose sandbox reaches "+string(level))
	}
	return level, nil
}

// rank orders the levels, lowest first; one that is neither is below both.
func rank(l plugin.Level) int {
	switch l {
	case plugin.LevelIsolated:
		return 1
	case plugin.LevelSandboxed:
		return 2
	}
	return 0
}

// read is one read step: the files its glob matches in the scratch folder,
// held to the caps at the read (readGlob).
func (b *bound) read(glob string) (_ plugin.Result, err error) {
	root, err := os.OpenRoot(filepath.FromSlash(b.scope.Scratch.Dir))
	if err != nil {
		return plugin.Result{}, fmt.Errorf("the scratch folder %s: %w", hosttrust.Quote(b.scope.Scratch.Dir), err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	files, err := readGlob(root, glob)
	if err != nil {
		return plugin.Result{}, err
	}
	return plugin.Result{Files: files}, nil
}

// stdinOf is a run's stdin as its bytes, nil for none: text or base64, never
// both and never neither, and at most plugin.MaxStdin bytes, judged before
// anything starts.
func stdinOf(in *plugin.Bytes) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	data, err := bytesOf(*in)
	if err != nil {
		return nil, fmt.Errorf("the stdin of the step: %w", err)
	}
	if len(data) > plugin.MaxStdin {
		return nil, fmt.Errorf("the stdin of the step holds %d bytes, more than the %d a run is given: %w", len(data), plugin.MaxStdin, port.ErrRefused)
	}
	return data, nil
}

// bytesOf is the bytes of a contract's Bytes: its text, or its base64
// decoded. One that is both or neither, or base64 that does not decode, is
// refused, matching port.ErrRefused.
func bytesOf(b plugin.Bytes) ([]byte, error) {
	switch {
	case (b.Text == nil) == (b.Base64 == nil):
		return nil, fmt.Errorf("bytes are text or base64, one of the two: %w", port.ErrRefused)
	case b.Text != nil:
		return []byte(*b.Text), nil
	}
	data, err := base64.StdEncoding.DecodeString(*b.Base64)
	if err != nil {
		return nil, fmt.Errorf("the base64 does not decode: %w: %w", port.ErrRefused, err)
	}
	return data, nil
}

// placed is one file a run places, its path in the system's form below the
// scratch folder and its bytes.
type placed struct {
	path string
	data []byte
}

// placesOf is the files a run places, judged before anything is written:
// each path as stepPath holds it, each file at most plugin.MaxFile bytes and
// all of them at most plugin.MaxFiles, the caps a read is held to.
func placesOf(files []plugin.File) ([]placed, error) {
	out := make([]placed, 0, len(files))
	total := 0
	for _, f := range files {
		local, err := stepPath(f.Path)
		if err != nil {
			return nil, err
		}
		data, err := bytesOf(f.Data)
		if err != nil {
			return nil, fmt.Errorf("the file %s the step places: %w", f.Path, err)
		}
		total += len(data)
		if len(data) > plugin.MaxFile || total > plugin.MaxFiles {
			return nil, fmt.Errorf("the file %s the step places takes its files past %d bytes, or itself past %d: %w", f.Path, plugin.MaxFiles, plugin.MaxFile, port.ErrRefused)
		}
		out = append(out, placed{path: local, data: data})
	}
	return out, nil
}

// placeAll writes each file below root, its missing folders made private and
// no link followed (confine), and never over a file that is there: the
// mirrored config set is never replaced by a step.
func placeAll(root *os.Root, files []placed) error {
	for _, p := range files {
		if err := placeOne(root, p); err != nil {
			return fmt.Errorf("the file %s the step places: %w", filepath.ToSlash(p.path), err)
		}
	}
	return nil
}

// placeOne is one file of placeAll.
func placeOne(root *os.Root, p placed) (err error) {
	dir := root
	if d := filepath.Dir(p.path); d != "." {
		sub, err := confine.MkdirPrivate(root, d)
		if err != nil {
			return outside(err)
		}
		defer func() { err = errors.Join(err, sub.Close()) }()
		dir = sub
	}
	f, err := confine.Open(dir, filepath.Base(p.path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("a file is there already, and a step places a file only where none is: %w: %w", port.ErrRefused, err)
	}
	if err != nil {
		return outside(err)
	}
	_, err = f.Write(p.data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// outside is err, matching port.ErrRefused too when it is confine's refusal
// of a path that leads out of the scratch folder or through a link.
func outside(err error) error {
	if errors.Is(err, confine.ErrOutside) {
		return fmt.Errorf("it leads out of the scratch folder or through a link: %w: %w", port.ErrRefused, err)
	}
	return err
}

// collect is the files a run named to be read back once it ended, held to
// the caps of a read. A path that is no step path is refused; a file the run
// did not write is left out, so the plugin reads what is there.
func collect(root *os.Root, paths []string) ([]plugin.File, error) {
	files := make([]plugin.File, 0, len(paths))
	total := 0
	for _, p := range paths {
		local, err := stepPath(p)
		if err != nil {
			return nil, err
		}
		data, err := readCapped(root, local, min(plugin.MaxFile, plugin.MaxFiles-total))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		total += len(data)
		files = append(files, plugin.File{Path: p, Data: stepBytes(data)})
	}
	return files, nil
}
