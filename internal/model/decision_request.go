package model

// NewDecisionRecord is a decision recorded on its own: its words, the options
// it did not take and what it rests on. Its who and its date come from the
// moment of the plan.
type NewDecisionRecord struct {
	// Slug is the decision's key, as history and rules name it.
	Slug string
	// Title names the decision and Text says what was decided and why.
	Title, Text string
	// Rejected are the options not taken, each with its reason; at least one.
	Rejected []string
	// Sources are the slugs of the sources it rests on, Rules the ids of the
	// rules and Bindings the bindings, each written <rule id> <tool> <key>.
	Sources  []string
	Rules    []RuleID
	Bindings []string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// SupersedeDecision replaces a decision with a new record that names it. The
// old record stays, marked superseded, and every rule that stood on it stands
// on the new one.
type SupersedeDecision struct {
	// Old is the slug of the decision replaced.
	Old string
	// Slug, Title, Text and Rejected are the new record's words.
	Slug        string
	Title, Text string
	Rejected    []string
	// Files are the paths the plan writes to.
	Files PlanFiles
}
