package root

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// boom is the panic the planted run raises.
const boom = "planted panic"

// panicRuns hands out a run record that is open and then panics at the first
// change, which is a write made under the lock.
type panicRuns struct {
	inner  legacy.Runs
	opened *panicRun
}

func (r *panicRuns) Begin(path string, argv []string, build string) legacy.Run {
	r.opened = &panicRun{Run: r.inner.Begin(path, argv, build)}
	return r.opened
}

type panicRun struct {
	legacy.Run
	finished   bool
	code       int
	word       string
	finishedAt int
}

func (r *panicRun) Change(string) { panic(boom) }

func (r *panicRun) Finish(code int, word, what string) {
	r.finished, r.code, r.word = true, code, word
	r.finishedAt++
	r.Run.Finish(code, word, what)
}

// TestAPanicEndsOnFailedWithTheTraceOnStderrAlone: a panic under the write
// lock is recovered once, at the root. The run ends on "failed" with the run
// id and a code that is never 0, the trace is on standard error and not in
// the answer, the lock is released once and the run record is closed.
// Risk: a crash a hook reads as a pass, or a lock held by a dead run.
func TestAPanicEndsOnFailedWithTheTraceOnStderrAlone(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, _ := process(t, f, "gen", "--apply")
	runs := &panicRuns{inner: p.Ports.Runs}
	p.Ports.Runs = runs
	code, sc := execute(t.Context(), t, p)
	out, diag := sc.output(), strings.Join(sc.diag.Said, "\n")
	if code == exit.Done || code != exit.Failed {
		t.Fatalf("exit %d, want %d (failed), never 0", code, exit.Failed)
	}
	if sc.closing() != model.Failed || !strings.Contains(out, "hit a bug and stopped") || !strings.Contains(out, "gen") {
		t.Errorf("the ending is %q, said:\n%s", sc.closing(), out)
	}
	if strings.Contains(out, "goroutine") || !strings.Contains(diag, "goroutine") || !strings.Contains(diag, boom) {
		t.Errorf("the trace belongs on stderr alone; stdout:\n%s\nstderr:\n%s", out, diag)
	}
	if f.Locked != 1 || f.Released != 1 {
		t.Errorf("locked %d, released %d; want the one lock released once", f.Locked, f.Released)
	}
	if r := runs.opened; r == nil || !r.finished || r.finishedAt != 1 || r.code != exit.Failed || r.word != string(model.Failed) {
		t.Errorf("the run record = %+v, want closed once on failed", runs.opened)
	}
}

// TestAPanicUnderJSONIsOneDocument: under --json the answer is the one
// document and says failed; the trace stays on stderr.
func TestAPanicUnderJSONIsOneDocument(t *testing.T) {
	t.Parallel()
	f := world(t)
	p, out := process(t, f, "gen", "--apply", "--json")
	runs := &panicRuns{inner: p.Ports.Runs}
	p.Ports.Runs = runs
	code, sc := execute(t.Context(), t, p)
	if code != exit.Failed {
		t.Fatalf("exit %d, want %d", code, exit.Failed)
	}
	var doc struct {
		Ended string `json:"ended"`
		Run   string `json:"run"`
		Error *struct {
			Kind string `json:"kind"`
			Msg  string `json:"msg"`
		} `json:"error"`
	}
	dec := json.NewDecoder(strings.NewReader(out.String()))
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not a document: %v\n%s", err, out)
	}
	if dec.More() {
		t.Errorf("stdout holds more than one document:\n%s", out)
	}
	if doc.Ended != string(model.Failed) || doc.Error == nil || doc.Error.Kind != kindFailed || !strings.Contains(doc.Error.Msg, "hit a bug") {
		t.Errorf("document = %+v\n%s", doc, out)
	}
	if strings.Contains(out.String(), "goroutine") || !strings.Contains(strings.Join(sc.diag.Said, "\n"), "goroutine") {
		t.Errorf("the trace belongs on stderr alone; stdout:\n%s", out)
	}
	if f.Locked != 1 || f.Released != 1 || runs.opened == nil || runs.opened.finishedAt != 1 {
		t.Errorf("locked %d, released %d, record %+v", f.Locked, f.Released, runs.opened)
	}
}

// crashingTree is a tree whose opening panics: a fault of guard edit's own,
// one that no error says.
type crashingTree struct{ port.Tree }

func (crashingTree) Open(string) (model.Root, error) { panic(boom) }

