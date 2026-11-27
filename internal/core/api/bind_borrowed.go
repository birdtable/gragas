package coreapi

// bind, rebind and unbind of a rule the rulebook borrows (S34, S70): a
// parent's rule in an overlay, read from the parent's bundle in the releases
// cache (A50), or an imported one. The lifecycle context decides; this file
// reads what it decides on and hands it in as arguments (arch.ports.8): the
// lender's rule with its bindings, the rulebook's own records of it, read by
// link, and link's Tighten as the guard, with the rank op of the installed
// plugin that serves the tool. A record is the rulebook's own, in bindings/, under its
// own prefix; the lender's rule is never written.

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Bind is the change that adds a binding to a rule of the rulebook in
// ask.Root, or, for a rule it borrows, writes a record that tightens it. A
// borrowed rule of a parent that is not in the cache at its pin is
// ErrNotReady, "parent not fetched", naming the pin. A value of a tool no
// installed plugin serves, compared with the lender's, is UnservedTools.
func Bind(ctx context.Context, p port.Ports, ask OverlayAsk, req model.BindRule) (*Change, *model.Refusal, error) {
	return bindPlans().to(ctx, p, ask, req.ID, &req, &req.Files)
}

// Rebind is the change that moves a binding of a rule to another entry, or
// changes its value, severity or remedy; of a borrowed rule, only a change
// that tightens it, as a record. Whether a changed value loosens is the rank
// op's word of the plugin that serves the tool, asked afresh at each reading;
// a value of a tool no installed plugin serves, compared, is UnservedTools.
func Rebind(ctx context.Context, p port.Ports, ask OverlayAsk, req model.RebindRule) (*Change, *model.Refusal, error) {
	return rebindPlans().to(ctx, p, ask, req.ID, &req, &req.Files)
}

// Unbind is the change that takes a binding out of a rule; of a borrowed
// rule it is refused.
func Unbind(ctx context.Context, p port.Ports, ask OverlayAsk, req model.UnbindRule) (*Change, *model.Refusal, error) {
	return unbindPlans().to(ctx, p, ask, req.ID, &req, &req.Files)
}

// plans are the two plans of bind, rebind or unbind: own, for a rule of the
// rulebook's own, and lent, for one it borrows. ranked is the request a
// reading plans, its own plan's values ranked by that reading's rank; nil
// for a request whose own plan ranks no value.
type plans[R any] struct {
	own    func(model.Book, R, lifecycle.Moment) (lifecycle.Plan, error)
	lent   func(model.Book, lifecycle.Lent, R, lifecycle.Compare, lifecycle.Moment) (lifecycle.Plan, error)
	ranked func(R, func(tool string) link.Rank) R
}

func bindPlans() plans[model.BindRule] {
	return plans[model.BindRule]{own: lifecycle.PlanBind, lent: lifecycle.PlanBindBorrowed}
}

// rebindPlans rank a changed value of the rulebook's own rule by the
// reading's rank, where the request brings no Loosens of its own.
func rebindPlans() plans[model.RebindRule] {
	return plans[model.RebindRule]{own: lifecycle.PlanRebind, lent: lifecycle.PlanRebindBorrowed,
		ranked: func(r model.RebindRule, rank func(tool string) link.Rank) model.RebindRule {
			if r.Loosens == nil {
				r.Loosens = func(old, next model.Binding) bool { return link.Loosens(rank(old.Tool), old, next) }
			}
			return r
		}}
}

func unbindPlans() plans[model.UnbindRule] {
	return plans[model.UnbindRule]{own: lifecycle.PlanUnbind, lent: unbindBorrowed}
}

// unbindBorrowed is the lent plan of unbind, which compares nothing: every
// unbind of a borrowed binding weakens it.
func unbindBorrowed(_ model.Book, l lifecycle.Lent, r model.UnbindRule, _ lifecycle.Compare, _ lifecycle.Moment) (lifecycle.Plan, error) {
	return lifecycle.PlanUnbindBorrowed(l, r)
}

