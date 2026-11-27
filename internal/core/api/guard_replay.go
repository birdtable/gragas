package coreapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// replay replays each appended line's command in order over HEAD's tree, then
// holds what the replays wrote to the index, and the rest to no lifecycle
// change.
func (s *staged) replay(g model.Guard) (model.Guard, string, string, error) {
	// the replays write over HEAD's tree: its book and its lines are read
	// already, and nothing reads the tree after
	tree := s.head
	wrote := map[string]bool{}
	for off := len(s.headHist); off < len(s.staging); {
		n := lineAt(s.staging, off)
		e, why := s.entryAt(off, n)
		if why != "" {
			return g, why, useCommand, nil
		}
		files, why, hint, err := s.replayOne(tree, e, n)
		if why != "" || err != nil {
			return g, why, hint, err
		}
		for _, f := range files {
			tree.write(f.Path, f.Data)
			wrote[f.Path] = true
		}
		got := tree.data[s.hist]
		if len(got) <= off || !bytes.HasPrefix(s.staging, got) {
			return g, fmt.Sprintf("line %d of %s is not the line its command writes over the tree before it", n, s.hist), replayAgain, nil
		}
		off = len(got)
	}
	for _, p := range tree.paths() {
		if !wrote[p] {
			continue
		}
		have, err := readOr(s.index, s.ask.Root, p)
		if err != nil {
			return g, "", "", err
		}
		if !bytes.Equal(have, tree.data[p]) {
			g.Path = p
			return g, p + " " + s.n.after + " is not what the commands of " + s.hist + " write: a hand edit is made with them, or the change they record is not", commitApart, nil
		}
	}
	// No rulebook as the commands leave it is the first commit's tree before
	// its init, whose every field is then a change with no line of its own.
	replayed, err := loadBook(tree, s.p.Schema, s.ask.Root)
	if err != nil {
		return g, "the rulebook could not be read as its commands leave it: " + err.Error(), replayAgain, nil
	}
	return s.lone(g, replayed)
}

// lone refuses a lifecycle field the index holds otherwise than the replays
// leave it: a change made by hand, with no line of its own.
func (s *staged) lone(g model.Guard, replayed model.Book) (model.Guard, string, string, error) {
	g.Changes = lifecycle.GuardEdit(replayed, s.after, shown)
	if len(g.Changes) == 0 {
		return g, "", "", s.ctx.Err()
	}
	c := g.Changes[0]
	why := fmt.Sprintf("%s of %s %s with no line of %s", c.Field, c.Entity, s.n.is, s.hist)
	if n := len(g.Changes); n > 1 {
		why = fmt.Sprintf("%d lifecycle fields %s with no line of %s, first %s of %s", n, s.n.are, s.hist, c.Field, c.Entity)
	}
	hint := c.Command
	if hint == "" {
		hint = "leave " + c.Field + " as it is: no command changes it by hand"
	}
	return g, why, hint, nil
}

// entryAt is the entry of the line at off, the file's line n, or why it is
// not one that can be replayed.
func (s *staged) entryAt(off, n int) (lifecycle.Entry, string) {
	raw := s.staging[off:]
	if i := bytes.IndexByte(raw, '\n'); i >= 0 {
		raw = raw[:i]
	}
	c := lifecycle.ReadHistory([][]byte{bytes.TrimRight(raw, "\r")})
	switch {
	case len(c.Skipped) > 0:
		return lifecycle.Entry{}, fmt.Sprintf("line %d of %s is not an entry: %s", n, s.hist, c.Skipped[0].Why)
	case len(c.Entries) != 1:
		return lifecycle.Entry{}, fmt.Sprintf("line %d of %s is empty", n, s.hist)
	case c.Entries[0].Command == "":
		return lifecycle.Entry{}, fmt.Sprintf("line %d of %s names no command, so the change it records cannot be made again", n, s.hist)
	}
	return c.Entries[0], ""
}

// replayOne makes e's command plan again over tree at its when, and answers the files the plan writes, or why the line is refused.
func (s *staged) replayOne(tree *revTree, e lifecycle.Entry, n int) ([]model.File, string, string, error) {
	at := fmt.Sprintf("line %d of %s", n, s.hist)
	when, err := time.Parse("2006-01-02T15:04:05Z", e.When)
	if err != nil {
		return nil, at + " names when it was made as " + e.When + ", which is not an instant to the second in UTC", useCommand, nil
	}
	p := refusing(s.p)
	p.Tree, p.Clock, p.History = tree, fixed{when}, noWho{s.p.History}
	p.Journal, p.Writer = noJournal{}, noWriter{}
	ch, ref, err := s.ask.Replay(s.ctx, p, e.Command)
	switch {
	case err != nil:
		return nil, at + " cannot be made again from its command, " + e.Command + ": " + err.Error(), noReplayHint, nil
	case ref != nil:
		return nil, at + "'s command, " + e.Command + ", ends " + string(ref.Word()) + " over the tree before it: " + ref.Observed(), replayAgain, nil
	case ch == nil:
		return nil, at + "'s command, " + e.Command + ", plans no change", useCommand, nil
	}
	r, out, ok := ch.Replay(s.ctx)
	if !ok {
		return nil, at + "'s command, " + e.Command + ", plans no change over the tree before it: " + outcomeText(out), replayAgain, nil
	}
	if r.Plan.Digest != e.Plan {
		if beforeWhoIsTheCommand(e) {
			return nil, fmt.Sprintf("%s was written by an older gragas, which recorded who made it as a name and address (%q) and not the command's own name, so the plan its command makes now has another digest than the line holds", at, e.Who), remakeLine, nil
		}
		return nil, fmt.Sprintf("%s names the plan %s, and its command plans %s over the tree before it: the digest was not made by the plan, or a file the plan reads was edited by hand before it ran",
			at, e.Plan, r.Plan.Digest), commitApart, nil
	}
	return r.Files, "", "", nil
}

