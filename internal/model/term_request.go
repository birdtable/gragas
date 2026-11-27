package model

// The requests of the term commands: what a person asked for, in the words of
// the command, before the lifecycle context makes a plan of it. A term's
// definition and its translations are prose and a file edit (S59), so none of
// these carries one after the term is made.

// NewTerm is the request of term new: a word the rulebook defines once.
type NewTerm struct {
	// Slug is the term's key, as a statement writes it in braces.
	Slug string
	// Definition says what the word means, in one line.
	Definition string
	// Literal is the header name, package or other literal the term stands
	// for; "" for none.
	Literal string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// EditTerm is the request of term edit: the lifecycle fields of a term, which
// is its literal. A nil pointer leaves the literal alone and a pointer to ""
// takes it away. The definition is prose: edit the file.
type EditTerm struct {
	// Slug is the term's key.
	Slug string
	// Literal is the new literal.
	Literal *string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// RetireTerm is the request of term retire: the term stays in its file with
// the status retired. It is refused while a rule uses the term, and it stands
// on a decision, as rule retire does: new or cited.
type RetireTerm struct {
	// Slug is the term's key.
	Slug string
	// Why is the decision the retirement stands on.
	Why Why
	// Files are the paths the plan writes to.
	Files PlanFiles
}
