package coreapi

// rule new and rule edit: the two lifecycle plans that write a rule's file.
// Each command is one composition of the same steps, in the order the write
// path wants them (architecture 5): load the book, ask the lifecycle context
// for the plan, render its delta into the files it changes, and hand the
// write path the history line the yes adds. This file decides nothing of
// rules and holds no write logic: the context plans, format renders and
// internal/apply writes.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// RuleNew is the change that adds a draft rule to the rulebook in ask.Root.
// A request the lifecycle context refuses comes back as a refusal with the
// line to type, and no change; an error is a book that cannot be read or an
// identity that is not configured.
func RuleNew(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.NewRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.Part+"."+req.Concern+".0", withFiles(&req, &req.Files, lifecycle.PlanRuleNew))
}

// RuleEdit is the change that edits the lifecycle fields of a rule of the
// rulebook in ask.Root, with the same answers as RuleNew.
func RuleEdit(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.EditRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanRuleEdit))
}

// planOf is a lifecycle plan function with its request bound, run once a
// reading over that reading's ctx.
type planOf func(context.Context, model.Book, model.PlanFiles, lifecycle.Moment) (lifecycle.Plan, error)

// withFiles binds a request to its lifecycle plan function: each reading
// sets the files the plan reads, at files, then plans req.
func withFiles[R any](req *R, files *model.PlanFiles, plan func(model.Book, R, lifecycle.Moment) (lifecycle.Plan, error)) planOf {
	return func(_ context.Context, b model.Book, f model.PlanFiles, m lifecycle.Moment) (lifecycle.Plan, error) {
		*files = f
		return plan(b, *req, m)
	}
}

// made is one reading of the book and the plan made from it: what Render and
// the record need after the plan function has run.
type made struct {
	l       format.Loaded
	plan    lifecycle.Plan
	files   model.PlanFiles
	at      model.Yes
	presets presetChange
}

// filesOf names the files a plan writes in the book as it was loaded. A
// name the book cannot place is a refusal.
type filesOf func(format.Loaded) (model.PlanFiles, error)

// byID are the files of a rule plan for the rule id, of a term plan for an
// id "", or of a source plan for sourcePlan. A term plan writes the terms
// file and the history (and a decision when it takes one), and no rule's
// file; a source plan writes the sources file and the history, whatever rules
// it notes. Where two rules took the id, later names the file of the later
// rule: the rule a renumber means.
func byID(id string, later bool) filesOf {
	return func(l format.Loaded) (model.PlanFiles, error) {
		files := model.PlanFiles{History: path.Join(l.Dir, format.HistoryFile)}
		switch id {
		case "":
			files.Terms, files.Decisions = path.Join(l.Dir, format.TermsFile), path.Join(l.Dir, format.DecisionsFile)
			return files, nil
		case decisionPlan:
			// a decision plan: the decisions file, the history and every rules
			// file that holds a rule standing on a decision, for the rules it
			// moves on
			files.Decisions = path.Join(l.Dir, format.DecisionsFile)
			files.RuleFiles, files.RuleSites = decisionRuleFiles(l)
			return files, nil
		case sourcePlan:
			files.Sources = path.Join(l.Dir, format.SourcesFile)
			return files, nil
		}
		rules, err := format.RuleFile(l, id)
		if err != nil && strings.Contains(id, ".") && borrows(l.Book, model.RuleID(id)) {
			// a rule outside the overlay's own parts is its parent's, or one no
			// layer holds: either way the overlay writes none there (S4)
			concern := id[:strings.LastIndex(id, ".")]
			return model.PlanFiles{}, &lifecycle.Refusal{
				Msg:  fmt.Sprintf("%s is no concern of this overlay's own parts, and an overlay never writes its parent's rule in place nor adds a rule in a part it does not hold (S4)", concern),
				Hint: "tighten a parent's rule with gragas bind, or exclude it with gragas rule exclude; a new rule goes in a part of this overlay's own"}
		}
		if err != nil {
			return model.PlanFiles{}, &lifecycle.Refusal{Msg: err.Error(), Hint: "name a part and a concern that structure.yaml holds"}
		}
		if site, ok := l.Clashed[model.RuleID(id)]; ok && later {
			rules = site.File
		}
		files.Rules, files.Sources, files.Decisions = rules, path.Join(l.Dir, format.SourcesFile), path.Join(l.Dir, format.DecisionsFile)
		return files, nil
	}
}