// beforeWhoIsTheCommand is whether e was written by a gragas that recorded the
// runner's git name and address as who, as an older gragas did: today who is
// the command's own name, the words its line begins with.
func beforeWhoIsTheCommand(e lifecycle.Entry) bool {
	return e.Who != "" && e.Command != e.Who && !strings.HasPrefix(e.Command, e.Who+" ")
}

// noWho is the history of a replay: the host's, but for who, which a replay
// never reads: the command hands in its own name, and a replay that asked
// git for a person's name and address would put it in a plan.
type noWho struct{ port.History }

func (noWho) Who(context.Context, model.Root) (model.Identity, error) {
	return model.Identity{}, reached("History.Who")
}

// outcomeText is what an outcome that planned nothing says.
func outcomeText(out Outcome) string {
	switch {
	case out.Nothing:
		return "it has nothing to do"
	case out.Refusal != nil:
		return out.Refusal.Observed()
	case out.Cause != nil:
		return out.Cause.Error()
	}
	return "it ends " + string(out.Word)
}

// fixed is a clock that answers one instant: the when of the line replayed.
type fixed struct{ at time.Time }

func (f fixed) Now() time.Time { return f.at }

// errReplayWrites is a replay that reached for a write: a replay plans and
// never writes.
var errReplayWrites = errors.New("a replayed command writes nothing")

// noJournal is the journal of a replay: none, and nothing written to it.
type noJournal struct{}

func (noJournal) Append(context.Context, model.Root, model.Record) error { return errReplayWrites }
func (noJournal) Keep(model.Root, string, string, []byte) error          { return errReplayWrites }
func (noJournal) Kept(model.Root, string, string) ([]byte, error) {
	return nil, fs.ErrNotExist
}
func (noJournal) KeptAt(model.Root, string, string) (string, error) { return "", fs.ErrNotExist }
func (noJournal) Records(context.Context, model.Root) ([]model.Record, int, error) {
	return nil, 0, nil
}
func (noJournal) RecordsOf(context.Context, []byte) ([]model.Record, int, error) {
	return nil, 0, nil
}
func (noJournal) Sweep(context.Context, model.Root) (int, error) { return 0, errReplayWrites }

// noWriter is the writer of a replay, which refuses every write.
type noWriter struct{}

func (noWriter) Replace(model.Root, string, []byte) error { return errReplayWrites }
func (noWriter) Append(model.Root, string, []byte) error  { return errReplayWrites }
func (noWriter) Remove(model.Root, string) error          { return errReplayWrites }

// reached is a replay that reached for a port a plan has no use for: a plan
// reads the tree before it, its clock and its history, and never takes a
// lock, runs or finds a tool, fetches, commits or tags, prepares a scratch
// folder, signs or verifies, or asks the host's accounts. Each
// of those ports refuses in a replay, naming itself, so a command whose plan
// reached one is refused and never run against the host.
type reached string

func (r reached) Error() string {
	return "a replay plans over the tree before it alone, and the command reached for the " + string(r) + " port, which a replay refuses"
}

// The ports of a replay that refuse every call.
type (
	noLock    struct{}
	noSteps   struct{}
	noRemote  struct{}
	noSandbox struct{}
)

func (noLock) Take(context.Context, model.Root, model.Lock) (model.Hold, error) {
	return model.Hold{}, reached("Lock")
}
func (noLock) Wait(context.Context, model.Root, model.Lock) (model.Hold, error) {
	return model.Hold{}, reached("Lock")
}
func (noLock) Release(model.Hold) error { return reached("Lock") }

func (noSteps) Bind(context.Context, plugin.Scope) (port.StepRunner, error) {
	return nil, reached("Steps")
}
func (noSteps) SweepCaches() (int, error) { return 0, reached("Steps") }
func (noSteps) CachesLeft() (int, error)  { return 0, reached("Steps") }

func (noRemote) Fetch(context.Context, string, string) (model.Root, error) {
	return "", reached("Remote")
}
func (noRemote) Commit(context.Context, model.Root, []string, string) (string, error) {
	return "", reached("Remote")
}
func (noRemote) Tag(context.Context, model.Root, string, string, string) error {
	return reached("Remote")
}

func (noSandbox) Prepare(context.Context, model.Prepare) (model.Scratch, error) {
	return model.Scratch{}, reached("Sandbox")
}
func (noSandbox) Release(model.Scratch) error { return reached("Sandbox") }
func (noSandbox) Sweep() (int, error)         { return 0, reached("Sandbox") }
func (noSandbox) Left() (int, error)          { return 0, reached("Sandbox") }

// refusing is p with every port a plan has no use for refusing.
func refusing(p port.Ports) port.Ports {
	p.Lock, p.Steps, p.Remote, p.Sandbox = noLock{}, noSteps{}, noRemote{}, noSandbox{}
	return p
}
