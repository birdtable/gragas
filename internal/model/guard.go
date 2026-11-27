package model

// Guard is guard edit's answer: the file a proposed edit writes, as a path in
// the repository, and every lifecycle field the edit would change by hand
// (S59). An edit with none is allowed. Display only: the core has judged it,
// and nothing reads it back.
type Guard struct {
	Path    string    `json:"path"`
	Changes []Guarded `json:"changes"`
}

// Guarded is one lifecycle field a proposed edit changes by hand, and the
// command that changes it with its history entry (S59, A29). Display only.
type Guarded struct {
	// Entity is the entity kind and its key, such as rule:go.errors.1, or the
	// file, such as history.jsonl.
	Entity string `json:"entity"`
	// Field is the field that changes, as the file spells it.
	Field string `json:"field"`
	// Before and After are the field's value, as compact JSON, "" for none.
	Before string `json:"before"`
	After  string `json:"after"`
	// Command is the line to type instead. A command not built yet ends in
	// "(not built yet)"; "" is a field no command changes.
	Command string `json:"command"`
}
