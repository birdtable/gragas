package lifecycle

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// GateIn is what the rulebook's gate judges. The caller reads every part of
// it, through History and the Tree, and makes the replay: Gate reads no port
// and no clock, so the same input gives the same findings (arch.ports.1).
type GateIn struct {
	// Book is the rulebook as it is now, and History its history.jsonl.
	Book    model.Book
	History []byte
	// HistoryFile is the path of history.jsonl below the root, for findings.
	HistoryFile string
	// Base is the rulebook at the merge base and BaseHistory its
	// history.jsonl. NoBase says the merge base holds no rulebook: the
	// branch made it, and every entry is the branch's own.
	Base        model.Book
	BaseHistory []byte
	NoBase      bool
	// Unrecorded is what judging each commit since the merge base found, as
	// guard staged judges a commit a merge brings in: each against its own
	// parent, its lines' commands made again over that parent's tree. A
	// commit is judged over its own tree, never over the other side's, so a
	// branch that merged another holds each side's plans as they were made.
	// The caller makes them with Unrecorded and UnrecordedCommit.
	Unrecorded []model.GateFinding
	// Releases are the rulebook's releases, each with the digests of the
	// entries its history.jsonl holds.
	Releases []Release
	// Gone says where a rulebook.yaml is missing or doubled although the
	// merge base holds one, as "HEAD holds none"; "" when it is not. Gate
	// then judges nothing else: there is no one rulebook to hold.
	Gone string
	// GoneFile is the rulebook.yaml the merge base holds, for that finding.
	GoneFile string
}

// Release is one release of the rulebook and the entries it shipped with.
type Release struct {
	// Name is the release's tag.
	Name string
	// Entries are the digests of the entries of history.jsonl at the tag.
	Entries []model.Digest
}

// Gate is the rulebook's gate over history (A38, S14, S64): every line of
// the merge base's history.jsonl stays where it was byte for byte; the
// chain now has no broken link, no gap and no line it cannot read; every
// lifecycle field is as the entries since the merge base leave it; a parent
// or import pin changes only with its upgrade entry; and no rule stays on
// trial past the governance's trial_releases. The findings are in the order
// of model.GateKinds, and of one kind in the order they are found.
func Gate(in GateIn) []model.GateFinding {
	if in.Gone != "" {
		return []model.GateFinding{gateFinding(model.GateBook, in.GoneFile, 0, "rulebook",
			in.Gone+", where the merge base holds one ("+in.GoneFile+"): the gate holds the history of that one rulebook",
			"put back the one rulebook.yaml the merge base holds, and remove any other")}
	}
	now := ReadFile(in.History)
	out := rewritten(in)
	out = append(out, chainFindings(in.HistoryFile, now)...)
	out = append(out, in.Unrecorded...)
	since := entriesSince(in, now)
	out = append(out, pins(in, since)...)
	out = append(out, governance(in)...)
	out = append(out, trials(in, now)...)
	order := model.GateKinds()
	slices.SortStableFunc(out, func(a, b model.GateFinding) int {
		return cmp.Compare(slices.Index(order, a.Kind), slices.Index(order, b.Kind))
	})
	return out
}

// rewritten lists each line of the merge base's history.jsonl that is not
// the same line, at the same place, now (store.state-files.5). git merges
// history.jsonl by putting the lines of both sides after the lines they
// share, so the merge base's lines are always the first ones.
func rewritten(in GateIn) []model.GateFinding {
	if in.NoBase {
		return nil
	}
	base, at := split(in.BaseHistory)
	now, _ := split(in.History)
	var out []model.GateFinding
	for i, line := range base {
		if i < len(now) && bytes.Equal(now[i], line) {
			continue
		}
		what := "removed or rewritten"
		if slices.ContainsFunc(now, func(l []byte) bool { return bytes.Equal(l, line) }) {
			what = "moved"
		}
		out = append(out, gateFinding(model.GateRewritten, in.HistoryFile, at[i], "history:"+in.HistoryFile,
			fmt.Sprintf("line %d of %s at the merge base is %s: every line the merge base holds stays as it is", at[i], in.HistoryFile, what),
			"put back the merge base's lines as they are: history.jsonl only grows"))
	}
	return out
}

// chainFindings are the chain's broken links and gaps, and each line it
// skipped.
func chainFindings(file string, c Chain) []model.GateFinding {
	var out []model.GateFinding
	for _, f := range c.Findings {
		kind, fix := model.GateBroken, "put back the line that was rewritten or removed; its digest is the one named"
		if f.Kind == Gap {
			kind, fix = model.GateGap, "put back the entries the numbers were taken by"
		}
		out = append(out, gateFinding(kind, file, f.Line, fmt.Sprintf("history:%s#%d", file, f.Number), f.Msg, fix))
	}
	for _, s := range c.Skipped {
		out = append(out, gateFinding(model.GateUnreadable, file, s.Line, "history:"+file,
			fmt.Sprintf("line %d of %s is not an entry, so it is skipped: %s", s.Line, file, s.Why),
			"put back the line as the command wrote it"))
	}
	return out
}

