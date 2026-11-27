package lifecycle_test

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// entry is one history line to write: its kind, the entity it is about and
// the status it leaves. number and previous are the line's own when set;
// otherwise the line takes its place in the file and names the line before.
// before is the status it found, when set; otherwise the line has no before.
type entry struct {
	kind, entity string
	status       model.Status
	before       model.Status
	number       int
	previous     string
}

// historyOf writes es as history.jsonl, each line numbered by its place and
// naming the digest of the line before unless it says otherwise, and answers
// the file and each line's digest.
func historyOf(t testing.TB, es ...entry) ([]byte, []model.Digest) {
	t.Helper()
	var b bytes.Buffer
	var ds []model.Digest
	prev := ""
	for i, e := range es {
		n, p := i+1, prev
		if e.number != 0 {
			n = e.number
		}
		if e.previous != "" {
			p = e.previous
		}
		status := e.status
		if status == "" {
			status = model.Active
		}
		o := canon.Object{
			"format": canon.Int(3), "number": canon.Int(n), "kind": canon.String(e.kind),
			"entity": canon.String(e.entity), "after": canon.Object{"status": canon.String(string(status))},
			"who": canon.String("ada"), "when": canon.String(fmt.Sprintf("2026-01-01T00:00:%02dZ", i+1)),
			"plan": canon.String(plan(i + 1)),
		}
		if p != "" {
			o["previous"] = canon.String(p)
		}
		if e.before != "" {
			o["before"] = canon.Object{"status": canon.String(string(e.before))}
		}
		line, err := canon.Encode(o, canon.Canonical)
		if err != nil {
			t.Fatal(err)
		}
		d, err := canon.DigestOf(o)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
		prev = d.String()
		ds = append(ds, model.Digest(prev))
	}
	return b.Bytes(), ds
}

// baseEntries are the merge base's three entries; the branch's two follow.
func baseEntries() []entry {
	return []entry{
		{kind: "init", entity: "rulebook:mine"},
		{kind: "new", entity: "rule:go.errors.1", status: model.Proposed},
		{kind: "promote", entity: "rule:go.errors.1"},
	}
}

func branchEntries() []entry {
	return []entry{
		{kind: "new", entity: "rule:go.errors.2", status: model.Proposed},
		{kind: "promote", entity: "rule:go.errors.2"},
	}
}

// gateBook is an overlay of base 1.0.0 with two sources and governance's
// trial_releases at 2.
func gateBook() model.Book {
	b := guardSourceBook()
	b.Identity = model.RulebookIdentity{
		Name:       "mine",
		Extends:    &model.Extends{Rulebook: "base", Version: "1.0.0"},
		Governance: model.Governance{TrialReleases: 2},
	}
	return b
}

// cleanGate is a gate with nothing to find: the merge base's three entries,
// the branch's two after them, and no commit that holds a field otherwise
// than its own entries leave it.
func cleanGate(t testing.TB) lifecycle.GateIn {
	t.Helper()
	base, _ := historyOf(t, baseEntries()...)
	now, _ := historyOf(t, append(baseEntries(), branchEntries()...)...)
	return lifecycle.GateIn{Book: gateBook(), History: now, HistoryFile: "history.jsonl", Base: gateBook(), BaseHistory: base}
}

func kindsOf(fs []model.GateFinding) []model.GateKind {
	var out []model.GateKind
	for _, f := range fs {
		out = append(out, f.Kind)
	}
	return out
}

// TestACleanHistoryHasNoGateFinding:the merge base's lines first and
// as they were, a chain with no break, gap or torn line, every field as the
// entries leave it and no pin changed: nothing is found. The control of
// every planted failure below.
func TestACleanHistoryHasNoGateFinding(t *testing.T) {
	t.Parallel()
	if got := lifecycle.Gate(cleanGate(t)); len(got) != 0 {
		t.Errorf("a clean history: %+v, want nothing", got)
	}
}

