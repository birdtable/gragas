// Command gragas-crash is the child the crash-injection journey kills
// (store.recovery.2, A37, architecture 5.2 and 13). It runs one scripted plan
// through internal/apply over the real host/fs (apply, recover), or one
// writing command of the tree as cmd/gragas wires it (run, gragas#340), with
// the writer and the journal counted; it numbers every boundary between two
// effects, and at the one it is told stops there, says so and waits to be
// killed: the journey kills it with SIGKILL, starts it again to recover, and
// holds the root to "before, or after with a closed record". It is built by
// the journeys and shipped nowhere, and it is not in the landing check's
// closure.
//
// main is the only place that exits (go.errors.4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/clock"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

const usage = `usage: gragas-crash apply -root dir -state dir -run id [-files n] [-stop n] [-plant record-first]
       gragas-crash recover -root dir -state dir -run id [-stop n]
       gragas-crash records -root dir -state dir
       gragas-crash run [-stop n] [-at unix] [-plant intent-late] -- <gragas arguments>

  apply    run the scripted plan over n data files (3 when not given); with
           -stop n, say "stopped n <stage>" at boundary n and wait to be
           killed; with no -stop, say each boundary and its stage once the
           run ends; -plant record-first appends the record of the yes before
           the run, outside the write path, as a planted bug
  recover  run a write with nothing to change, which rolls back every open
           record of the root first, and say what it did: a "word", a
           "recovered" line for each run it settled, a "failure" line when
           it failed, with its cause on stderr, and a "point" line for each
           boundary of the roll-back; with -stop n, say "stopped n
           rolling-back" at boundary n of the roll-back and wait to be killed
  records  say each run of the root's journal as the Journal port answers
           it, the runs journal list lists: a "run <id> <state>" line each,
           oldest first, where the state is open or the outcome its close
           records
`

func main() {
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system (gragas#1614).
	os.Exit(term.Main(context.Background(), "gragas-crash", os.Stderr, func(ctx context.Context, args []string) int {
		if len(args) > 0 && args[0] == "run" {
			return runTree(ctx, args[1:], os.Getenv, os.Stdout, os.Stderr)
		}
		return run(ctx, args, os.Stdout, os.Stderr)
	}))
}

// say writes lines to one stream and keeps the first error, which ends the
// run failed.
type say struct {
	w   io.Writer
	err error
}

func (s *say) line(format string, args ...any) {
	if s.err == nil {
		_, s.err = fmt.Fprintf(s.w, format+"\n", args...)
	}
}

// run is the program with its streams.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	out, errs := &say{w: stdout}, &say{w: stderr}
	code := runWith(ctx, args, out, errs)
	if code == exit.Done && out.err != nil {
		return exit.Failed
	}
	return code
}

// A child is one run of the program: its ports over the root, the run id,
// the boundaries, and its streams.
type child struct {
	p         port.Ports
	root      model.Root
	id        string
	b         *bounds
	out, errs *say
}

func runWith(ctx context.Context, args []string, out, errs *say) int {
	if len(args) == 0 {
		errs.line("%s", strings.TrimSuffix(usage, "\n"))
		return exit.Usage
	}
	fl := flag.NewFlagSet("gragas-crash "+args[0], flag.ContinueOnError)
	fl.SetOutput(errs.w)
	dir := fl.String("root", "", "the root the plan writes")
	state := fl.String("state", "", "the state folder: the lock and the journal")
	id := fl.String("run", "", "the run id")
	at := fl.Int("stop", -1, "the boundary to stop at")
	plant := fl.String("plant", "", "a planted bug: record-first")
	files := fl.Int("files", 3, "how many data files the plan writes")
	if err := fl.Parse(args[1:]); err != nil || *dir == "" || *state == "" || (*id == "" && args[0] != "records") || (*plant != "" && *plant != "record-first") || *files < 1 || *files > 200 {
		errs.line("%s", strings.TrimSuffix(usage, "\n"))
		return exit.Usage
	}
	tree := &hostfs.Tree{}
	root, err := tree.Open(*dir)
	if err != nil {
		errs.line("gragas-crash: %v", err)
		return exit.Failed
	}
	b := &bounds{stop: *at, out: out, data: DataPaths(*files), record: recordPath}
	clk, w := clock.Clock{}, hostfs.Writer{Tree: tree}
	lock := &hostfs.Lock{Dir: *state, Clock: clk, Tree: tree}
	plugins, trust, bundles := pluginHost(tree, w, clk, *state, nil)
	c := child{
		p: port.Ports{
			Tree: tree, Owned: tree, FileNames: tree, Clock: clk,
			Writer:  countingWriter{Writer: w, b: b},
			Lock:    lock,
			Plugins: plugins, Trust: trust, Bundles: bundles,
			Journal: countingJournal{Journal: hostfs.Journal{Tree: tree, Writer: w, Clock: clk, Dir: *state, Lock: lock}, b: b},
		},
		root: root, id: *id, b: b, out: out, errs: errs,
	}
	switch args[0] {
	case "apply":
		if *plant == "record-first" {
			// The planted bug appends the record of the yes before the run
			// begins, outside the journal and the count, as a write path that
			// wrote the record first would.
			if err := w.Append(root, recordPath, []byte(`{"plan":"planted"}`)); err != nil {
				errs.line("gragas-crash: %v", err)
				return exit.Failed
			}
		}
		return c.applyScript(ctx)
	case "recover":
		b.phase = stageRollingBack
		return c.recoverRoot(ctx)
	case "records":
		return c.records(ctx)
	}
	errs.line("%s", strings.TrimSuffix(usage, "\n"))
	return exit.Usage
}