// Unrecorded is a finding for each lifecycle field commit holds otherwise
// than its own lines' commands leave it, as GuardEdit lists the changes:
// a change made by hand and committed with no entry. history.jsonl itself is
// held by the lines of the merge base and the chain. commit is the short
// name of the commit, as a listing names it.
func Unrecorded(commit string, changes []model.Guarded) []model.GateFinding {
	var out []model.GateFinding
	for _, c := range changes {
		if c.Entity == "history.jsonl" {
			continue
		}
		fix := c.Command
		if fix == "" {
			fix = "put " + c.Field + " back as it was: no command changes it by hand"
		}
		out = append(out, gateFinding(model.GateUnrecorded, "", 0, c.Entity,
			fmt.Sprintf("commit %s makes %s of %s %s with no entry of its own (it was %s)", commit, c.Field, c.Entity, orNone(c.After), orNone(c.Before)), fix))
	}
	return out
}

// UnrecordedCommit is the finding of a commit that cannot be held to its own
// lines, why, with the line that mends it: a line no command made, a plan
// whose digest is not its command's, a rulebook it cannot read. file is the
// history.jsonl it is about.
func UnrecordedCommit(commit, file, why, fix string) model.GateFinding {
	return gateFinding(model.GateUnrecorded, file, 0, "commit:"+commit, "commit "+commit+" cannot be held to its own entries: "+why, fix)
}

// entriesSince are the entries now holds that the merge base does not: the
// branch's own.
func entriesSince(in GateIn, now Chain) []Entry {
	if in.NoBase {
		return now.Entries
	}
	had := map[model.Digest]bool{}
	for _, e := range ReadFile(in.BaseHistory).Entries {
		had[e.Digest] = true
	}
	var out []Entry
	for _, e := range now.Entries {
		if !had[e.Digest] {
			out = append(out, e)
		}
	}
	return out
}

// pin is one pin as the gate compares it: a parent or an import, by the
// rulebook it names.
type pin struct {
	entity, text string
}

// pinsOf are the pins of an identity, by entity: parent:<rulebook> and
// import:<rulebook>.
func pinsOf(id model.RulebookIdentity) map[string]string {
	out := map[string]string{}
	if e := id.Extends; e != nil {
		out["parent:"+e.Rulebook] = fmt.Sprintf("%s %s from %q, hash %q", e.Rulebook, e.Version, e.Source, e.ContentHash)
	}
	for _, i := range id.Imports {
		out["import:"+i.Rulebook] = fmt.Sprintf("%s %s from %q, hash %q, parts %s", i.Rulebook, i.Version, i.Source, i.ContentHash, strings.Join(i.Parts, ","))
	}
	return out
}

// pins lists each parent or import pin that changed, came or went since the
// merge base with no upgrade entry of its own. A rulebook the branch made
// takes its pins with its init entries. The upstream pins are bindings,
// which the lifecycle check holds.
func pins(in GateIn, since []Entry) []model.GateFinding {
	before, after := pinsOf(in.Base.Identity), pinsOf(in.Book.Identity)
	var changed []pin
	for k, v := range after {
		if before[k] != v {
			changed = append(changed, pin{k, v})
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			changed = append(changed, pin{k, ""})
		}
	}
	slices.SortFunc(changed, func(a, b pin) int { return cmp.Compare(a.entity, b.entity) })
	var out []model.GateFinding
	for _, p := range changed {
		if recordsPin(since, p.entity, in.NoBase) {
			continue
		}
		was := before[p.entity]
		out = append(out, gateFinding(model.GatePin, "rulebook.yaml", 0, p.entity,
			fmt.Sprintf("the pin %s is %s since the merge base (it was %s), with no upgrade entry", p.entity, orNone(p.text), orNone(was)),
			"put the pin back, then change it with gragas upgrade (not built yet)"))
	}
	return out
}

// recordsPin says an entry since the merge base records the pin: its
// upgrade entry, or for a rulebook the branch made, an init entry.
func recordsPin(since []Entry, entity string, noBase bool) bool {
	return slices.ContainsFunc(since, func(e Entry) bool {
		return (e.Kind == "upgrade" && e.Entity == entity) || (noBase && e.Kind == "init")
	})
}

// governance lists each field of the governance that changed since the
// merge base. The identity is a part guard edit leaves to the gate, and no
// command writes the governance with its entry yet, so each change is one
// made by hand. A rulebook the branch made has no governance before it.
func governance(in GateIn) []model.GateFinding {
	if in.NoBase {
		return nil
	}
	before, after := GovernanceFields(in.Base.Identity.Governance), GovernanceFields(in.Book.Identity.Governance)
	var out []model.GateFinding
	for i, b := range before {
		a := after[i]
		if a.Value == b.Value {
			continue
		}
		out = append(out, gateFinding(model.GateGovernance, "rulebook.yaml", 0, "governance."+b.Name,
			fmt.Sprintf("governance.%s is %s since the merge base (it was %s), and no command writes the governance with its entry", b.Name, orNone(a.Value), orNone(b.Value)),
			"put governance."+b.Name+" back as the merge base has it"))
	}
	return out
}

