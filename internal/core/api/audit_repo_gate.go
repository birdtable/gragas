package coreapi

import (
	"slices"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/model"
)

// The three repository checks as the project check lists them:
// each finding a model.GateFinding of its kind, and each check not made a
// model.NotChecked row, which halts the run only when a whole check whose
// rules bind could not be made for a cause the project can cure.

// repoRow is a check not made: whether it halts the run (a whole check whose
// rules bind could not be made, for a cause the project can cure), the cause,
// and how many of its rules bind.
type repoRow struct {
	audit.NotChecked
	halts    bool
	cause    model.GateCause
	bindings int
}

// row says a check, or a part of one, was not made, and leaves the ending as
// it is: no rule binds it, or the project chose so (no list, an empty list).
func (r *repoRun) row(key, why string) {
	r.Rows = append(r.Rows, repoRow{NotChecked: audit.Unjudged(key, why)})
}

// part says a part of a check was not made: the rest was judged, and the
// ending is as it is.
func (r *repoRun) part(key, why string) {
	n := audit.Unjudged(key, why)
	n.Partial = true
	r.Rows = append(r.Rows, repoRow{NotChecked: n})
}

// unfolded takes the rows of the tracked-files check for a path that was not
// compared for a twin that differs only by case (a name of it no system can
// hold). Each is a part of the check, as a file too large to read
// is a part of the names check: the links and every other path were judged, so
// it has no cause of model.GateCauses and leaves the ending as it is. It goes
// with the run's other rows, so masked reads it: the row holds a tracked path
// and the port's reason, and either may hold a name of the list.
func (r *repoRun) unfolded(rows []audit.NotChecked) {
	for _, n := range rows {
		n.Partial = true
		r.Rows = append(r.Rows, repoRow{NotChecked: n})
	}
}

// cannot says a whole check could not be made for a cause the project can
// cure. When a rule binds the check the row halts the run: it ends not ready,
// since a check not made is no pass, and the row carries cause for the caller
// that says what cures it. With none bound it is a row.
func (r *repoRun) cannot(key string, cause model.GateCause, why string) {
	n := bound(r.rules, key)
	r.Rows = append(r.Rows, repoRow{NotChecked: audit.Unjudged(key, why), halts: n > 0, cause: cause, bindings: n})
}

// bound is how many rules bind a check: one key for names, two each for the
// tracked files (link, fold) and the counts (debt, directive).
func bound(rules audit.RepoRules, check string) int {
	keys := map[string][]string{audit.KeyNames: {audit.KeyNames}, audit.KeyTracked: {audit.KeyLink, audit.KeyFold},
		audit.KeyCounts: {audit.KeyDebt, audit.KeyDirective}}[check]
	n := 0
	for _, k := range keys {
		if rules.Has(k) {
			n++
		}
	}
	return n
}

// masked is what the run found in both shapes, each path, message and reason
// with the names of the list hidden.
func (r repoRun) masked(l NameList) RepoFindings {
	out := RepoFindings{}
	for _, f := range r.Findings {
		f.File, f.Missing = maskName(l, f.File), maskName(l, f.Missing)
		c := ConfigFinding(f)
		out.Findings = append(out.Findings, c)
		g := model.GateFinding{Kind: repoKind(f.Key), Entity: c.where(),
			Msg: c.Rule + " (" + c.Tool + " " + c.Key + "): " + c.Missing, Fix: repoFix(f.Key)}
		if r.tracked[f.File] {
			g.File, g.Line = f.File, f.Line
		}
		out.Gate = append(out.Gate, g)
	}
	slices.SortStableFunc(out.Gate, func(x, y model.GateFinding) int {
		return slices.Index(model.ProjectKinds(), x.Kind) - slices.Index(model.ProjectKinds(), y.Kind)
	})
	for _, n := range r.Rows {
		n.Why = maskName(l, n.Why)
		out.NotChecked = append(out.NotChecked, ConfigNotChecked(n.NotChecked))
		if n.halts {
			out.Unjudged = append(out.Unjudged, model.NotChecked{Tool: n.Tool, Bindings: n.bindings, Cause: n.cause, Msg: n.Why})
		} else {
			out.Unjudged = append(out.Unjudged, model.NotChecked{Msg: n.String()})
		}
	}
	return out
}

// repoKind is the kind of the project check a finding of key is listed under.
func repoKind(key string) model.GateKind {
	switch key {
	case audit.KeyNames:
		return model.GateNames
	case audit.KeyLink, audit.KeyFold:
		return model.GateTrackedFiles
	}
	return model.GateCounts
}

// repoFix is the line that mends a finding of key.
func repoFix(key string) string {
	switch key {
	case audit.KeyNames:
		return "take the name out, and out of the commit that holds it before the branch is pushed"
	case audit.KeyLink:
		return "track the file the link points at in its place"
	case audit.KeyFold:
		return "rename the paths so that no two differ only by case"
	}
	return "take the new line out, or take out an old one of its kind"
}

// AddTo is p with the findings and rows of the three checks after its own:
// they are the last kinds of model.ProjectKinds, so p's order holds. A row of
// p's own that ends the run is a tool that could not be asked, the one cause
// the tool configs and the tier contract have, and is given it here, in one
// place, so that no row that ends the run is without its cause.
func (r RepoFindings) AddTo(p model.ProjectCheck) model.ProjectCheck {
	p.Findings = append(slices.Clone(p.Findings), r.Gate...)
	p.NotChecked = slices.Clone(p.NotChecked)
	for i, n := range p.NotChecked {
		if n.Tool != "" {
			p.NotChecked[i].Cause = model.CauseTool
		}
	}
	p.NotChecked = append(p.NotChecked, r.Unjudged...)
	return p
}

// cannotAll says none of the three checks could be made, and why.
func (r *repoRun) cannotAll(cause model.GateCause, why string) {
	for _, k := range []string{audit.KeyNames, audit.KeyTracked, audit.KeyCounts} {
		r.cannot(k, cause, why)
	}
}
