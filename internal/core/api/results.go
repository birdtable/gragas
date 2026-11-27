package coreapi

// The JSON result types, declared once here with their tags (cli.output.2):
// the command tree fills them and prints them unchanged.

// ResultSchema is the schema number of Result. 2: check reports enforced
// rules per part where 1 reported binding ones.
const ResultSchema = 2

// Result is a run's answer under --json when its command has no document of
// its own: the word it ended on, what it said, the changes it made or would
// make, its data, and the error when it did not get there. Its shape is
// a sibling tool's api.Result, the shared convention, with the schema number and the
// run id the rulebook asks for (cli.output.1, cli.errors.1).
type Result struct {
	Schema int `json:"schema"`
	// Command is the command as typed, without gragas: "check", "gen".
	Command string `json:"command"`
	// Apply is set on a command that has a plan and an apply: false is the
	// plan, true the act.
	Apply   *bool    `json:"apply,omitempty"`
	Ended   string   `json:"ended"`
	Msg     string   `json:"msg,omitempty"`
	Next    string   `json:"next,omitempty"`
	Run     string   `json:"run,omitempty"`
	Changes []string `json:"changes,omitempty"`
	Data    any      `json:"data,omitempty"`
	Error   *Error   `json:"error,omitempty"`
}

// Error is why a run did not get there: its kind, from the exit code, the
// message and the line to type.
type Error struct {
	Kind string `json:"kind"`
	Msg  string `json:"msg"`
	Hint string `json:"hint,omitempty"`
}

// Version is what version --json prints: the build, the Go release, the target
// and the versions of the format and the contract the build reads. A value the
// build cannot establish is null, and Notes says why, by field name
// (cli.output.9).
type Version struct {
	Contract string            `json:"contract"`
	Version  string            `json:"version"`
	Tag      *string           `json:"tag"`
	Commit   *string           `json:"commit"`
	Go       string            `json:"go"`
	Target   string            `json:"target"`
	Format   int               `json:"format"`
	Notes    map[string]string `json:"notes,omitempty"`
}

// Count is one part's line in the report: its entries, how many are
// enforced, and its tools.
type Count struct {
	Part     string `json:"part"`
	Name     string `json:"name"`
	Entries  int    `json:"entries"`
	Enforced int    `json:"enforced"`
	Tools    int    `json:"tools"`
}

// CommandTree is the committed command-tree file: the commands in the tree
// with their flags, positionals and exit codes, the fields of the --json
// answer, and the not-built list (cli.output.4). gragas-docs writes it from
// the registry and the tree.
type CommandTree struct {
	Schema      int           `json:"schema"`
	GeneratedBy string        `json:"generated_by"`
	JSONFields  []string      `json:"json_fields"`
	Global      []TreeFlag    `json:"global_flags"`
	Commands    []TreeCommand `json:"commands"`
	NotBuilt    []string      `json:"not_built"`
}

// TreeCommand is one command of the tree.
type TreeCommand struct {
	Path        string     `json:"path"`
	State       string     `json:"state"`
	Positionals string     `json:"positionals"`
	Flags       []TreeFlag `json:"flags"`
	Exits       []TreeExit `json:"exits"`
	JSONKind    string     `json:"json_kind"`
}

// TreeFlag is a flag with its type and default.
type TreeFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
}

// TreeExit is a closing word and the exit code it gives.
type TreeExit struct {
	Word string `json:"word"`
	Code int    `json:"code"`
}

// Help is what help --json and <command> --help --json print: the same words
// as the screen, from the registry and the tree, with the contract version in
// one top-level field (cli.output.1).
type Help struct {
	Contract string        `json:"contract"`
	Command  string        `json:"command"`
	Summary  string        `json:"summary"`
	About    string        `json:"about"`
	Usage    string        `json:"usage"`
	Commands []HelpCommand `json:"commands"`
	Flags    []TreeFlag    `json:"flags"`
	Global   []TreeFlag    `json:"global_flags"`
	Exits    []TreeExit    `json:"exits"`
}

// HelpCommand is a command listed under another: its name and what it does.
type HelpCommand struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}
