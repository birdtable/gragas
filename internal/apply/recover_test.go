package apply_test

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// threeFiles is a plan that replaces a, makes state/new.yaml, replaces b
// and adds a history line: one file of each kind a roll-back puts back.
func (f *fixture) threeFiles() testPlan {
	tp := f.plan(
		edit{path: "rules/a.yaml", data: "status: retired\n"},
		edit{path: "state/new.yaml", data: "made: yes\n"},
		edit{path: "rules/b.yaml", data: "status: trial\n"},
	)
	tp.yes = true
	return tp
}

// cancelAfter cancels its context once n writes have landed: a SIGINT or a
// SIGTERM arriving mid-write, as signal.NotifyContext delivers one.
type cancelAfter struct {
	port.Writer
	left   *int
	cancel context.CancelFunc
}

func (c cancelAfter) Replace(r model.Root, p string, data []byte) error {
	defer c.tick()
	return c.Writer.Replace(r, p, data)
}

func (c cancelAfter) Append(r model.Root, p string, line []byte) error {
	defer c.tick()
	return c.Writer.Append(r, p, line)
}

func (c cancelAfter) tick() {
	if *c.left--; *c.left == 0 {
		c.cancel()
	}
}

// TestACancelledContextMidWriteRollsEveryFileBackByteForByte: the context
// ends once a is replaced and state/new.yaml made. The run ends failed, kind
// cancelled, with its run id, and rolled back: a holds its old bytes,
// state/new.yaml is gone, b and the history were never touched, and the
// record closes "rolled back". Each put-back is journalled before it is made.
// The control is the same plan uncancelled, which ends done. Risk: a
// cancelled run that leaves a half-applied tree (go.concurrency.4,
// cli.commands.7).
func TestACancelledContextMidWriteRollsEveryFileBackByteForByte(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	before := f.snapshot(fixturePaths()...)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	left := 2
	f.p.Writer = cancelAfter{Writer: f.p.Writer, left: &left, cancel: cancel}

	o := apply.Run(ctx, f.p, f.root, f.threeFiles().spec())

	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailCancelled || o.Failure.Run() != runID {
		t.Fatalf("ended %q %+v, want failed, kind cancelled, with the run id", o.Word, o.Failure)
	}
	if !o.RolledBack {
		t.Errorf("the run does not say it was rolled back; cause %v", o.Cause)
	}
	if after := f.snapshot(fixturePaths()...); !maps.Equal(after, before) {
		t.Errorf("the tree after\n %q\nwant it as before\n %q", after, before)
	}
	tail := []string{
		"read state/new.yaml", "journal step", "remove state/new.yaml", "read state/new.yaml",
		"read rules/a.yaml", "journal step", "replace rules/a.yaml", "read rules/a.yaml",
		"journal close", "release root",
	}
	if log := f.spy.log; len(log) < len(tail) || !slices.Equal(log[len(log)-len(tail):], tail) {
		t.Errorf("the roll-back's calls:\n got %q\nwant them to end %q", log, tail)
	}
	if got := f.closeOf(runID)["outcome"]; got != canon.String("rolled back") {
		t.Errorf("the close says %v, want rolled back", got)
	}
	if open := f.open(); len(open) != 0 {
		t.Errorf("open records %v remain", open)
	}

	t.Run("the control, uncancelled, ends done", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		wantDone(t, f.run(f.threeFiles().spec()))
		if f.content("state/new.yaml") != "made: yes\n" {
			t.Errorf("state/new.yaml = %q", f.content("state/new.yaml"))
		}
	})
}

