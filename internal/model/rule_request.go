package model

import (
	"slices"
	"strings"
)

// The requests of the rule commands that change a rule's lifecycle: what a
// person asked for, in the words of the command, before the lifecycle context
// makes a plan of it. They are data (arch.layers.11); a client builds one and
// core/api hands it to the context.

// PlanFiles are the paths a rule plan writes to, relative to the root: the
// file the rule is written in, the terms, the files of sources and decisions,
// the history and an overlay's exclusions. The plan screen names them (spec
// 15.3).
type PlanFiles struct {
	Rules, Sources, Decisions, History string
	// Terms is the terms file; only the term plans name it.
	Terms string
	// RuleFiles are the rules files a decision plan may change, one to a line:
	// those that hold a rule standing on a decision. It is a string so that a
	// request holding PlanFiles stays comparable.
	RuleFiles string
	// RuleSites says where each of those rules lives, one "<rule id> <file>" to a
	// line, so a plan lists only the files of the rules it moves.
	RuleSites string
	// Exclusions is an overlay's exclusions file; only the exclusion plans
	// name it.
	Exclusions string
	// Bindings is the folder of the rulebook's binding records; only the
	// binding plans of a borrowed rule name it (S34, S70).
	Bindings string
}

// NewDecision is a decision a new rule records: its words. Its who and its
// date come from the moment of the plan.
type NewDecision struct {
	// Slug names the decision; "" takes decide-<the rule id as a slug>.
	Slug string
	// Title names it and Text says what was decided and why.
	Title, Text string
}