// exclusionFiles are the files of an exclusion plan: the overlay's
// exclusions, the sources and decisions its decision may add, and the
// history. No rule's file.
func exclusionFiles(l format.Loaded) (model.PlanFiles, error) {
	return model.PlanFiles{
		Exclusions: path.Join(l.Dir, format.ExclusionsFile), Sources: path.Join(l.Dir, format.SourcesFile),
		Decisions: path.Join(l.Dir, format.DecisionsFile), History: path.Join(l.Dir, format.HistoryFile),
	}, nil
}

func compose(ctx context.Context, p port.Ports, ask model.ChangeAsk, id string, plan planOf) (*Change, *model.Refusal, error) {
	return composeTo(ctx, p, ask, byID(id, false), plan)
}

// composeAt is compose, and where the id is one that two rules took, it writes
// to the file of the later rule when later is set: the rule a renumber means.
func composeAt(ctx context.Context, p port.Ports, ask model.ChangeAsk, id string, plan planOf, later bool) (*Change, *model.Refusal, error) {
	return composeTo(ctx, p, ask, byID(id, later), plan)
}

func composeTo(ctx context.Context, p port.Ports, ask model.ChangeAsk, to filesOf, plan planOf) (*Change, *model.Refusal, error) {
	// The first reading only asks whether the request can be planned at all,
	// so a bad request ends "refused" with its hint and not as a failed plan.
	first, err := reading(ctx, p, ask, to, plan)
	if ref, ok := errors.AsType[*lifecycle.Refusal](err); ok {
		w := ref.Word
		if w == "" {
			w = model.Refused
		}
		r, nerr := model.NewRefusal(w, ref.Msg, nil, ref.Hint)
		return nil, &r, nerr
	}
	if err != nil {
		return nil, nil, err
	}
	var cur made
	spec := Spec{
		Command: ask.Command, Applies: ask.Applies, Engine: ask.Engine, Run: ask.Run,
		Scope: append(append(written(first.files), first.presets.written...), recordScope(first.files)...),
		Plan: func(ctx context.Context) (model.Plan, error) {
			m, err := reading(ctx, p, ask, to, plan)
			if err != nil {
				return model.Plan{}, err
			}
			cur = m
			in, err := inputs(p, ask.Root, m.l, m.files, append(slices.Clone(m.presets.written), recordFiles(m.l.Dir, m.plan.Delta)...)...)
			if err != nil {
				return model.Plan{}, err
			}
			y := m.at
			return model.Plan{Command: ask.Command, Delta: m.plan.Delta, Sections: m.plan.Sections, Inputs: in, Yes: &y}, nil
		},
		Render: func(d model.Delta) ([]model.Target, error) {
			ts, err := format.Render(p.Tree, p.Schema, ask.Root, cur.l, d)
			if err != nil {
				return nil, err
			}
			return append(ts, cur.presets.targets...), nil
		},
		Record: func(y model.Yes) (model.Target, error) {
			return format.HistoryRecord(p.Tree, p.Schema, ask.Root, cur.l, entries(cur.plan)...)(y)
		},
	}
	ch := NewChange(p, ask.Root, spec)
	ch.unwritten = func() []string { return cur.presets.unserved }
	return ch, nil, nil
}