// TestAPanicInGuardEditEndsRefusedNeverFailed: guard edit carries
// kit.FailsClosed, so its panic, recovered once at the root, ends refused,
// exit 5, with the line to type; the trace and the two lines the hook hands
// the agent are on standard error, and under --json the answer is one
// document that says refused (Q6).
// Risk: a crashed guard that an agent's hook reads as a pass.
func TestAPanicInGuardEditEndsRefusedNeverFailed(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"guard", "edit"}, {"guard", "edit", "--json"}} {
		p, out := process(t, world(t), args...)
		p, sc := attach(t, p)
		root := seededRoot(sc)
		p.World.Tree = crashingTree{p.World.Tree}
		code := Execute(t.Context(), p)
		diag := strings.Join(sc.diag.Said, "\n")
		if code != exit.Refused {
			t.Fatalf("%v: exit %d, want %d (refused), never 0 or failed", args, code, exit.Refused)
		}
		// The crash is in Open, so the guard has no root to journal in: no
		// line, and standard error says why (as ruled).
		if !strings.Contains(diag, "No refusal is journalled: the crash came before the rulebook's root was opened") {
			t.Errorf("%v: standard error does not say why nothing is journalled:\n%s", args, diag)
		}
		if recs := crashRecords(t, sc, root); len(recs) != 0 {
			t.Errorf("%v: a crash before the root was open wrote %d journal lines", args, len(recs))
		}
		if !strings.Contains(diag, boom) || !strings.Contains(diag, "goroutine") ||
			!strings.Contains(diag, "refused: `gragas guard edit` hit a bug and stopped: "+boom+", so what it was asked is refused") ||
			!strings.Contains(diag, "instead: report the fault") {
			t.Errorf("%v: standard error:\n%s", args, diag)
		}
		if len(args) == 2 {
			if said := sc.output(); sc.closing() != model.Refused || !strings.Contains(said, "report the fault") || strings.Contains(said, "goroutine") {
				t.Errorf("the ending is %q, said:\n%s", sc.closing(), said)
			}
			continue
		}
		var doc struct {
			Ended string `json:"ended"`
		}
		dec := json.NewDecoder(strings.NewReader(out.String()))
		if err := dec.Decode(&doc); err != nil || dec.More() || doc.Ended != string(model.Refused) || strings.Contains(out.String(), "goroutine") {
			t.Errorf("under --json: %v, %+v\n%s", err, doc, out)
		}
	}
}

// TestOnlyTheGuardsFailClosed: the annotation that turns a crash into a
// refusal is on guard edit and guard staged and on no other command, so no
// other crash hides as a no.
func TestOnlyTheGuardsFailClosed(t *testing.T) {
	t.Parallel()
	var found []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if _, ok := c.Annotations[kit.FailsClosed]; ok {
			found = append(found, c.CommandPath())
		}
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(tree(t))
	if want := []string{"gragas guard edit", "gragas guard staged"}; !slices.Equal(found, want) {
		t.Errorf("fails-closed on %v, want only %v", found, want)
	}
}