// NewRule is the request of rule new: a draft rule with its anatomy and no
// more. What the person did not give stays empty, and the status is proposed
// (S24); a source and a decision are the form's, so the rule leaves proposed
// by a later command.
type NewRule struct {
	// Part and Concern are the slugs the rule's id is made of.
	Part, Concern string
	// Statement is the rule in one sentence and Strength how firmly it binds.
	Statement string
	Strength  Strength
	// Scope is where the rule applies; nil for everywhere.
	Scope *RuleScope
	// Definition says what the rule means.
	Definition string
	// Owner and ReviewBy are given or left empty.
	Owner, ReviewBy string
	// Cite are the slugs of sources the book holds.
	Cite []string
	// Own makes the owner's own decision the source: a source of that kind
	// pointing at Decision, which it then needs.
	Own bool
	// Decision is recorded and named by the rule's reasoning.
	Decision *NewDecision
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// EditRule is the request of rule edit: the lifecycle fields that a hand edit
// of the file could also change, and nothing that has a command of its own.
// A nil pointer leaves a field alone and a pointer to "" takes it away.
// Bindings have commands of their own, and the status, the id and the prose
// are never here.
type EditRule struct {
	// ID is the rule's current id.
	ID string
	// Owner, ReviewBy and ZeroBy are the new values.
	Owner, ReviewBy, ZeroBy *string
	// AddParts, AddPaths and AddKinds add to the rule's scope.
	AddParts, AddPaths, AddKinds []string
	// Relates, Conflicts, Depends and Covers add links to other rules.
	Relates, Conflicts, Depends, Covers []RuleID
	// Cite are the slugs of sources the book holds. Each is added to the
	// rule's sources as a record that backs the rule, unless the rule cites it
	// so already.
	Cite []string
	// Uncite are the slugs of sources the rule stops citing: every citation
	// of each is taken out of its sources, whatever it backs. A source no
	// rule cites after the edit stays in the book.
	Uncite []string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// ChangeAsk is what every plan command gives the operation that makes its
// change: where the rulebook is, the command as a person types it, the line
// that applies it, and the ids of the run.
type ChangeAsk struct {
	// Root is the repository the command works in.
	Root Root
	// Command is the command as typed and Applies the same with --apply.
	Command, Applies string
	// Who is what the change records as who made it: the command's own name,
	// such as "gragas rule exclude", and never a person's.
	Who string
	// Run is the run id the journal files the apply under; Engine is the
	// engine's version.
	Run, Engine string
	// NamesFile is the absolute path of the private names list as the system
	// writes it (the product reads it through a port), "" for none.
	NamesFile string
	// Cache is the absolute cache folder, "" for none: a parent's bundle is
	// read from its releases (A50).
	Cache string
}

// Why is the decision a change of a rule's life stands on: one already in the
// book, cited by its slug, or a new one taken with the change. Exactly one.
type Why struct {
	// Cite is the slug of a decision the book holds.
	Cite string
	// New is a decision recorded with the change.
	New *NewDecision
}

// RetireRule is the request of rule retire: the rule stays in its file with
// the status retired, and a successor may be named. Nothing is deleted.
type RetireRule struct {
	// ID is the rule's current id.
	ID string
	// Successor is the rule that takes its place, "" for none.
	Successor RuleID
	// Why is the decision the retirement stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// RestoreRule is the request of rule restore: a retired rule returns as
// active, with a new date it is next confirmed on. Both events stay in the
// history.
type RestoreRule struct {
	// ID is the rule's current id.
	ID string
	// ReviewBy is the date the restored rule is next confirmed, YYYY-MM-DD.
	ReviewBy string
	// Why is the decision the restore stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// RenumberRule is the request of rule renumber: the rule gets the next free
// number of its concern as its id, and keeps the id it had as an alias for
// good. Where two rules took one id, the later one is the rule renumbered.
type RenumberRule struct {
	// ID is the rule's current id.
	ID string
	// Why is the decision the renumber stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// ReplaceRule is the request of rule replace: a successor rule in the same
// concern, a draft like any new rule, and the old rule retired with the
// successor named. A change of what a rule means is a replace, never a reword.
type ReplaceRule struct {
	// ID is the rule that is replaced.
	ID string
	// Successor is the new rule; its part and concern are the old rule's.
	Successor NewRule
	// Why is the decision the replacement stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// PromoteRule is the request of rule promote: a rule moves up one step of its
// life, proposed to trial for a must or a should, trial to active once a
// release has gone by, proposed to active for a may. It stands on a decision.
// Grandfathered imports and restore reach active by their own commands; there
// is no way to skip the trial here (S24).
type PromoteRule struct {
	// ID is the rule's current id.
	ID string
	// Why is the decision the promotion stands on.
	Why Why
	// Releases is how many releases have gone by since the rule entered
	// trial. It is zero until releases are counted, so a trial is not
	// promoted before then; core/api reads it and the context takes it as given.
	Releases int
	// PresetDiff is the change the promotion makes to the presets, in words,
	// as the link context gives it; "" until bindings exist.
	PresetDiff string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// ExcludeRule is the request of rule exclude: an overlay drops a rule of its
// parent, on a decision (S4). The record names the overlay, the parent's rule
// id and a sequence number; nothing of the parent changes.
type ExcludeRule struct {
	// ID is the parent's rule id.
	ID string
	// Why is the decision the exclusion stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// ConfirmRule is the request of rule confirm: an active rule is read again by
// whoever runs the command, and its review-by date moves a review cadence on.
// The reviewer is the name the history entry records.
type ConfirmRule struct {
	// ID is the rule's current id.
	ID string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// RuleFileList is RuleFiles as a list of paths.
func (f PlanFiles) RuleFileList() []string {
	if f.RuleFiles == "" {
		return nil
	}
	return strings.Split(f.RuleFiles, "\n")
}

// IncludeRule is the request of rule include: an overlay lifts its exclusion
// of a parent's rule, on a decision. The record stays, marked lifted.
type IncludeRule struct {
	// ID is the parent's rule id.
	ID string
	// Why is the decision the lifting stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// RuleFilesOf are the rules files of the given rules, each once, in the order
// of the rules; a rule RuleSites does not place adds none.
func (f PlanFiles) RuleFilesOf(ids []RuleID) []string {
	site := map[string]string{}
	for line := range strings.SplitSeq(f.RuleSites, "\n") {
		if id, file, ok := strings.Cut(line, " "); ok {
			site[id] = file
		}
	}
	var out []string
	for _, id := range ids {
		if file, ok := site[string(id)]; ok && !slices.Contains(out, file) {
			out = append(out, file)
		}
	}
	return out
}

// BindRule is the request of bind: the rule takes one more binding, which is
// written as given. A binding the rule holds already plans nothing.
type BindRule struct {
	// ID is the rule's current id.
	ID string
	// Binding is the new binding: tool, target, key or value, options,
	// severity, rung, run point, remedy and pair path.
	Binding Binding
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// RebindRule is the request of rebind: the binding of the rule at Tool, Target
// and Key is changed. A nil field is left alone. The rung and the run point
// never change here (S34): a binding with another rung is a new binding.
type RebindRule struct {
	// ID is the rule's current id.
	ID string
	// Tool, Target and Key name the binding to change.
	Tool   string
	Target TargetType
	Key    string
	// ToTool, ToTarget and ToKey move the binding to another entry.
	ToTool   *string
	ToTarget *TargetType
	ToKey    *string
	// Value and Options replace the entry's value and options.
	Value   *any
	Options map[string]any
	// Severity and Remedy replace the binding's own.
	Severity *Severity
	Remedy   *string
	// Loosens says whether a binding moves to a value or severity that reports
	// less than the old one, by the tool's own scale of values. It is a value
	// the caller supplies (the plan reads no adapter); none means a changed
	// value cannot be compared, and a changed value then stands on a decision.
	Loosens func(old, next Binding) bool
	// Why is the decision a loosening stands on; none for a change that does
	// not loosen.
	Why *Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// UnbindRule is the request of unbind: the binding of the rule at Tool,
// Target and Key is taken out of it. The rule stays; its standing is derived
// again.
type UnbindRule struct {
	// ID is the rule's current id.
	ID string
	// Tool, Target and Key name the binding.
	Tool   string
	Target TargetType
	Key    string
	// Files are the paths the plan writes to.
	Files PlanFiles
}
