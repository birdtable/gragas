package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// BaseRefKey is the settings key that names the base ref, and BaseRefVar
// the variable that names it over the key. The gate holds HEAD's history
// against the merge base of HEAD and it; with none named it does not guess
// one, and check is not ready.
const (
	BaseRefKey = "base-ref"
	BaseRefVar = "GRAGAS_BASE_REF"
)

// CheckAsk is one check in a rulebook: the repository's root, the base ref
// the settings name, "" for none, the names file validate reads, the
// replayer of the history lines' commands, as guard staged's, and the opener
// of the repository's history. OpenHistory is called only once the folder is
// known to hold one rulebook: a folder with none is the old check's, which
// needs no git.
type CheckAsk struct {
	Root        model.Root
	BaseRef     string
	NamesFile   string
	Replay      Replayer
	OpenHistory func() (port.History, error)
}

// CheckRulebook is gragas check in a rulebook (S14, S40, A38): validate's
// findings of the working folder, then the gate over HEAD's history against
// the merge base of HEAD and the base ref, read through History and never
// from the disk (cli.process.2). Each commit since the merge base is judged
// as guard staged judges a commit a merge brings in, so a change committed
// with the hooks bypassed is found as the hook would have found it
// (agents.guards.1). The listing and its word come from this one function
// (cli.commands.17). No base ref, no git, a shallow clone, or a base ref with
// no one merge base with HEAD is a refusal, not ready, naming what to set or
// fetch. The error is a folder with no rulebook, or one not ready to read,
// as Validate's, or a repository that cannot be read.
//
// A working folder with no rulebook.yaml, or more than one, is the old
// check's (ErrNoBook, ErrTwoBooks) only when no base ref is named or the
// merge base holds no one rulebook either: where it holds one, the branch
// has removed or doubled it, and that is the gate's finding, so what the
// branch holds never switches the gate off. With a base ref named, a merge
// base that cannot be found is not ready here too.
func CheckRulebook(ctx context.Context, p port.Ports, ask CheckAsk) (model.RulebookCheck, *model.Refusal, error) {
	// The one rulebook first, before the names file or the journal is read.
	if _, err := format.Find(p.Tree, ask.Root); err != nil {
		if !noOneBook(err) || ask.BaseRef == "" || ask.Replay == nil || ask.OpenHistory == nil {
			return model.RulebookCheck{}, nil, err
		}
		return checkGone(ctx, p, ask, err)
	}
	v, err := Validate(ctx, p, ask.Root, ask.NamesFile)
	if err != nil {
		return model.RulebookCheck{}, nil, err
	}
	out := model.RulebookCheck{Validation: v, BaseRef: ask.BaseRef, Unavailable: lifecycle.NotAvailable()}
	if ask.BaseRef == "" {
		r, err := model.NewRefusal(model.NotReady,
			"no base ref is named, so there is no merge base to hold the history to: set "+BaseRefKey+" in the settings file, or "+BaseRefVar, nil,
			"base-ref: origin/main")
		return out, &r, err
	}
	if ask.Replay == nil || ask.OpenHistory == nil {
		return out, nil, errors.New("check needs a replayer and a history")
	}
	h, err := ask.OpenHistory()
	if r, ok := AsRefusal(err); ok {
		return out, r, nil
	}
	if err != nil {
		return out, nil, err
	}
	p.History = h
	in, ref, err := gateIn(ctx, p, ask, &out)
	if ref != nil || err != nil {
		return out, ref, err
	}
	out.Gate = lifecycle.Gate(in)
	return out, nil, ctx.Err()
}

