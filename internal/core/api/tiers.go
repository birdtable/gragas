package coreapi

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// TiersFile is the file of the Assets port that holds the tier table the
// tier contract is read against.
const TiersFile = "audit/tiers.yaml"

// SystemsTableFile is the file of the Assets port that holds the GOOS/GOARCH
// pairs a declared system may be.
const SystemsTableFile = "audit/systems.yaml"

// SystemsFile is where a repository declares the systems it builds for, until
// `gragas resolve` is built: the names (`os/arch`) separated by spaces or
// commas.
const SystemsFile = ".gragas/systems"

// TierTable is the tier table, loaded once by the entry point. When the file
// cannot be read, Refused says why and the table is empty: the tier contract is
// then not checked, never half checked. The zero value holds none and refuses
// nothing, as in a build whose Assets port answers port.ErrUnavailable.
type TierTable struct {
	table   audit.Tiers
	Refused string
}

// Count is the number of tiers the table holds; none when it is refused.
func (t TierTable) Count() int { return len(t.table.Tiers) }

// LoadTiers reads the tier table from the Assets port.
func LoadTiers(a port.Assets) TierTable {
	data, err := a.Read(TiersFile)
	known, kerr := a.Read(SystemsTableFile)
	switch {
	case errors.Is(err, port.ErrUnavailable):
		return TierTable{}
	case err != nil:
		return TierTable{Refused: fmt.Sprintf("%s could not be read: %v", TiersFile, err)}
	}
	if kerr != nil {
		return TierTable{Refused: fmt.Sprintf("%s could not be read: %v", SystemsTableFile, kerr)}
	}
	t, err := tierRows(data)
	if err != nil {
		return TierTable{Refused: fmt.Sprintf("%s: %v", TiersFile, err)}
	}
	if t.Known, err = knownSystems(known); err != nil {
		return TierTable{Refused: fmt.Sprintf("%s: %v", SystemsTableFile, err)}
	}
	return TierTable{table: t}
}

// tierRows is the table of the document, held to its shape: four tiers named
// pre-commit, pre-push, full and release in that order, each row with an id, a
// rule and a way to be held, every rule a rule id, and a land entry whose task
// calls tiers of the table, the hook's among them.
func tierRows(data []byte) (audit.Tiers, error) {
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		return audit.Tiers{}, fmt.Errorf("it is no mapping: %w", err)
	}
	var t audit.Tiers
	if t.Rules, err = tierRules(doc.Sub("rules")); err != nil {
		return t, err
	}
	if h := doc.Sub("hook"); h != nil {
		t.Hook, t.HookCalls = ordered.Strings(h.List("files")), ordered.Strings(h.List("calls"))
	}
	t.Empty = ordered.Strings(doc.List("empty"))
	want := []string{"pre-commit", "pre-push", "full", "release"}
	land := doc.Sub("land")
	if land == nil {
		return t, errors.New("it has no land mapping: the land job's task and the tiers it calls")
	}
	t.Land = audit.Land{Task: land.Str("task"), Calls: ordered.Strings(land.List("calls"))}
	if err := landCallsTiers(t.Land, want, t.HookCalls); err != nil {
		return t, err
	}
	rows := ordered.Maps(doc.List("tiers"))
	if len(rows) != len(want) {
		return t, fmt.Errorf("it names %d tiers, the contract has %d", len(rows), len(want))
	}
	for i, row := range rows {
		if row.Str("name") != want[i] {
			return t, fmt.Errorf("tier %d is %q, want %q", i+1, row.Str("name"), want[i])
		}
		tier, err := tierRow(row, want[i])
		if err != nil {
			return t, err
		}
		t.Tiers = append(t.Tiers, tier)
	}
	return t, nil
}

// tierRules is the rules mapping of the document, every value a rule id.
func tierRules(rules *ordered.Map) (audit.TierRules, error) {
	if rules == nil {
		return audit.TierRules{}, errors.New("it has no rules mapping")
	}
	r := audit.TierRules{
		Tier: rules.Str("tier"), Order: rules.Str("order"), Timeout: rules.Str("timeout"),
		Background: rules.Str("background"), Parallel: rules.Str("parallel"), Hook: rules.Str("hook"),
		HookCalls: rules.Str("hook_calls"), Empty: rules.Str("empty"), Systems: rules.Str("systems"), Runs: rules.Str("runs"),
		Land: rules.Str("land"),
	}
	for _, id := range []string{r.Tier, r.Order, r.Timeout, r.Background, r.Parallel, r.Hook, r.HookCalls, r.Empty, r.Systems, r.Runs, r.Land} {
		if !ruleID(id) {
			return r, fmt.Errorf("%q is not a rule id", id)
		}
	}
	return r, nil
}