// reading loads the book as it is now and plans against it, into the files
// to names.
func reading(ctx context.Context, p port.Ports, ask model.ChangeAsk, to filesOf, plan planOf) (made, error) {
	names, _, err := readBookNames(p.Owned, ask.NamesFile)
	if err != nil {
		return made{}, err
	}
	l, err := format.LoadWith(p.Tree, p.Schema, ask.Root, format.Options{Names: names})
	if nr, ok := errors.AsType[*format.NotReady](err); ok {
		return made{}, fmt.Errorf("%w: %w", ErrNotReady, nr)
	}
	if err != nil {
		return made{}, err
	}
	name, err := recorded(ask.Who)
	if err != nil {
		return made{}, err
	}
	files, err := to(l)
	if err != nil {
		return made{}, err
	}
	now := p.Clock.Now().UTC()
	pl, err := plan(ctx, l.Book, files, lifecycle.Moment{Who: name, Date: now.Format("2006-01-02"), When: now.Format("2006-01-02T15:04:05Z")})
	if err != nil {
		return made{}, err
	}
	var pc presetChange
	// only a binding moves a tool entry, and a promote moves one when its rule has a
	// binding: the plan of any other change, and a promote that writes no preset, is
	// what it was, so its plan digest, which its history line names, is too
	if slices.Contains([]string{"bind", "rebind", "unbind", "promote"}, pl.Record.Kind) {
		if pc, err = presetsOf(ctx, p, ask.Root, l, names, pl); err != nil {
			return made{}, err
		}
		if pl.Record.Kind == "promote" && len(pc.written) == 0 {
			pc = presetChange{}
		} else if touchesRules(pl.Delta) {
			pl = lifecycle.WithPresets(pl, pc.lines, pc.written)
			pl = lifecycle.WithStanding(pl, pc.standing, pc.proof)
		}
	}
	return made{l: l, plan: pl, files: files, presets: pc, at: model.Yes{Who: name, At: now.Unix(), Command: ask.Command}}, nil
}

// inputs are the files the plan read, with their digests: the files it writes
// and the book's two roots, so a plan made from another book is refused.
func inputs(p port.Ports, root model.Root, l format.Loaded, f model.PlanFiles, more ...string) ([]model.Input, error) {
	var out []model.Input
	for _, name := range append(append(written(f), more...), path.Join(l.Dir, format.IdentityFile), path.Join(l.Dir, format.StructureFile)) {
		b, err := p.Tree.Read(root, name)
		switch {
		case err == nil:
			out = append(out, model.Input{Path: name, Digest: apply.DigestBytes(b)})
		case errors.Is(err, fs.ErrNotExist):
			out = append(out, model.Input{Path: name})
		default:
			return nil, err
		}
	}
	return out, nil
}

// written are the files a plan may write, in the order the plan screen names
// them: the rule's file, the terms or the exclusions, then the sources, the
// decisions and the history. A plan that has no use for a file leaves its
// path empty.
func written(f model.PlanFiles) []string {
	var out []string
	for _, name := range append(append([]string{f.Rules}, f.RuleFileList()...), f.Terms, f.Exclusions, f.Sources, f.Decisions, f.History) {
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// entries are the lines a plan writes to the history: its record, then the
// records it adds, in order.
func entries(p lifecycle.Plan) []format.Entry {
	out := make([]format.Entry, 0, 1+len(p.Also))
	for _, r := range append([]lifecycle.Record{p.Record}, p.Also...) {
		out = append(out, format.Entry{Kind: r.Kind, Entity: r.Entity, Before: r.Before, After: r.After, Decision: r.Decision, Reverts: r.Reverts})
	}
	return out
}

// RulePromote is the change that moves a rule of the rulebook in ask.Root one
// step up its life. The release count and the preset diff are the request's:
// the count is zero until releases are counted and the diff is empty until
// bindings exist.
func RulePromote(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.PromoteRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanPromote))
}

// RuleConfirm is the change that moves the review-by of an active rule on and
// records who confirmed it.
func RuleConfirm(ctx context.Context, p port.Ports, ask model.ChangeAsk, req model.ConfirmRule) (*Change, *model.Refusal, error) {
	return compose(ctx, p, ask, req.ID, withFiles(&req, &req.Files, lifecycle.PlanConfirm))
}

// decisionRuleFiles are the rules files that hold a rule standing on a
// decision, one to a line: the files a decision plan may change. The second
// value says where each such rule lives, "<id> <file>" to a line.
func decisionRuleFiles(l format.Loaded) (files, sites string) {
	var list, at []string
	for _, r := range l.Book.Rules {
		if r.Reasoning == nil || r.Reasoning.Decision == "" {
			continue
		}
		if f, err := format.RuleFile(l, string(r.ID)); err == nil {
			at = append(at, string(r.ID)+" "+f)
			if !slices.Contains(list, f) {
				list = append(list, f)
			}
		}
	}
	return strings.Join(list, "\n"), strings.Join(at, "\n")
}
