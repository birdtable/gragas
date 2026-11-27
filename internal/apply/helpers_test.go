package apply_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/canon"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// The run id every test files its journal under.
const runID = "run-1"

// instant is what the fake clock holds: the lock's start time is this.
func instant() time.Time { return time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC) }

// A spy is the call log of the ports a run writes through, in order: the
// order of the calls is what the write path promises (cli.commands.23,
// store.consistency.4), and the fake keeps one log per port.
type spy struct {
	log   []string
	inner port.Ports
}

// newSpy wraps the lock, journal, writer and tree of w, and answers the ports
// to hand Run.
func newSpy(w *porttest.World) (*spy, port.Ports) {
	s := &spy{inner: w.Ports()}
	p := s.inner
	p.Lock, p.Journal, p.Writer, p.Tree = spyLock{s}, spyJournal{s}, spyWriter{s}, spyTree{s}
	return s, p
}

func (s *spy) add(format string, args ...any) { s.log = append(s.log, fmt.Sprintf(format, args...)) }

type spyLock struct{ s *spy }

func (x spyLock) Take(ctx context.Context, r model.Root, w model.Lock) (model.Hold, error) {
	h, err := x.s.inner.Lock.Take(ctx, r, w)
	x.s.add("take %s", w)
	return h, err
}

func (x spyLock) Wait(ctx context.Context, r model.Root, w model.Lock) (model.Hold, error) {
	h, err := x.s.inner.Lock.Wait(ctx, r, w)
	x.s.add("wait %s", w)
	return h, err
}

func (x spyLock) Release(h model.Hold) error {
	x.s.add("release %s", h.Which)
	return x.s.inner.Lock.Release(h)
}

type spyJournal struct{ s *spy }

func (x spyJournal) Append(ctx context.Context, r model.Root, rec model.Record) error {
	x.s.add("journal %s", rec.Kind)
	return x.s.inner.Journal.Append(ctx, r, rec)
}

func (x spyJournal) Keep(r model.Root, run, p string, old []byte) error {
	x.s.add("keep %s", p)
	return x.s.inner.Journal.Keep(r, run, p, old)
}

func (x spyJournal) Kept(r model.Root, run, p string) ([]byte, error) {
	return x.s.inner.Journal.Kept(r, run, p)
}

func (x spyJournal) KeptAt(r model.Root, run, p string) (string, error) {
	return x.s.inner.Journal.KeptAt(r, run, p)
}

func (x spyJournal) Records(ctx context.Context, r model.Root) ([]model.Record, int, error) {
	return x.s.inner.Journal.Records(ctx, r)
}

func (x spyJournal) RecordsOf(ctx context.Context, data []byte) ([]model.Record, int, error) {
	return x.s.inner.Journal.RecordsOf(ctx, data)
}

func (x spyJournal) Sweep(ctx context.Context, r model.Root) (int, error) {
	x.s.add("sweep")
	return x.s.inner.Journal.Sweep(ctx, r)
}

type spyWriter struct{ s *spy }

func (x spyWriter) Replace(r model.Root, p string, data []byte) error {
	x.s.add("replace %s", p)
	return x.s.inner.Writer.Replace(r, p, data)
}

func (x spyWriter) Append(r model.Root, p string, line []byte) error {
	x.s.add("append %s", p)
	return x.s.inner.Writer.Append(r, p, line)
}

func (x spyWriter) Remove(r model.Root, p string) error {
	x.s.add("remove %s", p)
	return x.s.inner.Writer.Remove(r, p)
}

type spyTree struct{ s *spy }

func (x spyTree) Read(r model.Root, p string) ([]byte, error) {
	x.s.add("read %s", p)
	return x.s.inner.Tree.Read(r, p)
}

func (x spyTree) ReadSoleWithin(r model.Root, p string, max int64) ([]byte, error) {
	x.s.add("read %s", p)
	return x.s.inner.Tree.ReadSoleWithin(r, p, max)
}

func (x spyTree) Open(d string) (model.Root, error)                { return x.s.inner.Tree.Open(d) }
func (x spyTree) Locate(a, m string) (model.Root, error)           { return x.s.inner.Tree.Locate(a, m) }
func (x spyTree) Stat(r model.Root, p string) (model.Entry, error) { return x.s.inner.Tree.Stat(r, p) }
func (x spyTree) Within(r model.Root, a string) (string, error)    { return x.s.inner.Tree.Within(r, a) }
func (x spyTree) Written(r model.Root, a string) (string, error)   { return x.s.inner.Tree.Written(r, a) }
func (x spyTree) OpenIn(r model.Root, d string) (model.Root, error) {
	return x.s.inner.Tree.OpenIn(r, d)
}
func (x spyTree) FS(r model.Root) (fs.FS, error) { return x.s.inner.Tree.FS(r) }
func (x spyTree) List(r model.Root, d string, sc model.Scope) ([]model.Entry, error) {
	return x.s.inner.Tree.List(r, d, sc)
}