// GovernanceField is one field of the governance: its name in rulebook.yaml
// and its value as text.
type GovernanceField struct{ Name, Value string }

// GovernanceFields is every field of g, in the order of model.Governance. A
// field added to model.Governance is added here too: the gate's test counts
// them, so a field the gate cannot see fails it.
func GovernanceFields(g model.Governance) []GovernanceField {
	return []GovernanceField{
		{"contribution", fmt.Sprint(g.Contribution)},
		{"checker_publishers", fmt.Sprintf("%q", g.CheckerPublishers)},
		{"checker_second_reviewer", fmt.Sprint(g.CheckerSecondReviewer)},
		{"checker_proof", fmt.Sprint(g.CheckerProof)},
		{"decision_required", fmt.Sprint(g.DecisionRequired)},
		{"trial_releases", fmt.Sprint(g.TrialReleases)},
		{"review_days", fmt.Sprint(g.ReviewDays)},
	}
}

// trials lists each rule on trial that has shipped on trial in as many
// releases as the governance allows, or more: that many releases each hold an
// entry of it that leaves it on trial. The governance is the merge base's: the branch
// never sets its own trial window (a change to it is governance's finding).
func trials(in GateIn, now Chain) []model.GateFinding {
	g := in.Book.Identity.Governance
	if !in.NoBase {
		g = in.Base.Identity.Governance
	}
	most := g.TrialReleases
	if most <= 0 {
		return nil
	}
	var out []model.GateFinding
	for _, r := range in.Book.Rules {
		if r.Status != model.Trial {
			continue
		}
		entity := "rule:" + string(r.ID)
		on := trialEntries(now, r)
		if len(on) == 0 {
			continue // no entry put it on trial: unrecorded lists the status
		}
		var shipped []string
		for _, rel := range in.Releases {
			if slices.ContainsFunc(rel.Entries, func(d model.Digest) bool { return on[d] }) {
				shipped = append(shipped, rel.Name)
			}
		}
		if len(shipped) < most {
			continue
		}
		out = append(out, gateFinding(model.GateTrial, "", 0, entity,
			fmt.Sprintf("%s has shipped on trial in %d releases (%s), and the governance's trial_releases is %d", r.ID, len(shipped), strings.Join(shipped, ", "), most),
			"gragas rule promote "+string(r.ID)+", or gragas rule retire "+string(r.ID)))
	}
	return out
}

// trialEntries is the digest of each entry of r that leaves it on trial.
// A release that holds any of them shipped r on trial, so the count picks no
// one entry to start from: an entry written later is in no release and takes
// none out of the count, whatever order a merge reads it in. r's entries are those of its id and of each id
// it was renumbered from, which it keeps as an alias.
func trialEntries(c Chain, r model.Rule) map[model.Digest]bool {
	ids := map[string]bool{"rule:" + string(r.ID): true}
	for _, a := range r.Aliases {
		ids["rule:"+string(a)] = true
	}
	out := map[model.Digest]bool{}
	for _, e := range c.Entries {
		if ids[e.Entity] && e.After.Status == model.Trial {
			out[e.Digest] = true
		}
	}
	return out
}

// gateFinding is one finding of kind k.
func gateFinding(k model.GateKind, file string, line int, entity, msg, fix string) model.GateFinding {
	return model.GateFinding{Kind: k, Entity: entity, Msg: msg, Fix: fix, File: file, Line: line}
}

// orNone is v, or "none" when it is empty.
func orNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// NotAvailable are the gate's values this build cannot give, each null with
// why (cli.output.9): the truth Gate does not take yet, behind its pins,
// which needs the fetched release, and review-by overdue, which waits for
// attention's kinds.
func NotAvailable() []model.Unavailable {
	return []model.Unavailable{
		{Name: "link findings: entry conflicts, coverage and the zero-by budgets", Reason: "check does not take them yet"},
		{Name: "proof marks", Reason: "check does not take them yet"},
		{Name: "the version bump's size", Reason: "check does not take it yet"},
		{Name: "checker code without tests", Reason: "check does not take it yet"},
		{Name: "a pin behind its parent's or import's release", Reason: "it needs the fetched release"},
		{Name: "review-by overdue", Reason: "it comes with attention's kinds"},
	}
}

// IsReleaseTag is whether name is the form a release tags its commit in: a v
// and major.minor.patch, each number with no sign and no leading zero. Any
// other tag of a clone is not a release.
func IsReleaseTag(name string) bool {
	rest, ok := strings.CutPrefix(name, "v")
	if !ok {
		return false
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" || len(p) > 1 && p[0] == '0' {
			return false
		}
	}
	return true
}

// NoReleaseRead is what check lists under "Not available yet" when the clone
// held no release tag, so a reader tells "no trial is past its window" from
// "no release was read".
func NoReleaseRead() model.Unavailable {
	return model.Unavailable{
		Name:   "trial findings against a release",
		Reason: "no release tag was read, so no trial is judged: fetch the tags with git fetch --tags, then run check again",
	}
}
