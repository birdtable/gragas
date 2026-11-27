package coreapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// GuardStagedCommand is the line guard staged is run by, as the journal
// names it.
const GuardStagedCommand = "gragas guard staged"

// Replayer runs one history line's command, as the line holds it, over p and
// answers the change it plans, never applied; or the refusal the command
// ended on before it planned; or an error when the line names no command
// that plans a change. The root is its owner: only it parses a command line.
type Replayer func(ctx context.Context, p port.Ports, line string) (*Change, *model.Refusal, error)

// StagedAsk is one guard staged: the repository's root, the replayer of the
// history lines' commands, and the run and engine a refusal is journalled
// under. A Run of "" is a run of the refusal's own. MergeHook is a run as
// git's pre-merge-commit hook, which git runs before it records the merge.
type StagedAsk struct {
	Root      model.Root
	Replay    Replayer
	Run       string
	Engine    string
	MergeHook bool
}

// Replay is the plan c makes over its own ports, sealed as Preview seals it,
// and the bytes each file it writes would hold, with no lock and no
// open-record check (apply.Remake).
func (c *Change) Replay(ctx context.Context) (apply.Remade, Outcome, bool) {
	return apply.Remake(ctx, c.p, c.root, c.spec)
}

// GuardStaged judges the change staged in the rulebook's repository at
// ask.Root, HEAD against the index, both read through History and never from
// the disk: allowed when every lifecycle field it changes is changed by a
// line it appends to history.jsonl, and every line it appends is one the
// line's own command writes over the tree before it (agents.guards.1, .4,
// store.consistency.4). Each appended line's command is replayed in order from
// HEAD's tree, its plan digest made again and held to the line's
// (arch.ports.19), and the files the replays write held to the index byte for
// byte; what is left must change no lifecycle field. HEAD's lines must stay
// in the index byte for byte (store.state-files.5). During a merge, as the
// worktree's own merge state names it and never a ref (History.Merging),
// every commit the merge brings in is judged first as its own commit is: by
// its lines replayed over its parent, a merge commit by the three-way rule.
// Then the merge itself is held to the three-way rule: history.jsonl is
// HEAD's lines and then the other side's that HEAD lacks, and each lifecycle
// field is as the side that changed it since the merge base leaves it.
// Whatever it cannot read, place or replay is refused, never
// allowed, and every refusal is journalled. The one error is a folder with
// no rulebook, or with one not ready to read.
//
// The lifecycle fields are those lifecycle.GuardedParts names. rulebook.yaml
// is not among them (lifecycle.UnguardedParts): its owners, like its name,
// version and governance, are allowed to change by a hand edit, in a plain
// commit and in a merge alike. The owners are display only
// (model.RulebookIdentity), no command writes them after init and migrate,
// and refusing a hand edit would leave them with no way to change. Where
// an owner reaches a value the guard holds, a command records it: import
// writes the book's first owner as a grandfathered rule's owner, with its
// history line.
func GuardStaged(ctx context.Context, p port.Ports, ask StagedAsk) (model.Guard, *model.Refusal, error) {
	g, why, hint, err := guardStaged(ctx, p, ask)
	switch {
	case errors.Is(err, ErrNoBook), errors.Is(err, ErrTwoBooks), errors.Is(err, ErrNotReady):
		return g, nil, err
	case err != nil:
		why, hint = "the repository could not be read, so the staged change is refused: "+err.Error(), "gragas validate"
	case why == "":
		return g, nil, nil
	}
	if ask.Run == "" {
		ask.Run = fmt.Sprintf("guard-%d-%s", p.Clock.Now().UnixNano(), digest.Hex([]byte(why))[:8])
	}
	if jerr := journalRefusal(context.WithoutCancel(ctx), p, GuardAsk{Root: ask.Root, Run: ask.Run, Engine: ask.Engine}, GuardStagedCommand, g, why); jerr != nil {
		why += ". The refusal could not be journalled: " + jerr.Error()
	}
	r, rerr := model.NewRefusal(model.Refused, why, nil, hint)
	return g, &r, rerr
}