// TestAWritingCommandRollsBackAnOpenRecordSaysSoAndGoesOnWithItsOwnPlan: run-0
// is killed once a is replaced and state/new.yaml made, which leaves its
// record open. The next writing command rolls run-0 back from the copies
// under its own lock, says so in Recovered, then applies its own plan; run-0's
// record closes "rolled back" by it. Risk: a writer that plans over a
// half-applied tree, or one that finishes the other run instead (A37).
func TestAWritingCommandRollsBackAnOpenRecordSaysSoAndGoesOnWithItsOwnPlan(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	before := f.snapshot(fixturePaths()...)
	f.interrupt("run-0", 2, f.threeFiles().spec())
	if f.content("rules/a.yaml") != "status: retired\n" || f.content("state/new.yaml") != "made: yes\n" {
		t.Fatalf("the kill did not leave a half-applied tree: a=%q new=%q", f.content("rules/a.yaml"), f.content("state/new.yaml"))
	}
	if open := f.open(); !slices.Equal(open, []string{"run-0"}) {
		t.Fatalf("open records %v, want run-0", open)
	}

	o := f.run(f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())

	wantDone(t, o)
	if want := []apply.Recovery{{Run: "run-0", Command: "gragas test --apply"}}; !slices.Equal(o.Recovered, want) {
		t.Errorf("recovered %+v, want %+v", o.Recovered, want)
	}
	want := maps.Clone(before)
	want["rules/b.yaml"] = "status: trial\n"
	if after := f.snapshot(fixturePaths()...); !maps.Equal(after, want) {
		t.Errorf("the tree after\n %q\nwant\n %q", after, want)
	}
	c := f.closeOf("run-0")
	if c["outcome"] != canon.String("rolled back") || c["by"] != canon.String(runID) {
		t.Errorf("run-0 closes %v, want rolled back by %s", c, runID)
	}
	if open := f.open(); len(open) != 0 {
		t.Errorf("open records %v remain", open)
	}
}

// notedDetail is a plugin.Noted as a start record holds it.
const notedDetail = `{"round":0,"step":1,"grant":"g","program":"p","words":["a"],"level":"isolated"}`

// TestStartRecordsBetweenTheStepsAreSkippedAndTheRunRollsBack: an open run
// whose journal holds a start record on each side of a step rolls back as
// it would without them. Planted failure: recovery
// reading a start record as a step refuses the run.
func TestStartRecordsBetweenTheStepsAreSkippedAndTheRunRollsBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	stray := `{"before":"","existed":false,"path":"state/stray.yaml"}`
	f.p.Journal = plantedCopy{Journal: f.p.Journal, detail: stray, start: notedDetail,
		step: `{"after":"` + string(apply.DigestBytes([]byte("x\n"))) + `","op":"replace","path":"state/stray.yaml"}`}
	f.change("state/stray.yaml", "x\n")
	o := f.run(f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())
	wantDone(t, o)
	if f.content("state/stray.yaml") != "<absent>" {
		t.Errorf("the file run-x made is %q, want it gone: a start record stopped the roll-back", f.content("state/stray.yaml"))
	}
}

// keptLies is a journal whose copy of path is other bytes than were kept.
type keptLies struct {
	port.Journal
	path string
}

func (j keptLies) Kept(r model.Root, run, p string) ([]byte, error) {
	if p == j.path {
		return []byte("planted: yes\n"), nil
	}
	return j.Journal.Kept(r, run, p)
}

// plantedCopy is a journal that answers one more open run, run-x, whose copy
// record holds detail, followed by a step record holding step when there
// is one, and a start record holding start on each side of it when there is
// one.
type plantedCopy struct {
	port.Journal
	detail, step, start string
}

func (j plantedCopy) Records(ctx context.Context, r model.Root) ([]model.Record, int, error) {
	recs, torn, err := j.Journal.Records(ctx, r)
	recs = append(recs,
		model.Record{Run: "run-x", Kind: model.KindIntent, Command: "gragas x --apply", Detail: []byte(`{}`)},
		model.Record{Run: "run-x", Kind: model.KindCopy, Command: "gragas x --apply", Detail: []byte(j.detail)},
	)
	start := model.Record{Run: "run-x", Kind: model.KindStart, Command: "gragas x --apply", Detail: []byte(j.start)}
	if j.start != "" {
		recs = append(recs, start)
	}
	if j.step != "" {
		recs = append(recs, model.Record{Run: "run-x", Kind: model.KindStep, Command: "gragas x --apply", Detail: []byte(j.step)})
	}
	if j.start != "" {
		recs = append(recs, start)
	}
	return recs, torn, err
}

// putsOther is a Writer whose Replace of path writes other bytes than it is
// handed: a disk, or a bug, that the read back after a put must catch.
type putsOther struct {
	port.Writer
	path string
}

func (w putsOther) Replace(r model.Root, p string, data []byte) error {
	if p == w.path {
		data = []byte("other: bytes\n")
	}
	return w.Writer.Replace(r, p, data)
}