// TestALineOfTheMergeBaseRewrittenIsListed (store.state-files.5): a
// merge-base line rewritten, removed or moved is listed by its line, as
// rewritten or moved; lines only added after the base are not. Risk: a
// rewritten entry goes unnoticed because the chain after it is rewritten
// too.
func TestALineOfTheMergeBaseRewrittenIsListed(t *testing.T) {
	t.Parallel()
	lines := func(in lifecycle.GateIn) [][]byte { return lifecycle.Lines(in.History) }
	join := func(ls ...[]byte) []byte { return append(bytes.Join(ls, []byte("\n")), '\n') }
	for _, tc := range []struct {
		name string
		edit func([][]byte) []byte
		line int
		what string
	}{
		{"rewritten", func(ls [][]byte) []byte {
			ls[1] = bytes.Replace(ls[1], []byte("go.errors.1"), []byte("go.errors.9"), 1)
			return join(ls...)
		}, 2, "is removed or rewritten"},
		{"removed", func(ls [][]byte) []byte { return join(append(ls[:1:1], ls[2:]...)...) }, 2, "is removed or rewritten"},
		{"moved", func(ls [][]byte) []byte { return join(ls[1], ls[0], ls[2], ls[3], ls[4]) }, 1, "is moved"},
	} {
		in := cleanGate(t)
		in.History = tc.edit(lines(in))
		got := lifecycle.Gate(in)
		if !slices.ContainsFunc(got, func(f model.GateFinding) bool {
			return f.Kind == model.GateRewritten && f.Line == tc.line && strings.Contains(f.Msg, tc.what)
		}) {
			t.Errorf("%s: %+v, want line %d listed as %q", tc.name, got, tc.line, tc.what)
		}
	}
}

// TestAGapIsAFindingAndASharedNumberIsNot (A38):two entries of one
// number naming the same entry before are a merge, and not a finding; a
// number no entry holds below a higher one is a gap.
func TestAGapIsAFindingAndASharedNumberIsNot(t *testing.T) {
	t.Parallel()
	_, ds := historyOf(t, baseEntries()...)
	in := cleanGate(t)
	in.History, _ = historyOf(t, append(baseEntries(),
		entry{kind: "new", entity: "rule:go.errors.2", status: model.Proposed, number: 4, previous: string(ds[2])},
		entry{kind: "new", entity: "rule:go.errors.3", status: model.Proposed, number: 4, previous: string(ds[2])})...)
	if got := lifecycle.Gate(in); len(got) != 0 {
		t.Errorf("a shared number: %+v, want nothing", got)
	}
	in.History, _ = historyOf(t, append(baseEntries(),
		entry{kind: "new", entity: "rule:go.errors.2", status: model.Proposed, number: 5, previous: string(ds[2])})...)
	if got := kindsOf(lifecycle.Gate(in)); !slices.Equal(got, []model.GateKind{model.GateGap}) {
		t.Errorf("number 4 missing: %v, want one gap", got)
	}
}

// TestALinkThatNamesAnotherEntryIsListed (A38):an entry after the
// base whose previous is not the digest of any entry before it is a broken
// link, listed by its line; the base's lines are untouched, so nothing is
// listed as rewritten.
func TestALinkThatNamesAnotherEntryIsListed(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	in.History, _ = historyOf(t, append(baseEntries(),
		entry{kind: "new", entity: "rule:go.errors.2", status: model.Proposed, previous: "sha256:" + strings.Repeat("0", 64)})...)
	got := lifecycle.Gate(in)
	if len(got) != 1 || got[0].Kind != model.GateBroken || got[0].Line != 4 {
		t.Errorf("line 4 names no entry: %+v, want one broken link at line 4", got)
	}
}

// TestALineThatIsNotAnEntryIsListedByItsLine:a torn line after the
// base is skipped by the chain, and listed with its line.
func TestALineThatIsNotAnEntryIsListedByItsLine(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	in.History = append(in.History, []byte("{\"format\":3,\"numb\n")...)
	got := lifecycle.Gate(in)
	if len(got) != 1 || got[0].Kind != model.GateUnreadable || got[0].Line != 6 || got[0].File != "history.jsonl" {
		t.Errorf("a torn line 6: %+v, want it listed at line 6", got)
	}
}

// TestAFieldTheEntriesDoNotMakeIsListed (agents.guards.1):a lifecycle
// field now otherwise than the entries since the base leave it is listed by
// its entity, with the command that changes it; a definition, which is no
// lifecycle field, is not. Risk: a hand edit pushed with the hooks bypassed
// lands.
func TestAFieldTheEntriesDoNotMakeIsListed(t *testing.T) {
	t.Parallel()
	before, after := gateBook(), gateBook()
	after.Sources[0].Locator = "https://example.org/go"
	after.Terms[0].Definition = "Say what was being done."
	got := lifecycle.Unrecorded("1a2b3c4", lifecycle.GuardEdit(before, after, show))
	if len(got) != 1 || got[0].Kind != model.GateUnrecorded || got[0].Entity != "source:effective-go" ||
		got[0].Fix != "gragas source edit effective-go --locator" ||
		!strings.Contains(got[0].Msg, "commit 1a2b3c4") || !strings.Contains(got[0].Msg, "https://example.org/go") {
		t.Errorf("a locator changed by hand: %+v, want it alone listed, naming the commit, with its command", got)
	}
	if got := lifecycle.Unrecorded("1a2b3c4", []model.Guarded{{Entity: "history.jsonl", Field: "entries"}}); len(got) != 0 {
		t.Errorf("history.jsonl: %+v, want nothing: its lines are held apart", got)
	}
}