// tierRow is one tier of the table, named name, each member with an id, a rule
// and words to find, and a flag only with its rule.
func tierRow(row *ordered.Map, name string) (audit.Tier, error) {
	tier := audit.Tier{Name: name, Whole: row.Truthy("whole"), Staged: row.Truthy("staged")}
	if err := stagedOnlyForTheCommit(tier); err != nil {
		return tier, err
	}
	for j, m := range ordered.Maps(row.List("members")) {
		mem := audit.Member{
			ID: m.Str("id"), Rule: m.Str("rule"), All: ordered.Strings(m.List("all")), Any: ordered.Strings(m.List("any")),
			Tree: m.Truthy("tree"), PerSystem: m.Truthy("per_system"), Flag: m.Str("flag"), FlagRule: m.Str("flag_rule"), LinuxOnly: m.Truthy("linux_only"),
		}
		if mem.ID == "" || !ruleID(mem.Rule) || len(mem.All)+len(mem.Any) == 0 || (mem.Flag != "") != (mem.FlagRule != "") || (mem.FlagRule != "" && !ruleID(mem.FlagRule)) {
			return tier, fmt.Errorf("tier %s member %d needs an id, a rule and words to find (and a flag with its rule)", name, j+1)
		}
		tier.Members = append(tier.Members, mem)
	}
	return tier, nil
}

// landCallsTiers refuses a land entry with no task, or whose calls are not tiers of the table, are named twice,
// or leave out a tier the hook calls (hook.calls, "task <tier>"): the hook's list must be a declared subset of
// the land job's.
func landCallsTiers(l audit.Land, tiers, hookCalls []string) error {
	if l.Task == "" || len(l.Calls) == 0 {
		return errors.New("its land entry needs a task and the tiers it calls")
	}
	seen := map[string]bool{}
	for _, c := range l.Calls {
		if !slices.Contains(tiers, c) || seen[c] {
			return fmt.Errorf("its land entry calls %q, which is not a tier of the table or is named twice", c)
		}
		seen[c] = true
	}
	hooked := 0
	for _, h := range hookCalls {
		tier, ok := strings.CutPrefix(h, "task ")
		if !ok || !slices.Contains(tiers, tier) {
			continue
		}
		hooked++
		if !seen[tier] {
			return fmt.Errorf("its land entry does not call %s, which the hook calls: the hook's list must be a subset of the land job's", tier)
		}
	}
	if hooked == 0 {
		return errors.New("its hook calls no tier of the table, so the land entry has no hook's list to hold as a subset")
	}
	return nil
}

// ruleID says s is part.concern.n.
func ruleID(s string) bool {
	p := strings.Split(s, ".")
	return len(p) == 3 && p[0] != "" && p[1] != "" && p[2] != ""
}

// knownSystems is the pairs of the table, each "os/arch".
func knownSystems(data []byte) ([]string, error) {
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		return nil, fmt.Errorf("it is no mapping: %w", err)
	}
	pairs := ordered.Strings(doc.List("systems"))
	if len(pairs) == 0 {
		return nil, errors.New("it holds no systems")
	}
	for _, p := range pairs {
		if os, arch, ok := strings.Cut(p, "/"); !ok || os == "" || arch == "" || strings.Contains(arch, "/") {
			return nil, fmt.Errorf("%q is not os/arch", p)
		}
	}
	return pairs, nil
}

// stagedOnlyForTheCommit refuses a tier row that is staged and is not the commit tier: the staged-inputs shape
// of skip is excused there alone.
func stagedOnlyForTheCommit(t audit.Tier) error {
	if t.Staged && t.Name != "pre-commit" {
		return fmt.Errorf("tier %s is staged: only the commit tier may carry the staged-inputs shape of skip", t.Name)
	}
	return nil
}