// TestAnOpenRecordThatCannotBeRolledBackIsNeverWrittenFrom: a copy whose
// bytes are not the digest its copy record names, copy and step records
// that name an unsafe path or say nothing whole, and a file a person
// changed since the run, are refused before any file is put back; a put
// back the tree does not then hold leaves the record open. The writing
// command ends failed, kind recover, naming the open run and why, writes
// nothing of its own, and the record stays open. Planted failures: each
// check of a record's shape, the check of each file's state before the
// first put, and the read back after a put, each taken out. Risk: a
// roll-back that writes into the tree whatever the state folder holds, or
// over what a person wrote.
func TestAnOpenRecordThatCannotBeRolledBackIsNeverWrittenFrom(t *testing.T) {
	t.Parallel()
	refused := func(t *testing.T, f *fixture, open string, why ...string) {
		t.Helper()
		writes := len(f.w.Writes)
		o := f.run(f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())
		if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRecover || o.Failure.Run() != runID {
			t.Fatalf("ended %q %+v, want failed, kind recover, with the run id", o.Word, o.Failure)
		}
		if o.Cause == nil || !strings.Contains(o.Cause.Error(), open) {
			t.Errorf("the cause %v does not name %s", o.Cause, open)
		}
		for _, w := range why {
			if o.Cause == nil || !strings.Contains(o.Cause.Error(), w) {
				t.Errorf("the cause %v does not say %q", o.Cause, w)
			}
		}
		if len(f.w.Writes) != writes {
			t.Errorf("wrote %v after the refusal", f.w.Writes[writes:])
		}
		if f.content("rules/b.yaml") != "status: active\n" {
			t.Errorf("b = %q: the command went on with its plan", f.content("rules/b.yaml"))
		}
	}
	t.Run("a copy that is not its record's", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.interrupt("run-0", 1, f.threeFiles().spec())
		f.p.Journal = keptLies{Journal: f.p.Journal, path: "rules/a.yaml"}
		refused(t, f, "run-0")
		if f.content("rules/a.yaml") != "status: retired\n" {
			t.Errorf("a = %q, want it as the killed run left it", f.content("rules/a.yaml"))
		}
		if open := f.open(); !slices.Equal(open, []string{"run-0"}) {
			t.Errorf("open records %v, want run-0 still", open)
		}
	})
	digestA := string(apply.DigestBytes([]byte("status: active\n")))
	stray := `{"before":"","existed":false,"path":"state/stray.yaml"}`
	for name, c := range map[string]struct{ detail, step, why string }{
		"a path that leaves the root":    {detail: `{"before":"","existed":false,"path":"../outside"}`, why: "../outside"},
		"an absolute path":               {detail: `{"before":"","existed":false,"path":"/outside.txt"}`, why: "/outside.txt"},
		"a path that climbs out, deep":   {detail: `{"before":"","existed":false,"path":"sub/../../x"}`, why: "sub/../../x"},
		"a step to an absolute path":     {detail: stray, step: `{"after":"","op":"remove","path":"/outside.txt"}`, why: "/outside.txt"},
		"a path into .git":               {detail: `{"before":"","existed":false,"path":".git/hooks/pre-commit"}`, why: ".git/hooks"},
		"a path to .git/config":          {detail: `{"before":"","existed":false,"path":".git/config"}`, why: ".git/config"},
		"a path to .GIT/hooks, in case":  {detail: `{"before":"","existed":false,"path":".GIT/hooks/pre-commit"}`, why: ".GIT/hooks"},
		"a step to .git/config":          {detail: stray, step: `{"after":"","op":"remove","path":".git/config"}`, why: ".git/config"},
		"a step to a hook":               {detail: stray, step: `{"after":"","op":"remove","path":".git/hooks/post-checkout"}`, why: ".git/hooks"},
		"a step to .GIT/hooks, in case":  {detail: stray, step: `{"after":"","op":"remove","path":".GIT/hooks/pre-commit"}`, why: ".GIT/hooks"},
		"a field more":                   {detail: `{"before":"","existed":false,"mode":"0755","path":"rules/a.yaml"}`, why: "not an object of a path"},
		"a file that existed, no digest": {detail: `{"before":"","existed":true,"path":"rules/a.yaml"}`, why: "a file that existed has a digest"},
		"a file made, with a digest":     {detail: `{"before":"` + digestA + `","existed":false,"path":"rules/a.yaml"}`, why: "a file that existed has a digest"},
		"a digest that is none":          {detail: `{"before":"md5:00","existed":true,"path":"rules/a.yaml"}`, why: "its digest before is not a digest"},
		"not an object":                  {detail: `["rules/a.yaml"]`, why: "not an object of a path"},
		"a path that is a number":        {detail: `{"before":"","existed":false,"path":7}`, why: "not an object of a path"},
		"a step whose op is a number":    {detail: stray, step: `{"after":"","op":7,"path":"state/stray.yaml"}`, why: "a step record"},
		"a step that leaves the root":    {detail: stray, step: `{"after":"","op":"remove","path":"../outside"}`, why: "../outside"},
		"a step with a field more":       {detail: stray, step: `{"after":"","mode":"0755","op":"remove","path":"state/stray.yaml"}`, why: "a step record"},
		"a step whose digest is none":    {detail: stray, step: `{"after":"md5:00","op":"replace","path":"state/stray.yaml"}`, why: "its digest after is not a digest"},
		"a step that is no object":       {detail: stray, step: `"state/stray.yaml"`, why: "a step record"},
		"a step whose op is no word":     {detail: stray, step: `{"after":"","op":true,"path":"state/stray.yaml"}`, why: "a step record"},
		"a step that holds a Noted":      {detail: stray, step: notedDetail, why: "a step record"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.change("state/stray.yaml", "x\n")
			f.p.Journal = plantedCopy{Journal: f.p.Journal, detail: c.detail, step: c.step}
			refused(t, f, "run-x", c.why)
			if f.content("rules/a.yaml") != "status: active\n" || f.content("state/stray.yaml") != "x\n" {
				t.Errorf("a = %q, stray = %q: a refused record wrote", f.content("rules/a.yaml"), f.content("state/stray.yaml"))
			}
		})
	}
	for name, edit := range map[string]struct{ path, data string }{
		"a file the run replaced, changed since": {"rules/a.yaml", "status: a person's edit\n"},
		"a file the run made, changed since":     {"state/new.yaml", "a person's notes\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.interrupt("run-0", 2, f.threeFiles().spec())
			f.change(edit.path, edit.data)
			now := string(apply.DigestBytes([]byte(edit.data)))
			refused(t, f, "run-0", edit.path, now, "is not overwritten")
			if f.content(edit.path) != edit.data {
				t.Errorf("%s = %q, want the person's bytes", edit.path, f.content(edit.path))
			}
			left := map[string]string{"rules/a.yaml": "status: retired\n", "state/new.yaml": "made: yes\n"}
			left[edit.path] = edit.data
			for p, want := range left {
				if f.content(p) != want {
					t.Errorf("%s = %q, want it as the killed run and the person left it, %q: nothing is put back before every file is checked", p, f.content(p), want)
				}
			}
			if open := f.open(); !slices.Equal(open, []string{"run-0"}) {
				t.Errorf("open records %v, want run-0 still", open)
			}
		})
	}
	t.Run("a put back the tree does not hold", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.interrupt("run-0", 2, f.threeFiles().spec())
		f.p.Writer = putsOther{Writer: f.p.Writer, path: "rules/a.yaml"}
		o := f.run(f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())
		if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRecover || o.Cause == nil || !strings.Contains(o.Cause.Error(), "does not hold its copy") {
			t.Errorf("ended %q %+v, cause %v; want failed, kind recover, saying the file does not hold its copy", o.Word, o.Failure, o.Cause)
		}
		if f.content("rules/b.yaml") != "status: active\n" {
			t.Errorf("b = %q: the command went on with its plan", f.content("rules/b.yaml"))
		}
		if open := f.open(); !slices.Equal(open, []string{"run-0"}) {
			t.Errorf("open records %v, want run-0 still", open)
		}
	})
	t.Run("the control, a copy record that holds, rolls back", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.p.Journal = plantedCopy{Journal: f.p.Journal, detail: stray,
			step: `{"after":"` + string(apply.DigestBytes([]byte("x\n"))) + `","op":"replace","path":"state/stray.yaml"}`}
		f.change("state/stray.yaml", "x\n")
		o := f.run(f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())
		wantDone(t, o)
		if f.content("state/stray.yaml") != "<absent>" {
			t.Errorf("the file run-x made is %q, want it gone", f.content("state/stray.yaml"))
		}
	})
}