// The lines to type a staged refusal ends on.
const (
	keepHistory  = "put back the lines HEAD has, as they are: history.jsonl only grows"
	useCommand   = "make the change through its command, which writes the line and the change together"
	commitApart  = "commit a hand edit on its own, then make the change through its command"
	replayAgain  = "unstage the change and run its command again over what HEAD holds"
	remakeLine   = "unstage the change and its line, then make the change again through its command with this gragas, which writes the line with the command's name as who"
	noReplayHint = "run the command again with every answer as a flag, so its line names them"
	mergeLines   = "take history.jsonl as HEAD has it, then add the other side's lines HEAD lacks, in that side's order: git merge-file --union does"
	mergeFields  = "leave each lifecycle field as the side that changed it leaves it, and make any other change through its command once the merge is committed"
	mergeBoth    = "abort the merge, make the field agree on one side through its command, then merge again"
	mergeTwice   = "abort the merge, drop one side's commit that changes the exclusion, then merge again: the other side's change stands, and any other is made through its command on the merge"
	mergeOne     = "merge one branch at a time"
	mergeCommit  = "git commit"
	mergeMoved   = "abort the merge, move the rulebook on the other side in a commit of its own, then merge again"
)

// staged is one change as it is read to be judged: the trees before and
// after, their books, history.jsonl at each, and the words its refusals use.
// For guard staged's own change the tree before is HEAD's and the tree after
// the index; for a commit a merge brings in, its parent's and its own.
type staged struct {
	ctx context.Context
	p   port.Ports
	ask StagedAsk
	n   wording
	// cache holds the trees of the commits a merge brings in, for that merge
	// alone. It is set on the staged change's own value while it judges a
	// merge; the staged values of the commits brought in carry none and reach
	// the cache through the receiver of open.
	cache         *revCache
	head, index   *revTree
	before, after model.Book
	// dir is the rulebook's folder after, and hist its history.jsonl
	dir, hist         string
	headHist, staging []byte
}

func guardStaged(ctx context.Context, p port.Ports, ask StagedAsk) (model.Guard, string, string, error) {
	if ask.Replay == nil {
		return model.Guard{}, "", "", errors.New("guard staged needs a replayer")
	}
	_, err := p.History.Files(ctx, ask.Root, "HEAD")
	noHead := errors.Is(err, port.ErrNoRev)
	if err != nil && !noHead {
		return model.Guard{}, "", "", err
	}
	s, g, why, hint, err := (*revCache)(nil).open(ctx, p, ask, side{rev: "HEAD", none: noHead}, "", theStaged())
	if why != "" || err != nil {
		return g, why, hint, err
	}
	// The merge state is the worktree's own, as git writes it, never a ref
	// a name answers: a tag or a branch named MERGE_HEAD is no merge, and
	// the staged change is judged as any other is.
	heads, err := p.History.Merging(ctx, ask.Root)
	switch {
	case err != nil:
		return g, "", "", err
	case len(heads) > 1:
		return g, fmt.Sprintf("the merge brings in %d branches at once, and a merge is judged against one other side", len(heads)), mergeOne, nil
	case len(heads) == 1:
		return s.merge(g, "HEAD", heads[0])
	case ask.MergeHook:
		return s.unrecorded(g)
	}
	return s.replay(g)
}

// side is the tree before a change: the rev, or none, a tree with no file,
// for a repository's first commit.
type side struct {
	rev  string
	none bool
}

// wording is what a change's refusals call the tree before, the change, the
// tree after, and the change of a field.
type wording struct {
	before, change, after string
	// is and are say a field changed: "is staged", "are staged"
	is, are string
}

// theStaged names the staged change, HEAD against the index.
func theStaged() wording {
	return wording{before: "HEAD", change: "the staged change", after: "as staged", is: "is staged", are: "are staged"}
}

// broughtIn names a commit a merge brings in, against its first parent.
func broughtIn(c model.Commit) wording {
	at := "commit " + short(c.ID)
	before := "its parent"
	switch len(c.Parents) {
	case 0:
		before = "no commit"
	case 1:
	default:
		before = "its first parent"
	}
	return wording{before: before, change: at, after: "at " + at, is: "is changed at " + at, are: "are changed at " + at}
}