// to plans req of the rule id by pl's own plan or its lent one, and fills in
// files, the request's files. Each reading ranks with rankers of its own,
// opened as it ranks and closed before it ends, so the first reading, the
// dry run and the apply each ask the plugins installed then: what ended a
// rank is that reading's error, in place of what it planned or refused.
func (pl plans[R]) to(ctx context.Context, p port.Ports, ask OverlayAsk, id string, req *R, files *model.PlanFiles) (*Change, *model.Refusal, error) {
	at := func(l format.Loaded) (model.PlanFiles, error) {
		if borrows(l.Book, model.RuleID(id)) {
			return model.PlanFiles{Bindings: path.Join(l.Dir, format.BindingsDir), History: path.Join(l.Dir, format.HistoryFile)}, nil
		}
		return byID(id, false)(l)
	}
	return composeTo(ctx, p, ask.ChangeAsk, at, func(ctx context.Context, b model.Book, f model.PlanFiles, m lifecycle.Moment) (lifecycle.Plan, error) {
		*files = f
		rk := newRankers(p.Plugins, ask.Root).reading(b.Identity)
		out, err := pl.plan(ctx, asked{p, ask, model.RuleID(id)}, b, *req, rk, m)
		cerr := rk.close()
		if _, refused := errors.AsType[*lifecycle.Refusal](err); cerr != nil && (err == nil || refused) {
			return lifecycle.Plan{}, cerr
		}
		return out, err
	})
}

// asked is what a request is planned against: the ports, the overlay
// asked of and the rule.
type asked struct {
	p   port.Ports
	ask OverlayAsk
	id  model.RuleID
}

// plan is one reading's plan of req, ranked by rk.
func (pl plans[R]) plan(ctx context.Context, a asked, b model.Book, req R, rk *rankers, m lifecycle.Moment) (lifecycle.Plan, error) {
	if pl.ranked != nil {
		req = pl.ranked(req, rk.of(ctx))
	}
	if !borrows(b, a.id) {
		return pl.own(b, req, m)
	}
	l, err := lentOf(a.p, a.ask, b, a.id)
	if err != nil {
		return lifecycle.Plan{}, err
	}
	out, err := pl.lent(b, l, req, guard(rk.of(ctx)), m)
	if err != nil || len(out.Delta.Changes) == 0 {
		return out, err
	}
	return withBorrowedStanding(out, l)
}

// borrows says whether id is a rule the book borrows: one it imported, or,
// in an overlay with a parent, one that is not its own. A rule of the book's
// own, and an id no rule of a book with no parent takes, are planned as the
// book's own, which refuses an id it does not hold.
func borrows(b model.Book, id model.RuleID) bool {
	if i := slices.IndexFunc(b.Rules, func(r model.Rule) bool { return r.ID == id }); i >= 0 {
		return b.Rules[i].Origin == model.Imported
	}
	return b.Identity.Kind == model.KindOverlay && b.Identity.Extends != nil
}

// lentOf is the borrowed rule id as its lender holds it, with the book's
// records of it. A record of bindings/ that cannot be read refuses the plan:
// what the rule is held to is not known while one is unread.
func lentOf(p port.Ports, ask OverlayAsk, b model.Book, id model.RuleID) (lifecycle.Lent, error) {
	read, bad := link.ReadRecords(b.BindingRecords)
	if len(bad) > 0 {
		return lifecycle.Lent{}, &lifecycle.Refusal{Msg: "a binding record cannot be read, so what a borrowed rule is held to is not known: " + bad[0].String(),
			Hint: "gragas validate lists each record it cannot read; mend the record, then plan again"}
	}
	var mine []model.Tightening
	for _, t := range read {
		if t.Subject == id {
			mine = append(mine, t)
		}
	}
	if i := slices.IndexFunc(b.Rules, func(r model.Rule) bool { return r.ID == id }); i >= 0 {
		return lifecycle.Lent{From: importOf(b, id), Imported: true, Rule: b.Rules[i], Records: mine}, nil
	}
	doc, err := parentBundle(p, ask.Contracts, ask.Cache, b)
	if err != nil {
		return lifecycle.Lent{}, err
	}
	e := b.Identity.Extends
	pin := e.Rulebook + " " + e.Version
	layer := parentLayer(e.Rulebook, doc)
	i := slices.IndexFunc(layer.Book.Rules, func(r model.Rule) bool { return r.ID == id })
	if i < 0 {
		return lifecycle.Lent{}, &lifecycle.Refusal{Msg: fmt.Sprintf("%s is no rule of this rulebook, nor of its parent %s", id, pin),
			Hint: "list the rules of the chain with gragas resolve"}
	}
	if slices.ContainsFunc(b.Exclusions, func(x model.Exclusion) bool { return x.Rule == id && x.Lifted == nil }) {
		return lifecycle.Lent{}, &lifecycle.Refusal{Msg: fmt.Sprintf("%s is excluded from this overlay, so it holds no binding to tighten", id),
			Hint: "bring it back first with gragas rule include " + string(id)}
	}
	return lifecycle.Lent{From: "the parent " + pin, Rule: layer.Book.Rules[i], Records: mine}, nil
}