// TestTheRefusalOfAChangedFileSaysTheWayOutAndTheWayOutWorks: a roll-back
// that finds a file a person changed keeps their bytes, and says what to
// do: move their version out of the root, copy back the copy the run kept
// of a file it replaced, named by where the journal says it lies, leave
// out a file it made, run the command again, and bring their version back.
// The test does each step as said, and the next run rolls the record back
// and goes on with its plan. Cases: a file the run replaced, a file it
// made, both, and a replaced file the person removed, which has no
// version to move. Planted failures: the copy step left out, and another
// copy's path named. Risk: a person's edit kept with no way out the
// message gives, so every writing command refuses for good.
func TestTheRefusalOfAChangedFileSaysTheWayOutAndTheWayOutWorks(t *testing.T) {
	t.Parallel()
	const a, made, b = "rules/a.yaml", "state/new.yaml", "rules/b.yaml"
	for name, c := range map[string]struct {
		edits map[string]string // "" is the file removed
		says  []string
		// writes is how many files the killed run wrote: 2 is a and made, 3 b too
		writes int
	}{
		"a file the run replaced": {
			edits: map[string]string{a: "status: a person's edit\n"},
			says:  []string{"so it is not overwritten", "To keep your version and finish the roll-back: move your version of " + a + " out of the root;", "then bring your version back"},
		},
		"a file the run made": {
			edits: map[string]string{made: "a person's notes\n"},
			says:  []string{"so it is not overwritten", "move your version of " + made + " out of the root; the run made " + made + ", so nothing goes back in its place; run the command again"},
		},
		"both": {
			edits: map[string]string{a: "status: a person's edit\n", made: "a person's notes\n"},
			says:  []string{"so none is overwritten", "move your versions of " + a + " and " + made + " out of the root", "then bring your versions back"},
		},
		"a replaced file the person removed": {
			edits: map[string]string{a: ""},
			says:  []string{"it is no file", "To finish the roll-back: copy "},
		},
		"two files the run replaced": {
			edits:  map[string]string{a: "status: a person's edit\n", b: "status: another edit\n"},
			says:   []string{"so none is overwritten", "move your versions of " + a + " and " + b + " out of the root"},
			writes: 3,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.interrupt("run-0", max(c.writes, 2), f.threeFiles().spec())
			for p, data := range c.edits {
				if data == "" {
					f.remove(p)
					continue
				}
				f.change(p, data)
			}

			o := f.run(f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec())

			if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailRecover || o.Cause == nil {
				t.Fatalf("ended %q %+v, want failed, kind recover", o.Word, o.Failure)
			}
			said := o.Cause.Error()
			for _, w := range append(c.says, "run the command again") {
				if !strings.Contains(said, w) {
					t.Errorf("the cause does not say %q:\n%s", w, said)
				}
			}
			mine := map[string]string{}
			for p, data := range c.edits {
				if !strings.Contains(said, p+" changed since run run-0: it is "+shownDigest(data)) {
					t.Errorf("the cause does not name %s and its digest now:\n%s", p, said)
				}
				if data != "" {
					mine[p] = f.content(p)
					f.remove(p) // move your version out of the root
				}
				if p == made {
					if strings.Contains(said, "to "+p) {
						t.Errorf("the cause has a copy put back at %s, which the run made:\n%s", p, said)
					}
					continue
				}
				at, err := f.p.Journal.KeptAt(f.root, "run-0", p)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(said, "copy "+at+" to "+p+" in the root") {
					t.Fatalf("the cause does not say to copy %s to %s:\n%s", at, p, said)
				}
				kept, err := f.p.Journal.Kept(f.root, "run-0", p)
				if err != nil {
					t.Fatal(err)
				}
				f.change(p, string(kept)) // copy the copy the run kept back
			}

			s := f.plan(edit{path: "rules/b.yaml", data: "status: trial\n"}).spec()
			s.Run = "run-2"
			again := f.run(s) // run the command again
			wantDone(t, again)
			if want := []apply.Recovery{{Run: "run-0", Command: "gragas test --apply"}}; !slices.Equal(again.Recovered, want) {
				t.Errorf("recovered %+v, want %+v", again.Recovered, want)
			}
			if f.content(a) != "status: active\n" || f.content(made) != "<absent>" || f.content(b) != "status: trial\n" {
				t.Errorf("a = %q, new = %q, b = %q: want run-0 rolled back and the plan applied", f.content(a), f.content(made), f.content(b))
			}
			if open := f.open(); len(open) != 0 {
				t.Errorf("open records %v remain", open)
			}
			for p, data := range mine { // then bring your version back
				f.change(p, data)
			}
		})
	}
}