// recordPath is the file the record of the yes is appended to.
const recordPath = "history.jsonl"

// DataPaths are the plan's n data files, in the order they are written:
// every third one, from the second, made under state/, the others rules
// replaced. The journey seeds each rule with Before.
func DataPaths(n int) []string {
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

// Before is what a rule holds before the plan, and After what every data
// file holds once it is applied.
const (
	Before = "status: active\n"
	After  = "status: retired\n"
)

// applyScript runs the scripted plan.
func (c child) applyScript(ctx context.Context) int {
	digest := func(path string) (model.Digest, error) {
		data, err := c.p.Tree.Read(c.root, path)
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		return apply.DigestBytes(data), nil
	}
	o := apply.Run(ctx, c.p, c.root, apply.Spec{
		Command: "gragas-crash apply", Engine: "v0.0.0-crash", Run: c.id,
		Scope: []string{"rules/*.yaml", "state/*.yaml", recordPath},
		Plan: func(context.Context) (model.Plan, error) {
			return model.Plan{
				Delta: model.Delta{Changes: []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}},
				Yes:   &model.Yes{Who: "A Person <a@example.test>", At: 1794132000, Command: "gragas-crash apply"},
			}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) {
			var out []model.Target
			for _, path := range c.b.data {
				before, err := digest(path)
				if err != nil {
					return nil, err
				}
				out = append(out, model.Target{Path: path, Before: before, Data: []byte(After)})
			}
			return out, nil
		},
		Record: func(y model.Yes) (model.Target, error) {
			before, err := digest(recordPath)
			return model.Target{Path: recordPath, Before: before, Data: []byte(`{"plan":"` + string(y.Plan) + `"}`), Append: true}, err
		},
	})
	if o.Word != model.Done {
		c.errs.line("gragas-crash: ended %s: %v %v", o.Word, o.Failure, o.Cause)
		return exit.Failed
	}
	for i, s := range c.b.stages {
		c.out.line("point %d %s", i, s)
	}
	return exit.Done
}

// records says each run of the root's journal as coreapi.JournalList reads
// it through the Journal port, as journal list does, so the journey judges a
// run by the engine's reader and never by the fields of a line, and the
// form of a line can move without blinding it (gragas#1943). A journal the
// port refuses ends the run failed, with the refusal on stderr.
func (c child) records(ctx context.Context) int {
	l, err := coreapi.JournalList(ctx, c.p, c.root)
	if err != nil {
		c.errs.line("gragas-crash: %v", err)
		return exit.Failed
	}
	for _, r := range l.Runs {
		c.out.line("run %s %s", r.Run, r.State)
	}
	return exit.Done
}

// recoverRoot runs a write whose plan changes nothing: the write path rolls
// back every open record of the root before it plans (A37), and ends done,
// nothing to do. It says the word, each run it settled, and the kind of a
// failure, one line each, and the cause of a failure on stderr.
func (c child) recoverRoot(ctx context.Context) int {
	o := apply.Run(ctx, c.p, c.root, apply.Spec{
		Command: "gragas-crash recover", Engine: "v0.0.0-crash", Run: c.id, Scope: []string{"rules/*.yaml"},
		Plan:   func(context.Context) (model.Plan, error) { return model.Plan{}, nil },
		Render: func(model.Delta) ([]model.Target, error) { return nil, nil },
	})
	c.out.line("word %s", o.Word)
	for _, rc := range o.Recovered {
		c.out.line("recovered %s", rc.Run)
	}
	if o.Failure != nil {
		c.out.line("failure %s", o.Failure.Kind())
	}
	for i, s := range c.b.stages {
		c.out.line("point %d %s", i, s)
	}
	if o.Cause != nil {
		c.errs.line("gragas-crash: recover ended %s: %v", o.Word, o.Cause)
	}
	if o.Word != model.Done {
		return exit.Failed
	}
	return exit.Done
}