// gateIn reads what the gate judges: the books and history.jsonl at HEAD and
// at the merge base, each commit since judged, and the releases.
func gateIn(ctx context.Context, p port.Ports, ask CheckAsk, out *model.RulebookCheck) (lifecycle.GateIn, *model.Refusal, error) {
	base, err := p.History.Base(ctx, ask.Root, ask.BaseRef, "HEAD")
	if r, ok := gitRefusal(err, ask.BaseRef); ok {
		return lifecycle.GateIn{}, r, nil
	}
	if err != nil {
		return lifecycle.GateIn{}, nil, err
	}
	out.Base = base
	head, err := newRevTree(ctx, p.Tree, p.History, ask.Root, "HEAD", false)
	if err != nil {
		return lifecycle.GateIn{}, nil, err
	}
	at, err := newRevTree(ctx, p.Tree, p.History, ask.Root, base, false)
	if err != nil {
		return lifecycle.GateIn{}, nil, err
	}
	var in lifecycle.GateIn
	was, err := format.Load(at, p.Schema, ask.Root)
	switch {
	case noOneBook(err):
		// No book, or two (the old format, or two fixtures tying at one
		// depth): there is no one base book to hold HEAD to.
		in.NoBase = true
	case err != nil:
		return in, nil, fmt.Errorf("the merge base's rulebook could not be read: %w", err)
	default:
		in.Base = was.Book
		if in.BaseHistory, err = readOr(at, ask.Root, join(was.Dir, format.HistoryFile)); err != nil {
			return in, nil, err
		}
	}
	now, err := format.Load(head, p.Schema, ask.Root)
	if noOneBook(err) && !in.NoBase {
		// The working folder holds one, HEAD none or two: committed so.
		in.Gone, in.GoneFile = "HEAD "+holds(err), join(was.Dir, format.IdentityFile)
		return in, nil, nil
	}
	if err != nil {
		return in, nil, fmt.Errorf("HEAD's rulebook could not be read: %w", err)
	}
	if !in.NoBase && now.Dir != was.Dir {
		// The shallowest book wins, so a branch can move the book by adding a
		// shallower one: the gate judges the one book of the merge base.
		in.Gone, in.GoneFile = "HEAD's rulebook is "+join(now.Dir, format.IdentityFile)+", so the old one is no longer judged", join(was.Dir, format.IdentityFile)
		return in, nil, nil
	}
	hist := join(now.Dir, format.HistoryFile)
	in.Book, in.HistoryFile = now.Book, hist
	if in.History, err = readOr(head, ask.Root, hist); err != nil {
		return in, nil, err
	}
	if in.Unrecorded, out.Commits, err = judgeCommits(ctx, p, ask, base, hist); err != nil {
		return in, nil, err
	}
	in.Releases, out.Releases, err = releases(ctx, p, ask.Root, hist)
	if err == nil && out.Releases == 0 {
		out.Unavailable = append(out.Unavailable, lifecycle.NoReleaseRead())
	}
	return in, nil, err
}

// judgeCommits judges each commit since base as guard staged judges a commit
// a merge brings in: against its own parent, so a branch that merged
// another holds each side's plans over the tree they were made on. Every
// commit is judged, never only up to the first refused: a commit after a
// hand edit is held to its own lines over a parent that holds the edit.
func judgeCommits(ctx context.Context, p port.Ports, ask CheckAsk, base, hist string) ([]model.GateFinding, int, error) {
	commits, err := p.History.Commits(ctx, ask.Root, base, "HEAD")
	if err != nil {
		return nil, 0, err
	}
	s := &staged{ctx: ctx, p: p, ask: StagedAsk{Root: ask.Root, Replay: ask.Replay}}
	var out []model.GateFinding
	for _, c := range commits {
		g, why, hint, err := s.broughtIn(c)
		if errors.Is(err, port.ErrNoBase) {
			// A merge whose sides have no one merge base cannot be held to
			// one: it is a finding, never a pass.
			why, hint, err = "it merges two sides with no one merge base: "+err.Error(), "make the merge again from sides that share one merge base", nil
		}
		switch {
		case err != nil:
			return nil, 0, err
		case why == "":
		case len(g.Changes) > 0:
			out = append(out, lifecycle.Unrecorded(short(c.ID), g.Changes)...)
		default:
			file := g.Path
			if file == "" {
				file = hist
			}
			out = append(out, lifecycle.UnrecordedCommit(short(c.ID), file, why, hint))
		}
	}
	return out, len(commits), ctx.Err()
}

