package model

// What the tools command lists in a rulebook: each tool its rules bind to, the
// tool entries those bindings hold and the rule that holds each (docs/spec.md,
// the tools listing, S46). Types only (A4); the core builds them and the
// command draws them. Every field says what it decides or that it is display
// only (arch.ports.19).

// ToolEntryRow is one entry of a tool that a rule holds: the target and the
// key, and each rule that has a binding on it.
type ToolEntryRow struct {
	// Target is the target type of the entry. Display only.
	Target TargetType `json:"target"`
	// Key is the tool's entry. Display only.
	Key string `json:"key"`
	// Rules are the rules that hold the entry, by id. Decides: which rules the
	// listing says explain the entry.
	Rules []RuleID `json:"rules"`
}

// ToolPlugin is the installed plugin that serves a tool: its name and its own
// version, as its manifest says them.
type ToolPlugin struct {
	// Name is the plugin's name. Display only.
	Name string `json:"name"`
	// Version is the plugin's own version. Display only.
	Version string `json:"version"`
}

// ToolRow is one tool: the version the rulebook's tested set names for it, the
// installed plugin that serves it, and the entries its rules hold.
type ToolRow struct {
	// Tool is the tool's slug. Display only.
	Tool string `json:"tool"`
	// Tested is the tool version the rulebook's tested set names, "" where it
	// names none. Display only.
	Tested string `json:"tested,omitempty"`
	// Plugin is the installed plugin that serves the tool, nil where none
	// does. Decides: whether the listing says the tool has no plugin, and,
	// for a tool a rule binds to, that the listing is not ready.
	Plugin *ToolPlugin `json:"plugin,omitempty"`
	// Entries are the entries the rulebook's bindings hold in the tool, by
	// target and key. Decides: which entries the listing says are held.
	Entries []ToolEntryRow `json:"entries"`
}

// ToolsListing is the tools of a rulebook.
type ToolsListing struct {
	// Dir is the folder of the rulebook below the root, "" for the root.
	// Display only.
	Dir string `json:"dir"`
	// Rules is how many rules hold at least one binding. Display only.
	Rules int `json:"rules"`
	// Rows are the tools, by name. Decides: which tools the listing shows.
	Rows []ToolRow `json:"rows"`
	// Checkers are the rulebook's checker-code declarations by name: its own
	// under its own name and publisher, and an import's by reference under the
	// import's. Display only.
	Checkers []CheckerRow `json:"checkers,omitempty"`
}