// bounds numbers the boundaries between effects: one before and one after
// every journal record, kept copy and file write, each with the stage of the
// run it falls in.
type bounds struct {
	stop   int
	out    *say
	data   []string
	record string
	stages []string
	// intent, written and recorded are how far the run has gone.
	intent, recorded bool
	written          int
	// phase, when it is set, is the stage of every boundary: a recovery's
	// are all of its roll-back.
	phase string
	// open is a run whose data files are not known ahead, the run mode's:
	// every write past the first that is not the record falls between files.
	open bool
	// intentLate is the planted bug of the run mode: a write path that
	// journals nothing, its intent, its other records or its copies, before
	// it writes;
	// heldIntent is that the intent was held back, so the run stops for the
	// kill once its first file is written.
	intentLate, heldIntent bool
}

// The five stages a boundary of the plan falls in, and the one of a
// recovery.
const (
	stageRollingBack  = "rolling-back"
	stageBeforeIntent = "before-intent"
	stageAfterIntent  = "after-intent"
	stageBetween      = "between-files"
	stageBeforeRecord = "before-record"
	stageAfterRecord  = "after-record"
)

func (b *bounds) stage() string {
	switch {
	case b.phase != "":
		return b.phase
	case !b.intent:
		return stageBeforeIntent
	case b.recorded:
		return stageAfterRecord
	case b.written == 0:
		return stageAfterIntent
	case b.open || b.written < len(b.data):
		return stageBetween
	}
	return stageBeforeRecord
}

// at is one boundary. At the one to stop at, it says so and waits for the
// kill: the process never goes past it, unless the line could not be
// written.
func (b *bounds) at() {
	n := len(b.stages)
	b.stages = append(b.stages, b.stage())
	if n != b.stop {
		return
	}
	b.out.line("stopped %d %s", n, b.stages[n])
	if b.out.err != nil {
		return // nobody heard it stop, so it runs on and ends
	}
	for {
		time.Sleep(time.Hour)
	}
}

// wrote moves the run on once a file is written.
func (b *bounds) wrote(path string) {
	if path == b.record || (b.open && isRecord(path)) {
		b.recorded = true
		return
	}
	b.written++
}

type countingWriter struct {
	port.Writer
	b *bounds
}

func (c countingWriter) Replace(r model.Root, path string, data []byte) error {
	return c.write(path, func() error { return c.Writer.Replace(r, path, data) })
}

func (c countingWriter) Append(r model.Root, path string, line []byte) error {
	return c.write(path, func() error { return c.Writer.Append(r, path, line) })
}

// write is a step on each side of one write of path, which moves the run on
// once it is written.
func (c countingWriter) write(path string, w func() error) error {
	c.b.at()
	err := w()
	if err == nil {
		c.b.wrote(path)
	}
	if err == nil && c.b.heldIntent {
		c.b.plantedStop()
	}
	c.b.at()
	return err
}

func (c countingWriter) Remove(r model.Root, path string) error {
	c.b.at()
	err := c.Writer.Remove(r, path)
	c.b.at()
	return err
}

type countingJournal struct {
	port.Journal
	b *bounds
}

func (c countingJournal) Append(ctx context.Context, r model.Root, rec model.Record) error {
	c.b.at()
	if c.b.intentLate {
		// The planted bug journals nothing of the run, its intent and every
		// other record held back, so the first write lands with nothing to
		// roll it back from.
		c.b.intent, c.b.heldIntent = true, true
		c.b.at()
		return nil
	}
	err := c.Journal.Append(ctx, r, rec)
	if err == nil && rec.Kind == model.KindIntent {
		c.b.intent = true
	}
	c.b.at()
	return err
}

func (c countingJournal) Keep(r model.Root, run, path string, old []byte) error {
	c.b.at()
	if c.b.intentLate {
		// The planted bug journals no copy either: nothing stands before the
		// first write that a recovery could roll it back from.
		c.b.at()
		return nil
	}
	err := c.Journal.Keep(r, run, path, old)
	c.b.at()
	return err
}

// plantedStop is the planted run's stop: its first file is written and its
// intent never journalled. It says "stopped n planted" and waits to be
// killed.
func (b *bounds) plantedStop() {
	b.out.line("stopped %d planted", len(b.stages))
	if b.out.err != nil {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}