// shownDigest is the digest a message shows for a file holding data, or
// no file for "".
func shownDigest(data string) string {
	if data == "" {
		return "no file"
	}
	return string(apply.DigestBytes([]byte(data)))
}

// recoverLine is the line a reading command names for an interrupted run.
const recoverLine = "gragas journal recover"

// TestAReadingCommandOverAnOpenRecordEndsNotReadyNamingTheRunAndTheRecovery:
// over run-0's open record, the check ends "not ready", naming run-0 and its
// command, with the recovery line it was given as the hint, writes no file of
// the root, journals the refusal and puts the lock back. Its controls: no
// open record is ready and journals nothing; a lock another run holds is that
// run writing, "busy"; no recovery line is a wiring failure. Risk: a report
// read from a half-applied tree (A37).
func TestAReadingCommandOverAnOpenRecordEndsNotReadyNamingTheRunAndTheRecovery(t *testing.T) {
	t.Parallel()
	check := apply.Check{Run: "run-9", Command: "gragas status", Recover: recoverLine}
	interrupted := func(t *testing.T) *fixture {
		t.Helper()
		f := newFixture(t)
		f.interrupt("run-0", 1, f.threeFiles().spec())
		return f
	}

	t.Run("not ready", func(t *testing.T) {
		t.Parallel()
		f := interrupted(t)
		writes, snap := len(f.w.Writes), f.snapshot(fixturePaths()...)
		o, ok := apply.Ready(t.Context(), f.p, f.root, check)
		if ok || o.Word != model.NotReady || o.Refusal == nil {
			t.Fatalf("ready %v, ended %q %+v, want not ready", ok, o.Word, o.Refusal)
		}
		if obs := o.Refusal.Observed(); !strings.Contains(obs, "run-0") || !strings.Contains(obs, "gragas test --apply") {
			t.Errorf("observed %q, want run-0 and its command named", obs)
		}
		if o.Refusal.Hint() != recoverLine {
			t.Errorf("hint %q, want %q", o.Refusal.Hint(), recoverLine)
		}
		if len(f.w.Writes) != writes || !maps.Equal(f.snapshot(fixturePaths()...), snap) {
			t.Errorf("the check wrote %v", f.w.Writes[writes:])
		}
		if c := f.closeOf("run-9"); c["outcome"] != canon.String("not ready") || !slices.Equal(c["open"].(canon.Array), canon.Array{canon.String("run-0")}) {
			t.Errorf("the refusal is journalled as %v", c)
		}
		if open := f.open(); !slices.Equal(open, []string{"run-0"}) {
			t.Errorf("open records %v, want run-0 left for the recovery", open)
		}
		if h, err := f.inner().Lock.Take(t.Context(), f.root, model.RootLock); err != nil {
			t.Errorf("the lock was not put back: %v", err)
		} else if err := f.inner().Lock.Release(h); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("the hint is the line it is given", func(t *testing.T) {
		t.Parallel()
		f := interrupted(t)
		c := check
		c.Recover = "gragas rule retire x --apply"
		if o, _ := apply.Ready(t.Context(), f.p, f.root, c); o.Refusal == nil || o.Refusal.Hint() != c.Recover {
			t.Errorf("ended %q %+v, want the hint %q", o.Word, o.Refusal, c.Recover)
		}
	})
	t.Run("busy while another run holds the lock", func(t *testing.T) {
		t.Parallel()
		f := interrupted(t)
		f.w.HoldLock(f.root, model.RootLock, instant().Unix())
		o, ok := apply.Ready(t.Context(), f.p, f.root, check)
		if ok || o.Word != model.Busy || o.Refusal == nil || !strings.Contains(o.Refusal.Observed(), "run-0") {
			t.Fatalf("ready %v, ended %q %+v, want busy naming run-0", ok, o.Word, o.Refusal)
		}
		if f.closeOf("run-9")["outcome"] != canon.String("busy") {
			t.Errorf("the refusal is journalled as %v", f.closeOf("run-9"))
		}
	})
	t.Run("the control, nothing open, is ready", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		wantDone(t, f.run(f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()))
		n := len(f.records())
		if o, ok := apply.Ready(t.Context(), f.p, f.root, check); !ok || o.Word != "" {
			t.Errorf("ready %v, ended %q %+v %+v", ok, o.Word, o.Refusal, o.Failure)
		}
		if len(f.records()) != n {
			t.Errorf("a ready check journalled %d records", len(f.records())-n)
		}
	})
	t.Run("no recovery line is wiring", func(t *testing.T) {
		t.Parallel()
		f := interrupted(t)
		c := check
		c.Recover = ""
		if o, ok := apply.Ready(t.Context(), f.p, f.root, c); ok || o.Failure == nil || o.Failure.Kind() != apply.FailWiring {
			t.Errorf("ready %v, ended %q %+v, want failed wiring", ok, o.Word, o.Failure)
		}
	})
}