// TestNoCommandRecoversBesideTheRoot: the tree has one recover, in this
// package's Execute; a command never recovers on its own.
func TestNoCommandRecoversBesideTheRoot(t *testing.T) {
	t.Parallel()
	top := filepath.Join("..", "..", "..")
	var sites []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(top, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			src, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(src, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "recover" {
						rel, _ := filepath.Rel(top, p)
						sites = append(sites, filepath.ToSlash(rel))
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(sites) != 1 || sites[0] != "internal/cli/root/root.go" {
		t.Errorf("recover() is called in %v, want only internal/cli/root/root.go", sites)
	}
}

// openedTree is a tree that opens its root and panics at the first read
// after: a fault of guard edit's own, once the guard has a root to journal in.
type openedTree struct{ port.Tree }

func (openedTree) Locate(string, string) (model.Root, error)                   { panic(boom) }
func (openedTree) Read(model.Root, string) ([]byte, error)                     { panic(boom) }
func (openedTree) Stat(model.Root, string) (model.Entry, error)                { panic(boom) }
func (openedTree) List(model.Root, string, model.Scope) ([]model.Entry, error) { panic(boom) }
func (openedTree) OpenIn(model.Root, string) (model.Root, error)               { panic(boom) }
func (openedTree) FS(model.Root) (fs.FS, error)                                { panic(boom) }

// seededRoot is the working folder of an attached run, seeded with one
// rulebook file, so a command can open it as its root, and the instant the
// guard names a run of its own by.
func seededRoot(sc screens) model.Root {
	sc.out.Instant(time.Unix(clock, 0))
	return sc.out.Seed(".", map[string]string{"rulebook/parts.yaml": "part: x\n"})
}

// crashRecords is the journal of root after a run.
func crashRecords(t *testing.T, sc screens, root model.Root) []model.Record {
	t.Helper()
	recs, torn, err := sc.out.Ports().Journal.Records(t.Context(), root)
	if err != nil || torn != 0 {
		t.Fatalf("the journal reads %d torn, %v", torn, err)
	}
	return recs
}

// TestAPanicInGuardEditJournalsItsRefusalOnceTheRootIsOpen: a guard
// edit that crashes after it opened the rulebook's root ends refused, exit 5,
// with one journal line: a close under the guard's run, outcome refused, the
// cause a crash, the command named, and no word of the panic's own text.
// Risk: a crashed guard that refuses the edit and keeps the refusal nowhere
// but the agent's standard error.
func TestAPanicInGuardEditJournalsItsRefusalOnceTheRootIsOpen(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"guard", "edit"}, {"guard", "edit", "--json"}} {
		p, _ := process(t, world(t), args...)
		p, sc := attach(t, p)
		root := seededRoot(sc)
		p.World.Tree = openedTree{p.World.Tree}
		if code := Execute(t.Context(), p); code != exit.Refused {
			t.Fatalf("%v: exit %d, want %d (refused)", args, code, exit.Refused)
		}
		if diag := strings.Join(sc.diag.Said, "\n"); strings.Contains(diag, "No refusal is journalled") || strings.Contains(diag, "could not be journalled") {
			t.Errorf("%v: standard error says the refusal is not kept:\n%s", args, diag)
		}
		recs := crashRecords(t, sc, root)
		if len(recs) != 1 {
			t.Fatalf("%v: %d journal lines, want the one refusal", args, len(recs))
		}
		r := recs[0]
		var d struct{ Outcome, Cause, Why string }
		if err := json.Unmarshal(r.Detail, &d); err != nil {
			t.Fatal(err)
		}
		if r.Kind != model.KindClose || r.Command != "gragas guard edit" || !strings.HasPrefix(r.Run, "guard-") ||
			d.Outcome != "refused" || d.Cause != "crash" || !strings.Contains(d.Why, "gragas guard edit") {
			t.Errorf("%v: the journal line is %+v, detail %+v", args, r, d)
		}
		if strings.Contains(string(r.Detail), boom) {
			t.Errorf("%v: the journal line holds the panic's text: %s", args, r.Detail)
		}
	}
}

// crashingHistory is a repository whose first read panics: a fault of guard
// staged's own, after its root is open.
type crashingHistory struct{ port.History }

func (crashingHistory) Files(context.Context, model.Root, string) ([]model.Entry, error) {
	panic(boom)
}

// TestAPanicInGuardStagedJournalsItsRefusalOnceTheRootIsOpen: guard staged fails closed as guard edit does. A crash after it
// opened the rulebook's root ends refused, exit 5, with one journal line, a
// close under the guard's run that names guard staged, outcome refused, the
// cause a crash and no word of the panic's own text; a crash before the root
// is open journals nothing and says so.
// Risk: a crashed commit hook that refuses the commit and keeps the refusal
// nowhere, or keeps it under guard edit's name.
func TestAPanicInGuardStagedJournalsItsRefusalOnceTheRootIsOpen(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"guard", "staged"}, {"guard", "staged", "--json"}} {
		p, _ := process(t, world(t), args...)
		p, sc := attach(t, p)
		root := seededRoot(sc)
		p.History = func(string) (port.History, error) { return crashingHistory{}, nil }
		if code := Execute(t.Context(), p); code != exit.Refused {
			t.Fatalf("%v: exit %d, want %d (refused)", args, code, exit.Refused)
		}
		diag := strings.Join(sc.diag.Said, "\n")
		if !strings.Contains(diag, boom) || strings.Contains(diag, "No refusal is journalled") || strings.Contains(diag, "could not be journalled") {
			t.Errorf("%v: the crash is not the planted one, or the refusal is not kept:\n%s", args, diag)
		}
		recs := crashRecords(t, sc, root)
		if len(recs) != 1 {
			t.Fatalf("%v: %d journal lines, want the one refusal", args, len(recs))
		}
		r := recs[0]
		var d struct{ Outcome, Cause, Why string }
		if err := json.Unmarshal(r.Detail, &d); err != nil {
			t.Fatal(err)
		}
		if r.Kind != model.KindClose || r.Command != "gragas guard staged" || !strings.HasPrefix(r.Run, "guard-") ||
			d.Outcome != "refused" || d.Cause != "crash" || !strings.Contains(d.Why, "gragas guard staged") || strings.Contains(d.Why, "guard edit") {
			t.Errorf("%v: the journal line is %+v, detail %+v", args, r, d)
		}
		if strings.Contains(string(r.Detail), boom) {
			t.Errorf("%v: the journal line holds the panic's text: %s", args, r.Detail)
		}
	}

	p, _ := process(t, world(t), "guard", "staged")
	p, sc := attach(t, p)
	root := seededRoot(sc)
	p.World.Tree = crashingTree{p.World.Tree}
	if code := Execute(t.Context(), p); code != exit.Refused {
		t.Fatalf("a crash before the root: exit %d, want %d (refused)", code, exit.Refused)
	}
	if diag := strings.Join(sc.diag.Said, "\n"); !strings.Contains(diag, "No refusal is journalled: the crash came before the rulebook's root was opened") {
		t.Errorf("standard error does not say why nothing is journalled:\n%s", diag)
	}
	if recs := crashRecords(t, sc, root); len(recs) != 0 {
		t.Errorf("a crash before the root was open wrote %d journal lines", len(recs))
	}
}

