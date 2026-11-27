package model

import "slices"

// GateKind is what one finding of a rulebook's gate is about: history held
// against the merge base, a pin, or a trial (S14, S40, A38).
type GateKind string

const (
	// GateBook is a HEAD or a working folder that holds no rulebook.yaml, or
	// more than one, where the merge base holds one: the gate is never
	// switched off by what the branch holds.
	GateBook GateKind = "rulebook gone or doubled"
	// GateRewritten is a line of the merge base's history.jsonl that is no
	// longer where it was, byte for byte: removed, rewritten or moved.
	GateRewritten GateKind = "history rewritten"
	// GateBroken is an entry whose previous digest names no entry before it,
	// or an entry after the first that names none.
	GateBroken GateKind = "history chain broken"
	// GateGap is a sequence number no entry holds although a higher one does.
	// A number two entries share is never one (A38).
	GateGap GateKind = "history gap"
	// GateUnreadable is a line of history.jsonl that is not an entry: it is
	// skipped, counted and listed.
	GateUnreadable GateKind = "history line unreadable"
	// GateUnrecorded is a lifecycle field that differs from what the entries
	// since the merge base leave it as: a change made with no entry, or an
	// entry whose command cannot be made again.
	GateUnrecorded GateKind = "lifecycle field changed without its entry"
	// GatePin is a parent or import pin that changed since the merge base
	// with no upgrade entry (S64).
	GatePin GateKind = "pin changed without its upgrade entry"
	// GateGovernance is a field of the governance that changed since the
	// merge base: no command writes the governance with its entry yet, so
	// every such change is one made by hand.
	GateGovernance GateKind = "governance changed without its entry"
	// GateTrial is a rule still on trial in as many releases as the
	// governance's trial_releases allows, or more.
	GateTrial GateKind = "trial window passed"
)

// GateKinds are the kinds in the order a listing groups them.
func GateKinds() []GateKind {
	return []GateKind{GateBook, GateRewritten, GateBroken, GateGap, GateUnreadable, GateUnrecorded, GatePin, GateGovernance, GateTrial}
}

// GateFinding is one finding of the gate. Its Finding says what it is about
// and the line that mends it.
type GateFinding struct {
	Kind GateKind `json:"kind"`
	Finding
}

// Unavailable is a value the gate cannot give yet, null with the reason
// (cli.output.9): the ticket that adds it.
type Unavailable struct {
	// Name is what the value would be, as the listing names it.
	Name string `json:"name"`
	// Value is always null.
	Value *struct{} `json:"value"`
	// Reason says why it is not given, naming the ticket that adds it.
	Reason string `json:"reason"`
}

// RulebookCheck is gragas check in a rulebook (S14, S40): validate's
// findings of the working folder, then the gate's over HEAD's history
// against the merge base of HEAD and the base ref the settings name.
type RulebookCheck struct {
	// Validation is validate's, of the working folder.
	Validation Validation
	// BaseRef is the base ref as the settings name it, and Base the full
	// object name of the merge base of it and HEAD.
	BaseRef, Base string
	// Commits is how many commits since the merge base were judged.
	Commits int
	// Releases is how many release tags the clone held, of the form a release
	// tags its commit in; 0 is a clone fetched without them, or a rulebook
	// never released.
	Releases int
	// Gate is the gate's findings, in the order of GateKinds.
	Gate []GateFinding
	// Unavailable are the gate's values this build cannot give yet.
	Unavailable []Unavailable
}

// Word is how check ends in a rulebook: findings when validate or the gate
// found one, otherwise done. A notice never changes it (S40).
func (c RulebookCheck) Word() Word {
	if len(c.Gate) > 0 {
		return Findings
	}
	return c.Validation.Word()
}