// TestACommitThatCannotBeHeldIsListedInItsKindsPlace:what the
// caller found judging each commit is listed under its kind, after the
// chain's findings and before a pin's, whatever order it comes in.
func TestACommitThatCannotBeHeldIsListedInItsKindsPlace(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	in.Book.Identity.Extends.Version = "1.1.0"
	in.Unrecorded = []model.GateFinding{lifecycle.UnrecordedCommit("1a2b3c4", "history.jsonl", "line 4 names no command", "make the change through its command")}
	got := lifecycle.Gate(in)
	if !slices.Equal(kindsOf(got), []model.GateKind{model.GateUnrecorded, model.GatePin}) || got[0].Entity != "commit:1a2b3c4" ||
		got[0].Msg != "commit 1a2b3c4 cannot be held to its own entries: line 4 names no command" {
		t.Errorf("a commit that cannot be held: %+v, want it listed before the pin", got)
	}
}

// TestAPinChangedWithoutItsUpgradeEntryIsListed (S64):a parent or
// import pin that changed, came or went since the base is a finding naming
// the pin, unless an upgrade entry of the branch names it; a rulebook the
// branch made takes its pins with its init entry.
func TestAPinChangedWithoutItsUpgradeEntryIsListed(t *testing.T) {
	t.Parallel()
	upgrade := func(entity string) entry { return entry{kind: "upgrade", entity: entity} }
	for _, tc := range []struct {
		name   string
		edit   func(*model.RulebookIdentity)
		branch []entry
		noBase bool
		want   string // the pin listed, "" for none
	}{
		{"the parent's version", func(id *model.RulebookIdentity) { id.Extends.Version = "1.1.0" }, nil, false, "parent:base"},
		{"the parent's source", func(id *model.RulebookIdentity) { id.Extends.Source = "elsewhere" }, nil, false, "parent:base"},
		{"the parent's content hash", func(id *model.RulebookIdentity) {
			id.Extends.ContentHash = model.Digest("sha256:" + strings.Repeat("a", 64))
		}, nil, false, "parent:base"},
		{"the parent gone", func(id *model.RulebookIdentity) { id.Extends = nil }, nil, false, "parent:base"},
		{"an import added", func(id *model.RulebookIdentity) {
			id.Imports = []model.Import{{Rulebook: "other", Version: "2.0.0", Parts: []string{"go"}}}
		}, nil, false, "import:other"},
		{"an upgrade entry of another pin", func(id *model.RulebookIdentity) { id.Extends.Version = "1.1.0" }, []entry{upgrade("import:other")}, false, "parent:base"},
		{"its upgrade entry", func(id *model.RulebookIdentity) { id.Extends.Version = "1.1.0" }, []entry{upgrade("parent:base")}, false, ""},
		{"a branch's own rulebook, with its init entry", func(*model.RulebookIdentity) {}, nil, true, ""},
	} {
		in := cleanGate(t)
		tc.edit(&in.Book.Identity)
		in.History, _ = historyOf(t, append(append(baseEntries(), branchEntries()...), tc.branch...)...)
		if tc.noBase {
			in.NoBase, in.Base, in.BaseHistory = true, model.Book{}, nil
		}
		got := lifecycle.Gate(in)
		switch {
		case tc.want == "" && len(got) != 0:
			t.Errorf("%s: %+v, want nothing", tc.name, got)
		case tc.want != "" && (len(got) != 1 || got[0].Kind != model.GatePin || got[0].Entity != tc.want):
			t.Errorf("%s: %+v, want the pin %s listed", tc.name, got, tc.want)
		}
	}
}