// releases are the repository's release tags, each with the digests of the
// entries history.jsonl holds at it, and how many release tags it read. A tag
// that is not of the release form (lifecycle.IsReleaseTag) is no release and
// is not read; a release tag with no history.jsonl at that path shipped no
// entry, and is read but left out of the list.
func releases(ctx context.Context, p port.Ports, root model.Root, hist string) ([]lifecycle.Release, int, error) {
	tags, err := p.History.Tags(ctx, root)
	if err != nil {
		return nil, 0, err
	}
	var out []lifecycle.Release
	read := 0
	for _, t := range tags {
		if !lifecycle.IsReleaseTag(t.Name) {
			continue
		}
		read++
		b, err := p.History.Blob(ctx, root, t.Commit, hist)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		r := lifecycle.Release{Name: t.Name}
		for _, e := range lifecycle.ReadFile(b).Entries {
			r.Entries = append(r.Entries, e.Digest)
		}
		out = append(out, r)
	}
	return out, read, ctx.Err()
}

// AsRefusal is err as a refusal when an adapter made it one: git's not
// ready (no git, one too old, a shallow clone) or refused (git at a path it
// will not run). Core never imports the adapter, so it is known by its
// methods.
func AsRefusal(err error) (*model.Refusal, bool) {
	r, ok := errors.AsType[interface {
		error
		Word() model.Word
		Observed() string
		Hint() string
	}](err)
	if !ok {
		return nil, false
	}
	out, nerr := model.NewRefusal(r.Word(), r.Observed(), nil, r.Hint())
	return &out, nerr == nil
}

// gitRefusal is err as the refusal check ends on, when it is one: git's own
// not ready or refused, which names what to set, a base ref the repository
// does not have, or one with no one merge base with HEAD.
func gitRefusal(err error, ref string) (*model.Refusal, bool) {
	if err == nil {
		return nil, false
	}
	if r, ok := AsRefusal(err); ok {
		return r, true
	}
	var why, hint string
	switch {
	case errors.Is(err, port.ErrNoRev):
		why, hint = "the base ref "+ref+" is not in this repository, so there is no merge base to hold the history to", "git fetch origin, then gragas check again"
	case errors.Is(err, port.ErrNoBase):
		why, hint = "the base ref "+ref+" and HEAD have no one merge base: "+err.Error(), "set "+BaseRefKey+" to the branch this one was made from"
	default:
		return nil, false
	}
	out, nerr := model.NewRefusal(model.NotReady, why, nil, hint)
	return &out, nerr == nil
}

// CheckSchema is the schema number of Check, the document check in a
// rulebook writes under --json.
const CheckSchema = 1

// Check is check's document under --json in a rulebook. Validate is
// validate's own document of the working folder; Gate the gate's findings,
// in the order of their kinds; NotAvailable each value this build cannot
// give, null with the ticket that adds it. Base is the merge base's full
// name, "" when the run ended before it was found.
type Check struct {
	Schema       int                 `json:"schema"`
	Contract     string              `json:"contract"`
	Command      string              `json:"command"`
	Word         string              `json:"word"`
	BaseRef      string              `json:"base_ref"`
	Base         string              `json:"base"`
	Commits      int                 `json:"commits"`
	Releases     int                 `json:"releases"`
	Validate     Validation          `json:"validate"`
	Gate         []model.GateFinding `json:"gate"`
	NotAvailable []model.Unavailable `json:"not_available"`
}

// CheckDoc is c as the document, listing at most limit of validate's
// findings. Every gate finding is listed: a gate finding is a commit's or a
// line's, and there are as many as the branch made.
func CheckDoc(c model.RulebookCheck, limit int) Check {
	v := ValidationDoc(c.Validation, limit, "")
	v.Command = "check"
	return Check{Schema: CheckSchema, Contract: model.ContractVersion, Command: "check", Word: string(c.Word()),
		BaseRef: c.BaseRef, Base: c.Base, Commits: c.Commits, Releases: c.Releases, Validate: v,
		Gate: append([]model.GateFinding{}, c.Gate...), NotAvailable: append([]model.Unavailable{}, c.Unavailable...)}
}