// The kinds of a finding of the project check: a closed set,
// declared once here and listed by ProjectKinds, which check's --fail-on and
// the tests read. They are the kinds of GateFinding that CheckProject sets;
// the rulebook's gate keeps GateKinds.
const (
	// GateToolConfig is a tool's config that holds a rule's binding weaker than
	// the rule says, or does not hold it at all.
	GateToolConfig GateKind = "tool-config"
	// GateTierContract is a finding of the tier contract: a tier that does not
	// run what the rules say it runs.
	GateTierContract GateKind = "tier-contract"
	// GateExemption is a row of the project's exemptions that is expired, names
	// a retired rule, or names no rule.
	GateExemption GateKind = "exemption"
	// GateNames is a name of the private names list in a tracked file, in its
	// path, in a commit message of the range or in a text handed in.
	// The finding says where, never the name.
	GateNames GateKind = "names"
	// GateTrackedFiles is a tracked file that is a symbolic link, or tracked
	// paths that differ only by case.
	GateTrackedFiles GateKind = "tracked-files"
	// GateCounts is a debt marker or a lint directive that is new since the
	// merge base, when the count of its kind rose.
	GateCounts GateKind = "counts"
)

// ProjectKinds are the kinds of the project check, in the order a listing
// groups them.
func ProjectKinds() []GateKind {
	return []GateKind{GateToolConfig, GateTierContract, GateExemption, GateNames, GateTrackedFiles, GateCounts}
}

// GateCause is why a row of the project check ends the run "not ready", as a
// word a caller switches on: what cures the run follows the cause. A row's
// Msg is a sentence for a person and is never read for it.
type GateCause string

// The causes, in the order of GateCauses.
const (
	// CauseTool: a tool that has bindings could not be asked.
	CauseTool GateCause = "tool"
	// CauseHistory: the repository's history could not be opened or read (no
	// git is named, git refused, or its files could not be listed or read).
	CauseHistory GateCause = "history"
	// CauseBase: the merge base of HEAD and the base ref could not be found.
	CauseBase GateCause = "base"
	// CauseNamesList: the names list that is named could not be read.
	CauseNamesList GateCause = "names-list"
	// CauseText: a text the person named could not be read.
	CauseText GateCause = "text"
)

// GateCauses is every cause, so that a table with a line for each is whole.
func GateCauses() []GateCause {
	return []GateCause{CauseTool, CauseHistory, CauseBase, CauseNamesList, CauseText}
}

// NotChecked is a row of the project check that says something was not
// judged. A row with a Tool names a tool that has bindings and could not be
// asked, with how many of its bindings were not judged: such a row ends the
// run "not ready", and its Cause says why. A row with no Tool (no rule binds,
// a part not taken, no names list named) is said and changes nothing.
type NotChecked struct {
	// Tool is the tool behind the row, "" for a row with none.
	Tool string `json:"tool,omitempty"`
	// Bindings is how many of the tool's bindings were not judged.
	Bindings int `json:"bindings,omitempty"`
	// Cause is why the row ends the run not ready, "" for a row that does not.
	Cause GateCause `json:"cause,omitempty"`
	// Msg says what was not checked, and why.
	Msg string `json:"msg"`
}

// ProjectCheck is gragas check in a project: the findings of its tool
// configs, tier contract and exemptions, the rows the exemptions excuse, and
// what could not be judged.
type ProjectCheck struct {
	// Findings are the findings in the order of ProjectKinds.
	Findings []GateFinding `json:"findings"`
	// Exempt says which findings an exemption row excuses, one sentence each.
	Exempt []string `json:"exempt"`
	// NotChecked are the rows that say something was not judged.
	NotChecked []NotChecked `json:"not_checked"`
}

// Unjudged are the rows of NotChecked that have a tool behind them.
func (p ProjectCheck) Unjudged() []NotChecked {
	var out []NotChecked
	for _, n := range p.NotChecked {
		if n.Tool != "" {
			out = append(out, n)
		}
	}
	return out
}

// Word is how check ends in a project, before --fail-on: not ready when a
// tool with bindings was not checked, whatever else was found, otherwise
// done. A finding ends the run only when the kind is named by --fail-on, so
// the caller decides findings (FailOn).
func (p ProjectCheck) Word() Word {
	if len(p.Unjudged()) > 0 {
		return NotReady
	}
	return Done
}

// FailOn is whether a finding of one of the kinds listed is in the check.
func (p ProjectCheck) FailOn(kinds []GateKind) bool {
	for _, f := range p.Findings {
		if slices.Contains(kinds, f.Kind) {
			return true
		}
	}
	return false
}