// An edit is one file of the test-only plan type: the content it should hold,
// replaced whole, or the line it should end with, appended.
type edit struct {
	path, data string
	appendLine bool
	// drop takes the file away: its goal is that it is not there.
	drop bool
}

// A testPlan is the test-only plan type that drives Run (no command uses it).
// It edits files of a root and reads the others it names as inputs. An edit
// whose file already holds its goal is left out of the delta, so a plan of
// edits that all hold is the empty delta, as a retire of a retired rule is.
type testPlan struct {
	w      *porttest.World
	root   model.Root
	inputs []string
	edits  []edit
	// yes, when set, adds a record appended to history.jsonl.
	yes bool
	// moved runs after the plan read the tree and before it answers: the
	// tree moving under the plan.
	moved func()
	// at, when set, is the time of the yes the plan holds; the default is instant().
	at func() time.Time
}

func (tp testPlan) now() time.Time {
	if tp.at != nil {
		return tp.at()
	}
	return instant()
}

func (tp testPlan) holds(e edit) bool {
	cur, ok := tp.w.File(tp.root, e.path)
	if e.drop {
		return !ok
	}
	if e.appendLine {
		return ok && (strings.HasSuffix(cur, e.data+"\n"))
	}
	return ok && cur == e.data
}

func (tp testPlan) digest(p string) model.Digest {
	cur, ok := tp.w.File(tp.root, p)
	if !ok {
		return ""
	}
	return apply.DigestBytes([]byte(cur))
}

func (tp testPlan) plan(context.Context) (model.Plan, error) {
	var pl model.Plan
	paths := append([]string(nil), tp.inputs...)
	for _, e := range tp.edits {
		paths = append(paths, e.path)
		if !tp.holds(e) {
			pl.Delta.Changes = append(pl.Delta.Changes, model.Change{Entity: "file:" + e.path, Field: "content", After: e.data})
		}
	}
	if tp.yes {
		paths = append(paths, "history.jsonl")
		pl.Yes = &model.Yes{Who: "A Person <a@example.test>", At: tp.now().Unix(), Command: "gragas test --apply"}
	}
	for _, p := range paths {
		pl.Inputs = append(pl.Inputs, model.Input{Path: p, Digest: tp.digest(p)})
	}
	if tp.moved != nil {
		tp.moved()
	}
	return pl, nil
}

func (tp testPlan) render(d model.Delta) ([]model.Target, error) {
	var out []model.Target
	for _, e := range tp.edits {
		if tp.holds(e) {
			continue
		}
		out = append(out, model.Target{Path: e.path, Before: tp.digest(e.path), Data: []byte(e.data), Append: e.appendLine, Delete: e.drop})
	}
	return out, nil
}

func (tp testPlan) record(y model.Yes) (model.Target, error) {
	line := fmt.Sprintf(`{"who":%q,"at":%d,"plan":%q,"command":%q}`, y.Who, y.At, y.Plan, y.Command)
	return model.Target{Path: "history.jsonl", Before: tp.digest("history.jsonl"), Data: []byte(line), Append: true}, nil
}

// spec is the Spec that runs tp.
func (tp testPlan) spec() apply.Spec {
	s := apply.Spec{
		Command: "gragas test --apply", Engine: "v0.0.0-test", Run: runID,
		Scope: []string{"rules/*.yaml", "history.jsonl", "state/*"},
		Plan:  tp.plan, Render: tp.render,
	}
	if tp.yes {
		s.Record = tp.record
	}
	return s
}

// fixture is a rulebook root with the files the tests edit, a fixed clock, a
// spy and the ports to run through.
type fixture struct {
	t    *testing.T
	w    *porttest.World
	root model.Root
	spy  *spy
	p    port.Ports
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	w := porttest.NewWorld(t)
	w.Instant(instant())
	root := w.Seed("/work/rulebook", map[string]string{
		"identity.yaml":    "name: demo\n",
		"rules/a.yaml":     "status: active\n",
		"rules/b.yaml":     "status: active\n",
		"history.jsonl":    `{"n":1}` + "\n",
		"state/waive.yaml": "waivers: []\n",
	})
	s, p := newSpy(w)
	return &fixture{t: t, w: w, root: root, spy: s, p: p}
}