// TestAPanicOfACommandThatDoesNotFailClosedJournalsNothing: a crash of
// a command with no kit.FailsClosed ends failed, exit 1, and writes no
// journal line, so only a fail-closed command's crash is a refusal kept.
func TestAPanicOfACommandThatDoesNotFailClosedJournalsNothing(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "journal", "list")
	p, sc := attach(t, p)
	root := seededRoot(sc)
	p.World.Tree = crashingTree{p.World.Tree}
	if code := Execute(t.Context(), p); code != exit.Failed || exit.Failed != 1 {
		t.Fatalf("exit %d, want 1 (failed)", code)
	}
	if said := strings.Join(sc.diag.Said, "\n"); !strings.Contains(said, boom) {
		t.Errorf("the crash is not the planted one; standard error:\n%s", said)
	}
	if recs := crashRecords(t, sc, root); len(recs) != 0 {
		t.Errorf("a crash that is not a refusal wrote %d journal lines: %+v", len(recs), recs)
	}
}

// failingHistory is a repository whose reads fail, as a damaged object store
// or a git that will not start answers.
type failingHistory struct{ port.History }

func (failingHistory) Files(context.Context, model.Root, string) ([]model.Entry, error) {
	return nil, errors.New("the object store is damaged")
}

// TestAHistoryGuardStagedCannotReadIsRefusedAndJournalled: a
// repository guard staged cannot read ends refused, exit 5, saying so with the
// cause and journalling one close, outcome refused, never done or failed.
// Risk: a commit hook that lets a change through because it could not see it.
func TestAHistoryGuardStagedCannotReadIsRefusedAndJournalled(t *testing.T) {
	t.Parallel()
	p, _ := process(t, world(t), "guard", "staged")
	p, sc := attach(t, p)
	root := seededRoot(sc)
	p.History = func(string) (port.History, error) { return failingHistory{}, nil }
	if code := Execute(t.Context(), p); code != exit.Refused {
		t.Fatalf("exit %d, want %d (refused)", code, exit.Refused)
	}
	diag := strings.Join(sc.diag.Said, "\n")
	if !strings.Contains(diag, "the repository could not be read, so the staged change is refused") || !strings.Contains(diag, "the object store is damaged") {
		t.Errorf("standard error does not say the repository could not be read, and why:\n%s", diag)
	}
	recs := crashRecords(t, sc, root)
	if len(recs) != 1 {
		t.Fatalf("%d journal lines, want the one refusal", len(recs))
	}
	var d struct{ Outcome, Why string }
	if err := json.Unmarshal(recs[0].Detail, &d); err != nil {
		t.Fatal(err)
	}
	if recs[0].Command != "gragas guard staged" || d.Outcome != "refused" || !strings.Contains(d.Why, "could not be read") {
		t.Errorf("the journal line is %+v, detail %+v", recs[0], d)
	}
}