// TestAnImportsContentHashChangedWithoutItsUpgradeEntryIsListed (S69):
// the content hash is part of the pin, so an import that moves only its hash
// is a pin changed.
func TestAnImportsContentHashChangedWithoutItsUpgradeEntryIsListed(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	imp := model.Import{Rulebook: "other", Version: "2.0.0", Parts: []string{"go"}, ContentHash: model.Digest("sha256:" + strings.Repeat("a", 64))}
	in.Base.Identity.Imports = []model.Import{imp}
	imp.ContentHash = model.Digest("sha256:" + strings.Repeat("b", 64))
	in.Book.Identity.Imports = []model.Import{imp}
	got := lifecycle.Gate(in)
	if len(got) != 1 || got[0].Kind != model.GatePin || got[0].Entity != "import:other" {
		t.Errorf("an import's hash changed: %+v, want the pin import:other listed", got)
	}
}

// TestABranchsOwnRulebookWithNoInitEntryListsItsPin (S64):the
// planted failure of the init case: a rulebook the branch made whose
// history has no init entry has its parent pin listed.
func TestABranchsOwnRulebookWithNoInitEntryListsItsPin(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	in.NoBase, in.Base, in.BaseHistory = true, model.Book{}, nil
	in.History, _ = historyOf(t, append(baseEntries()[1:], branchEntries()...)...)
	got := lifecycle.Gate(in)
	if !slices.ContainsFunc(got, func(f model.GateFinding) bool { return f.Kind == model.GatePin && f.Entity == "parent:base" }) {
		t.Errorf("no init entry: %+v, want the pin parent:base listed", got)
	}
}

