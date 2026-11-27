package model

// What explain says in a rulebook (docs/spec.md S37): a rule, or a tool entry,
// by the bindings, presets and standing behind it. Types only (A4); the core
// builds them and the command draws them.

// ExplainedBinding is one binding of an explained rule.
type ExplainedBinding struct {
	// Tool, Target and Key name the tool entry. Display only.
	Tool   string     `json:"tool"`
	Target TargetType `json:"target"`
	Key    string     `json:"key"`
	// Value is the value the binding sets, nil for none. Display only.
	Value any `json:"value,omitempty"`
	// Severity, Rung and RunPoint are what the binding says. Display only.
	Severity Severity `json:"severity"`
	Rung     Rung     `json:"rung"`
	RunPoint RunPoint `json:"run_point"`
	// Proof is what the proof says of the binding. The grid is not read yet,
	// so a binding below rung 6 reads not run. Display only.
	Proof Truth `json:"proof"`
	// Preset is the preset file the entry is written to, "" where the binding
	// is not shipped. Display only.
	Preset string `json:"preset,omitempty"`
}

// ExplainedRule is one rule as explain tells it.
type ExplainedRule struct {
	// Rule is the rule's id as the rulebook holds it now. Decides.
	Rule RuleID `json:"rule"`
	// Status is the status people typed. Display only.
	Status Status `json:"status"`
	// Standing and Marks are the derived standing. Display only.
	Standing Standing `json:"standing"`
	Marks    []Mark   `json:"marks"`
	// Bindings are the rule's bindings, in order. Display only.
	Bindings []ExplainedBinding `json:"bindings"`
	// Page and Explorer are the rule's links where the identity gives a base
	// address, "" otherwise (S72). Display only.
	Page     string `json:"page,omitempty"`
	Explorer string `json:"explorer,omitempty"`
}

// Explained is the answer to one explain: the rules that hold what was asked.
type Explained struct {
	// Asked is what was typed: a rule id or alias, or a tool and an entry.
	Asked string `json:"asked"`
	// Alias is the id the rule had when the asked one is an old id of it, "".
	// Display only.
	Alias RuleID `json:"alias,omitempty"`
	// Tool and Entry are set for the entry form. Display only.
	Tool  string `json:"tool,omitempty"`
	Entry string `json:"entry,omitempty"`
	// Rules are the rules asked about, or that hold the entry asked about.
	Rules []ExplainedRule `json:"rules"`
	// Unmapped says the entry asked about is held by no rule.
	Unmapped bool `json:"unmapped"`
}

// ResolveReport is the effective rulebook of a chain as resolve lists it.
type ResolveReport struct {
	// Dir is the folder of the rulebook below the root. Display only.
	Dir string `json:"dir"`
	// Resolved is the chain with provenance. Decides.
	Resolved Resolved `json:"resolved"`
}
