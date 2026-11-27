package model

// The requests of the source commands: what a person asked for, in the words of
// the command, before the lifecycle context makes a plan of it. A source is
// recorded and edited, never replaced or retired (S15): a rule that cites it
// keeps citing it, and an edit is noted in the history of every such rule.

// NewSource is the request of source new: a thing a rule can cite.
type NewSource struct {
	// Slug is the source's key, as a rule cites it.
	Slug string
	// Kind is what the source is: one of SourceKinds.
	Kind SourceKind
	// Title names the source.
	Title string
	// Locator is where it is found: a URL, an ISBN, a DOI, an RFC or, for an
	// own decision, a decision slug.
	Locator string
	// Attribution says who wrote it; "" for none.
	Attribution string
	// Files are the paths the plan writes to.
	Files PlanFiles
}

// EditSource is the request of source edit: the title, the locator or the
// attribution of a source. A nil pointer leaves the field alone, and a pointer
// to "" takes an attribution away; a title and a locator are never empty.
type EditSource struct {
	// Slug is the source's key.
	Slug string
	// Title is the new title.
	Title *string
	// Locator is the new locator.
	Locator *string
	// Attribution is the new attribution.
	Attribution *string
	// Files are the paths the plan writes to.
	Files PlanFiles
}
