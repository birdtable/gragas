package model

// What the coverage command lists in a rulebook, both ways (docs/spec.md S7,
// S47): the rules a tool holds and the tool entries a rule explains. Types
// only (A4); the core builds them and the command draws them. Every field says
// what it decides or that it is display only (arch.ports.19).

// UpstreamSet is one upstream preset of a tool and the entries it held at the
// pin, as the flattened set names them.
type UpstreamSet struct {
	// Preset is the name a rung-0 binding adopts the set by, as its key.
	// Decides: which binding adopts the set.
	Preset string `json:"preset"`
	// Entries are the entry keys the set held at the pin. Decides: the
	// entries the set explains.
	Entries []string `json:"entries"`
}

// Upstream is the flattened upstream set of one tool, read from the
// rulebook's tree. Written by upgrade; coverage only reads it.
type Upstream struct {
	// Tool is the tool's slug. Decides: whose bindings adopt the sets.
	Tool string `json:"tool"`
	// Sets are the upstream presets. Decides.
	Sets []UpstreamSet `json:"sets"`
	// Observed is every entry key the upstream config holds now. Decides:
	// which entries coverage counts and which of them nothing explains.
	Observed []string `json:"observed"`
}

// CoverRule is one rule on the rules-to-entries side.
type CoverRule struct {
	// Rule is the rule's id. Display only.
	Rule RuleID `json:"rule"`
	// Standing is the rule's derived standing. Decides: whether the rule is
	// held.
	Standing Standing `json:"standing"`
	// Held says whether an effective binding holds the rule. Decides.
	Held bool `json:"held"`
	// Entries are the entries its bindings name, as "tool target: key".
	// Display only.
	Entries []string `json:"entries"`
	// ZeroBy is the date a planned rule should be held by, "" where none is
	// set. Display only.
	ZeroBy string `json:"zero_by,omitempty"`
}

// CoverEntry is one entry, or one adopted upstream set, on the
// entries-to-rules side. An adopted set is one covered unit (S47).
type CoverEntry struct {
	// Tool is the tool's slug. Display only.
	Tool string `json:"tool"`
	// Entry is "target: key". Display only.
	Entry string `json:"entry"`
	// Rules are the rules that hold it, by id. Decides: which rules explain
	// the entry.
	Rules []RuleID `json:"rules"`
	// Set is the preset's name where the unit is an upstream set adopted
	// whole at rung 0, "" for a plain entry. Decides: whether it counts as
	// one set.
	Set string `json:"set,omitempty"`
	// Members are the entries of the set, which expand it. Display only.
	Members []string `json:"members,omitempty"`
}

// CoverUnmapped is an entry the upstream config holds that no rule explains.
type CoverUnmapped struct {
	// Tool is the tool's slug. Display only.
	Tool string `json:"tool"`
	// Entry is the entry's key. Display only.
	Entry string `json:"entry"`
}

// Coverage is the two directions of a rulebook.
type Coverage struct {
	// Dir is the folder of the rulebook below the root, "" for the root.
	// Display only.
	Dir string `json:"dir"`
	// Rules are the rules, by id. Decides: which rules the listing shows.
	Rules []CoverRule `json:"rules"`
	// Entries are the units a rule explains, by tool and entry. Decides.
	Entries []CoverEntry `json:"entries"`
	// Unmapped are the upstream entries nothing explains. Decides.
	Unmapped []CoverUnmapped `json:"unmapped"`
}

// RulesHeld is how many rules an effective binding holds.
func (c Coverage) RulesHeld() int {
	n := 0
	for _, r := range c.Rules {
		if r.Held {
			n++
		}
	}
	return n
}

// CoverageCheck is the id every result of coverage carries in SARIF, listed
// once in the tool's rules: an upstream entry no rule explains. It is the
// check's own id, not a rulebook rule, and is here so the core and the
// commands hold no rule id in a string (arch.ports.2).
const CoverageCheck = EnginePart + ".coverage.1"