// open reads one change to judge: the tree before at from and the tree
// after at rev to ("" is the index), their books, and history.jsonl at
// each, which must keep every line the tree before holds, byte for byte
// (store.state-files.5). For the index, a rulebook that cannot be read is
// the error it is. For a commit a merge brings in, it is refused; and a
// commit with no rulebook after a parent with none, or with more than one
// after a parent with more than one, is answered as no change to judge: a
// nil staged with nothing to say.
func (cache *revCache) open(ctx context.Context, p port.Ports, ask StagedAsk, from side, to string, n wording) (*staged, model.Guard, string, string, error) {
	s := &staged{ctx: ctx, p: p, ask: ask, n: n}
	head, before, berr, err := cache.tree(ctx, p, ask, from.rev, from.none)
	if err != nil {
		return nil, model.Guard{}, "", "", err
	}
	// a replay writes over the tree before, so it gets a fork of the cache's
	s.head = cache.treeBefore(head)
	index, after, err, ierr := cache.tree(ctx, p, ask, to, false)
	if ierr != nil {
		return nil, model.Guard{}, "", "", ierr
	}
	s.index = index
	switch {
	case err == nil:
	case to == "":
		return nil, model.Guard{}, "", "", guardLoadErr(err)
	case errors.Is(err, ErrNoBook) && errors.Is(berr, ErrNoBook),
		errors.Is(err, ErrTwoBooks) && errors.Is(berr, ErrTwoBooks):
		// no one book before or after: the commit makes none and changes none.
		// Not the staged change, which is not ready (above).
		return nil, model.Guard{}, "", "", nil
	case errors.Is(err, ErrNoBook) && berr == nil:
		g := model.Guard{Path: join(before.Dir, format.HistoryFile)}
		return nil, g, n.change + " removes the rulebook " + n.before + " holds", "put the rulebook back: no command removes one", nil
	default:
		return nil, model.Guard{}, n.change + "'s rulebook could not be read, so it cannot be judged: " + err.Error(), "gragas validate", nil
	}
	s.after, s.dir, s.hist = after.Book, after.Dir, join(after.Dir, format.HistoryFile)
	g := model.Guard{Path: s.hist}
	switch {
	case errors.Is(berr, ErrNoBook), errors.Is(berr, ErrTwoBooks):
		// the commit that makes the rulebook: its whole history is replayed
		// from a tree with nothing in it. A tree before with more than one
		// rulebook.yaml at its shallowest depth holds no one book, and the tree
		// after holds exactly one (it was read above), so the commit makes it,
		// as any commit is judged. A tree before with one book in another
		// folder is not this case: it is the move below. A tree before that
		// holds a format 2 folder is not emptied either: it holds no
		// rulebook.yaml, and the line that makes the book there is a migrate,
		// which plans over that folder and is nothing to do over a tree with
		// no file.
		if !s.head.holdsV2() {
			s.head = s.head.empty()
		}
	case berr != nil:
		return nil, g, n.before + "'s rulebook could not be read, so " + n.change + " cannot be judged: " + berr.Error(), "gragas validate", nil
	case before.Dir != after.Dir:
		return nil, g, n.change + " moves the rulebook from " + orRoot(before.Dir) + " to " + orRoot(after.Dir), "move a rulebook in a commit of its own, through no command", nil
	default:
		s.before = before.Book
	}
	if s.headHist, err = readOr(s.head, ask.Root, s.hist); err != nil {
		return nil, g, "", "", err
	}
	if s.staging, err = readOr(s.index, ask.Root, s.hist); err != nil {
		return nil, g, "", "", err
	}
	if !bytes.HasPrefix(s.staging, s.headHist) {
		return nil, g, s.hist + " " + n.after + " does not keep every line " + n.before + " has, byte for byte: a line was rewritten, removed or put in another order", keepHistory, nil
	}
	return s, g, "", "", nil
}
