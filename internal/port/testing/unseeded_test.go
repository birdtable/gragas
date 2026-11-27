package porttest_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// unseededCase is one method of a port, called on a World nobody seeded.
type unseededCase struct {
	method string
	call   func(ctx context.Context, p port.Ports) error
	// answers is what a method that cannot be unseeded answers instead, with no
	// failure: a thing that is not there, or the answer the port always gives.
	// nil means the call fails the test, naming method.
	answers error
	// quiet is set for a method that has no error to answer and none is wanted.
	quiet bool
	// pure is set for a method that reads nothing a World is seeded with,
	// only what it is given: it answers with no error and fails no test.
	pure bool
}

// unseededTable is every method of the seventeen ports.
func unseededTable() []unseededCase {
	root := model.Root("/x")
	return []unseededCase{
		{method: "Tree.Open", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.Open("/x"); return err }},
		{method: "Tree.Locate", answers: fs.ErrNotExist, call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.Locate("/x", "m"); return err }},
		{method: "Tree.Read", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.Read(root, "a"); return err }},
		{method: "Tree.ReadSoleWithin", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.ReadSoleWithin(root, "a", 1); return err }},
		{method: "Tree.Stat", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.Stat(root, "a"); return err }},
		{method: "Tree.List", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.List(root, "", model.Every); return err }},
		{method: "Owned.ReadOwned", answers: fs.ErrNotExist, call: func(_ context.Context, p port.Ports) error { _, err := p.Owned.ReadOwned("/x/a"); return err }},
		{method: "Tree.Written", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.Written(root, "/x/a"); return err }},
		{method: "Tree.Within", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.Within(root, "/x/a"); return err }},
		{method: "Tree.OpenIn", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.OpenIn(root, "a"); return err }},
		{method: "Tree.FS", call: func(_ context.Context, p port.Ports) error { _, err := p.Tree.FS(root); return err }},
		{method: "Writer.Replace", call: func(_ context.Context, p port.Ports) error { return p.Writer.Replace(root, "a", nil) }},
		{method: "Writer.Append", call: func(_ context.Context, p port.Ports) error { return p.Writer.Append(root, "a", nil) }},
		{method: "Writer.Remove", call: func(_ context.Context, p port.Ports) error { return p.Writer.Remove(root, "a") }},
		{method: "Lock.Take", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Lock.Take(ctx, root, model.RootLock)
			return err
		}},
		{method: "Lock.Wait", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Lock.Wait(ctx, root, model.RootLock)
			return err
		}},
		{method: "Lock.Release", answers: errNotHeld, call: func(_ context.Context, p port.Ports) error {
			if err := p.Lock.Release(model.Hold{Root: root, Which: model.RootLock}); err != nil {
				return errNotHeld
			}
			return nil
		}},
		{method: "Journal.Append", call: func(ctx context.Context, p port.Ports) error {
			return p.Journal.Append(ctx, root, model.Record{Run: "r", Kind: "intent"})
		}},
		{method: "Journal.Keep", call: func(_ context.Context, p port.Ports) error { return p.Journal.Keep(root, "r", "a", nil) }},
		{method: "Journal.Kept", call: func(_ context.Context, p port.Ports) error { _, err := p.Journal.Kept(root, "r", "a"); return err }},
		{method: "Journal.KeptAt", call: func(_ context.Context, p port.Ports) error { _, err := p.Journal.KeptAt(root, "r", "a"); return err }},
		{method: "Journal.Records", call: func(ctx context.Context, p port.Ports) error { _, _, err := p.Journal.Records(ctx, root); return err }},
		// RecordsOf reads the bytes it is given and names no root, so no
		// World needs seeding for it.
		{method: "Journal.RecordsOf", pure: true, call: func(ctx context.Context, p port.Ports) error {
			_, _, err := p.Journal.RecordsOf(ctx, nil)
			return err
		}},
		{method: "Journal.Sweep", call: func(ctx context.Context, p port.Ports) error { _, err := p.Journal.Sweep(ctx, root); return err }},
		{method: "History.Base", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.History.Base(ctx, root, "a", "b")
			return err
		}},
		{method: "History.Files", call: func(ctx context.Context, p port.Ports) error { _, err := p.History.Files(ctx, root, "v1"); return err }},
		{method: "History.WorkObjects", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.History.WorkObjects(ctx, root, []string{"a"})
			return err
		}},
		{method: "History.Blob", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.History.Blob(ctx, root, "v1", "a")
			return err
		}},
		{method: "History.Blobs", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.History.Blobs(ctx, root, "v1", []string{"a"})
			return err
		}},
		{method: "History.Tags", call: func(ctx context.Context, p port.Ports) error { _, err := p.History.Tags(ctx, root); return err }},
		{method: "History.Messages", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.History.Messages(ctx, root, strings.Repeat("a", 40), strings.Repeat("b", 40))
			return err
		}},
		{method: "History.Who", call: func(ctx context.Context, p port.Ports) error { _, err := p.History.Who(ctx, root); return err }},
		{method: "History.Merging", call: func(ctx context.Context, p port.Ports) error { _, err := p.History.Merging(ctx, root); return err }},
		{method: "History.Commits", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.History.Commits(ctx, root, "a", "b")
			return err
		}},
		{method: "Remote.Fetch", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Remote.Fetch(ctx, "example.com/r", "v1")
			return err
		}},
		{method: "Remote.Commit", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Remote.Commit(ctx, root, []string{"a"}, "m")
			return err
		}},
		{method: "Remote.Tag", call: func(ctx context.Context, p port.Ports) error {
			return p.Remote.Tag(ctx, root, "v1", "abc", "m")
		}},
		{method: "Sandbox.Prepare", answers: errAnswered, call: func(ctx context.Context, p port.Ports) error {
			// A sandbox reports any level asked, with nothing seeded.
			if _, err := p.Sandbox.Prepare(ctx, model.Prepare{Level: model.Sandboxed}); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Sandbox.Release", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			// An absolute folder in this system's form, slash-separated, as Prepare gives.
			dir, err := filepath.Abs(filepath.FromSlash("/scratch/1"))
			if err != nil {
				return err
			}
			if err := p.Sandbox.Release(model.Scratch{Dir: filepath.ToSlash(dir)}); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Sandbox.Sweep", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			if _, err := p.Sandbox.Sweep(); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Sandbox.Left", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			if _, err := p.Sandbox.Left(); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Schema.Check", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			// An unknown name is one problem, a finding and no failure.
			got, err := p.Schema.Check("none.json", []byte(`{}`))
			if err != nil {
				return err
			}
			if len(got) != 1 {
				return errWrongAnswer
			}
			return errAnswered
		}},
		{method: "Clock.Now", quiet: true, call: func(_ context.Context, p port.Ports) error { p.Clock.Now(); return nil }},
		{method: "Assets.Read", answers: fs.ErrNotExist, call: func(_ context.Context, p port.Ports) error { _, err := p.Assets.Read("a"); return err }},
		{method: "Assets.List", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			got, err := p.Assets.List("")
			if err != nil {
				return err
			}
			if len(got) != 0 {
				return errWrongAnswer
			}
			return errAnswered
		}},
		{method: "Terminal.Width", quiet: true, call: func(_ context.Context, p port.Ports) error { p.Terminal.Width(); return nil }},
		{method: "Terminal.Colors", quiet: true, call: func(_ context.Context, p port.Ports) error { p.Terminal.Colors(); return nil }},
		{method: "Terminal.CanAsk", quiet: true, call: func(_ context.Context, p port.Ports) error { p.Terminal.CanAsk(); return nil }},
		{method: "Terminal.Pick", quiet: true, call: func(_ context.Context, p port.Ports) error {
			p.Terminal.Pick("t", []model.Choice{{Label: "a", Value: "a"}}, "--f")
			return nil
		}},
		{method: "Terminal.Ask", call: func(ctx context.Context, p port.Ports) error { _, err := p.Terminal.Ask(ctx, "ok?"); return err }},
		{method: "Terminal.Show", call: func(_ context.Context, p port.Ports) error { return p.Terminal.Show(oneScreen()) }},
		{method: "Terminal.Banner", call: func(_ context.Context, p port.Ports) error { return p.Terminal.Banner("v1") }},
		{method: "Terminal.Say", call: func(_ context.Context, p port.Ports) error { return p.Terminal.Say("hi") }},
		{method: "Terminal.Line", call: func(ctx context.Context, p port.Ports) error { _, err := p.Terminal.Line(ctx); return err }},
		{method: "FileNames.Portable", answers: fs.ErrInvalid, call: func(_ context.Context, p port.Ports) error { _, err := p.FileNames.Portable(""); return err }},
		// A machine that never approved anything has no record: Approved is
		// no and not an error, as an empty record is, Prior is
		// a first approval, matching fs.ErrNotExist, Approve
		// makes one, and Revoke of what is not there is not an error.
		{method: "Trust.Prior", answers: fs.ErrNotExist, call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Trust.Prior(ctx, "standin")
			return err
		}},
		{method: "Trust.Approved", answers: errAnswered, call: func(ctx context.Context, p port.Ports) error {
			ok, err := p.Trust.Approved(ctx, standin)
			switch {
			case err != nil:
				return err
			case ok:
				return errWrongAnswer
			}
			return errAnswered
		}},
		{method: "Trust.Approve", answers: errAnswered, call: func(ctx context.Context, p port.Ports) error {
			if err := p.Trust.Approve(ctx, model.Approval{Digest: standin, Ref: "standin", Grants: standin}); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Trust.Revoke", answers: errAnswered, call: func(ctx context.Context, p port.Ports) error {
			if err := p.Trust.Revoke(ctx, standin); err != nil {
				return err
			}
			return errAnswered
		}},
		// A root nothing was installed in has no plugin: a listing is empty
		// and not an error, and an open is port.ErrNotInstalled, as the port
		// says.
		{method: "Plugins.List", answers: errAnswered, call: func(ctx context.Context, p port.Ports) error {
			return noneListed(p.Plugins.List(ctx, "/x"))
		}},
		{method: "Plugins.Open", answers: port.ErrNotInstalled, call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Plugins.Open(ctx, "/x", model.PluginRef{Name: "standin", Version: "1.0.0"})
			return err
		}},
		// A module nobody scripted is no module a plugin's is: the try is
		// port.ErrModule, and nothing ran.
		{method: "Plugins.Try", answers: port.ErrModule, call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Plugins.Try(ctx, plugin.Manifest{}, []byte("\x00asm\x01\x00\x00\x00"))
			return err
		}},
		// A tool nobody seeded is not on this machine: the step is not ready.
		{method: "Steps.Bind", answers: port.ErrStepNotReady, call: func(ctx context.Context, p port.Ports) error {
			m := plugin.Manifest{Tool: "lint-a", Programs: []plugin.Program{{Found: &plugin.ProgramFound{ID: "lint-a", Versions: ">=1.0.0"}}}}
			_, err := p.Steps.Bind(ctx, plugin.Scope{Manifest: m, Root: root, Floor: plugin.LevelIsolated,
				Note:    func(context.Context, plugin.Noted) error { return nil },
				Scratch: model.Scratch{Dir: "/x/scratch", Level: model.Isolated, Seconds: 60}})
			return err
		}},
		// A runner that kept no cache has none to count or remove.
		{method: "Steps.SweepCaches", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			if _, err := p.Steps.SweepCaches(); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Steps.CachesLeft", answers: errAnswered, call: func(_ context.Context, p port.Ports) error {
			if _, err := p.Steps.CachesLeft(); err != nil {
				return err
			}
			return errAnswered
		}},
		{method: "Bundles.List", call: func(ctx context.Context, p port.Ports) error { _, err := p.Bundles.List(ctx, "/x"); return err }},
		{method: "Bundles.Read", call: func(ctx context.Context, p port.Ports) error {
			_, err := p.Bundles.Read(ctx, "/x", model.PluginRef{Name: "standin", Version: "1.0.0"})
			return err
		}},
	}
}