// TestARuleOnTrialPastItsWindowIsListed:a rule whose entry putting
// it on trial has shipped in trial_releases releases is listed, naming
// them, with the commands that end the trial; one release fewer, a rule no
// longer on trial, or no window, is not.
func TestARuleOnTrialPastItsWindowIsListed(t *testing.T) {
	t.Parallel()
	es := append(baseEntries(), entry{kind: "trial", entity: "rule:go.errors.1", status: model.Trial})
	history, ds := historyOf(t, es...)
	put := ds[3]
	rel := func(name string, entries []model.Digest) lifecycle.Release {
		return lifecycle.Release{Name: name, Entries: entries}
	}
	for _, tc := range []struct {
		name     string
		releases []lifecycle.Release
		most     int
		status   model.Status
		listed   bool
	}{
		{"two releases of two", []lifecycle.Release{rel("v1.1.0", ds), rel("v1.2.0", ds)}, 2, model.Trial, true},
		{"one release of two", []lifecycle.Release{rel("v1.0.0", ds[:3]), rel("v1.1.0", ds)}, 2, model.Trial, false},
		{"promoted since", []lifecycle.Release{rel("v1.1.0", ds), rel("v1.2.0", ds)}, 2, model.Active, false},
		{"no window", []lifecycle.Release{rel("v1.1.0", ds), rel("v1.2.0", ds)}, 0, model.Trial, false},
	} {
		in := cleanGate(t)
		in.History, in.BaseHistory = history, history
		in.Book.Rules = []model.Rule{{ID: "go.errors.1", Status: tc.status}}
		in.Book.Identity.Governance.TrialReleases = tc.most
		in.Base.Identity.Governance.TrialReleases = tc.most
		in.Releases = tc.releases
		got := lifecycle.Gate(in)
		if !tc.listed {
			if len(got) != 0 {
				t.Errorf("%s: %+v, want nothing", tc.name, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Kind != model.GateTrial || got[0].Entity != "rule:go.errors.1" ||
			!strings.Contains(got[0].Msg, "v1.1.0, v1.2.0") || got[0].Fix != "gragas rule promote go.errors.1, or gragas rule retire go.errors.1" {
			t.Errorf("%s: %+v, want rule:go.errors.1 listed with its releases (entry %s)", tc.name, got, put)
		}
	}
}

// TestTheTrialCountsEachReleaseThatShippedTheRuleOnTrial: an entry no release holds yet takes no release
// out of the count. A rule edit, a source edit or a renumber that keeps the
// rule on trial, a second entry that puts it on trial, merged from a branch
// forked before the first, and an entry made while it was proposed, merged
// after it went on trial and before a later edit, each leave it listed.
// Risk: a recorded edit before each release, or a merge, keeps a rule on
// trial with no finding.
func TestTheTrialCountsEachReleaseThatShippedTheRuleOnTrial(t *testing.T) {
	t.Parallel()
	_, base := historyOf(t, baseEntries()...)
	forked := string(base[2]) // the parent of the entry that put it on trial
	onTrial := entry{kind: "trial", entity: "rule:go.errors.1", status: model.Trial, before: model.Active}
	for _, tc := range []struct {
		name  string
		later []entry
		rule  model.Rule
	}{
		{"a rule edit and a source edit since", []entry{
			{kind: "edit", entity: "rule:go.errors.1", status: model.Trial, before: model.Trial},
			{kind: "source-edit", entity: "rule:go.errors.1", status: model.Trial, before: model.Trial},
		}, model.Rule{ID: "go.errors.1", Status: model.Trial}},
		{"renumbered since", []entry{
			{kind: "renumber", entity: "rule:go.errors.9", status: model.Trial, before: model.Trial},
		}, model.Rule{ID: "go.errors.9", Aliases: []model.RuleID{"go.errors.1"}, Status: model.Trial}},
		{"a second promote to trial merged, of the same number and later", []entry{
			{kind: "promote", entity: "rule:go.errors.1", status: model.Trial, before: model.Proposed, number: 4, previous: forked},
		}, model.Rule{ID: "go.errors.1", Status: model.Trial}},
		{"an edit made while proposed merged, then an edit on trial", []entry{
			{kind: "edit", entity: "rule:go.errors.1", status: model.Proposed, before: model.Proposed, number: 4, previous: forked},
			{kind: "edit", entity: "rule:go.errors.1", status: model.Trial, before: model.Trial, number: 5},
		}, model.Rule{ID: "go.errors.1", Status: model.Trial}},
	} {
		es := append(append(baseEntries(), onTrial), tc.later...)
		history, ds := historyOf(t, es...)
		in := cleanGate(t)
		in.History, in.BaseHistory = history, history
		in.Book.Rules = []model.Rule{tc.rule}
		// each release holds the entry that put the rule on trial, and
		// neither holds a later one
		in.Releases = []lifecycle.Release{{Name: "v1.1.0", Entries: ds[:4]}, {Name: "v1.2.0", Entries: ds[:4]}}
		got := lifecycle.Gate(in)
		if len(got) != 1 || got[0].Kind != model.GateTrial || got[0].Entity != "rule:"+string(tc.rule.ID) ||
			!strings.Contains(got[0].Msg, "v1.1.0, v1.2.0") {
			t.Errorf("%s: %+v, want rule:%s listed with its releases", tc.name, got, tc.rule.ID)
		}
	}
}

// TestTheFindingsAreInTheOrderOfTheKinds:whatever the order they
// are found in, the findings are in model.GateKinds's order. The chain finds
// a gap at line 4 before a broken link at line 5, and the gate lists the
// broken link first.
func TestTheFindingsAreInTheOrderOfTheKinds(t *testing.T) {
	t.Parallel()
	_, ds := historyOf(t, baseEntries()...)
	in := cleanGate(t)
	in.Book.Identity.Extends.Version = "1.1.0"
	in.Unrecorded = []model.GateFinding{lifecycle.UnrecordedCommit("1a2b3c4", "history.jsonl", "line 4 names no command", "")}
	in.History, _ = historyOf(t, append(baseEntries(),
		entry{kind: "new", entity: "rule:go.errors.2", status: model.Proposed, number: 5, previous: string(ds[2])},
		entry{kind: "new", entity: "rule:go.errors.3", status: model.Proposed, number: 6, previous: "sha256:" + strings.Repeat("0", 64)})...)
	got := kindsOf(lifecycle.Gate(in))
	want := []model.GateKind{model.GateBroken, model.GateGap, model.GateUnrecorded, model.GatePin}
	if !slices.Equal(got, want) {
		t.Errorf("the kinds found: %v, want %v", got, want)
	}
}

// TestTheBranchDoesNotSetItsOwnTrialWindow: trial_releases lowered to 0 or raised on the branch, by hand,
// leaves the trial judged by the merge base's window, and the change is
// listed as the governance's.
// Risk: a branch switches the trial finding off by editing the identity,
// which guard edit leaves to the gate.
func TestTheBranchDoesNotSetItsOwnTrialWindow(t *testing.T) {
	t.Parallel()
	es := append(baseEntries(), entry{kind: "trial", entity: "rule:go.errors.1", status: model.Trial})
	history, ds := historyOf(t, es...)
	for _, most := range []int{0, 9} {
		in := cleanGate(t)
		in.History, in.BaseHistory = history, history
		in.Book.Rules = []model.Rule{{ID: "go.errors.1", Status: model.Trial}}
		in.Book.Identity.Governance.TrialReleases = most
		in.Releases = []lifecycle.Release{{Name: "v1.1.0", Entries: ds}, {Name: "v1.2.0", Entries: ds}}
		got := lifecycle.Gate(in)
		if want := []model.GateKind{model.GateGovernance, model.GateTrial}; !slices.Equal(kindsOf(got), want) {
			t.Fatalf("trial_releases %d on the branch: %v, want %v: %+v", most, kindsOf(got), want, got)
		}
		if g := got[0]; g.Entity != "governance.trial_releases" || !strings.Contains(g.Msg, "it was 2") {
			t.Errorf("trial_releases %d: the governance finding %+v; want the field and the base's value", most, g)
		}
		if !strings.Contains(got[1].Msg, "trial_releases is 2") {
			t.Errorf("trial_releases %d: the trial finding %q; want the merge base's window", most, got[1].Msg)
		}
	}
}

// TestAGovernanceChangedSinceTheMergeBaseIsListed:any field of the
// governance changed by the branch is listed by its name; a rulebook the
// branch made has no governance before it, and lists none.
// Risk: a branch drops the decision a promotion needs, unseen.
func TestAGovernanceChangedSinceTheMergeBaseIsListed(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	in.Base.Identity.Governance.DecisionRequired = true
	got := lifecycle.Gate(in)
	if len(got) != 1 || got[0].Kind != model.GateGovernance || got[0].Entity != "governance.decision_required" ||
		!strings.Contains(got[0].Msg, "is false since the merge base (it was true)") {
		t.Errorf("decision_required dropped: %+v", got)
	}
	in.NoBase, in.Base = true, model.Book{}
	if got := lifecycle.Gate(in); slices.Contains(kindsOf(got), model.GateGovernance) {
		t.Errorf("a rulebook the branch made: %+v, want no governance finding", got)
	}
}

// TestARulebookGoneWhereTheBaseHeldOneIsTheOnlyFinding: with Gone set, the gate lists that and judges nothing else.
// Risk: a branch switches the gate off by removing its rulebook or adding a
// second one.
func TestARulebookGoneWhereTheBaseHeldOneIsTheOnlyFinding(t *testing.T) {
	t.Parallel()
	in := cleanGate(t)
	in.Gone, in.GoneFile = "HEAD holds no rulebook.yaml", "rulebook.yaml"
	got := lifecycle.Gate(in)
	if len(got) != 1 || got[0].Kind != model.GateBook || !strings.Contains(got[0].Msg, "HEAD holds no rulebook.yaml, where the merge base holds one") || got[0].File != "rulebook.yaml" {
		t.Errorf("a rulebook gone: %+v", got)
	}
}

// TestTheGateSeesEveryFieldOfTheGovernance: GovernanceFields names each
// field of model.Governance once, by its name in rulebook.yaml, and a change
// to any one of them is seen.
// Risk: a field added to the governance that a branch may change unseen.
func TestTheGateSeesEveryFieldOfTheGovernance(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[model.Governance]()
	fields := lifecycle.GovernanceFields(model.Governance{})
	if len(fields) != typ.NumField() {
		t.Fatalf("GovernanceFields names %d fields, model.Governance has %d: add the new one", len(fields), typ.NumField())
	}
	for i, f := range fields {
		if tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ","); tag != f.Name {
			t.Errorf("field %d is %q, model.Governance's is %q", i, f.Name, tag)
		}
	}
	changed := model.Governance{Contribution: "x", CheckerPublishers: []string{"x"}, CheckerSecondReviewer: true,
		CheckerProof: "x", DecisionRequired: true, TrialReleases: 1, ReviewDays: 1}
	for i, f := range lifecycle.GovernanceFields(changed) {
		if f.Value == fields[i].Value {
			t.Errorf("%s: a change is not seen (%s)", f.Name, f.Value)
		}
	}
}

// TestOnlyATagOfTheReleaseFormIsARelease: a v and major.minor.patch, and
// nothing else. Planted failure: every tag of the clone counted, as before.
func TestOnlyATagOfTheReleaseFormIsARelease(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]bool{
		"v1.2.3": true, "v0.0.0": true, "v10.20.30": true,
		"v1": false, "v1.2": false, "1.2.3": false, "v1.2.3-rc1": false, "v1.2.3.4": false,
		"v01.2.3": false, "v1.-2.3": false, "v1..3": false, "latest": false, "backup-2026": false, "v": false,
	} {
		if got := lifecycle.IsReleaseTag(tag); got != want {
			t.Errorf("IsReleaseTag(%q) = %v, want %v", tag, got, want)
		}
	}
	if lifecycle.IsReleaseTag("") {
		t.Error("IsReleaseTag of no tag = true, want false")
	}
}