func (f *fixture) plan(edits ...edit) testPlan {
	return testPlan{w: f.w, root: f.root, edits: edits}
}

func (f *fixture) run(s apply.Spec) apply.Outcome {
	f.t.Helper()
	return apply.Run(f.t.Context(), f.p, f.root, s)
}

// records are the journal's lines of the fixture's root, as the fake kept them.
func (f *fixture) records() []model.Record {
	f.t.Helper()
	recs, _, err := f.inner().Journal.Records(f.t.Context(), f.root)
	if err != nil {
		f.t.Fatalf("Records: %v", err)
	}
	return recs
}

func (f *fixture) inner() port.Ports { return f.spy.inner }

// kinds is the kind of each record, in order.
func kinds(recs []model.Record) []string {
	var out []string
	for _, r := range recs {
		out = append(out, r.Kind)
	}
	return out
}

// content is the file at p now, "<absent>" when it is not there.
func (f *fixture) content(p string) string {
	c, ok := f.w.File(f.root, p)
	if !ok {
		return "<absent>"
	}
	return c
}

// wantDone fails the test unless the run ended done.
func wantDone(t *testing.T, o apply.Outcome) {
	t.Helper()
	if o.Word != model.Done {
		t.Fatalf("ended %q (refusal %v, failure %v, cause %v), want done", o.Word, o.Refusal, o.Failure, o.Cause)
	}
}

// a spent-error that a test's wrapped port answers.
var errBoom = errors.New("boom")

// killed is what a killingWriter panics with: the process dying there.
type killed struct{}

// A killingWriter writes through to its Writer and, once left writes have
// landed, panics, as a process killed right after a write: nothing of the
// run after that point happens, and the lock goes with the process.
type killingWriter struct {
	port.Writer
	left *int
}

func (k killingWriter) Replace(r model.Root, p string, data []byte) error {
	if err := k.Writer.Replace(r, p, data); err != nil {
		return err
	}
	k.tick()
	return nil
}

func (k killingWriter) Append(r model.Root, p string, line []byte) error {
	if err := k.Writer.Append(r, p, line); err != nil {
		return err
	}
	k.tick()
	return nil
}

func (k killingWriter) tick() {
	if *k.left--; *k.left == 0 {
		panic(killed{})
	}
}

// interrupt runs s as the run id run, killed once n files are written: its
// record stays open and the files it wrote stay written, as a process killed
// there leaves them.
func (f *fixture) interrupt(run string, n int, s apply.Spec) {
	f.t.Helper()
	p, left := f.p, n
	p.Writer = killingWriter{Writer: p.Writer, left: &left}
	s.Run = run
	defer func() {
		if v := recover(); v != nil && v != (killed{}) {
			panic(v)
		}
	}()
	apply.Run(f.t.Context(), p, f.root, s)
	f.t.Fatalf("run %s was not killed after %d writes", run, n)
}

// open is the run ids of the root's open records.
func (f *fixture) open() []string {
	f.t.Helper()
	open, err := apply.Open(f.t.Context(), f.inner(), f.root)
	if err != nil {
		f.t.Fatalf("Open: %v", err)
	}
	var ids []string
	for _, o := range open {
		ids = append(ids, o.Run)
	}
	return ids
}

// snapshot is every path of paths with what it holds now, "<absent>" for
// one that is not there.
func (f *fixture) snapshot(paths ...string) map[string]string {
	out := map[string]string{}
	for _, p := range paths {
		out[p] = f.content(p)
	}
	return out
}

// fixturePaths are the files newFixture seeds, and the one the tests make.
func fixturePaths() []string {
	return []string{"identity.yaml", "rules/a.yaml", "rules/b.yaml", "history.jsonl", "state/waive.yaml", "state/new.yaml"}
}

// closeOf is the detail of run's last close record, decoded.
func (f *fixture) closeOf(run string) canon.Object {
	f.t.Helper()
	var last []byte
	for _, r := range f.records() {
		if r.Run == run && r.Kind == model.KindClose {
			last = r.Detail
		}
	}
	if last == nil {
		f.t.Fatalf("run %s has no close record", run)
	}
	v, err := canon.Decode(last)
	if err != nil {
		f.t.Fatalf("the close of %s: %v", run, err)
	}
	o, ok := v.(canon.Object)
	if !ok {
		f.t.Fatalf("the close of %s is %T", run, v)
	}
	return o
}