// noneListed is errAnswered for an empty listing with no error, the answer a
// root nothing was installed in gives; any other answer is errWrongAnswer, or
// the listing's own error.
func noneListed(refs []model.PluginRef, err error) error {
	switch {
	case err != nil:
		return err
	case len(refs) != 0:
		return errWrongAnswer
	}
	return errAnswered
}

// standin is the digest of the planted module, standin, in the digest's form.
const standin = model.Digest("sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

// The answers of the methods an unseeded World does not fail the test for.
var (
	errNotHeld     = errors.New("not held")
	errAnswered    = errors.New("answered")
	errWrongAnswer = errors.New("not the answer the port always gives")
)

// methodsOf is every "Port.Method" the seventeen ports declare.
func methodsOf() []string {
	var out []string
	for f := range reflect.TypeFor[port.Ports]().Fields() {
		for m := range f.Type.Methods() {
			out = append(out, f.Name+"."+m.Name)
		}
	}
	return out
}

// TestTheUnseededTableCoversEveryMethod: the table names each method of the
// seventeen ports once and no other, so a new method fails here until it has a
// row. Planted: a table with one row gone.
func TestTheUnseededTableCoversEveryMethod(t *testing.T) {
	t.Parallel()
	missing := func(table []unseededCase) []string {
		have := map[string]bool{}
		for _, c := range table {
			have[c.method] = true
		}
		var out []string
		for _, m := range methodsOf() {
			if !have[m] {
				out = append(out, m)
			}
		}
		return out
	}
	table := unseededTable()
	if len(methodsOf()) < 37 {
		t.Fatalf("the ports declare %d methods, want at least the thirty-seven read", len(methodsOf()))
	}
	if got := missing(table); len(got) != 0 {
		t.Errorf("the table lacks %v", got)
	}
	if len(table) != len(methodsOf()) {
		t.Errorf("the table has %d rows for %d methods", len(table), len(methodsOf()))
	}
	t.Run("a row gone is seen", func(t *testing.T) {
		t.Parallel()
		if got := missing(table[1:]); len(got) != 1 || got[0] != "Tree.Open" {
			t.Errorf("missing = %v, want [Tree.Open]", got)
		}
	})
}

// TestAnUnseededCallFailsTheTestNamingThePortAndMethod: every method of every
// port, called on a World nobody seeded, either fails the test naming that
// port and method and answers ErrUnseeded, or answers what the port always
// answers for a thing not there, with no failure.
func TestAnUnseededCallFailsTheTestNamingThePortAndMethod(t *testing.T) {
	t.Parallel()
	table := unseededTable()
	if len(table) == 0 {
		t.Fatal("no method was tried")
	}
	for _, c := range table {
		t.Run(c.method, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			err := c.call(t.Context(), porttest.NewWorld(rec).Ports())
			switch {
			case c.pure:
				if err != nil || len(rec.said) != 0 {
					t.Errorf("%s of what it was given = %v, reported %q, want no error and no failure", c.method, err, rec.said)
				}
			case c.answers != nil:
				if len(rec.said) != 0 {
					t.Errorf("%s failed the test for a call that is answered: %q", c.method, rec.said)
				}
				if !errors.Is(err, c.answers) {
					t.Errorf("%s answered %v, want %v", c.method, err, c.answers)
				}
			case c.quiet:
				rec.saidOnly(t, c.method, "not seeded")
			default:
				if !errors.Is(err, porttest.ErrUnseeded) {
					t.Errorf("%s answered %v, want ErrUnseeded", c.method, err)
				}
				rec.saidOnly(t, c.method, "not seeded")
			}
		})
	}
	t.Run("a call that was seeded does not fail", func(t *testing.T) {
		t.Parallel()
		rec := &recorder{}
		w := porttest.NewWorld(rec)
		w.Seed("/x", map[string]string{"a": "1"})
		w.SeedTerminal(porttest.Term{})
		p := w.Ports()
		if _, err := p.Tree.Read("/x", "a"); err != nil || p.Terminal.Width() != 0 || len(rec.said) != 0 {
			t.Errorf("a seeded call = %v, reported %q", err, rec.said)
		}
	})
}