// importOf names the import a rule of the book came from: the one whose
// parts hold the rule's part.
func importOf(b model.Book, id model.RuleID) string {
	part, _, _ := strings.Cut(string(id), ".")
	for _, im := range b.Identity.Imports {
		if slices.Contains(im.Parts, part) {
			return "the import " + im.Rulebook + " " + im.Version
		}
	}
	return "an import"
}

// guard is link's Tighten as the lifecycle context's Compare, each binding's
// value ranked by the plugin of the tool it was at.
func guard(rank func(tool string) link.Rank) lifecycle.Compare {
	return func(was, next model.Binding) (bool, string, string) {
		same, against := link.Tighten(rank(was.Tool), was, next)
		if against != nil {
			return false, against.Field, against.Msg
		}
		return same, "", ""
	}
}

// withBorrowedStanding is the plan with what it does to the borrowed rule:
// no preset of the rulebook's own moves, and the rule's standing before and
// after, with the proof to run for a binding it adds. The record the plan
// writes is read back by link as any record is, so a plan whose record could
// not be read is never shown.
func withBorrowedStanding(pl lifecycle.Plan, l lifecycle.Lent) (lifecycle.Plan, error) {
	after := slices.Clone(l.Records)
	for _, c := range pl.Delta.Changes {
		key, ok := strings.CutPrefix(c.Entity, "record:")
		if !ok {
			continue
		}
		t, bad := link.ReadRecord(key, []byte(c.After))
		if len(bad) > 0 {
			return lifecycle.Plan{}, fmt.Errorf("the record the plan writes cannot be read back: %s", bad[0])
		}
		if i := slices.IndexFunc(after, func(o model.Tightening) bool { return o.Path == key }); i >= 0 {
			after[i] = t
		} else {
			after = append(after, t)
		}
	}
	was := model.Book{Rules: []model.Rule{link.Borrowed(l.Rule, l.Records)}}
	now := model.Book{Rules: []model.Rule{link.Borrowed(l.Rule, after)}}
	_, proof := standingOf(was, now)
	var standing []string
	before, err := link.Derive(was, nil, model.Chain{})
	next, nerr := link.Derive(now, nil, model.Chain{})
	if err == nil && nerr == nil && len(before) == 1 && len(next) == 1 {
		standing = []string{fmt.Sprintf("%s: %s before, %s after", l.Rule.ID, before[0], next[0])}
	}
	return lifecycle.WithStanding(lifecycle.WithPresets(pl, nil, nil), standing, proof), nil
}

// recordScope is the pattern of the records a plan of a borrowed rule's
// binding may write: a file directly in the book's bindings/, whose name only
// the plan knows. None for a plan that names no bindings folder.
func recordScope(f model.PlanFiles) []string {
	if f.Bindings == "" {
		return nil
	}
	return []string{f.Bindings + "/*.yaml"}
}

// recordFiles are the files of the records a delta writes, below the book's
// folder dir.
func recordFiles(dir string, d model.Delta) []string {
	var out []string
	for _, c := range d.Changes {
		if key, ok := strings.CutPrefix(c.Entity, "record:"); ok {
			out = append(out, path.Join(dir, key))
		}
	}
	return out
}
